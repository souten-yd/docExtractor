package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/souten-yd/docExtractor/internal/archive"
	"github.com/souten-yd/docExtractor/internal/diagnostics"
	"github.com/souten-yd/docExtractor/internal/jobs"
	"github.com/souten-yd/docExtractor/internal/organizer"
	appsettings "github.com/souten-yd/docExtractor/internal/settings"
	"github.com/souten-yd/docExtractor/internal/updater"
	webui "github.com/souten-yd/docExtractor/internal/web"
)

var version = "dev"

func main() {
	var (
		listen       = flag.String("listen", ":8765", "HTTP listen address")
		root         = flag.String("root", "/share/Download/Temp", "default archive inbox/library root")
		browseRoot   = flag.String("browse-root", "/share", "root exposed by the Web folder picker")
		dataDir      = flag.String("data-dir", "./var", "application data directory")
		settingsFile = flag.String("settings-file", "", "persistent Web settings file (default: <data-dir>/settings.json)")
		workers      = flag.Int("workers", 2, "parallel archive workers (1-3)")
		bufferMiB    = flag.Int("buffer-mib", 8, "stream buffer per worker in MiB")
		maxDictMiB   = flag.Int64("max-dict-mib", 512, "maximum RAR decode dictionary per worker in MiB")
		compression  = flag.String("compression", "balanced", "fast, balanced, or compact")
		fullVerify   = flag.Bool("full-verify", false, "read every ZIP entry after generation")
	)
	flag.Parse()
	if err := os.MkdirAll(*dataDir, 0o750); err != nil { log.Fatal(err) }
	settingsPath := strings.TrimSpace(*settingsFile); if settingsPath == "" { settingsPath = filepath.Join(*dataDir,"settings.json") }
	settingStore, err := appsettings.Open(settingsPath, appsettings.Settings{Root:*root})
	if err != nil { log.Printf("settings load failed; using default root: %v",err); settingStore=appsettings.New(settingsPath,appsettings.Settings{Root:*root}) }

	diag, err := diagnostics.New(diagnostics.Config{RootDir:filepath.Join(*dataDir,"diagnostics"),RetentionDays:14,PrivacyMode:true}); if err!=nil{log.Fatal(err)}
	loadedSettings:=settingStore.Get(); effectiveRoot:=loadedSettings.Root
	org, err := organizer.New(organizer.Config{Root:effectiveRoot,ConfidenceThreshold:0.72,Aliases:loadedSettings.SeriesAliases})
	if err!=nil && filepath.Clean(effectiveRoot)!=filepath.Clean(*root){
		log.Printf("saved root unavailable (%s); falling back to default: %v",effectiveRoot,err)
		org,err=organizer.New(organizer.Config{Root:*root,ConfidenceThreshold:0.72,Aliases:loadedSettings.SeriesAliases})
		if err==nil{settingStore=appsettings.New(settingsPath,appsettings.Settings{Root:org.Root(),SeriesAliases:loadedSettings.SeriesAliases})}
	}
	if err!=nil{log.Fatal(err)}
	verify:=archive.VerifyCentral;if *fullVerify{verify=archive.VerifyFull}
	processor:=archive.New(archive.Config{BufferSize:*bufferMiB*1024*1024,MaxDictionarySize:*maxDictMiB*1024*1024,Compression:archive.CompressionMode(*compression),Verify:verify})

	recoveryCtx,recoveryCancel:=context.WithTimeout(context.Background(),60*time.Second);recovery:=archive.RecoverPartials(recoveryCtx,org.Root(),verify);recoveryCancel()
	if recovery.Found>0||recovery.Errors>0{log.Printf("partial recovery found=%d promoted=%d removed_stale=%d invalid_kept=%d errors=%d",recovery.Found,recovery.Promoted,recovery.RemovedStale,recovery.InvalidKept,recovery.Errors)}
	if systemLog,logErr:=diag.Job("system");logErr==nil{
		_ = systemLog.Write(diagnostics.Event{Component:"startup",Stage:"partial-recovery",Message:"startup recovery completed",Fields:map[string]any{"found":recovery.Found,"promoted":recovery.Promoted,"removed_stale":recovery.RemovedStale,"invalid_kept":recovery.InvalidKept,"errors":recovery.Errors}})
		_ = systemLog.Write(diagnostics.Event{Component:"startup",Stage:"runtime-config",Message:"runtime performance configuration",Fields:map[string]any{"workers":*workers,"buffer_mib_per_worker":*bufferMiB,"max_rar_dictionary_mib_per_worker":*maxDictMiB,"compression":*compression,"verify":string(verify),"series_aliases":len(org.Aliases())}})
	}
	jobManager,err:=jobs.New(*workers,64,makeProcessor(processor,diag));if err!=nil{log.Fatal(err)};defer jobManager.Close()
	updateManager,updateErr:=updater.New(updater.Config{CurrentVersion:version,DataDir:*dataDir});if updateErr!=nil{log.Printf("self updater disabled: %v",updateErr);updateManager=nil}
	handler:=(&webui.Server{Organizer:org,Jobs:jobManager,Diagnostics:diag,Settings:settingStore,Updater:updateManager,BrowseRoot:*browseRoot,Version:version}).Handler()
	httpServer:=&http.Server{Addr:*listen,Handler:handler,ReadHeaderTimeout:10*time.Second,IdleTimeout:60*time.Second}
	ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer stop();go cleanupLoop(ctx,diag)
	go func(){log.Printf("docExtractor %s listening on %s root=%s workers=%d",version,*listen,org.Root(),jobManager.Workers());if err:=httpServer.ListenAndServe();err!=nil&&err!=http.ErrServerClosed{log.Printf("http server error: %v",err);stop()}}()
	<-ctx.Done();shutdownCtx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel();_ = httpServer.Shutdown(shutdownCtx)
}

