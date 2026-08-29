package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultReleaseAPI = "https://api.github.com/repos/souten-yd/docExtractor/releases/latest"
	defaultMaxPackage = int64(100 * 1024 * 1024)
)

type State string

const (
	StateIdle        State = "idle"
	StateChecking    State = "checking"
	StateDownloading State = "downloading"
	StateVerifying   State = "verifying"
	StateInstalling  State = "installing"
	StateSucceeded   State = "succeeded"
	StateFailed      State = "failed"
)

type Status struct {
	CurrentVersion   string    `json:"current_version"`
	LatestVersion    string    `json:"latest_version,omitempty"`
	TargetVersion    string    `json:"target_version,omitempty"`
	UpdateAvailable  bool      `json:"update_available"`
	State            State     `json:"state"`
	Message          string    `json:"message,omitempty"`
	Error            string    `json:"error,omitempty"`
	CheckedAt        time.Time `json:"checked_at,omitempty"`
	PublishedAt      time.Time `json:"published_at,omitempty"`
	ReleaseURL       string    `json:"release_url,omitempty"`
	AssetName        string    `json:"asset_name,omitempty"`
	AssetSize        int64     `json:"asset_size,omitempty"`
	DownloadedBytes  int64     `json:"downloaded_bytes,omitempty"`
	TotalBytes       int64     `json:"total_bytes,omitempty"`
	ExpectedSHA256   string    `json:"expected_sha256,omitempty"`
	DownloadedSHA256 string    `json:"downloaded_sha256,omitempty"`
}

type Config struct {
	CurrentVersion  string
	DataDir         string
	ReleaseAPI      string
	HTTPClient      *http.Client
	MaxPackageBytes int64
	Installer       func(packagePath, logPath string) error
}

type Manager struct {
	mu              sync.RWMutex
	currentVersion  string
	dataDir         string
	statusPath      string
	installLogPath  string
	releaseAPI      string
	client          *http.Client
	maxPackageBytes int64
	installer       func(string, string) error
	status          Status
	running         bool
}

type releaseResponse struct {
	TagName     string         `json:"tag_name"`
	HTMLURL     string         `json:"html_url"`
	PublishedAt time.Time      `json:"published_at"`
	Assets      []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

type releaseInfo struct {
	Version     string
	ReleaseURL  string
	PublishedAt time.Time
	AssetName   string
	AssetURL    string
	AssetSize   int64
	SHA256      string
}

func New(cfg Config) (*Manager, error) {
	if strings.TrimSpace(cfg.DataDir) == "" {
		return nil, errors.New("updater data directory is required")
	}
	updateDir := filepath.Join(cfg.DataDir, "updates")
	if err := os.MkdirAll(updateDir, 0o750); err != nil {
		return nil, fmt.Errorf("create updater directory: %w", err)
	}
	api := strings.TrimSpace(cfg.ReleaseAPI)
	if api == "" {
		api = defaultReleaseAPI
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 10 * time.Minute,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 8 {
					return errors.New("too many redirects")
				}
				if req.URL.Scheme != "https" {
					return errors.New("refusing non-HTTPS update redirect")
				}
				return nil
			},
		}
	}
	maxBytes := cfg.MaxPackageBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxPackage
	}
	installer := cfg.Installer
	if installer == nil {
		installer = launchDetachedInstaller
	}
	m := &Manager{
		currentVersion:  strings.TrimSpace(cfg.CurrentVersion),
		dataDir:         updateDir,
		statusPath:      filepath.Join(updateDir, "status.json"),
		installLogPath:  filepath.Join(updateDir, "install.log"),
		releaseAPI:      api,
		client:          client,
		maxPackageBytes: maxBytes,
		installer:       installer,
		status: Status{
			CurrentVersion: strings.TrimSpace(cfg.CurrentVersion),
			State:          StateIdle,
		},
	}
	m.loadStatus()
	m.status.CurrentVersion = m.currentVersion
	if m.status.TargetVersion != "" && sameVersion(m.status.TargetVersion, m.currentVersion) && m.status.State == StateInstalling {
		m.status.State = StateSucceeded
		m.status.UpdateAvailable = false
		m.status.Error = ""
		m.status.Message = "更新が完了しました"
		_ = m.persistLocked()
	}
	return m, nil
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Manager) Check(ctx context.Context) (Status, error) {
	m.mu.Lock()
	if m.running || m.status.State == StateInstalling {
		st := m.status
		m.mu.Unlock()
		return st, errors.New("update operation is already running")
	}
	m.status.State = StateChecking
	m.status.Message = "最新版を確認しています"
	m.status.Error = ""
	_ = m.persistLocked()
	m.mu.Unlock()

	rel, err := m.fetchLatest(ctx)
	if err != nil {
		m.fail("更新確認に失敗しました", err)
		return m.Status(), err
	}
	m.applyRelease(rel)
	return m.Status(), nil
}

