package organizer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReconcileMultiSelectsNewestSameVolume(t *testing.T){
	r1:=filepath.Join(t.TempDir(),"lib1");r2:=filepath.Join(t.TempDir(),"lib2");out:=filepath.Join(t.TempDir(),"merged")
	for _,p:=range []string{r1,r2,out}{if err:=os.MkdirAll(p,0o755);err!=nil{t.Fatal(err)}}
	a:=filepath.Join(r1,"BLACK LAGOON","BLACK LAGOON 第01巻 単行本.zip")
	b:=filepath.Join(r2,"BLACK_LAGOON","BLACK LAGOON 第01巻 修正版.zip")
	writeTestComicZIP(t,a,"BLACK LAGOON 第01巻")
	writeTestComicZIP(t,b,"BLACK LAGOON 第01巻 修正版")
	now:=time.Now();if err:=os.Chtimes(a,now.Add(-24*time.Hour),now.Add(-24*time.Hour));err!=nil{t.Fatal(err)};if err:=os.Chtimes(b,now,now);err!=nil{t.Fatal(err)}
	o,err:=New(Config{Root:r1,OutputRoot:out,ConfidenceThreshold:.72});if err!=nil{t.Fatal(err)}
	report,err:=o.ReconcileScanMulti([]string{r1,r2},out);if err!=nil{t.Fatal(err)}
	if report.Summary.Superseded!=1{t.Fatalf("superseded=%d want 1: %#v",report.Summary.Superseded,report.Items)}
	if report.Summary.Review!=0{t.Fatalf("review=%d want 0",report.Summary.Review)}
	var winner,loser *ReconcileItem
	for i:=range report.Items{it:=&report.Items[i];if it.AutoSelected{winner=it};if it.Action=="superseded"{loser=it}}
	if winner==nil||filepath.Clean(winner.Source)!=filepath.Clean(b){t.Fatalf("newest file not selected: %#v",winner)}
	if loser==nil||filepath.Clean(loser.Source)!=filepath.Clean(a){t.Fatalf("older file not superseded: %#v",loser)}
}

func TestReconcileMultiRequiresChoiceWhenNewestTied(t *testing.T){
	r1:=filepath.Join(t.TempDir(),"lib1");r2:=filepath.Join(t.TempDir(),"lib2");out:=filepath.Join(t.TempDir(),"merged")
	for _,p:=range []string{r1,r2,out}{if err:=os.MkdirAll(p,0o755);err!=nil{t.Fatal(err)}}
	a:=filepath.Join(r1,"Made in Abyss","Made in Abyss 第02巻 A.zip")
	b:=filepath.Join(r2,"Made_in_Abyss","Made in Abyss 第02巻 B.zip")
	writeTestComicZIP(t,a,"Made in Abyss 第02巻 A");writeTestComicZIP(t,b,"Made in Abyss 第02巻 B")
	stamp:=time.Now().Truncate(time.Second);if err:=os.Chtimes(a,stamp,stamp);err!=nil{t.Fatal(err)};if err:=os.Chtimes(b,stamp,stamp);err!=nil{t.Fatal(err)}
	o,err:=New(Config{Root:r1,OutputRoot:out,ConfidenceThreshold:.72});if err!=nil{t.Fatal(err)}
	report,err:=o.ReconcileScanMulti([]string{r1,r2},out);if err!=nil{t.Fatal(err)}
	if len(report.Choices)!=1{t.Fatalf("choices=%d want 1: %#v",len(report.Choices),report)}
	if report.Summary.Review!=2{t.Fatalf("review=%d want 2",report.Summary.Review)}
	if _,err:=o.ReconcileExecuteMulti([]string{r1,r2},out,nil);err==nil{t.Fatal("expected unresolved choice to reject execution")}
	choice:=report.Choices[0];selected:=choice.Candidates[0].Source
	result,err:=o.ReconcileExecuteMulti([]string{r1,r2},out,map[string]string{choice.ID:selected});if err!=nil{t.Fatal(err)}
	if result.Quarantined!=1{t.Fatalf("quarantined=%d want 1",result.Quarantined)}
}
