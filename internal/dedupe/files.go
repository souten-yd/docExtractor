package dedupe

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Decision string

const (
	ExactDuplicate Decision = "exact-duplicate"
	KeepExisting   Decision = "keep-existing"
	UseCandidate   Decision = "use-candidate"
)

func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.CopyBuffer(h, f, make([]byte, 4*1024*1024)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CompareFiles applies the same ordering used by existing-library reconcile:
// exact SHA-256 duplicates keep the active copy; different same-volume files
// keep the uniquely newer copy. Equal timestamps keep the active copy so an
// unattended archive batch remains deterministic and reversible.
func CompareFiles(existing, candidate string) (Decision, error) {
	existingInfo, err := os.Lstat(existing)
	if err != nil {
		return "", err
	}
	candidateInfo, err := os.Lstat(candidate)
	if err != nil {
		return "", err
	}
	if !existingInfo.Mode().IsRegular() || !candidateInfo.Mode().IsRegular() {
		return "", errors.New("duplicate comparison requires regular files")
	}
	if existingInfo.Size() == candidateInfo.Size() {
		a, err := HashFile(existing)
		if err != nil {
			return "", err
		}
		b, err := HashFile(candidate)
		if err != nil {
			return "", err
		}
		if a == b {
			return ExactDuplicate, nil
		}
	}
	if candidateInfo.ModTime().After(existingInfo.ModTime()) {
		return UseCandidate, nil
	}
	return KeepExisting, nil
}

func UniqueQuarantinePath(root, series, name string) string {
	base := filepath.Join(root, ".docExtractor-duplicates", series, name)
	if _, err := os.Lstat(base); errors.Is(err, os.ErrNotExist) {
		return base
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 2; n < 10000; n++ {
		p := filepath.Join(root, ".docExtractor-duplicates", series, fmt.Sprintf("%s (%d)%s", stem, n, ext))
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return p
		}
	}
	return base + ".duplicate"
}
