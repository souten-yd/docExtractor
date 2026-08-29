package archive

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func makeTestZIP(t *testing.T, filename string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(filename); if err != nil { t.Fatal(err) }
	zw := zip.NewWriter(f)
	for name, body := range entries { w,err:=zw.Create(name);if err!=nil{t.Fatal(err)};if _,err=w.Write([]byte(body));err!=nil{t.Fatal(err)} }
	if err:=zw.Close();err!=nil{t.Fatal(err)};if err:=f.Close();err!=nil{t.Fatal(err)}
}

func TestVerifyAndRenameZIPWithoutRewrite(t *testing.T) {
	dir := t.TempDir(); src:=filepath.Join(dir,"source.zip"); dst:=filepath.Join(dir,"Series","source.zip")
	makeTestZIP(t,src,map[string]string{"001.jpg":"fake-jpeg"})
	p:=New(Config{});result,err:=p.Process(context.Background(),Task{Source:src,Destination:dst,DeleteSource:true},nil);if err!=nil{t.Fatal(err)}
	if result.BytesWritten!=0{t.Fatalf("unexpected rewrite: %+v",result)}
	if _,err:=os.Stat(src);!os.IsNotExist(err){t.Fatalf("source still exists")};if _,err:=os.Stat(dst);err!=nil{t.Fatal(err)}
}

func TestSingleFolderRenamesZIPToFolderNameWithoutRewrite(t *testing.T) {
	dir:=t.TempDir();src:=filepath.Join(dir,"english.zip");dst:=filepath.Join(dir,"out","fallback","english.zip")
	makeTestZIP(t,src,map[string]string{"日本語作品 第01巻/001.jpg":"a","日本語作品 第01巻/002.jpg":"b"})
	p:=New(Config{});res,err:=p.Process(context.Background(),Task{Source:src,Destination:dst,DeleteSource:true},nil);if err!=nil{t.Fatal(err)}
	if res.BytesWritten!=0{t.Fatalf("single folder ZIP should not be rewritten: %+v",res)}
	target:=outputForFolder(dst,"日本語作品 第01巻");if _,err:=os.Stat(target);err!=nil{t.Fatalf("expected %s: %v",target,err)}
}

func TestMultipleFoldersBecomeSeparateZIPs(t *testing.T) {
	dir:=t.TempDir();src:=filepath.Join(dir,"bundle.zip");dst:=filepath.Join(dir,"out","fallback","bundle.zip")
	makeTestZIP(t,src,map[string]string{"日本語作品 第01巻/001.jpg":"a","日本語作品 第02巻/001.jpg":"b"})
	p:=New(Config{});res,err:=p.Process(context.Background(),Task{Source:src,Destination:dst,DeleteSource:true},nil);if err!=nil{t.Fatal(err)}
	if res.Operation!="split-folders(2)"{t.Fatalf("unexpected result: %+v",res)}
	for _,folder:=range []string{"日本語作品 第01巻","日本語作品 第02巻"}{target:=outputForFolder(dst,folder);if _,err:=os.Stat(target);err!=nil{t.Fatalf("missing %s: %v",target,err)};zr,err:=zip.OpenReader(target);if err!=nil{t.Fatal(err)};if len(zr.File)!=1||zr.File[0].Name!="001.jpg"{t.Fatalf("unexpected split contents for %s",folder)};_ = zr.Close()}
	if _,err:=os.Stat(src);!os.IsNotExist(err){t.Fatal("source should be removed only after both outputs succeed")}
}

func TestCommonRoot(t *testing.T) {
	if got:=commonRoot([]string{"book/001.jpg","book/002.jpg"});got!="book"{t.Fatalf("got %q",got)}
	if got:=commonRoot([]string{"001.jpg","002.jpg"});got!=""{t.Fatalf("got %q",got)}
}

func TestSafeArchiveName(t *testing.T) {
	if _,err:=safeArchiveName("../../etc/passwd","");err==nil{t.Fatal("expected traversal rejection")}
	got,err:=safeArchiveName("book/001.jpg","book");if err!=nil||got!="001.jpg"{t.Fatalf("got=%q err=%v",got,err)}
}
