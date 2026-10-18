# Series folder repair and rating preservation (v0.2.31)

## Evaluation

The supplied QNAP layout contains 1,033 directories and 7,452 files. Of those files, 418 are in `.docExtractor-duplicates`; the normal library has 7,034 archives. Filename-only replay with author evidence produces 915 series-family groups, versus 939 with author evidence omitted (using the corrected parser in both comparisons). 24 resulting groups span more than one existing directory. This is a grouping assessment, not evidence of byte-identical duplicate files: the export contains no sizes, timestamps, hashes or archive contents.

Patterns include truncated long names, optional subtitles, inserted qualifiers, family/derivative titles, and volume ranges accidentally included in a directory name. Do not publish the user's complete library inventory or require an archive-content scan for naming.

## Causes and implementation

- Reconcile parsed authors but omitted them when constructing clustering plans. Pass the authors into the existing same-author prefix/qualifier matching and forward progress notifications.
- Filename-derived naming discarded folder-only ratings. Separate boundary annotations from identity, preserve them through parent evidence, saved aliases and canonical selection, and reattach them to the selected series.
- Equal-frequency display candidates lacked a total deterministic tie break. Use the existing concise display preference as the final tie break, preventing traversal-dependent reorganization.
- Recognize `v06s` and `第01-02巻s`; a range has no single-volume identity.
- Preserve numbered sequel identity in duplicate/variant selection even when the existing family-grouping policy shares one directory.

## Rating contract

Only leading/trailing ⭐, ⭐️, 🌟, ★ and ☆ runs (including spaces and text/emoji variation selectors) are rating annotations. Internal stars, such as those in `マジカル★エクスプローラー`, `煩悩☆西遊記` and `[☆1]`, remain title characters. Arbitrary emoji/tags are not stripped.

Merge keeps the highest star count, not the sum across volumes. Equal counts use deterministic lexical ordering; the selected glyphs/variation selectors survive. Reconcile places the selected rating before the canonical title. Parent ratings transfer only when the folder agrees with the filename by normalized/near title or supported prefix evidence, never from unrelated collection buckets. Saved unstarred aliases do not erase ratings. New archive processing reuses the exact existing output directory spelling, including suffix ratings, on a normalized exact title match. It does not infer fuzzy matches from directory names alone.

## Safety and use

Use the existing library-management scan/preview and execute controls after upgrading. There is no startup migration and no new endpoint or automatic execution. The unchanged execution path keeps archive filenames, moves files without recompression, quarantines verified duplicates/selected older variants and does not intentionally overwrite occupied destinations. Folder-family membership is broader than duplicate identity: subtitle/derivative titles and editions must not be discarded merely because they share a folder and volume number. Quarantine remains excluded unless explicitly included.

Existing filesystem safeguards and cross-filesystem move limitations remain in effect. Execution is not a transaction against external concurrent writers; do not run another organizer during execution. The layout alone cannot restore a rating already removed; recover it from an older listing/backup or set it again. No QNAP live data was modified during development.

## Validation

Regression coverage includes boundary stars and internal title stars, saved aliases, strongest-rating selection independent of order, multi-root subtitle merge with payload preservation, execute/rescan idempotence, duplicate quarantine retaining the live rating, incoming-volume preview/execution parity, unrelated rated buckets, distinct subtitle work identity, numbered sequels and volume ranges. Run `go test ./...`, `go vet ./...` and the static linux/amd64 build; CI repeats these before release.
