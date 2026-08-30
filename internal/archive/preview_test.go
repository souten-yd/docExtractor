package archive

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewOutputTargetsSplitsTopFolders(t *testing.T){
	d:=t.TempDir(); src:=filepath.Join(d,"bundle.zip"); f,err:=os.Create(src);if err!=nil{t.Fatal(err)}; zw:=zip.NewWriter(f)
	for _,name:=range []string{"作品A 第01巻/001.jpg","作品B 第02巻/001.jpg"}{w,err:=zw.Create(name);if err!=nil{t.Fatal(err)};_,_=w.Write([]byte("x"))}
	if err:=zw.Close();err!=nil{t.Fatal(err)};if err:=f.Close();err!=nil{t.Fatal(err)}
	defaultDst:=filepath.Join(d,"out","bundle","bundle.zip")
	p,err:=PreviewOutputTargets(src,defaultDst);if err!=nil{t.Fatal(err)}
	if len(p.Targets)!=2{t.Fatalf("targets=%v",p.Targets)}
	if filepath.Base(p.Targets[0])!="作品A 第01巻.zip"||filepath.Base(p.Targets[1])!="作品B 第02巻.zip"{t.Fatalf("targets=%v",p.Targets)}
}
