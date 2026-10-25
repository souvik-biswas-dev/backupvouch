# CONTEXT (handoff)

Last updated: 2026-10-25 (steps 7 and 8 on branch step8-dashboard, PR #7; demo and dashboard run on a real kind cluster)
Project: BackupVouch, a Kubernetes operator in Go (kubebuilder v4) that runs PostgreSQL backups and verifies them by restoring into a scratch Postgres.
Repo: github.com/souvik-biswas-dev/backupvouch. Step 6 was merged to `main` as PR #5 (merge commit `fedef4c`); in the screenshot taken just after, its CI checks were still running, so check the Actions tab. Step 7 is in `backupvouch-step7.zip`, not yet committed.

## Status of the plan

| Step | What | Status |
|---|---|---|
| 0 | scaffold with kubebuilder | done |
| 1 | documentation review | done |
| 1b | brand: logo, favicon, social preview, docs/BRAND.md | done |
| 2 | API types | done |
| 3 | scheduling and backup Job (`internal/schedule`, `internal/jobs`, `cmd/runner backup`, `Dockerfile.runner`) | done |
| 4 | restore verification (`cmd/runner verify`, `internal/checks`, verify Job builder) | done |
| 5 | controller (state machine, prune runner and builder, metrics, validation, envtest happy path) | done |
| 6 | tests and CI (failure-path envtest specs, `hack/e2e.sh`, Makefile `verify`/`e2e`, CI workflows, TESTING.md) | merged (PR #5). Locally `make verify` passed (all packages ok, controller coverage 75.2%). `make lint` failed with a Go toolchain mismatch (export data version, not a code finding) and `make e2e` stopped because kind was missing; the results after fixing those were not reported |
| 7 | packaging and demo | merged (PR #6); `make demo` now passes after two fixes, see the step 8 section. Was: `hack/lib.sh`, `hack/demo.sh`, `config/samples/demo/`, `make build-installer`/`demo`, `docs/INSTALL.md`, `publish-images.yml` |
| 8 | optional dashboard (`internal/dashboard`, `--dashboard-bind-address`, `docs/DESIGN.md`, D21, D22) | done and seen in a browser; the findings below are polish |
| 9 | final docs and polish | not started |

## Last verified state (steps 0 to 5, by the author, before step 6)

`make manifests generate fmt vet test` passed, all packages ok. Coverage: api/v1alpha1 36.2%, cmd/runner 30.7%, internal/checks 48.2%, internal/controller 57.9%, internal/jobs 90.1%, internal/metrics 95.5%, internal/schedule 89.6%, internal/validation 82.6%. `make lint` has not been confirmed. Nothing in step 6 has been compiled or run (the chat sandbox has Go 1.24 and no module access, the module needs Go 1.26).

## What exists (checked against the code in the step 5 zip)

- `api/v1alpha1`: `BackupPolicy` types with markers, constants (conditions, reasons, labels, finalizer, re-verify annotation), `SetDefaults`, check validation helpers, tests.
- `internal/schedule`: cron parsing with time zone, `IsDue`, `NextRun`, `LastScheduledTime`.
- `internal/jobs`: `BuildBackupJob`, `BuildVerifyJob`, `BuildPruneJob` with hardening (UID 70, read-only root, no token, drop ALL, seccomp), deterministic names, owner references.
- `internal/checks`: check evaluation and `RunCheck` (read-only transaction, statement timeout, message sanitising). `RunCheck` needs a database and is not unit-tested.
- `internal/validation`: cron, time zone, runner image allow-list (`SpecValid`).
- `internal/metrics`: the four gauges, `UpdatePolicyMetrics`, `DeletePolicyMetrics`.
- `internal/controller`: reconciler (finalizer, `SpecValid`, phases Idle/BackingUp/Verifying/Pruning, re-verify annotation, history of 10, cleanup of old Jobs, early `SecretMissing`), `pod_reader.go` (termination message parsing, Pod inspection).
- `cmd/main.go` (manager, `--allowed-runner-images`), `cmd/runner` (`backup`, `verify`, `prune`), `Dockerfile`, `Dockerfile.runner` (postgres 16.8, UID 70).
- `config/`: CRD, RBAC, manager, default kustomization (metrics over HTTPS), samples (full CR and an example Secrets file), scaffold network-policy and prometheus overlays (not enabled).
- `assets/brand/`, all docs in `docs/`, `SECURITY.md`, `README.md` (still says "design phase"), workflows `lint.yml`, `test.yml`, `test-e2e.yml`.

Added in step 7: `hack/lib.sh` (the stages, shared), `hack/demo.sh`, `config/samples/demo/{s3,postgres,policy,kustomization}.yaml` (`s3.yaml` replaced `minio.yaml` in step 7a, see D20) (moved from `hack/e2e/`), Makefile `VERSION`, pinned default `IMG`, a checking `build-installer`, `demo`, `demo-clean`, `demo-transcript`, `docs/INSTALL.md`, `.github/workflows/publish-images.yml`, D19, `dist/` in `.gitignore`. `hack/e2e.sh` is now just the list of stage calls.

Added in step 6: `internal/controller/backuppolicy_failures_test.go`, `hack/e2e.sh`, `hack/e2e/{minio,postgres,policy}.yaml`, Makefile targets `verify`, `e2e`, `docker-build-runner`, updated workflows, rewritten `docs/TESTING.md`, D17 and D18. Removed: scaffold `test/e2e` and `test/utils` and their Makefile targets (D17).

## Where the code differs from the docs

Fixed in step 6:

1. Events RBAC was on the core `events` resource but the recorder (`mgr.GetEventRecorder`) uses `events.k8s.io`. Marker, `config/rbac/role.yaml` and SECURITY.md now say `events.k8s.io` (D18). VERIFY in the e2e run.
2. The controller asked for `create` and `delete` on `backuppolicies`, which SECURITY.md did not justify and the code never uses. Removed.
3. `failBackup` and `failVerify` deleted the Job without background propagation, which orphans the Pod for `batch/v1` Jobs. Fixed (D18).
4. `Dockerfile.runner` built with `golang:1.24-alpine` while `go.mod` needs Go 1.26. Now `golang:1.26-alpine`.
5. SECURITY.md said "RBAC (planned)", TESTING.md said "nothing has been run". Updated.

Still open (code behaves differently from `docs/ARCHITECTURE.md` or `docs/API.md`, not changed because they are behaviour changes; each needs a small fix and a test):

| # | Doc says | Code does | Impact |
|---|---|---|---|
| a | Metrics are recomputed from status on every reconcile (rule 6) | `UpdatePolicyMetrics` is called only when a backup or verification finishes | After an operator restart the gauges are missing until the next run; alerts on `backupvouch_last_verified_timestamp_seconds` can misfire. Fix: call it at the start of `Reconcile` |
| b | The controller resumes at the correct stage after a crash (rule 4, 6) | `reconcileVerifying` only requeues when its Job is missing, while `BackingUp` and `Pruning` recreate it | A deleted verify Job leaves the policy in `Verifying` forever. Fix: recreate it from `status.lastBackup` |
| c | The controller trusts only reasons from a fixed list (D11) | `isValidBackupFailureReason` guards backup, but a verify result's `reason` is copied as is | Unknown reasons become condition reasons. Fix: whitelist `ChecksumMismatch`, `RestoreFailed`, `CheckFailed`, `StorageUnreachable` |
| d | `lastVerification.message` is a short reason, check messages are carried | Verify failures store the fixed text "one or more checks or restore operations failed"; per-check messages and the runner's message are dropped | Less help when a check fails. Fix: use the failed check messages |
| e | The newest terminated Pod of a Job is read (rule 5) | `InspectJobPods` keeps the last terminated message in list order | With `backoffLimit: 1` a retry creates a second Pod; list order is not guaranteed. Fix: sort by creation time |
| f | The Jobs of the last 3 runs are kept (rule 8) | Cleanup runs only at the end of `Pruning`, not after failed runs | Jobs of failed runs pile up until a run succeeds |
| g | `suspend` pauses scheduling | A suspended Idle policy returns before `reconcileIdle`, so it also ignores the re-verify annotation and never initialises conditions | Minor, decide whether that is intended and document it |
| h | Prune result `{deleted, kept}` (ARCHITECTURE) | The prune message is not read, events carry no counts | Minor |
| i | `docs/API.md` conditions table lists only the False reasons | Code also sets True reasons `SpecValid`, `BackupSucceeded`, `VerificationPassed` and Unknown `NoRunYet` | Doc gap, see API.md note |

## Step 6b (2026-10-25): items a to f written, NOT yet compiled or run

Written in a chat sandbox without Go, so run `make verify` first. Files: `internal/controller/backuppolicy_controller.go`, `pod_reader.go`, new `verify_result.go`, tests `verify_result_test.go`, `pod_reader_test.go`, `backuppolicy_failures_test.go`.

- a: `metrics.UpdatePolicyMetrics` is called on every reconcile (after the finalizer check). Envtest spec "operator restart".
- b: a missing verify Job is recreated from `status.lastBackup` (`recreateVerifyJob`); without a successful backup it fails with `InvalidRunnerResult`. Envtest spec.
- c: verify reasons are whitelisted (`ChecksumMismatch`, `RestoreFailed`, `CheckFailed`, `StorageUnreachable`, plus `InvalidRunnerResult`, which the runner also writes); anything else becomes `RestoreFailed`.
- d: `lastVerification.message` and the event now carry the failed check messages, or a reason-specific text when no check ran; failed runs also store the restore duration. The runner has no free-text message field, so a restore or storage failure only gets the reason text.
- e: `InspectJobPods` sorts Pods by creation time, newest wins. Unit test.
- f: `cleanupOldJobs` also runs after failed backups and verifications.
- Restore duration is now rounded UP (a 0.4 s restore shows 1 s instead of "Not recorded").
- Still open: g (suspend behaviour, decide and document), h (prune counts), i (API.md conditions table). Docs `ARCHITECTURE.md` and `RUNBOOK.md` were not edited.
- The dashboard still prints "no check name was recorded" for a checksum failure (no check ran); change that text in `internal/dashboard/web/js/render.js` to show `message` instead.

## Known issues and VERIFY items

- Step 6 e2e failed in CI because the MinIO images no longer exist (D20); fixed in step 7a by using SeaweedFS and curl. Still unverified until a green CI run: the tags `chrislusf/seaweedfs:4.42` and `curlimages/curl:8.12.1`, SeaweedFS `weed mini` flags (`-s3.config`, `-bucket`), SigV4 `PUT` with `UNSIGNED-PAYLOAD` accepted by SeaweedFS, kind `v0.30.0` in CI. Step 6 items that the log showed working: operator deploy and rollout, CRD, leader election, the events RBAC and metrics server start.
- `config/rbac/role.yaml` was edited by hand. Run `make manifests`; if it differs, commit the generated file (CI fails on a diff otherwise).
- `make lint` was never confirmed. Run it once.
- `go mod tidy` was removed from CI (it hid an untidy go.mod). Run it locally before pushing.
- Coverage gaps: `cmd/runner` 30.7% (the paths that need Postgres or S3 are covered only by e2e), `internal/checks` 48.2% (`RunCheck` needs a database), `api/v1alpha1` 36.2% (mostly generated deepcopy), `cmd/main.go` has no tests.
- The scaffold's `config/manager/manager.yaml` has no `--allowed-runner-images`; only the default runner image is allowed (D14). A kind demo works because the runner is loaded under the default tag. How to set the flag is documented in `docs/INSTALL.md`.
- Comparisons with Velero, CloudNativePG, K8up and FireDrill in `SPEC.md` are from memory; check them before the README claims anything. Name availability on GitHub, pkg.go.dev and Artifact Hub is unchecked (D9).
- Favicon should be looked at once in a real browser tab.
- README still says "design phase" (step 9).

## Decisions

See `DECISIONS.md`, D1 to D19. D17: e2e is a bash script, the scaffold Ginkgo e2e is removed. D18: events RBAC group and Job deletion propagation. D19: shared steps and fixtures for e2e and demo, pinned default image, release workflow.

## Step 7: what was done and how far it was checked

- Shared steps: `hack/e2e.sh` and `hack/demo.sh` both call the functions in `hack/lib.sh`; the demo only adds narration and prints the policy status and events after each result (D19). The code was moved, not rewritten.
- Checked in the chat sandbox: `bash -n` and `shellcheck` are clean on all three scripts; both scripts ran to the end against stub `kubectl`/`kind`/`make`/`docker` commands (e2e silent, demo narrated, `config/manager/kustomization.yaml` untouched afterwards); the real `make build-installer` recipe ran with a real kustomize v5.8.1 and produced `dist/install.yaml` with image `ghcr.io/souvik-biswas-dev/backupvouch:v0.1.0` and no `:latest`, the guard rejected `controller:latest`, `ghcr.io/x/y`, `localhost:5000/foo` and `ghcr.io/x/y:`, and the kustomization was restored; `kustomize build config/samples/demo` renders all 8 objects; the workflow YAML parses and its image-name step logic gives the right names.
- NOT checked: any real cluster run of `hack/demo.sh` or the refactored `hack/e2e.sh`, `docker build` of either image, the publish workflow (needs a tag push), applying `dist/install.yaml`.
- The demo transcript in `docs/TESTING.md` is a marked placeholder; there was no cluster to record real output from. Run `make demo-transcript` and paste the result.
- Open items a to i above are still open (the plan calls the fix for a to e "Step 6b"). The demo's failing case reports `ChecksumMismatch`; item d means `status.lastVerification.message` is only generic text for check failures. Item b (a deleted verify Job leaves the policy in `Verifying`) matters if you restart or delete things while presenting.

## Step 8 and the demo: what happened on a real machine (2026-10-25)

- Branch `step8-dashboard`, PR #7, last commit `2f3f631`. Locally: `make verify` passes (dashboard coverage 77.3%), `make dashboard-js-test` passes (18 tests). Fonts were fetched with `make dashboard-fonts` and committed.
- `main` had conflict markers committed by mistake (Makefile, hack/e2e.sh, docs); PR #7 replaces those files with clean ones. Check the markers are gone: `grep -rIn '^<<<<<<<' . --exclude-dir=.git`.
- `make demo` first FAILED twice and two real bugs were fixed (both are in the branch): (1) the `s3` Pod crash-looped because Kubernetes injected `S3_PORT=tcp://...` for the Service named `s3` and SeaweedFS tried to parse it: fixed with `enableServiceLinks: false` in `config/samples/demo/s3.yaml`; (2) the verify Job hung forever because `pg_ctl start` left the scratch Postgres holding the output pipe that `CombinedOutput` waits on: fixed with `-l <logfile>` and `WaitDelay` in `cmd/runner/verify.go`.
- After the fixes `make demo` finished: healthy run verified in about 10 s, sabotage rejected with `ChecksumMismatch`, events and metric as expected. This is the first real run of steps 6, 7 and 8 together.
- Dashboard (`make dashboard-enable`, `make dashboard`, http://localhost:9090) works through `kubectl port-forward` to the loopback listener (the earlier VERIFY is settled). Seen in Chrome: headline, verdict, facts, track with the passed (check) and failed (cut check) runs, the next-run dashed mark, the "What failed" block and the hint.

## Dashboard findings from the first look (not fixed yet)

1. The page was shown dark (Chrome forced dark mode or an extension): the logo wordmark (dark ink) is almost invisible on that background. Decide: either set `<meta name="color-scheme" content="light">` is already there, so check Chrome's "Auto Dark Mode for Web Contents" flag, or add a real dark theme using `logo-dark.svg` and the dark brand colours.
2. The track is dominated by the 22 h to the next run, so runs made minutes apart sit on top of each other at the left edge. Consider capping the right end of the window (for example show "next" as an arrow at the edge when it is far) or zooming the window to the runs plus a margin.
3. "Restore took: Not recorded" on a passed and a failed run: `restoreDurationSeconds` is empty because the runner reports under one second or the controller drops it; check `internal/controller` (item d below is related: the failed-check names and runner message are also dropped, so the page can only say "no check name was recorded").
4. Not yet looked at: 375 px width, keyboard use of the track (arrows, Enter), reduced motion, the draw-in animation, favicon in the tab (a teal check shows in the tab, looks right).

## CI state of PR #7

Tests passes. E2E failed before the two demo fixes and should now pass (re-check the Actions tab; the tags `chrislusf/seaweedfs:4.42` and `curlimages/curl:8.12.1` worked locally). Lint fails in CI for a reason not yet seen: locally `make lint` fails only with a toolchain error (golangci-lint built for an older Go than the installed 1.27.2), so paste the log of the failing "Lint / Run on Ubuntu" job to find the real finding. The `kind` binary (v0.30.0) is installed in /usr/local/bin on the author's machine; the demo cluster `backupvouch-demo` may still be running (`kind delete cluster --name backupvouch-demo`).

## Next

1. Paste the failing Lint job log from PR #7 and fix it; confirm E2E is green; merge PR #7.
2. Dashboard polish from the findings above (dark mode and logo, crowded track, restore duration).
3. Verify Step 6b (above), then the dashboard text for failures without a check name.
4. Step 9: final docs (README still says "design phase"), put the real `make demo-transcript` output into `docs/TESTING.md`.

## To verify locally (step 8 first)

    make dashboard-fonts                     # downloads fonts, then commit internal/dashboard/web/fonts
    make verify                              # compiles and tests internal/dashboard (run make fmt if it reformats)
    make dashboard-js-test
    make demo && make dashboard-enable && make dashboard   # open http://localhost:9090, look at 1280 and 375 px, tab through a track

## Earlier checks

    rm -rf hack/e2e                          # moved to config/samples/demo; unzip cannot delete
    chmod +x hack/e2e.sh hack/demo.sh
    make verify                              # unchanged code, should still pass
    make lint                                # see the toolchain note from the step 6 chat if it fails
    make build-installer                     # dist/install.yaml; check: grep image: dist/install.yaml ; git status must stay clean
    make e2e                                 # the refactored script, needs docker, kind, kubectl
    make demo                                # then: make demo-clean
    make demo-transcript                     # paste the real output into docs/TESTING.md

If `make e2e` fails after the refactor, rerun with `E2E_KEEP_CLUSTER=true make e2e` and paste the output with the Fix prompt. `git diff config/rbac/role.yaml` should show no change after `make manifests`.
