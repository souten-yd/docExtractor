package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/souten-yd/docExtractor/internal/archive"
	"github.com/souten-yd/docExtractor/internal/classifier"
)

type Config struct {
	Root                string
	ConfidenceThreshold float64
}

type Organizer struct {
	cfg Config
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
	if cfg.Root == "" {
		return nil, errors.New("root is required")
	}
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, err
	}
	cfg.Root = filepath.Clean(root)
	if cfg.ConfidenceThreshold <= 0 {
		cfg.ConfidenceThreshold = 0.72
	}
	return &Organizer{cfg: cfg}, nil
}

func (o *Organizer) Root() string { return o.cfg.Root }

func (o *Organizer) Scan() ([]Plan, error) {
	entries, err := os.ReadDir(o.cfg.Root)
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
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".partial") {
			continue
		}
		plan, err := o.PlanName(entry.Name())
		if err != nil {
			plans = append(plans, Plan{Name: entry.Name(), Source: filepath.Join(o.cfg.Root, entry.Name()), NeedsReview: true, Error: err.Error()})
			continue
		}
		plans = append(plans, plan)
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].Name < plans[j].Name })
	return plans, nil
}

func (o *Organizer) PlanName(name string) (Plan, error) {
	if filepath.Base(name) != name || name == "." || name == ".." {
		return Plan{}, errors.New("name must be a single file in the configured root")
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".zip" && ext != ".rar" {
		return Plan{}, fmt.Errorf("unsupported extension: %s", ext)
	}
	source := filepath.Join(o.cfg.Root, name)
	if !o.allowed(source) {
		return Plan{}, errors.New("source escapes configured root")
	}
	if st, err := os.Stat(source); err != nil || st.IsDir() {
		if err != nil {
			return Plan{}, err
		}
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
	destination := filepath.Join(o.cfg.Root, parsed.Series, outName)
	needsReview := parsed.Confidence < o.cfg.ConfidenceThreshold
	if _, err := os.Stat(destination); err == nil {
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

func (o *Organizer) allowed(filename string) bool {
	rel, err := filepath.Rel(o.cfg.Root, filepath.Clean(filename))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
