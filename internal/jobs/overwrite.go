package jobs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type overwriteTxn struct {
	destination string
	backup      string
	active      bool
}

func beginOverwrite(task Task) (*overwriteTxn, error) {
	tx:=&overwriteTxn{destination:filepath.Clean(task.Destination)}
	if !task.Overwrite { return tx,nil }
	st,err:=os.Lstat(tx.destination)
	if errors.Is(err,os.ErrNotExist){return tx,nil}
	if err!=nil{return nil,err}
	if !st.Mode().IsRegular(){return nil,errors.New("existing destination is not a regular file")}
	tx.backup=fmt.Sprintf("%s.docextractor-backup-%d",tx.destination,time.Now().UTC().UnixNano())
	if err:=os.Rename(tx.destination,tx.backup);err!=nil{return nil,fmt.Errorf("cannot stage existing destination for overwrite: %w",err)}
	tx.active=true
	return tx,nil
}

func (t *overwriteTxn) finish(success bool) error {
	if t==nil||!t.active{return nil}
	if success {
		if err:=os.Remove(t.backup);err!=nil&&!errors.Is(err,os.ErrNotExist){return fmt.Errorf("new output is complete but old destination backup could not be removed: %w",err)}
		t.active=false;return nil
	}
	// On failure, remove any incomplete destination and restore the previous file.
	_ = os.Remove(t.destination)
	if err:=os.Rename(t.backup,t.destination);err!=nil{return fmt.Errorf("processing failed and previous destination restore also failed; backup remains at %s: %w",filepath.Base(t.backup),err)}
	t.active=false
	return nil
}
