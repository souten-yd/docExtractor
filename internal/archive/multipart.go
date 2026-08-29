package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var multipartRARPattern = regexp.MustCompile(`(?i)^(.*)\.part([0-9]+)\.rar$`)

type MultipartRAR struct {
	Primary string   `json:"primary"`
	Base    string   `json:"base"`
	Parts   []string `json:"parts"`
	Missing []int    `json:"missing,omitempty"`
}

func ParseMultipartRARName(name string) (base string, part int, ok bool) {
	m := multipartRARPattern.FindStringSubmatch(filepath.Base(name))
	if len(m) != 3 {
		return "", 0, false
	}
	n, err := strconv.Atoi(m[2])
	if err != nil || n < 1 {
		return "", 0, false
	}
	return m[1], n, true
}

func IsSecondaryRARVolume(name string) bool {
	_, part, ok := ParseMultipartRARName(name)
	return ok && part > 1
}

// DiscoverMultipartRAR returns the volume set containing filename. It only
// recognizes the modern *.partN.rar convention. All paths returned are full
// paths and sorted by part number.
func DiscoverMultipartRAR(filename string) (MultipartRAR, bool, error) {
	filename = filepath.Clean(filename)
	base, _, ok := ParseMultipartRARName(filepath.Base(filename))
	if !ok {
		return MultipartRAR{}, false, nil
	}
	dir := filepath.Dir(filename)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return MultipartRAR{}, true, err
	}
	type numbered struct{ n int; path string }
	parts := make([]numbered, 0, 4)
	for _, e := range entries {
		if e.IsDir() { continue }
		b, n, match := ParseMultipartRARName(e.Name())
		if !match || !strings.EqualFold(b, base) { continue }
		parts = append(parts, numbered{n:n, path:filepath.Join(dir,e.Name())})
	}
	if len(parts) == 0 {
		return MultipartRAR{}, true, fmt.Errorf("multipart RAR has no readable volumes")
	}
	sort.Slice(parts, func(i,j int) bool { return parts[i].n < parts[j].n })
	max := parts[len(parts)-1].n
	present := make(map[int]string,len(parts))
	for _,p := range parts { present[p.n]=p.path }
	missing := make([]int,0)
	for n:=1;n<=max;n++ { if _,exists:=present[n]; !exists { missing=append(missing,n) } }
	paths := make([]string,0,len(parts))
	for _,p := range parts { paths=append(paths,p.path) }
	primary := present[1]
	return MultipartRAR{Primary:primary,Base:base,Parts:paths,Missing:missing}, true, nil
}

func MultipartOutputStem(name string) string {
	if base, part, ok := ParseMultipartRARName(filepath.Base(name)); ok && part == 1 {
		return base
	}
	return strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
}

func RemoveMultipartRARVolumes(primary string) error {
	set, ok, err := DiscoverMultipartRAR(primary)
	if err != nil { return err }
	if !ok { return os.Remove(primary) }
	var errs []string
	// Remove the primary last. If cleanup is interrupted, the remaining primary
	// still makes the incomplete source set obvious to the next scan.
	for i:=len(set.Parts)-1;i>=0;i-- {
		p:=set.Parts[i]
		if filepath.Clean(p)==filepath.Clean(set.Primary) { continue }
		if err:=os.Remove(p); err!=nil && !os.IsNotExist(err) { errs=append(errs,fmt.Sprintf("%s: %v",filepath.Base(p),err)) }
	}
	if set.Primary!="" {
		if err:=os.Remove(set.Primary); err!=nil && !os.IsNotExist(err) { errs=append(errs,fmt.Sprintf("%s: %v",filepath.Base(set.Primary),err)) }
	}
	if len(errs)>0 { return fmt.Errorf("multipart RAR cleanup failed: %s",strings.Join(errs,"; ")) }
	return nil
}