func (m *Manager) Start() error {
	m.mu.Lock()
	if m.running || m.status.State == StateInstalling {
		m.mu.Unlock()
		return errors.New("update operation is already running")
	}
	m.running = true
	m.status.State = StateChecking
	m.status.Message = "更新を準備しています"
	m.status.Error = ""
	_ = m.persistLocked()
	m.mu.Unlock()
	go m.run()
	return nil
}

func (m *Manager) run() {
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	rel, err := m.fetchLatest(ctx)
	if err != nil {
		m.fail("更新確認に失敗しました", err)
		return
	}
	m.applyRelease(rel)
	if !isNewer(rel.Version, m.currentVersion) {
		m.mu.Lock()
		m.status.State = StateIdle
		m.status.UpdateAvailable = false
		m.status.Message = "最新版です"
		_ = m.persistLocked()
		m.mu.Unlock()
		return
	}

	packagePath, err := m.download(ctx, rel)
	if err != nil {
		m.fail("更新パッケージの取得に失敗しました", err)
		return
	}

	m.mu.Lock()
	m.status.State = StateInstalling
	m.status.TargetVersion = rel.Version
	m.status.Message = "更新を適用しています。接続が切れた場合はそのままお待ちください"
	m.status.Error = ""
	_ = m.persistLocked()
	m.mu.Unlock()

	if err := m.installer(packagePath, m.installLogPath); err != nil {
		m.fail("更新インストーラを開始できませんでした", err)
		return
	}
}

func (m *Manager) fetchLatest(ctx context.Context) (releaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.releaseAPI, nil)
	if err != nil {
		return releaseInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "docExtractor/"+m.currentVersion)
	resp, err := m.client.Do(req)
	if err != nil {
		return releaseInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return releaseInfo{}, fmt.Errorf("GitHub Releases returned HTTP %d", resp.StatusCode)
	}
	var raw releaseResponse
	dec := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024))
	if err := dec.Decode(&raw); err != nil {
		return releaseInfo{}, fmt.Errorf("decode release metadata: %w", err)
	}
	version := normalizeVersion(raw.TagName)
	if _, ok := parseVersion(version); !ok {
		return releaseInfo{}, fmt.Errorf("invalid release version %q", raw.TagName)
	}
	if err := requireHTTPSGitHubURL(raw.HTMLURL); err != nil {
		return releaseInfo{}, fmt.Errorf("invalid release URL: %w", err)
	}
	expectedName := "docExtractor_" + version + "_x86_64.qpkg"
	for _, asset := range raw.Assets {
		if asset.Name != expectedName {
			continue
		}
		if asset.Size <= 0 || asset.Size > m.maxPackageBytes {
			return releaseInfo{}, fmt.Errorf("release asset size %d is outside allowed range", asset.Size)
		}
		if err := requireHTTPSGitHubURL(asset.BrowserDownloadURL); err != nil {
			return releaseInfo{}, fmt.Errorf("invalid asset URL: %w", err)
		}
		digest := strings.TrimPrefix(strings.TrimSpace(asset.Digest), "sha256:")
		if len(digest) != 64 {
			return releaseInfo{}, errors.New("release asset does not provide a valid SHA-256 digest")
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return releaseInfo{}, errors.New("release asset SHA-256 digest is invalid")
		}
		return releaseInfo{
			Version: version, ReleaseURL: raw.HTMLURL, PublishedAt: raw.PublishedAt,
			AssetName: asset.Name, AssetURL: asset.BrowserDownloadURL, AssetSize: asset.Size,
			SHA256: strings.ToLower(digest),
		}, nil
	}
	return releaseInfo{}, fmt.Errorf("release asset %s was not found", expectedName)
}

func (m *Manager) applyRelease(rel releaseInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.CurrentVersion = m.currentVersion
	m.status.LatestVersion = rel.Version
	m.status.UpdateAvailable = isNewer(rel.Version, m.currentVersion)
	m.status.CheckedAt = time.Now().UTC()
	m.status.PublishedAt = rel.PublishedAt
	m.status.ReleaseURL = rel.ReleaseURL
	m.status.AssetName = rel.AssetName
	m.status.AssetSize = rel.AssetSize
	m.status.TotalBytes = rel.AssetSize
	m.status.ExpectedSHA256 = rel.SHA256
	m.status.Error = ""
	if m.status.UpdateAvailable {
		m.status.State = StateIdle
		m.status.Message = "新しいバージョンがあります"
	} else {
		m.status.State = StateIdle
		m.status.Message = "最新版です"
	}
	_ = m.persistLocked()
}

