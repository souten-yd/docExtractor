# GitHub Actions cost policy

The repository intentionally keeps CI small because the target is a single x86_64 QNAP appliance.

- Pull requests run one Linux job only. There is no OS/Go-version matrix.
- CI runs only when Go/module/workflow files change.
- `concurrency.cancel-in-progress` cancels obsolete runs after a newer commit is pushed.
- PR CI never uploads build artifacts.
- QPKG packaging runs only for `v*` tags or an explicit manual dispatch.
- Tagged QPKGs are attached directly to GitHub Releases instead of being duplicated in Actions artifact storage.
- Manual QPKG artifacts are retained for one day only.
- The Web UI uses no npm/frontend build chain, removing a second dependency/cache job entirely.

This keeps normal development to a short `go test` + `go vet` job and avoids package builds that nobody downloads.
