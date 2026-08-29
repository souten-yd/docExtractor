package web

import (
	"fmt"
	"testing"

	"github.com/souten-yd/docExtractor/internal/organizer"
)

func TestReconcileSnapshotPagesAndReload(t *testing.T) {
	root:=t.TempDir()
	report:=organizer.ReconcileReport{Roots:[]string{root},Root:root,OutputRoot:root,Summary:organizer.ReconcileSummary{Files:450}}
	for i:=0;i<450;i++{report.Items=append(report.Items,organizer.ReconcileItem{Relative:fmt.Sprintf("book-%03d.zip",i),Series:"Series"})}
	for i:=0;i<7;i++{report.Choices=append(report.Choices,organizer.ReconcileChoice{ID:fmt.Sprintf("choice-%d",i),Series:"Series"})}
	dir,meta,err:=saveReconcileSnapshot(root,"manage",report);if err!=nil{t.Fatal(err)}
	if meta.ItemsTotal!=450||meta.ChoicesTotal!=7{t.Fatalf("unexpected totals: %+v",meta)}
	page,err:=loadSnapshotPage[organizer.ReconcileItem](dir,"items",190,30,450);if err!=nil{t.Fatal(err)}
	if len(page)!=30||page[0].Relative!="book-190.zip"||page[29].Relative!="book-219.zip"{t.Fatalf("unexpected page: first=%q last=%q len=%d",page[0].Relative,page[len(page)-1].Relative,len(page))}
	loaded,err:=loadReconcileSnapshot(dir);if err!=nil{t.Fatal(err)}
	if len(loaded.Items)!=450||len(loaded.Choices)!=7||loaded.Summary.Files!=450{t.Fatalf("unexpected restored report: items=%d choices=%d summary=%+v",len(loaded.Items),len(loaded.Choices),loaded.Summary)}
}
