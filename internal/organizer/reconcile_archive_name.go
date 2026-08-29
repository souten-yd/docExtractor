package organizer

import "github.com/souten-yd/docExtractor/internal/archive"

func isSecondaryRARVolumeName(name string) bool {
	return archive.IsSecondaryRARVolume(name)
}
