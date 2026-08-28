# docExtractor design

## Target

- QNAP TS-253Be (Intel Celeron J3455 / x86_64)
- 16 GB RAM configuration
- QPKG installation
- Web UI driven operation
- Input archives may be 2 GB or larger

## Core principles

1. Read once / write once / rename afterwards.
2. Never fully load an archive or entry into RAM.
3. Avoid intermediate extraction to disk whenever streaming is possible.
4. Keep the source archive until output verification succeeds.
5. Prefer metadata-only rename/move on the same filesystem.
6. Use ZIP64 and 64-bit sizes/offsets.
7. Nested archives are detected, but are not recursively unpacked by default.

## Archive pipeline

### ZIP input

If the ZIP already satisfies the target format, do not recompress it. Classification and same-filesystem rename are sufficient.

When a rewrite is required, prefer raw entry copy where possible instead of inflate + deflate.

### RAR input

RAR data is decoded as a stream and written directly to the final ZIP `.partial` file. No complete extraction workspace is created.

For already-compressed image formats (JPEG, PNG, WebP, AVIF, HEIC), Fast/Balanced modes may store entries without Deflate to reduce J3455 CPU cost.

### Nested archive policy

Archive unwrapping is not recursive extraction.

Examples:

- `book.rar -> images`: stream-convert to ZIP.
- `book.rar -> book.zip`: extract the inner ZIP as the final artifact and stop.
- `book.rar -> images + bonus.zip`: preserve `bonus.zip` as an entry.
- multiple nested archives: preserve by default and show them in preview.

This avoids unnecessary data rewriting and prevents accidental archive-expansion explosions.

## Large file and RAM policy

2 GB+ files are normal inputs. `io.ReadAll`-style whole-file reads are forbidden in archive paths.

Per-worker bounded buffers should normally stay in the 16-64 MB range, with optional bounded read-ahead. The 16 GB RAM is primarily left available for QTS/Linux filesystem page cache rather than used as a giant RAM disk.

## Concurrency

Default: `Auto`, initially equivalent to 2 concurrent archive jobs on TS-253Be.

Selectable values: Auto / 1 / 2 / 3.

The limiting factors are J3455 CPU and storage I/O, not RAM. Auto mode should later use observed throughput and queue pressure to avoid starting a worker when an additional worker reduces aggregate throughput.

## Safe output transaction

1. Analyze source without modification.
2. Determine final series directory and filename.
3. Check free-space safety margin.
4. Write directly to `<final>.partial`.
5. Close and reopen output.
6. Verify ZIP structure, entry count, sizes and selected CRC policy.
7. Atomically rename `.partial` to final name.
8. Only then remove the source if the configured policy allows it.

A NAS restart or crash must leave either the original source or a clearly identifiable `.partial` file.

## Filename picker

Filename classification is separated from file mutation.

The scanner produces:

- series title
- volume/chapter number
- author/group when present
- edition/special markers
- confidence score
- proposed destination

The Web UI shows a dry-run preview before execution. Low-confidence classifications are highlighted for manual correction rather than automatically moved.

The parser should use layered rules instead of the legacy `]` and `第` substring positions only. Later stages may add normalization dictionaries and learned/fuzzy matching without changing the archive pipeline.

## Web UI

Primary pages:

- Dashboard
- Scan / Preview
- Jobs
- Logs / Diagnostics
- Settings

The UI should remain lightweight and require advanced settings only when the user opens them.

## Logging and diagnostics

Logging is part of the job engine, not console-only output.

Each job writes structured JSONL events with fields such as:

- timestamp
- level
- job ID
- component
- stage (`scan`, `classify`, `rar-read`, `zip-write`, `verify`, `rename`, `cleanup`)
- duration
- bytes read/written
- error summary

Initial Web API:

- `GET /api/logs/jobs`
- `GET /api/logs/jobs/{jobID}` (recent events)
- `GET /api/logs/jobs/{jobID}/download` (JSONL)
- `GET /api/diagnostics/download?job_id=...` (support ZIP)

The support ZIP contains logs and small runtime metadata only. It must never include source archives, extracted pages, or other document payloads.

### Privacy

Privacy mode redacts secret/token/password fields and reduces path fields to the basename before writing the log. Raw configuration secrets must not be included in diagnostics bundles.

### Retention

Default job-log retention is 14 days and should be configurable. Old logs are removed by a low-priority cleanup task.

## Diagnostics bundle roadmap

The support ZIP will include, where safely available:

- selected job JSONL
- docExtractor version/build commit
- QTS version
- QPKG version
- CPU architecture/core count
- total/available memory
- configured worker count
- archive mode/verification mode
- free disk space for configured roots
- recent application startup/shutdown errors

It will not include file contents or secret values.

## Initial implementation status

The `feat/qpkg-foundation` branch contains the first diagnostics foundation:

- Go module
- structured per-job JSONL log manager
- retention cleanup hook
- privacy redaction
- Web log listing/view/download endpoints
- diagnostic ZIP generation
- unit tests for redaction and bundle creation

Next implementation units are the archive stream abstraction, ZIP/RAR handlers, job queue, classifier, Web UI, QPKG packaging, and GitHub Actions build/release workflow.
