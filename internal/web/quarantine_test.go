package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/souten-yd/docExtractor/internal/jobs"
	"github.com/souten-yd/docExtractor/internal/organizer"
)

func TestQuarantineListAndDeleteCannotTouchLibraryFile(t *testing.T){
	share:=t.TempDir();library:=filepath.Join(share,"Library");q:=filepath.Join(library,".docExtractor-duplicates","Series");if err:=os.MkdirAll(q,0o755);err!=nil{t.Fatal(err)}
	quarantined:=filepath.Join(q,"dup.zip");normal:=filepath.Join(library,"Series","keep.zip");if err:=os.WriteFile(quarantined,[]byte("duplicate"),0o644);err!=nil{t.Fatal(err)};if err:=os.MkdirAll(filepath.Dir(normal),0o755);err!=nil{t.Fatal(err)};if err:=os.WriteFile(normal,[]byte("keep"),0o644);err!=nil{t.Fatal(err)}
	org,err:=organizer.New(organizer.Config{Root:library,OutputRoot:library});if err!=nil{t.Fatal(err)};jm,err:=jobs.New(1,2,func(context.Context,string,jobs.Task,func(jobs.Update))error{return nil});if err!=nil{t.Fatal(err)};defer jm.Close();h:=(&Server{Organizer:org,Jobs:jm,BrowseRoot:share}).Handler()
	req:=httptest.NewRequest(http.MethodGet,"/api/quarantine?root="+library,nil);res:=httptest.NewRecorder();h.ServeHTTP(res,req);if res.Code!=http.StatusOK{t.Fatalf("list status=%d body=%s",res.Code,res.Body.String())};var list quarantineListResponse;if err:=json.Unmarshal(res.Body.Bytes(),&list);err!=nil{t.Fatal(err)};if list.Count!=1{t.Fatalf("count=%d want 1",list.Count)}
	body:=`{"roots":[`+jsonQuote(library)+`],"paths":[`+jsonQuote(normal)+`],"confirm":"DELETE QUARANTINED"}`;req=httptest.NewRequest(http.MethodPost,"/api/quarantine/delete",strings.NewReader(body));req.Header.Set("Content-Type","application/json");res=httptest.NewRecorder();h.ServeHTTP(res,req);if res.Code!=http.StatusOK{t.Fatalf("delete outside status=%d",res.Code)};if _,err:=os.Stat(normal);err!=nil{t.Fatal("normal library file was touched")}
	body=`{"roots":[`+jsonQuote(library)+`],"paths":[`+jsonQuote(quarantined)+`],"confirm":"DELETE QUARANTINED"}`;req=httptest.NewRequest(http.MethodPost,"/api/quarantine/delete",strings.NewReader(body));req.Header.Set("Content-Type","application/json");res=httptest.NewRecorder();h.ServeHTTP(res,req);if res.Code!=http.StatusOK{t.Fatalf("delete quarantine status=%d body=%s",res.Code,res.Body.String())};if _,err:=os.Stat(quarantined);!os.IsNotExist(err){t.Fatalf("quarantined file still exists: %v",err)}
}

func jsonQuote(s string)string{b,_:=json.Marshal(s);return string(b)}
