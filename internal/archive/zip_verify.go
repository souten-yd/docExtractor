package archive

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

func VerifyZIP(ctx context.Context, filename string, mode VerifyMode) (int, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return 0, err
	}
	defer zr.Close()
	count := 0
	buf := make([]byte, 1024*1024)
	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		if _, err := safeArchiveName(f.Name, ""); err != nil {
			return count, err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return count, fmt.Errorf("ZIP symlink is not allowed: %s", f.Name)
		}
		count++
		if mode != VerifyFull {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return count, err
		}
		_, copyErr := copyWithContext(ctx, io.Discard, rc, buf, nil)
		closeErr := rc.Close()
		if copyErr != nil {
			return count, copyErr
		}
		if closeErr != nil {
			return count, closeErr
		}
	}
	if count == 0 {
		return 0, errors.New("ZIP contains no files")
	}
	return count, nil
}

// VerifyZIPNoNestedArchives is the post-condition for rewritten archives.
// A successful recursive normalization may not leave ZIP/RAR/CBZ/CBR/7z
// payloads inside the final ZIP.
func VerifyZIPNoNestedArchives(ctx context.Context, filename string, mode VerifyMode) (int, error) {
	count, err := VerifyZIP(ctx, filename, mode)
	if err != nil {
		return count, err
	}
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return count, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if isArchiveLikeName(f.Name) {
			return count, fmt.Errorf("nested archive remains after normalization: %s", f.Name)
		}
	}
	return count, nil
}
