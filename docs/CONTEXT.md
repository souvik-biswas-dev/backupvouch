# CONTEXT (handoff)

Last updated: 2026-10-25, after PR #9 (step 9) was merged and the SPEC comparison was checked.
Project: BackupVouch, a Kubernetes operator in Go (kubebuilder v4) that runs PostgreSQL backups and verifies them by restoring into a scratch Postgres.
Repo: github.com/souvik-biswas-dev/backupvouch, local path `~/oss/backupvouch`. Author: Souvik. Free Claude plan, so work is done in small steps and handed over with this file.

## Where things stand

| Step | What | Status |
|---|---|---|
| 0 to 9 | scaffold, docs, brand, API, Jobs, verification, controller, tests, demo, dashboard, final docs | done and merged (PR #9 merged, `make verify` and CI green) |
| 10 | pre-release: LICENSE, name checks, first release | **not done** (see below) |

Verified by a real `make demo` run (transcript in `docs/TESTING.md`): healthy backup is restored and passes `RestoreVerified=True`; a corrupted stored backup is rejected with `ChecksumMismatch`, event `VerificationFailed`, metric 0. The dashboard shows the sabotage case with the right wording and the "next in 21 h" arrow.

## Comparison with other tools (checked 2026-10-25, in `docs/SPEC.md`)
- Velero: backs up and restores Kubernetes objects and volumes; no scheduled restore-and-check described.
- CloudNativePG: runs Postgres, has backup, scheduled backup and recovery; no scheduled restore check described.
- K8up: restic backup operator; its `Check` tests the restic repository, not a restore of the data.
- FireDrill (kirilurbonas/FireDrill, Apache-2.0): verifies backups made by other tools, with signed evidence and RTO/RPO; does not take backups. It is ahead of BackupVouch on verification features.
- "Not described in the documentation" is not proof of absence; re-check before quoting elsewhere.

## Do now (in this order)
1. Add the `LICENSE` file (the README already links to it, so the link is broken until you do):
   `curl -fsSL https://www.apache.org/licenses/LICENSE-2.0.txt -o LICENSE` then `head -3 LICENSE` (starts with "Apache License", "Version 2.0, January 2004") and `wc -c LICENSE` (about 11 KB).
2. Name checks (commands below), then decide if the name stays.
3. Unzip the latest zip (README.md, docs/SPEC.md, docs/CONTEXT.md), run `make fmt && make verify`, commit, PR, merge.

Name check commands (needs network):

    curl -s "https://api.github.com/search/repositories?q=backupvouch+in:name" | grep -E '"(total_count|full_name)"'
    curl -s "https://artifacthub.io/api/v1/packages/search?ts_query_web=backupvouch" | head -c 600; echo
    curl -s -o /dev/null -w '%{http_code}\n' "https://pkg.go.dev/github.com/souvik-biswas-dev/backupvouch"
    open "https://pkg.go.dev/search?q=backupvouch"       # xdg-open on Linux
    whois backupvouch.dev | grep -i -E 'domain name|registrar|status|no match|not found'

Read the results: GitHub `total_count` should be 1 (yours); Artifact Hub should list no package called backupvouch; pkg.go.dev search should show nothing else with that name. The API group is `backup.backupvouch.dev`: if you do not own `backupvouch.dev`, someone else can register it. That does not break anything today, but a Kubernetes API group is meant to be a domain you control. Register the domain, or change the group before the first release (changing it later is a breaking API change).

## Still open before a first release (v0.1.0)
- `.github/workflows/publish-images.yml` has never run (needs a `v0.1.0` tag; the tag must equal the tag of `DefaultRunnerImage`). New packages on ghcr.io are private: make them public.
- `dist/install.yaml` (`make build-installer`) was never applied to a clean cluster. Test it in a fresh kind cluster after the images are public.
- `config/manager/manager.yaml` has no `--allowed-runner-images` (D14).
- Dashboard checks nobody has done: 375 px layout, keyboard use of the track (arrows, Home, End, Enter), reduced motion.
- Prune counts: the controller does not read `{deleted, kept}` (documented, not a bug).
- Coverage gaps: `cmd/runner` about 31%, `internal/checks` about 48%, `api/v1alpha1` about 36%, `cmd/main.go` none.
- Not yet tried: a real cluster that is not kind, a Postgres other than the demo one, a large database (restore time and scratch size).

## How to work
1. Upload this file plus the files to change; the assistant edits and returns a zip.
2. Always `git switch main && git pull --ff-only` first, then a new branch, then `unzip -o <zip>`; `git status --short` should list only the zip's files.
3. Before every commit: `make fmt && make verify`, `make dashboard-js-test`, `git status --short config/ api/` (empty), `go mod tidy && git diff --stat go.mod go.sum` (empty), `grep -rIn '^<<<<<<<' . --exclude-dir=.git --exclude-dir=bin` (nothing).
4. `make lint` fails locally with a toolchain mismatch; CI lint is the real one. Lint rules: no `sort` package (use `slices`/`cmp`), `new(expr)` instead of a `ptr` helper, preallocate slices, no variable names that shadow an import.
5. Branch per step, `git commit -s`, PR, wait for Tests, Lint and E2E, merge.
6. The assistant's sandbox has no Go: Go code it writes is only compiled when you run `make verify`.

## Prompt to start the next chat
"Read CONTEXT.md. Step 9 is merged. I added the LICENSE and ran the name checks; here is the output: [paste]. Help me with [the name or API group decision / the v0.1.0 release steps / testing install.yaml in a clean kind cluster]."
