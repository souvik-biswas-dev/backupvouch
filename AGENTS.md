# AGENTS.md

Guidance for AI coding assistants (and humans) working in this repository. Read this and `docs/CONTEXT.md` before changing anything.

## Project

BackupVouch is a Go Kubernetes operator (kubebuilder v4) with one CRD: `BackupPolicy` (`backup.backupvouch.pages.dev/v1alpha1`). It runs PostgreSQL backups as Jobs, verifies each one by restoring it into a scratch Postgres, and reports the result. See `docs/SPEC.md` and `docs/ARCHITECTURE.md`.

## Rules

- **Docs are the source of truth.** Field names, states, metric names and reasons are defined in `docs/API.md` and `docs/ARCHITECTURE.md`. If code needs to differ, change the doc in the same change and add a record to `docs/DECISIONS.md`.
- **Keep the kubebuilder layout.** Don't move `api/`, `internal/controller/`, `cmd/`, `config/`. Run `make manifests generate` after editing API types or RBAC markers.
- **Small changes.** One step or one concern at a time. Don't refactor unrelated code.
- **No secrets in logs, errors, events or status.** Secrets are only referenced by name and mounted into Jobs.
- **Errors:** wrap with `%w`, return them, never swallow. No `panic` in runtime code.
- **Idempotent reconcile.** Reconcile can run twice or after a crash at any point. Use deterministic Job names and owner references, and never create duplicates.
- **Least privilege.** Every RBAC marker must be justified in `SECURITY.md`. Containers run as non-root with pinned image tags (never `:latest`).
- **Tests are required.** Table-driven unit tests for pure logic, envtest for the controller, kind for end-to-end. See `docs/TESTING.md`.
- **Uncertain code:** if an API or behaviour is not certain, mark it with `// VERIFY:` so it gets checked on a machine with Go.
- **Never claim tests pass unless they were run.**

## Commands

    make manifests generate fmt vet test     # verify
    make e2e                                 # kind end-to-end (after step 6)

## Commits

Imperative subject, capitalized, no trailing period, 50 characters or fewer. Body wrapped at 72 columns explaining what and why. Sign off with `git commit -s`. If an AI assistant contributed, add an `Assisted-by: <tool>/<model>` trailer.

## Handoff

At the end of every work session update `docs/CONTEXT.md`: what is done, what is next, known issues, and what still has to be verified locally.
