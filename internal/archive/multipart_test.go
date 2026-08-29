package archive

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseMultipartRARName(t *testing.T) {
	base, part, ok := ParseMultipartRARName("Made_in_Abyss_01-14.part2.rar")
	if !ok || base != "Made_in_Abyss_01-14" || part != 2 { t.Fatalf("got %q %d %v",base,part,ok) }
	if IsSecondaryRARVolume("Made_in_Abyss_01-14.part1.rar") { t.Fatal("part1 must be primary") }
	if !IsSecondaryRARVolume("Made_in_Abyss_01-14.part3.rar") { t.Fatal("part3 must be secondary") }
	if got:=MultipartOutputStem("Made_in_Abyss_01-14.part1.rar"); got!="Made_in_Abyss_01-14" { t.Fatalf("stem=%q",got) }
}

func TestDiscoverMultipartRAR(t *testing.T) {
	dir:=t.TempDir()
	for _,name:=range []string{"Book.part1.rar","Book.part2.rar","Book.part3.rar","Other.part1.rar"} {
		if err:=os.WriteFile(filepath.Join(dir,name),[]byte("x"),0o600);err!=nil{t.Fatal(err)}
	}
	set,ok,err:=DiscoverMultipartRAR(filepath.Join(dir,"Book.part1.rar"))
	if err!=nil||!ok{t.Fatalf("ok=%v err=%v",ok,err)}
	if set.Base!="Book"||len(set.Parts)!=3||len(set.Missing)!=0{t.Fatalf("set=%+v",set)}
	got:=[]string{filepath.Base(set.Parts[0]),filepath.Base(set.Parts[1]),filepath.Base(set.Parts[2])}
	want:=[]string{"Book.part1.rar","Book.part2.rar","Book.part3.rar"}
	if !reflect.DeepEqual(got,want){t.Fatalf("got=%v want=%v",got,want)}
}

func TestDiscoverMultipartRARMissingPart(t *testing.T) {
	dir:=t.TempDir()
	for _,name:=range []string{"Book.part1.rar","Book.part3.rar"} { if err:=os.WriteFile(filepath.Join(dir,name),[]byte("x"),0o600);err!=nil{t.Fatal(err)} }
	set,ok,err:=DiscoverMultipartRAR(filepath.Join(dir,"Book.part1.rar"))
	if err!=nil||!ok{t.Fatalf("ok=%v err=%v",ok,err)}
	if !reflect.DeepEqual(set.Missing,[]int{2}){t.Fatalf("missing=%v",set.Missing)}
}