func makeProcessor(p *archive.Processor, dm *diagnostics.Manager) jobs.Processor {
	return func(ctx context.Context, jobID string, task jobs.Task, update func(jobs.Update)) error {
		logger, _ := dm.Job(jobID); start := time.Now()
		if logger != nil { _ = logger.Write(diagnostics.Event{Component:"worker",Stage:"start",Message:"job started",Fields:map[string]any{"source_path":task.Source,"destination_path":task.Destination}}) }
		var logMu sync.Mutex; lastStage:=""; stageStarted:=start; var stageRead,stageWritten,lastLoggedBytes int64
		result,err:=p.Process(ctx,archive.Task{Source:task.Source,Destination:task.Destination,DeleteSource:task.DeleteSource,OutputTargets:task.OutputTargets,ReconcileOutputs:task.ReconcileOutputs},func(pg archive.Progress){
			update(jobs.Update{Stage:pg.Stage,Progress:pg.Progress,BytesRead:pg.BytesRead,BytesWritten:pg.BytesWritten});if logger==nil{return};logMu.Lock();defer logMu.Unlock();now:=time.Now();stageChanged:=pg.Stage!=""&&pg.Stage!=lastStage
			if stageChanged{if lastStage!=""&&lastStage!="done"{writeStageMetric(logger,lastStage,now.Sub(stageStarted),pg.BytesRead-stageRead,pg.BytesWritten-stageWritten)};lastStage=pg.Stage;stageStarted=now;stageRead=pg.BytesRead;stageWritten=pg.BytesWritten}
			maxBytes:=pg.BytesRead;if pg.BytesWritten>maxBytes{maxBytes=pg.BytesWritten};if stageChanged||maxBytes-lastLoggedBytes>=256*1024*1024{lastLoggedBytes=maxBytes;elapsed:=time.Since(start);_ = logger.Write(diagnostics.Event{Component:"worker",Stage:pg.Stage,Message:"progress",BytesRead:pg.BytesRead,BytesWritten:pg.BytesWritten,Fields:map[string]any{"elapsed_ms":elapsed.Milliseconds(),"io_mib_per_sec":throughputMiB(pg.BytesRead+pg.BytesWritten,elapsed)}})}
		})
		if logger!=nil{logMu.Lock();if lastStage!=""&&lastStage!="done"{writeStageMetric(logger,lastStage,time.Since(stageStarted),result.BytesRead-stageRead,result.BytesWritten-stageWritten)};logMu.Unlock()}
		duration:=time.Since(start);finalUpdate:=jobs.Update{BytesRead:result.BytesRead,BytesWritten:result.BytesWritten,Operation:result.Operation,Entries:result.Entries};if err==nil{finalUpdate.Stage="done";finalUpdate.Progress=1};update(finalUpdate)
		if logger!=nil{event:=diagnostics.Event{Component:"worker",Stage:"done",Message:"job completed",DurationMS:duration.Milliseconds(),BytesRead:result.BytesRead,BytesWritten:result.BytesWritten,Fields:map[string]any{"operation":result.Operation,"entries":result.Entries,"read_mib_per_sec":throughputMiB(result.BytesRead,duration),"write_mib_per_sec":throughputMiB(result.BytesWritten,duration),"io_mib_per_sec":throughputMiB(result.BytesRead+result.BytesWritten,duration)}};if err!=nil{event.Level="error";event.Stage="failed";event.Message="job failed";event.Error=err.Error()};_ = logger.Write(event)};return err
	}
}

func writeStageMetric(logger *diagnostics.JobLogger,stage string,duration time.Duration,bytesRead,bytesWritten int64){if bytesRead<0{bytesRead=0};if bytesWritten<0{bytesWritten=0};_ = logger.Write(diagnostics.Event{Component:"worker",Stage:stage,Message:"stage completed",DurationMS:duration.Milliseconds(),BytesRead:bytesRead,BytesWritten:bytesWritten,Fields:map[string]any{"read_mib_per_sec":throughputMiB(bytesRead,duration),"write_mib_per_sec":throughputMiB(bytesWritten,duration),"io_mib_per_sec":throughputMiB(bytesRead+bytesWritten,duration)}})}
func throughputMiB(bytes int64,d time.Duration)float64{if bytes<=0||d<=0{return 0};return(float64(bytes)/(1024*1024))/d.Seconds()}
func cleanupLoop(ctx context.Context,dm *diagnostics.Manager){_ = dm.Cleanup(time.Now().UTC());ticker:=time.NewTicker(24*time.Hour);defer ticker.Stop();for{select{case<-ctx.Done():return;case now:=<-ticker.C:_=dm.Cleanup(now.UTC())}}}
