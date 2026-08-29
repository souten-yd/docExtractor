package organizer

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeTestComicZIP(t *testing.T,path,folder string){t.Helper();if err:=os.MkdirAll(filepath.Dir(path),0o755);err!=nil{t.Fatal(err)};f,err:=os.Create(path);if err!=nil{t.Fatal(err)};zw:=zip.NewWriter(f);w,err:=zw.Create(folder+"/001.jpg");if err!=nil{t.Fatal(err)};if _,err=w.Write([]byte("same-image-data"));err!=nil{t.Fatal(err)};if err:=zw.Close();err!=nil{t.Fatal(err)};if err:=f.Close();err!=nil{t.Fatal(err)}}

func TestReconcileQuarantinesExactDuplicate(t *testing.T){
	root:=t.TempDir();a:=filepath.Join(root,"BLACK_LAGOON","BLACK LAGOON 第01巻.zip");b:=filepath.Join(root,"BLACK LAGOON ブラック・ラグーン","BLACK LAGOON 第01巻.zip");writeTestComicZIP(t,a,"BLACK LAGOON 第01巻");raw,err:=os.ReadFile(a);if err!=nil{t.Fatal(err)};if err:=os.MkdirAll(filepath.Dir(b),0o755);err!=nil{t.Fatal(err)};if err:=os.WriteFile(b,raw,0o644);err!=nil{t.Fatal(err)}
	o,err:=New(Config{Root:root,OutputRoot:root,ConfidenceThreshold:.72});if err!=nil{t.Fatal(err)};report,err:=o.ReconcileScan(root);if err!=nil{t.Fatal(err)};if report.Summary.Duplicates!=1{t.Fatalf("duplicates=%d want 1: %#v",report.Summary.Duplicates,report.Items)};result,err:=o.ReconcileExecute(root);if err!=nil{t.Fatal(err)};if result.Quarantined!=1{t.Fatalf("quarantined=%d want 1",result.Quarantined)};matches,err:=filepath.Glob(filepath.Join(root,".docExtractor-duplicates","*","*.zip"));if err!=nil{t.Fatal(err)};if len(matches)!=1{t.Fatalf("quarantine files=%d want 1",len(matches))}
}

func TestReconcileLeavesConflictingNonDuplicateUntouched(t *testing.T){
	root:=t.TempDir();a:=filepath.Join(root,"Series A","Series A 第01巻.zip");b:=filepath.Join(root,"Series_A","Series A 第01巻.zip");writeTestComicZIP(t,a,"Series A 第01巻");writeTestComicZIP(t,b,"Series A 第02巻")
	o,err:=New(Config{Root:root,OutputRoot:root,ConfidenceThreshold:.72});if err!=nil{t.Fatal(err)};report,err:=o.ReconcileScan(root);if err!=nil{t.Fatal(err)};if report.Summary.Files!=2{t.Fatalf("files=%d want 2",report.Summary.Files)};if _,err:=os.Stat(a);err!=nil{t.Fatal(err)};if _,err:=os.Stat(b);err!=nil{t.Fatal(err)}
}
