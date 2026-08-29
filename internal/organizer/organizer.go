package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/souten-yd/docExtractor/internal/archive"
	"github.com/souten-yd/docExtractor/internal/classifier"
)

type Config struct {
	Root                string
	ConfidenceThreshold float64
}

type Organizer struct {
	mu                  sync.RWMutex
	root                string
	confidenceThreshold float64
}

type Plan struct {
	Name        string  `json:"name"`
	Source      string  `json:"source"`
	Destination string  `json:"destination"`
	Series      string  `json:"series"`
	Author      string  `json:"author,omitempty"`
	Volume      int     `json:"volume,omitempty"`
	HasVolume   bool    `json:"has_volume"`
	Confidence  float64 `json:"confidence"`
	NeedsReview bool    `json:"needs_review"`
	Action      string  `json:"action"`
	Entries     int     `json:"entries,omitempty"`
	Error       string  `json:"error,omitempty"`
}

func New(cfg Config) (*Organizer, error) {
	root, err := normalizeRoot(cfg.Root)
	if err != nil {
		return nil, err
	}
	if cfg.ConfidenceThreshold <= 0 {
		cfg.ConfidenceThreshold = 0.72
	}
	return &Organizer{root: root, confidenceThreshold: cfg.ConfidenceThreshold}, nil
}

func (o *Organizer) Root() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.root
}

func (o *Organizer) SetRoot(root string) error {
	normalized, err := normalizeRoot(root)
	if err != nil {
		return err
	}
	o.mu.Lock()
	o.root = normalized
	o.mu.Unlock()
	return nil
}

func (o *Organizer) Scan() ([]Plan, error) {
	root := o.Root()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	plans := make([]Plan, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".zip" && ext != ".rar" {
			continue
		}
		plan, err := o.planNameAt(root, entry.Name())
		if err != nil {
			plans = append(plans, Plan{Name: entry.Name(), Source: filepath.Join(root, entry.Name()), NeedsReview: true, Error: err.Error()})
			continue
		}
		plans = append(plans, plan)
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].Name < plans[j].Name })
	return plans, nil
}

func (o *Organizer) PlanName(name string) (Plan, error) {
	return o.planNameAt(o.Root(), name)
}

func (o *Organizer) planNameAt(root, name string) (Plan, error) {
	if filepath.Base(name) != name || name == "." || name == ".." {
		return Plan{}, errors.New("name must be a single file in the configured root")
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".zip" && ext != ".rar" {
		return Plan{}, fmt.Errorf("unsupported extension: %s", ext)
	}
	source := filepath.Join(root, name)
	if !allowedAt(root, source) {
		return Plan{}, errors.New("source escapes configured root")
	}
	st, err := os.Lstat(source)
	if err != nil {
		return Plan{}, err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return Plan{}, errors.New("symbolic-link archives are not allowed")
	}
	if !st.Mode().IsRegular() {
		return Plan{}, errors.New("source is not a regular archive")
	}

	parsed := classifier.Parse(name)
	outName := name
	action := "rename-zip"
	entries := 0
	if ext == ".rar" {
		outName = strings.TrimSuffix(name, filepath.Ext(name)) + ".zip"
		info, err := archive.InspectRAR(source)
		if err != nil {
			return Plan{}, fmt.Errorf("RAR inspection failed: %w", err)
		}
		entries = info.RegularFiles
		if info.SingleNestedZIP {
			action = "unwrap-nested-zip"
		} else {
			action = "rar-to-zip"
		}
	}
	seriesDir := filepath.Join(root, parsed.Series)
	if err := rejectSymlinkComponents(root, seriesDir); err != nil {
		return Plan{}, err
	}
	destination := filepath.Join(seriesDir, outName)
	needsReview := parsed.Confidence < o.confidenceThreshold
	if _, err := os.Lstat(destination); err == nil {
		return Plan{
			Name: name, Source: source, Destination: destination, Series: parsed.Series,
			Author: parsed.Author, Volume: parsed.Volume, HasVolume: parsed.HasVolume,
			Confidence: parsed.Confidence, NeedsReview: true, Action: action, Entries: entries,
			Error: "destination already exists",
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Plan{}, err
	}
	return Plan{
		Name: name, Source: source, Destination: destination, Series: parsed.Series,
		Author: parsed.Author, Volume: parsed.Volume, HasVolume: parsed.HasVolume,
		Confidence: parsed.Confidence, NeedsReview: needsReview, Action: action, Entries: entries,
	}, nil
}

func normalizeRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("root is not accessible: %w", err)
	}
	if !st.IsDir() {
		return "", errors.New("root is not a directory")
	}
	return abs, nil
}

func allowedAt(root, filename string) bool {
	rel, err := filepath.Rel(root, filepath.Clean(filename))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func rejectSymlinkComponents(root, target string) error {
	rel, err := filepath.Rel(root, filepath.Clean(target))
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errors.New("destination escapes configured root")
	}
	current := root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		st, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination path contains symbolic link: %s", part)
		}
		if !st.IsDir() {
			return fmt.Errorf("destination path component is not a directory: %s", part)
		}
	}
	return nil
}
