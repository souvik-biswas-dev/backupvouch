# CONTEXT (handoff)

Last updated: 2026-10-24
Project: BackupVouch, a Kubernetes operator in Go (kubebuilder v4) that runs PostgreSQL backups and verifies them by restoring into a scratch Postgres.

## Status of the plan

| Step | What | Status |
|---|---|---|
| 0 | scaffold with kubebuilder (done by the author locally) | not started |
| 1 | documentation foundation | drafted (this repo's documents), needs review for consistency |
| 1b | brand: vector logo, favicon, social preview, docs/BRAND.md | not started |
| 2 | API types | not started |
| 3 | scheduling and backup Job | not started |
| 4 | restore verification | not started |
| 5 | controller | not started |
| 6 | tests and CI | not started |
| 7 | packaging and demo | not started |
| 8 | optional dashboard | not started |
| 9 | final docs and polish | not started |

## Decisions so far

See `DECISIONS.md` (D1 to D9). The project was renamed from the working name `restoreguard` to BackupVouch. Names, fields, states, metrics and reasons are defined in `API.md` and `ARCHITECTURE.md`.

## Known issues

- Nothing has been compiled or run yet. Claude chats have no Go toolchain, so every step must be verified locally with `make manifests generate fmt vet test`.
- Comparisons with Velero, CloudNativePG, K8up and FireDrill in `SPEC.md` are from memory and must be checked against current docs.

## Next

Step 1 review (if not done), then Step 1b (brand), then Step 2.

## To verify locally

Nothing yet.
