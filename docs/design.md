# docExtractor design

## Target

- QNAP TS-253Be / Intel Celeron J3455 / x86_64
- 16 GB RAM
- QTS 5.x / QPKG
- Web UI driven operation
- 2 GB+ ZIP/RAR files are normal inputs

## Core principles

1. **Read once / write once / rename afterwards.**
2. Never load a complete archive or large entry into RAM.
3. Avoid intermediate extraction to disk whenever streaming is possible.
4. Keep the source archive until the generated output passes verification.
5. Prefer same-filesystem metadata rename for existing ZIP files.
6. ZIP64 / 64-bit file sizes are mandatory assumptions.
7. Nested ZIP/RAR/CBZ/CBR archives are recursively normalized; published ZIPs must not contain nested archives.
8. Logs and temporary files must not create significant write amplification.

## Archive pipeline

### Existing ZIP

Default operation:

`verify central directory -> create series directory -> rename()`

The ZIP payload is not recompressed. If source and destination are on different filesystems, docExtractor returns an explicit cross-device error instead of silently copying several GB.

### RAR -> ZIP

`RAR reader -> bounded RAM buffer -> ZIP writer -> .partial -> verify -> atomic rename -> remove source`

There is no complete extraction workspace. Stream buffer defaults to 8 MiB/worker and is bounded to 64 MiB. RAR dictionary memory defaults to a 512 MiB/worker limit even though the target NAS has 16 GB RAM.

Before conversion, the destination filesystem is checked for estimated output space plus safety margin.

### Recursive archive normalization

- `ZIP/RAR -> images`: retain image/file entries in the normalized ZIP.
- `ZIP/RAR -> nested ZIP/RAR/CBZ/CBR`: spool only the embedded archive payload, recurse, and remove the archive layer.
- Multiple volume folders or nested volume archives become one verified ZIP per top-level normalized folder.
- The dry-run follows the same metadata/grouping rules and reports the final predicted ZIP targets.
- Depth, archive-count, entry-count, and expanded-byte limits stop malformed archives and archive bombs.
- Nested 7z is rejected because the QPKG cannot decode it; it is never silently left in a published ZIP.

## Compression

`fast`: STORE all generated ZIP entries.

`balanced` (default): STORE JPEG/PNG/WebP/AVIF/HEIC/GIF and already-compressed archive/media formats; use Deflate BestSpeed for other data.

`compact`: STORE already-compressed formats and use normal Deflate for other data.

The expected manga workload is already-compressed images, so avoiding redundant Deflate is usually a better use of the J3455 than increasing RAM consumption.

## Concurrency

Default: 2 concurrent jobs. Accepted range: 1-3.

The hard limit is intentionally small because J3455 CPU and NAS storage I/O become bottlenecks before 16 GB RAM does. A later auto-tuner may compare aggregate throughput for 1/2/3 workers, but it must not increase concurrency merely because free RAM exists.

## Filename classification

Classification is completed before mutation and returns:

- author/group candidate
- series title
- volume number
- confidence score and reasons
- proposed destination

Rules currently handle Japanese `第NN巻/卷`, `Vol.N`, `Volume N`, `vN`, numeric tails, full-width digits, common edition suffixes, and common leading metadata tags such as `[一般コミック]` or `[Digital]` before an author tag.

Low-confidence plans are highlighted in Web UI instead of being automatically executed. `GroupKey` normalization is available for a later sibling-consensus stage without coupling it to archive conversion.

## Transaction / crash safety

1. Analyze source without modification.
2. Compute final path.
3. Reject destination collisions.
4. Check free space for RAR conversion.
5. Write `<final>.partial` directly in the final directory.
6. Flush/close and verify generated ZIP.
7. Rename `.partial` atomically to the final name.
8. Remove original RAR only after output success.

A stale `.partial` can be overwritten on a future retry; it is never considered a library ZIP.

## Web UI / QTS

The QPKG service binds `127.0.0.1:8765` by default and is exposed through QTS HTTP Proxy at `/docExtractor`. The HTTP router accepts both prefixed and prefix-stripped requests to tolerate QTS proxy behavior.

Implemented Web operations:

- scan and dry-run preview
- confidence/action display
- safe selection execution
- explicit execution of review-needed items
- bounded job queue
- progress/stage/read/write display
- cancellation
- inline job log viewing
- JSONL job-log download
- diagnostics ZIP download

No frontend package manager or build chain is required.

## Logging and diagnostics

Per-job structured JSONL contains timestamps, stage, component, duration, bytes read/written and error summaries. Progress logging is throttled to stage changes or roughly every 256 MiB.

Privacy mode masks token/password/secret fields and reduces path fields to basenames.

A selected-job diagnostics ZIP includes that job log. A global diagnostics ZIP includes up to ten recent job logs. Safe runtime metadata can include Go/OS/architecture, CPU count, Go memory stats, Linux MemTotal/MemAvailable and QTS version metadata. Archive payloads are never added.

Default retention: 14 days.

## GitHub Actions cost policy

- one short Linux CI job; no matrix
- path filters
- obsolete runs cancelled by concurrency group
- no PR build artifacts
- no npm/frontend build
- QPKG build only on `v*` tags or manual dispatch
- tag builds publish directly to GitHub Releases
- manual QPKG artifact retention: 1 day
- development changes are batched before branch ref updates to avoid triggering CI for every edited file

## Current implementation status

Implemented on `feat/qpkg-foundation`:

- Go single-binary service
- ZIP verify/rename fast path
- streaming RAR -> ZIP conversion
- recursive ZIP/RAR/CBZ/CBR normalization and per-folder splitting
- bounded memory/dictionary limits
- free-space preflight
- path/link safety checks
- 1-3 worker job queue and cancellation
- filename classifier with confidence
- Web control UI
- JSONL diagnostics and support bundle
- QTS proxy-aware routing
- x86_64 QPKG skeleton
- low-cost CI and release workflow

Before the first stable release, remaining validation is primarily real QPKG build/install testing and representative real RAR/ZIP fixture testing on the TS-253Be.