func (m *Manager) download(ctx context.Context, rel releaseInfo) (string, error) {
	m.mu.Lock()
	m.status.State = StateDownloading
	m.status.TargetVersion = rel.Version
	m.status.DownloadedBytes = 0
	m.status.DownloadedSHA256 = ""
	m.status.TotalBytes = rel.AssetSize
	m.status.Message = "更新パッケージをダウンロードしています"
	_ = m.persistLocked()
	m.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.AssetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "docExtractor/"+m.currentVersion)
	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > m.maxPackageBytes {
		return "", errors.New("update package is larger than allowed maximum")
	}

	finalPath := filepath.Join(m.dataDir, rel.AssetName)
	partialPath := finalPath + ".partial"
	_ = os.Remove(partialPath)
	f, err := os.OpenFile(partialPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()

	h := sha256.New()
	buf := make([]byte, 1024*1024)
	var written int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			written += int64(n)
			if written > m.maxPackageBytes {
				_ = os.Remove(partialPath)
				return "", errors.New("update package exceeded maximum size")
			}
			if _, err := f.Write(buf[:n]); err != nil {
				_ = os.Remove(partialPath)
				return "", err
			}
			_, _ = h.Write(buf[:n])
			m.mu.Lock()
			m.status.DownloadedBytes = written
			m.mu.Unlock()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = os.Remove(partialPath)
			return "", readErr
		}
	}
	if written != rel.AssetSize {
		_ = os.Remove(partialPath)
		return "", fmt.Errorf("download size mismatch: got %d, expected %d", written, rel.AssetSize)
	}
	if err := f.Sync(); err != nil {
		_ = os.Remove(partialPath)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(partialPath)
		return "", err
	}

	m.mu.Lock()
	m.status.State = StateVerifying
	m.status.Message = "SHA-256を検証しています"
	m.status.DownloadedBytes = written
	_ = m.persistLocked()
	m.mu.Unlock()

	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, rel.SHA256) {
		_ = os.Remove(partialPath)
		return "", fmt.Errorf("SHA-256 mismatch: got %s", actual)
	}
	if err := os.Chmod(partialPath, 0o750); err != nil {
		_ = os.Remove(partialPath)
		return "", err
	}
	_ = os.Remove(finalPath)
	if err := os.Rename(partialPath, finalPath); err != nil {
		return "", err
	}
	m.mu.Lock()
	m.status.DownloadedSHA256 = actual
	_ = m.persistLocked()
	m.mu.Unlock()
	return finalPath, nil
}

func (m *Manager) fail(message string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.State = StateFailed
	m.status.Message = message
	m.status.Error = err.Error()
	_ = m.persistLocked()
}

func (m *Manager) InstallLog(maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 || maxBytes > 1024*1024 {
		maxBytes = 256 * 1024
	}
	f, err := os.Open(m.installLogPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []byte{}, nil
		}
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	start := st.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(f, maxBytes))
}

func (m *Manager) loadStatus() {
	raw, err := os.ReadFile(m.statusPath)
	if err != nil {
		return
	}
	var st Status
	if json.Unmarshal(raw, &st) == nil {
		m.status = st
	}
}

func (m *Manager) persistLocked() error {
	raw, err := json.MarshalIndent(m.status, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.statusPath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, m.statusPath)
}

func launchDetachedInstaller(packagePath, logPath string) error {
	if _, err := os.Stat("/bin/sh"); err != nil {
		return errors.New("/bin/sh is unavailable")
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	devnull, err := os.Open("/dev/null")
	if err != nil {
		_ = logf.Close()
		return err
	}
	cmd := exec.Command("/bin/sh", "-c", "sleep 2; exec /bin/sh \"$1\"", "docExtractor-updater", packagePath)
	cmd.Stdin = devnull
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = devnull.Close()
		_ = logf.Close()
		return err
	}
	_ = devnull.Close()
	_ = logf.Close()
	go func() { _ = cmd.Wait() }()
	return nil
}

func requireHTTPSGitHubURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" {
		return errors.New("URL is not HTTPS")
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "api.github.com" && !strings.HasSuffix(host, ".githubusercontent.com") {
		return fmt.Errorf("unexpected GitHub host %q", host)
	}
	return nil
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func sameVersion(a, b string) bool {
	return normalizeVersion(a) == normalizeVersion(b)
}

func isNewer(latest, current string) bool {
	lv, lok := parseVersion(latest)
	cv, cok := parseVersion(current)
	if !lok || !cok {
		return false
	}
	for i := 0; i < len(lv); i++ {
		if lv[i] > cv[i] {
			return true
		}
		if lv[i] < cv[i] {
			return false
		}
	}
	return false
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = normalizeVersion(v)
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i := range parts {
		if parts[i] == "" || strings.ContainsAny(parts[i], "-+") {
			return out, false
		}
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
