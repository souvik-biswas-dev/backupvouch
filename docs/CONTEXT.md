# CONTEXT (handoff)

Last updated: 2026-10-25 (step 6 written, not yet run)
Project: BackupVouch, a Kubernetes operator in Go (kubebuilder v4) that runs PostgreSQL backups and verifies them by restoring into a scratch Postgres.
Repo: github.com/souvik-biswas-dev/backupvouch, `main` was at commit `bd5d81a` when step 6 started. Step 6 is in the zip `backupvouch-step6.zip`, not yet committed.

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
| 6 | tests and CI | written, **not run**: failure-path envtest specs, `hack/e2e.sh`, Makefile `verify`/`e2e`/`docker-build-runner`, CI workflows, TESTING.md. Needs `make verify` and `make e2e` on your machine |
| 7 | packaging and demo | not started (see "Left for step 7") |
| 8 | optional dashboard | not started |
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

## Known issues and VERIFY items

- Step 6 code is unverified. Expect small fixes. The least certain parts: `markJobFailed` accepted by the envtest API server (strict Job status validation), `kubectl run -i` with piped stdin in `hack/e2e.sh`, the image tags `minio/minio:RELEASE.2025-04-22T22-12-26Z` and `minio/mc:RELEASE.2025-04-16T18-13-26Z`, `/bin/sh` in the MinIO image, kind `v0.30.0` in CI, events visible through `kubectl get events` after the RBAC change.
- `config/rbac/role.yaml` was edited by hand. Run `make manifests`; if it differs, commit the generated file (CI fails on a diff otherwise).
- `make lint` was never confirmed. Run it once.
- `go mod tidy` was removed from CI (it hid an untidy go.mod). Run it locally before pushing.
- Coverage gaps: `cmd/runner` 30.7% (the paths that need Postgres or S3 are covered only by e2e), `internal/checks` 48.2% (`RunCheck` needs a database), `api/v1alpha1` 36.2% (mostly generated deepcopy), `cmd/main.go` has no tests.
- The scaffold's `config/manager/manager.yaml` has no `--allowed-runner-images`; only the default runner image is allowed (D14). A kind demo works because the runner is loaded under the default tag.
- Comparisons with Velero, CloudNativePG, K8up and FireDrill in `SPEC.md` are from memory; check them before the README claims anything. Name availability on GitHub, pkg.go.dev and Artifact Hub is unchecked (D9).
- Favicon should be looked at once in a real browser tab.
- README still says "design phase" (step 9).

## Decisions

See `DECISIONS.md`, D1 to D18. D17: e2e is a bash script, the scaffold Ginkgo e2e is removed. D18: events RBAC group and Job deletion propagation.

## Left for step 7 (so it does not redo step 6)

Already present from step 6: `make docker-build-runner`, `make deploy`/`make e2e`, `hack/e2e/` fixtures (MinIO, Postgres with seed data, policy). Step 7 still has to do: an install kustomization and `make build-installer` check, `config/samples/demo/` (can reuse `hack/e2e/*.yaml`), `hack/demo.sh` with the narrated story, the optional ghcr.io publishing workflow, and the demo transcript in TESTING.md. Consider fixing open items a, b and c first, since the demo will show restarts and failures.

## Next

1. Run the verify commands below, paste failures back (Fix prompt).
2. Commit step 6 (`git commit -s`, add `Assisted-by:` trailer per AGENTS.md).
3. Step 7, or first the small fixes a to e above.

## To verify locally

    make manifests generate fmt vet test     # same as: make verify
    git diff config/rbac/role.yaml           # should show no change after make manifests, else commit it
    make lint
    make e2e                                 # needs docker, kind, kubectl; paste the output if it fails
    # to keep the cluster for debugging:  E2E_KEEP_CLUSTER=true make e2e
