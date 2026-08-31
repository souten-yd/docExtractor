package archive

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/souten-yd/docExtractor/internal/classifier"
)

type zipGroup struct {
	Name  string
	Files []*zip.File
}

func zipGroups(filename string) ([]zipGroup, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil { return nil, err }
	defer zr.Close()
	groups := map[string][]*zip.File{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() { continue }
		clean, err := safeArchiveName(f.Name, "")
		if err != nil { return nil, err }
		parts := strings.Split(clean, "/")
		group := ""
		if len(parts) > 1 { group = parts[0] }
		groups[group] = append(groups[group], f)
	}
	out := make([]zipGroup, 0, len(groups))
	for name, files := range groups { out = append(out, zipGroup{Name:name, Files:files}) }
	sort.Slice(out, func(i,j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

func outputForFolder(defaultDst, folder string) string {
	folder = strings.TrimSpace(folder)
	if folder == "" { return defaultDst }
	base := classifier.SafeFolderName(folder)
	if base == "" || base == "Unknown" { base = "archive" }
	parsed := classifier.Parse(folder)
	series := classifier.SafeFolderName(parsed.Series)
	if series == "" || series == "Unknown" { series = base }
	// Organizer destinations are <output-root>/<series>/<archive>.  Re-root a
	// folder-derived output at the configured output root so one source archive
	// may legitimately produce archives belonging to different series.
	outputRoot := filepath.Dir(filepath.Dir(defaultDst))
	return filepath.Join(outputRoot, series, base+".zip")
}

// OutputForFolder exposes the execution target calculation to the organizer so
// its cached dry-run plan and the worker use the same path rules.
func OutputForFolder(defaultDst, folder string) string { return outputForFolder(defaultDst, folder) }

func configuredOutputTarget(defaultDst, folder string, configured map[string]string) (string, error) {
	target := configured[folder]
	if target == "" { return outputForFolder(defaultDst, folder), nil }
	target = filepath.Clean(target)
	outputRoot := filepath.Dir(filepath.Dir(defaultDst))
	rel, err := filepath.Rel(outputRoot, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) { return "", errors.New("configured output target escapes output root") }
	if !strings.EqualFold(filepath.Ext(target), ".zip") { return "", errors.New("configured output target must be a ZIP") }
	return target, nil
}

func checkOutputTarget(target string, overwrite bool) error {
	st, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) { return nil }
	if err != nil { return err }
	if !st.Mode().IsRegular() { return fmt.Errorf("output target is not a regular file: %s", target) }
	if !overwrite { return fmt.Errorf("destination already exists: %s", filepath.Base(target)) }
	return nil
}

func copyZIPRaw(ctx context.Context, src, dst string, overwrite bool) (int, int64, error) {
	zr, err := zip.OpenReader(src)
	if err != nil { return 0, 0, err }
	defer zr.Close()
	if err := checkOutputTarget(dst, overwrite); err != nil { return 0, 0, err }
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil { return 0, 0, err }
	partial := dst + ".partial"
	_ = os.Remove(partial)
	out, err := os.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil { return 0, 0, err }
	zw := zip.NewWriter(out)
	entries := 0
	ok := false
	defer func(){ _ = zw.Close(); _ = out.Close(); if !ok { _ = os.Remove(partial) } }()
	for _, f := range zr.File {
		if err := ctx.Err(); err != nil { return entries, 0, err }
		if f.FileInfo().IsDir() { continue }
		r, err := f.OpenRaw(); if err != nil { return entries, 0, err }
		h := f.FileHeader
		w, err := zw.CreateRaw(&h); if err != nil { return entries, 0, err }
		if _, err := io.Copy(w, r); err != nil { return entries, 0, err }
		entries++
	}
	if err := zw.Close(); err != nil { return entries, 0, err }
	if err := out.Sync(); err != nil { return entries, 0, err }
	if err := out.Close(); err != nil { return entries, 0, err }
	verified, err := VerifyZIPNoNestedArchives(ctx, partial, VerifyCentral)
	if err != nil { return entries, 0, err }
	if verified != entries { return entries, 0, fmt.Errorf("ZIP verification entry mismatch: wrote %d verified %d", entries, verified) }
	st, err := os.Stat(partial); if err != nil { return entries, 0, err }
	if err := os.Rename(partial, dst); err != nil { return entries, 0, err }
	ok = true
	return entries, st.Size(), nil
}

func moveOrCopyVerifiedZIP(ctx context.Context, src, dst string, overwrite, reconcile bool, report func(Progress)) (Result, error) {
	if filepath.Clean(src) == filepath.Clean(dst) {
		entries, err := VerifyZIPNoNestedArchives(ctx, src, VerifyCentral)
		if err != nil { return Result{}, err }
		st, _ := os.Stat(src)
		return Result{Operation:"already-normalized", Entries:entries, BytesRead:st.Size()}, nil
	}
	entries, err := VerifyZIPNoNestedArchives(ctx, src, VerifyCentral)
	if err != nil { return Result{}, err }
	st, err := os.Stat(src); if err != nil { return Result{}, err }
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil { return Result{}, err }
	report(Progress{Stage:"rename-or-copy", Progress:.94, BytesRead:st.Size()})
	if reconcile {
		f,err:=os.CreateTemp(filepath.Dir(dst),".docextractor-candidate-*.zip");if err!=nil{return Result{},err};candidate:=f.Name();if err:=f.Close();err!=nil{_ = os.Remove(candidate);return Result{},err};_ = os.Remove(candidate)
		moved:=false;written:=int64(0)
		if err:=os.Rename(src,candidate);err==nil{moved=true}else if errors.Is(err,syscall.EXDEV){copied,n,copyErr:=copyZIPRaw(ctx,src,candidate,false);if copyErr!=nil{return Result{},copyErr};if copied!=entries{return Result{},fmt.Errorf("cross-device ZIP copy changed entry count")};written=n}else{return Result{},err}
		if err:=os.Chtimes(candidate,st.ModTime(),st.ModTime());err!=nil{if moved{_ = os.Rename(candidate,src)}else{_ = os.Remove(candidate)};return Result{},err}
		decision,err:=publishCandidate(candidate,dst,true);if err!=nil{if moved{_ = os.Rename(candidate,src)}else{_ = os.Remove(candidate)};return Result{},err}
		if !moved{if err:=os.Remove(src);err!=nil{return Result{Operation:"reconcile-folder-zip",Entries:entries,BytesRead:st.Size(),BytesWritten:written},fmt.Errorf("output complete but source removal failed: %w",err)}}
		return Result{Operation:"reconcile-folder-zip("+string(decision)+")",Entries:entries,BytesRead:st.Size(),BytesWritten:written},nil
	}
	if err := checkOutputTarget(dst, overwrite); err != nil { return Result{}, err }
	if overwrite { _ = os.Remove(dst) }
	if err := os.Rename(src, dst); err == nil {
		return Result{Operation:"rename-folder-zip", Entries:entries, BytesRead:st.Size()}, nil
	} else if !errors.Is(err, syscall.EXDEV) { return Result{}, err }
	// A user-selected result directory can be on another QNAP volume.  In that
	// case a copy is unavoidable; make it atomic and verify before source delete.
	copiedEntries, written, err := copyZIPRaw(ctx, src, dst, overwrite)
	if err != nil { return Result{}, err }
	if copiedEntries != entries { return Result{}, fmt.Errorf("cross-device ZIP copy changed entry count") }
	if err := os.Remove(src); err != nil { return Result{Operation:"copy-folder-zip", Entries:entries, BytesRead:st.Size(), BytesWritten:written}, fmt.Errorf("output complete but source removal failed: %w", err) }
	return Result{Operation:"copy-folder-zip", Entries:entries, BytesRead:st.Size(), BytesWritten:written}, nil
}

func (p *Processor) splitNormalizedZIP(ctx context.Context, normalized, defaultDst, preferredSingleName string, sourceTime time.Time, configured map[string]string, reconcile bool, report func(Progress)) (Result, error) {
	zr, err := zip.OpenReader(normalized)
	if err != nil { return Result{}, err }
	defer zr.Close()
	groupMap := map[string][]*zip.File{}
	for _, f := range zr.File {
		if err := ctx.Err(); err != nil { return Result{}, err }
		if f.FileInfo().IsDir() { continue }
		clean, err := safeArchiveName(f.Name, ""); if err != nil { return Result{}, err }
		parts := strings.Split(clean, "/")
		g := ""
		if len(parts) > 1 { g = parts[0] }
		groupMap[g] = append(groupMap[g], f)
	}
	if len(groupMap) == 0 { return Result{}, errors.New("archive contains no regular files") }
	if len(groupMap) == 1 {
		if files, ok := groupMap[""]; ok && preferredSingleName != "" {
			groupMap = map[string][]*zip.File{preferredSingleName:files}
		}
	}
	groups := make([]string,0,len(groupMap)); for g := range groupMap { groups=append(groups,g) }; sort.Strings(groups)
	targets := make(map[string]string,len(groups)); seen := map[string]struct{}{}
	for _, g := range groups {
		target,err := configuredOutputTarget(defaultDst,g,configured);if err!=nil{return Result{},err}
		key := strings.ToLower(filepath.Clean(target)); if _, ok:=seen[key];ok{return Result{},fmt.Errorf("multiple folders resolve to same output: %s",filepath.Base(target))};seen[key]=struct{}{}
		if !reconcile{if err:=checkOutputTarget(target,false);err!=nil{return Result{},err}};targets[g]=target
	}
	partials := map[string]string{}
	cleanup := func(){for _,x:=range partials{_ = os.Remove(x)}}
	var totalWritten int64; totalEntries:=0
	for index,g := range groups {
		target:=targets[g]; if err:=os.MkdirAll(filepath.Dir(target),0o750);err!=nil{cleanup();return Result{},err}
		partial:=target+".partial";_ = os.Remove(partial);partials[g]=partial
		out,err:=os.OpenFile(partial,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0o640);if err!=nil{cleanup();return Result{},err}
		zw:=zip.NewWriter(out)
		for _,f:=range groupMap[g]{
			name,err:=safeArchiveName(f.Name,"");if err!=nil{_ = zw.Close();_ = out.Close();cleanup();return Result{},err}
			if g!=""{prefix:=g+"/";name=strings.TrimPrefix(name,prefix);if name==""{continue}}
			h:=f.FileHeader;h.Name=path.Clean(name)
			r,err:=f.OpenRaw();if err!=nil{_ = zw.Close();_ = out.Close();cleanup();return Result{},err}
			w,err:=zw.CreateRaw(&h);if err!=nil{_ = zw.Close();_ = out.Close();cleanup();return Result{},err}
			if _,err=io.Copy(w,r);err!=nil{_ = zw.Close();_ = out.Close();cleanup();return Result{},err};totalEntries++
		}
		if err:=zw.Close();err!=nil{_ = out.Close();cleanup();return Result{},err};if err:=out.Sync();err!=nil{_ = out.Close();cleanup();return Result{},err};if err:=out.Close();err!=nil{cleanup();return Result{},err}
		if _,err:=VerifyZIPNoNestedArchives(ctx,partial,p.cfg.Verify);err!=nil{cleanup();return Result{},fmt.Errorf("split ZIP verification failed for %s: %w",filepath.Base(target),err)}
		if !sourceTime.IsZero(){if err:=os.Chtimes(partial,sourceTime,sourceTime);err!=nil{cleanup();return Result{},err}}
		st,err:=os.Stat(partial);if err!=nil{cleanup();return Result{},err};totalWritten+=st.Size();report(Progress{Stage:"split-folders",Progress:.75+.2*float64(index+1)/float64(len(groups)),BytesWritten:totalWritten})
	}
	for _,g:=range groups{target:=targets[g];if _,err:=publishCandidate(partials[g],target,reconcile);err!=nil{cleanup();return Result{},err};delete(partials,g)}
	return Result{Operation:fmt.Sprintf("split-folders(%d)",len(groups)),Entries:totalEntries,BytesWritten:totalWritten},nil
}
