# CONTEXT (handoff)

Last updated: 2026-10-25, after the free-domain decision and the first batch of production-readiness files.
Project: BackupVouch, a Kubernetes operator in Go (kubebuilder v4) that runs PostgreSQL backups and verifies them by restoring into a scratch Postgres.
Repo: github.com/souvik-biswas-dev/backupvouch, local path `~/oss/backupvouch`. Author: Souvik. Free Claude plan, so work is done in small steps and handed over with this file. No budget for a paid domain.

## Where things stand

| Step | What | Status |
|---|---|---|
| 0 to 9 | scaffold to final docs | done and merged (PR #9) |
| 10a | LICENSE, checked comparison in `docs/SPEC.md` | PR `step10-license-and-spec` pushed; merge it if CI is green |
| 10b | API domain moved to the free `backupvouch.pages.dev` (decision D23), project site, monitoring, release and contributor files | **written, not applied or run yet** (see "Do now") |
| 11 | pre-release proof on real databases, then `v0.1.0` | not started |

Comparison with other tools (checked 2026-10-25, in `docs/SPEC.md`): Velero, CloudNativePG and K8up take or restore backups and do not describe a scheduled restore-and-check (K8up's `Check` tests its restic repository only). FireDrill verifies backups made by other tools, with signed evidence and RTO/RPO, and does not take backups.

## What the latest zip contains
- `hack/rename-domain.sh`: renames the API domain everywhere (API group, annotation, labels, finalizer, RBAC markers, samples, docs, scripts, the CRD file name) and appends decision D23. It reads the old domain from `PROJECT`, so running it twice does nothing. Tested on a copy: 28 files change, no leftover of the old name except the history sentence in D23.
- `site/`: a one-page static project site (no JavaScript, self-hosted fonts, security headers in `_headers`) to upload to Cloudflare Pages.
- `docs/MONITORING.md`, `docs/monitoring/prometheusrule.yaml`, `docs/monitoring/grafana-dashboard.json`: alert rules and a dashboard. Valid YAML/JSON, never run against a live Prometheus or Grafana.
- `docs/RELEASE.md` (release steps, cosign verification, Pages deploy), `docs/UPGRADING.md`, `CONTRIBUTING.md`, `CHANGELOG.md`, issue and PR templates, `.github/dependabot.yml`.
- `hack/install-test.sh`: installs from `dist/install.yaml` into a fresh kind cluster and runs the healthy and sabotage checks. Needs public images, so it can only pass after the first release.
- `.github/workflows/security.yml`: weekly govulncheck and Trivy scan, scheduled or manual only (never blocks a PR).
- `.github/workflows/publish-images.yml`: now also attaches an SBOM and provenance and signs both images keyless with cosign. The cosign step is untested and the installer action is not pinned by SHA.

## Do now (in this order)
1. Create the Pages project: Cloudflare dashboard, Workers & Pages, Create, Pages, Upload assets, project name `backupvouch`, upload the `site/` folder (after step 3). If the name is taken, pick another and use it as the argument in step 3.
2. Merge PR `step10-license-and-spec` if green, then `git switch main && git pull --ff-only`.
3. New branch and apply:

       git switch -c step10b-pages-domain
       unzip -o ~/Downloads/<this zip>
       bash hack/rename-domain.sh backupvouch.pages.dev      # or your chosen name
       git add -A
       make fmt && make verify && make dashboard-js-test
       git diff --stat                    # must print nothing: generated files match what the rename wrote
       go mod tidy && git diff --stat go.mod go.sum
       grep -rIn 'backupvouch\.dev' . --exclude-dir=.git --exclude-dir=bin    # only the D23 sentence

4. `make demo`: it must still end with `DEMO FINISHED`. The old transcript in `docs/TESTING.md` now has the new names by text replacement; to make it a raw recording again run `make demo-transcript` and replace the block:

       python3 - <<'PY'
       t = open('dist/demo-transcript.txt').read()
       body = t[t.index('==> HEALTHY CASE'):t.index('\n', t.index('DEMO FINISHED'))].rstrip()
       p = 'docs/TESTING.md'; s = open(p).read()
       a = s.index('```text\n', s.index('Recorded with `make demo-transcript`')) + len('```text\n')
       b = s.index('\n```', a)
       open(p, 'w').write(s[:a] + body + s[b:])
       PY

5. Commit (`git commit -s`), push, open the PR, wait for Tests, Lint, E2E. Paste red lines if any.
6. Everything from here on is the release: follow `docs/RELEASE.md`.

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
"Read CONTEXT.md. I applied the domain rename and the new files; here is the output of `make verify`, `git diff --stat` and `make demo` [paste]. Then help me with [the release steps / testing on my own staging Postgres / the Postgres version matrix]."
