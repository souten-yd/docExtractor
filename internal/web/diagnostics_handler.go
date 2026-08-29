package web

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/souten-yd/docExtractor/internal/diagnostics"
)

type DiagnosticsHandler struct {
	Manager     *diagnostics.Manager
	Version     string
	ArchiveRoot string
	Workers     int
}

func (h DiagnosticsHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/logs/jobs", h.listJobs)
	mux.HandleFunc("GET /api/logs/jobs/{jobID}", h.getJobLog)
	mux.HandleFunc("GET /api/logs/jobs/{jobID}/download", h.downloadJobLog)
	mux.HandleFunc("GET /api/diagnostics/download", h.downloadBundle)
}

func (h DiagnosticsHandler) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.Manager.ListJobs()
	if err != nil {
		http.Error(w, "cannot list logs", http.StatusInternalServerError)
		return
	}
	writeJSON(w, jobs)
}

func (h DiagnosticsHandler) getJobLog(w http.ResponseWriter, r *http.Request) {
	events, err := h.Manager.Tail(r.PathValue("jobID"), 500)
	if err != nil {
		http.Error(w, "log not found", http.StatusNotFound)
		return
	}
	writeJSON(w, events)
}

func (h DiagnosticsHandler) downloadJobLog(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")
	f, err := h.Manager.OpenJobLog(jobID)
	if err != nil {
		http.Error(w, "log not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="docExtractor-job-%s.jsonl"`, jobID))
	http.ServeContent(w, r, "", time.Time{}, f)
}

func (h DiagnosticsHandler) downloadBundle(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.URL.Query().Get("job_id"))
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	snapshot := map[string]any{
		"generated_at": time.Now().UTC(),
		"version":      h.Version,
		"go_version":   runtime.Version(),
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"cpus":         runtime.NumCPU(),
		"goroutines":   runtime.NumGoroutine(),
		"workers":      h.Workers,
		"go_memory": map[string]uint64{
			"heap_alloc_bytes": ms.HeapAlloc,
			"heap_sys_bytes":   ms.HeapSys,
			"sys_bytes":        ms.Sys,
		},
	}
	if mem := readLinuxMemory(); len(mem) > 0 {
		snapshot["system_memory"] = mem
	}
	if qts := readQTSInfo(); len(qts) > 0 {
		snapshot["qts"] = qts
	}
	if disk := diskSnapshot(h.ArchiveRoot); len(disk) > 0 {
		snapshot["archive_storage"] = disk
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="docExtractor-diagnostics.zip"`)
	if err := h.Manager.WriteBundle(w, jobID, snapshot); err != nil {
		// Headers may already be sent; avoid leaking filesystem details.
		return
	}
}

func readLinuxMemory() map[string]uint64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[string]uint64{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		if key != "MemTotal" && key != "MemAvailable" {
			continue
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil {
			out[strings.ToLower(key)+"_bytes"] = kb * 1024
		}
	}
	return out
}

func readQTSInfo() map[string]string {
	f, err := os.Open("/etc/config/uLinux.conf")
	if err != nil {
		return nil
	}
	defer f.Close()
	allowed := map[string]string{
		"version": "version", "build number": "build_number", "build date": "build_date", "model": "model",
	}
	out := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		name, ok := allowed[key]
		if !ok {
			continue
		}
		value := strings.Trim(strings.TrimSpace(parts[1]), `"`)
		if value != "" {
			out[name] = value
		}
	}
	return out
}

func diskSnapshot(root string) map[string]any {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(root, &fs); err != nil {
		return nil
	}
	bsize := uint64(fs.Bsize)
	return map[string]any{
		"root_name":       filepath.Base(filepath.Clean(root)),
		"total_bytes":     uint64(fs.Blocks) * bsize,
		"available_bytes": uint64(fs.Bavail) * bsize,
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
