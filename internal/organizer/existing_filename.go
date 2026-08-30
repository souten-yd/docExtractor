package organizer

import "github.com/souten-yd/docExtractor/internal/classifier"

// inferExistingArchiveName is intentionally filesystem-only.
//
// Existing-file reprocess and multi-library management operate on libraries
// that were already unpacked/repacked and named by the legacy organizer. Their
// archive member names describe scan layout (for example, "単ページ",
// "Screenshot", or source bucket names) and must not override the archive
// filename. Archive member inspection is reserved for the archive-processing
// workflow.
//
// Keep the parser independent from raw-archive classification so future
// archive-processing changes cannot alter an already-organized library.
func inferExistingArchiveName(filename string) nameEvidence {
	parsed := classifier.ParseExistingFilename(filename)
	return nameEvidence{
		Parsed:         parsed,
		Coverage:       classifier.ParseCoverage(filename),
		Source:         "filesystem-filename",
		Evidence:       []string{"filesystem archive filename only", "legacy processed-library parser"},
		Candidates:     []string{filename},
		CandidateCount: 1,
	}
}
