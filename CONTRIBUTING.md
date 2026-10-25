# Contributing

Thanks for looking at BackupVouch. It is a small project, so a short issue before a big change saves everyone time.

## Set up

You need Go (the version in `go.mod`), docker, kind, kubectl and node (for the dashboard tests).

    git clone https://github.com/souvik-biswas-dev/backupvouch && cd backupvouch
    make verify               # generate, fmt, vet and all tests (envtest downloads its binaries once)
    make dashboard-js-test    # the dashboard's plain JavaScript tests
    make demo                 # the whole thing in a kind cluster: backup, verify, sabotage

## Before you open a pull request

1. `make fmt && make verify`, then `make dashboard-js-test` if you touched `internal/dashboard`.
2. `git status --short config/ api/` must be empty, and so must `go mod tidy && git diff --stat go.mod go.sum`. CI fails if generated files are not committed.
3. Sign your commits (`git commit -s`). By signing off you confirm the Developer Certificate of Origin (developercertificate.org).
4. One topic per pull request. Update the docs in `docs/` when behaviour changes, and add a line to `CHANGELOG.md` under "Unreleased".

CI runs Tests, Lint and an end-to-end run in kind. `make lint` locally may fail with a Go toolchain mismatch; CI builds its own linter.

Code rules that the linter enforces: no `sort` package (use `slices` and `cmp`), preallocate slices that are appended to in a plain loop, and no variable names that shadow an import.

## What fits the project

BackupVouch proves PostgreSQL backups by restoring them. Changes that make that more reliable, easier to understand or easier to operate are welcome. Larger features (point-in-time recovery, other databases) need an issue first, because `docs/SPEC.md` lists them as out of scope for now.

## Reporting a security problem

Do not open a public issue. Follow [SECURITY.md](SECURITY.md).
