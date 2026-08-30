package web

import (
	"bufio"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
)

func (s *Server) archivePreviewExport(w http.ResponseWriter, r *http.Request) {
	st := s.archiveScanState()
	st.mu.RLock()
	running := st.Running
	phase := st.Phase
	plans := make([]interface{}, 0)
	_ = plans
	st.mu.RUnlock()
	if running {
		http.Error(w, "archive scan is still running", http.StatusConflict)
		return
	}
	items := archiveScanPlansSnapshot(st)
	if phase != "done" || len(items) == 0 {
		http.Error(w, "no completed archive scan result; run scan first", http.StatusConflict)
		return
	}

	stamp := time.Now().Format("20060102-150405")
	filename := "docextractor-archive-dryrun-" + stamp + ".txt"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")
	bw := bufio.NewWriterSize(w, 64*1024)
	defer bw.Flush()

	fmt.Fprintln(bw, "docExtractor archive processing dry-run")
	fmt.Fprintf(bw, "generated_at: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(bw, "input_root: %s\n", s.Organizer.Root())
	fmt.Fprintf(bw, "output_root: %s\n", s.Organizer.OutputRoot())
	fmt.Fprintf(bw, "items: %d\n", len(items))
	fmt.Fprintln(bw, "note: this report does not publish, move, rewrite, or delete source files")
	fmt.Fprintln(bw, "note: predicted outputs were cached during scan; exporting this report does not reopen archives")
	fmt.Fprintln(bw)

	for i, p := range items {
		fmt.Fprintf(bw, "[%04d] %s\n", i+1, p.Name)
		fmt.Fprintf(bw, "  source: %s\n", p.Source)
		fmt.Fprintf(bw, "  series: %s\n", p.Series)
		if p.HasVolume {
			fmt.Fprintf(bw, "  volume: %d\n", p.Volume)
		} else {
			fmt.Fprintln(bw, "  volume: -")
		}
		fmt.Fprintf(bw, "  action: %s\n", p.Action)
		fmt.Fprintf(bw, "  confidence: %.3f\n", p.Confidence)
		fmt.Fprintf(bw, "  review: %t\n", p.NeedsReview)
		fmt.Fprintf(bw, "  skipped: %t\n", p.Skipped)
		if p.Error != "" {
			fmt.Fprintf(bw, "  error: %s\n", p.Error)
		}
		if len(p.Evidence) > 0 {
			fmt.Fprintf(bw, "  evidence: %v\n", p.Evidence)
		}
		if p.Error == "" && p.Destination != "" {
			if p.PreviewError != "" {
				fmt.Fprintf(bw, "  preview_error: %s\n", p.PreviewError)
				fmt.Fprintf(bw, "  planned_output: %s\n", filepath.ToSlash(p.Destination))
			} else {
				fmt.Fprintf(bw, "  predicted_outputs: %d\n", len(p.PredictedOutputs))
				for _, target := range p.PredictedOutputs {
					fmt.Fprintf(bw, "    -> %s\n", filepath.ToSlash(target))
				}
				if p.PreviewNested {
					fmt.Fprintln(bw, "  nested_archives: yes")
				} else {
					fmt.Fprintln(bw, "  nested_archives: no")
				}
				if p.PreviewWarning != "" {
					fmt.Fprintf(bw, "  warning: %s\n", p.PreviewWarning)
				}
			}
		}
		fmt.Fprintln(bw)
	}
}
