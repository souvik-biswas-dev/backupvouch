# CONTEXT (handoff)

Last updated: 2026-10-25, after `v0.1.0` was tagged, the images published and made public, and the clean-install test passed.
Project: BackupVouch, a Kubernetes operator in Go (kubebuilder v4) that runs PostgreSQL backups and verifies them by restoring into a scratch Postgres.
Repo: github.com/souvik-biswas-dev/backupvouch, local path `~/oss/backupvouch`. Author: Souvik. Free Claude plan, so work is done in small steps and handed over with this file. No budget for a paid domain.

## Where things stand

| Step | What | Status |
|---|---|---|
| 0 to 10 | scaffold to LICENSE, domain move (D23), site, monitoring, release files | done and merged (PRs #9, #10, #11) |
| 11a | `v0.1.0` tagged; images on ghcr.io, public, cosign signatures verified; `hack/install-test.sh` PASSED against the public images | done |
| 11b | GitHub release page for `v0.1.0` (with `install.yaml` attached, marked pre-release) | **check:** on 2026-10-25 the repo page still showed "1 tag" and no release. Publish it if it is not there |
| 11c | docs update `release-docs-v0.1.0` (CHANGELOG 0.1.0, this file) | apply this zip, PR, merge |
| 11d | repository "About": description, website, topics, social preview | not done |
| 12 | real-database proof (`docs/REAL-DB-TEST.md`) | not started: this is the most important next step |
| 13 | launch (`docs/LAUNCH.md`) | only after step 12 |

Live: site https://backupvouch.pages.dev (Cloudflare Pages, uploaded from `site/`; re-upload the folder after changing it, or switch the project to Git). The API group `backup.backupvouch.pages.dev` belongs to that Cloudflare account, so keep that account.

Comparison with other tools (checked 2026-10-25, in `docs/SPEC.md`): Velero, CloudNativePG and K8up take or restore backups and do not describe a scheduled restore-and-check (K8up's `Check` tests its restic repository only). FireDrill verifies backups made by other tools, with signed evidence and RTO/RPO, and does not take backups.

## Do now (in this order)
1. **Publish the GitHub release** if the repo page still shows no release: Releases, Draft a new release, existing tag `v0.1.0`, notes from the `0.1.0` section of `CHANGELOG.md` plus the install and cosign commands, attach `dist/install.yaml` (`make build-installer`), tick Pre-release. Then open `https://github.com/souvik-biswas-dev/backupvouch/releases/download/v0.1.0/install.yaml` in a private window: it must download (about 30 kB).
2. **Apply the docs zip:** `git switch main && git pull --ff-only && git switch -c release-docs-v0.1.0 && unzip -o ~/Downloads/<zip> && git add -A && git commit -s -m "docs: record the v0.1.0 release, real-database test plan, launch plan" && git push -u origin release-docs-v0.1.0`, open the PR, wait for Tests, Lint, E2E, merge.
3. **Repository About** (gear icon on the repo page): description "Kubernetes operator that proves PostgreSQL backups can be restored: back up, restore into a scratch Postgres, run SQL checks", website `https://backupvouch.pages.dev`, topics `kubernetes kubernetes-operator postgresql backup disaster-recovery golang kubebuilder`. Settings, Social preview: upload `assets/brand/social-preview.png`.
4. **Dependabot** opened about 8 pull requests (Go modules, Docker base images, Actions). Merge one at a time, only when Tests, Lint and E2E are green, and run `make verify` locally first for any Kubernetes or controller-runtime bump. Close ones you do not want; do not let them pile up.
5. **Real-database test** (`docs/REAL-DB-TEST.md`), then bring the results to a new chat.
6. Only after that: the launch plan in `docs/LAUNCH.md`.

## Production-readiness plan (what is done, what is next)
Done as files (needs your review, none of it is proven in use): CONTRIBUTING, CHANGELOG, templates, Dependabot, security scan, image signing and SBOM in the workflow, alert rules, Grafana dashboard, upgrade notes, release steps, install test script, project site.

Needs Go code or a real database, so it is NOT done and each is a separate small step (do them in this order, one chat each, always with `make verify` output):
1. Prove it on real databases: run against your staging Postgres, a larger one, one with extensions, several schemas and custom roles. Note what breaks (ownership during restore, restore time, scratch size).
2. Postgres 14, 15, 17: the runner image is Postgres 16 only. Needs `Dockerfile.runner` build arg, a version matrix in CI, and a real-database test.
3. Test coverage of `cmd/runner` (about 31%) and `internal/checks` (about 48%) against a real Postgres in CI.
4. TLS to the database and to storage, and encrypting dumps before upload (new API fields: needs design first, then code and tests).
5. Failure tests on a real cluster: kill the operator mid-run, delete Jobs, confirm recovery with no duplicate runs.
6. Helm chart: try `kubebuilder edit --plugins=helm/v2-alpha` (check the plugin name for your kubebuilder version) and review the output.
7. API to `v1beta1`/`v1` with conversion; a security review of the threat model; weeks of real use with a few other users before calling it 1.0.
Already possible today: Job `resources` (`spec.resources` exists), manual re-verification, retention.
Out of scope on purpose: point-in-time recovery and WAL archiving (CloudNativePG, pgBackRest and Barman do this well; a better path is verifying their backups later).

## Other open items
- The VS Code popup "Reopen in Container" (a `.devcontainer` folder exists from the scaffold) can be dismissed with "Don't Show Again".
- Prune counts: the controller does not read `{deleted, kept}` (documented, not a bug).
- Dashboard checks nobody has done: 375 px layout, keyboard use of the track, reduced motion.
- `config/manager/manager.yaml` has no `--allowed-runner-images` (D14).
- `dist/install.yaml` was never applied to a clean cluster (`hack/install-test.sh` after the release).
- Docs mention `releases/download/v0.1.0/install.yaml`: attach it with `gh release create` (RELEASE.md step 6).
- The three Cloudflare-free-tier facts to remember: the `pages.dev` name is free but belongs to your Cloudflare account; if you lose the account the API group name could be taken by someone else; a custom domain can be added later, but changing the API group after a release is a breaking change.

## How to work
1. Upload this file plus the files to change; the assistant edits and returns a zip.
2. Always `git switch main && git pull --ff-only` first, then a new branch, then `unzip -o <zip>`; `git status --short` should list only the zip's files.
3. Before every commit: `make fmt && make verify`, `make dashboard-js-test`, `git status --short config/ api/` (empty, unless a rename or API change is the point of the PR), `go mod tidy && git diff --stat go.mod go.sum` (empty), `grep -rIn '^<<<<<<<' . --exclude-dir=.git --exclude-dir=bin` (nothing).
4. `make lint` fails locally with a toolchain mismatch; CI lint is the real one. Lint rules: no `sort` package (use `slices`/`cmp`), `new(expr)` instead of a `ptr` helper, preallocate slices, no variable names that shadow an import.
5. Branch per step, `git commit -s`, PR, wait for Tests, Lint and E2E, merge.
6. The assistant's sandbox has no Go: Go code it writes is only compiled when you run `make verify`.

## Prompt to start the next chat
Use one of these (upload `docs/CONTEXT.md`, and the other files named):

1. Real-database results: "Read CONTEXT.md. v0.1.0 is released. I ran BackupVouch against [describe: Postgres version, size, extensions]. Here is `kubectl describe backuppolicy` and the Pod logs [paste]. Tell me what broke and fix it, one step at a time, with a zip and the local test commands."
2. Postgres versions: "Read CONTEXT.md. Add support for PostgreSQL 14, 15 and 17 in the runner image with a CI matrix. Give me a zip and the commands; I will paste `make verify` output."
3. Launch: "Read CONTEXT.md and docs/LAUNCH.md. I tested on [N] real databases, results [paste]. Help me finish the 2-minute demo script and the Show HN post, using only what is verified."
4. Next production-readiness item (see the plan above): name the item.
