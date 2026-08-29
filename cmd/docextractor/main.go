package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/souten-yd/docExtractor/internal/archive"
	"github.com/souten-yd/docExtractor/internal/diagnostics"
	"github.com/souten-yd/docExtractor/internal/jobs"
	"github.com/souten-yd/docExtractor/internal/organizer"
	webui "github.com/souten-yd/docExtractor/internal/web"
)

var version = "dev"

func main() {
	var (
		listen      = flag.String("listen", ":8765", "HTTP listen address")
		root        = flag.String("root", "/share/Download/Temp", "archive inbox/library root")
		dataDir     = flag.String("data-dir", "./var", "application data directory")
		workers     = flag.Int("workers", 2, "parallel archive workers (1-3)")
		bufferMiB   = flag.Int("buffer-mib", 8, "stream buffer per worker in MiB")
		maxDictMiB  = flag.Int64("max-dict-mib", 512, "maximum RAR decode dictionary per worker in MiB")
		compression = flag.String("compression", "balanced", "fast, balanced, or compact")
		fullVerify  = flag.Bool("full-verify", false, "read every ZIP entry after generation")
	)
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o750); err != nil {
		log.Fatal(err)
	}
	diag, err := diagnostics.New(diagnostics.Config{
		RootDir: filepath.Join(*dataDir, "diagnostics"), RetentionDays: 14, PrivacyMode: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	org, err := organizer.New(organizer.Config{Root: *root, ConfidenceThreshold: 0.72})
	if err != nil {
		log.Fatal(err)
	}
	verify := archive.VerifyCentral
	if *fullVerify {
		verify = archive.VerifyFull
	}
	processor := archive.New(archive.Config{
		BufferSize: *bufferMiB * 1024 * 1024, MaxDictionarySize: *maxDictMiB * 1024 * 1024,
		Compression: archive.CompressionMode(*compression), Verify: verify,
	})

	recoveryCtx, recoveryCancel := context.WithTimeout(context.Background(), 60*time.Second)
	recovery := archive.RecoverPartials(recoveryCtx, org.Root(), verify)
	recoveryCancel()
	if recovery.Found > 0 || recovery.Errors > 0 {
		log.Printf("partial recovery found=%d promoted=%d removed_stale=%d invalid_kept=%d errors=%d", recovery.Found, recovery.Promoted, recovery.RemovedStale, recovery.InvalidKept, recovery.Errors)
	}
	if systemLog, logErr := diag.Job("system"); logErr == nil {
		_ = systemLog.Write(diagnostics.Event{
			Component: "startup", Stage: "partial-recovery", Message: "startup recovery completed",
			Fields: map[string]any{
				"found": recovery.Found, "promoted": recovery.Promoted, "removed_stale": recovery.RemovedStale,
				"invalid_kept": recovery.InvalidKept, "errors": recovery.Errors,
			},
		})
	}

	jobManager, err := jobs.New(*workers, 64, makeProcessor(processor, diag))
	if err != nil {
		log.Fatal(err)
	}
	defer jobManager.Close()

	handler := (&webui.Server{Organizer: org, Jobs: jobManager, Diagnostics: diag, Version: version}).Handler()
	httpServer := &http.Server{
		Addr: *listen, Handler: handler,
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go cleanupLoop(ctx, diag)
	go func() {
		log.Printf("docExtractor %s listening on %s root=%s workers=%d", version, *listen, org.Root(), jobManager.Workers())
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("http server error: %v", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}

func makeProcessor(p *archive.Processor, dm *diagnostics.Manager) jobs.Processor {
	return func(ctx context.Context, jobID string, task jobs.Task, update func(jobs.Update)) error {
		logger, _ := dm.Job(jobID)
		start := time.Now()
		if logger != nil {
			_ = logger.Write(diagnostics.Event{Component: "worker", Stage: "start", Message: "job started", Fields: map[string]any{"source_path": task.Source, "destination_path": task.Destination}})
		}
		var logMu sync.Mutex
		lastStage := ""
		var lastLoggedBytes int64
		result, err := p.Process(ctx, archive.Task{Source: task.Source, Destination: task.Destination, DeleteSource: task.DeleteSource}, func(pg archive.Progress) {
			update(jobs.Update{Stage: pg.Stage, Progress: pg.Progress, BytesRead: pg.BytesRead, BytesWritten: pg.BytesWritten})
			if logger == nil {
				return
			}
			// Log stage transitions and roughly every 256 MiB only. This keeps debug value without turning logs into write amplification.
			logMu.Lock()
			defer logMu.Unlock()
			maxBytes := pg.BytesRead
			if pg.BytesWritten > maxBytes {
				maxBytes = pg.BytesWritten
			}
			if pg.Stage != lastStage || maxBytes-lastLoggedBytes >= 256*1024*1024 {
				lastStage = pg.Stage
				lastLoggedBytes = maxBytes
				_ = logger.Write(diagnostics.Event{Component: "worker", Stage: pg.Stage, Message: "progress", BytesRead: pg.BytesRead, BytesWritten: pg.BytesWritten})
			}
		})
		if logger != nil {
			event := diagnostics.Event{Component: "worker", Stage: "done", Message: "job completed", DurationMS: time.Since(start).Milliseconds(), BytesRead: result.BytesRead, BytesWritten: result.BytesWritten, Fields: map[string]any{"operation": result.Operation, "entries": result.Entries}}
			if err != nil {
				event.Level = "error"
				event.Stage = "failed"
				event.Message = "job failed"
				event.Error = err.Error()
			}
			_ = logger.Write(event)
		}
		return err
	}
}

func cleanupLoop(ctx context.Context, dm *diagnostics.Manager) {
	_ = dm.Cleanup(time.Now().UTC())
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			_ = dm.Cleanup(now.UTC())
		}
	}
}
