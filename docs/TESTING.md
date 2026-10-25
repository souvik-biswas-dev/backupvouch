# TESTING

Status: step 9. Unit and envtest layers pass locally (`make verify`, controller coverage 77.8% after step 6b). `make e2e` and `make demo` have run against a real kind cluster.

## Layers

| Layer | What it covers | Tooling | Where |
|---|---|---|---|
| Unit | cron scheduling, defaulting and validation helpers, Job builders, check evaluation | Go tests, table-driven | next to the code |
| Controller | state transitions, idempotency, failure paths | envtest (real API server and etcd, no kubelet) | `internal/controller/` |
| End-to-end | the whole story in a real cluster, including sabotage | kind, SeaweedFS (S3), seeded Postgres, a bash script | `hack/e2e.sh` (steps in `hack/lib.sh`), fixtures in `config/samples/demo/` |

envtest does not run Jobs, so controller tests set Job and Pod status by hand and assert what the controller does next. Recent Kubernetes versions validate Job status strictly (for example a completed Job needs a completion time and matching conditions), so the test helper must build a consistent status; check this against the Kubernetes version of the scaffold. The helpers `markJobComplete` and `markJobFailed` in the controller tests build a consistent status; if a Kubernetes upgrade rejects it, fix the helpers (VERIFY on a new envtest version).

## What the controller tests prove

Files: `internal/controller/backuppolicy_controller_test.go` (happy path, invalid schedule), `backuppolicy_failures_test.go` (idempotency and failure paths), `pod_reader_test.go` (termination message parsing, plain Go tests).

| Requirement | Covered by |
|---|---|
| A due schedule creates exactly one backup Job, even if reconcile runs several times | failures: "creates exactly one backup Job" |
| A restart between creating a Job and recording it creates no duplicate | failures: same spec, simulated by resetting status |
| An overlapping schedule is skipped | failures: same spec |
| A successful backup leads to a verification Job, a passing verification to a prune Job, history is recorded | happy path |
| A passing verification sets `RestoreVerified=True` | happy path |
| Metrics are updated | `internal/metrics` unit tests only, not asserted in envtest |
| Backup failure reasons: runner error message, `BackupTimeout`, `InvalidRunnerResult`, `SecretMissing` (and the stuck Job is removed) | failures: "backup failures" |
| Verification failure reasons: `ChecksumMismatch`, `CheckFailed` with failed check names, `VerificationTimeout`, `InvalidRunnerResult` | failures: "verification failures" |
| Retention does not run after a failed verification | failures: `ChecksumMismatch` spec |
| The re-verify annotation starts a verification and records `lastReverifyID` | happy path |
| Deleting the policy removes the Jobs through the finalizer (envtest has no garbage collector, so this checks the finalizer logic, not the cascading delete) | happy path |
| Invalid schedule sets `SpecValid=False` | invalid schedule spec |
| Events are emitted | not asserted in envtest (no recorder is wired); `hack/e2e.sh` checks `VerificationPassed` and `VerificationFailed` |

Not covered yet (candidates for later): the same re-verify value is never acted on twice, missed schedules produce a single run (covered by `internal/schedule` unit tests, not through the controller), the prune Job receives the protected object key, (a verification Job that disappeared mid-run is recreated since step 6b).

## The sabotage demo

The point of the project: show that a bad backup is caught.

1. Deploy an S3 store and a Postgres with seed data, apply a BackupPolicy.
2. Wait for `RestoreVerified=True`.
3. Corrupt the stored dump (truncate it, or overwrite bytes).
4. Re-verify the corrupted object with the annotation `backupvouch.dev/reverify=<unix seconds>`. Waiting for the next scheduled run would not work, because that run takes a fresh, good backup.
5. Expect `RestoreVerified=False` with a reason such as `ChecksumMismatch` or `RestoreFailed`, an event, and `backupvouch_verification_success` at 0.

`hack/e2e.sh` exits non-zero if either the healthy case or the sabotaged case behaves differently from this.

## What `hack/e2e.sh` does

1. Reads the default runner image from `internal/jobs/backup.go`, builds the operator image (`make docker-build`) and the runner image (`make docker-build-runner`) under the tag the operator uses by default, creates a kind cluster (`backupvouch-e2e`) and loads both images.
2. Deploys the operator with `make deploy` and switches its metrics endpoint to plain HTTP so the script can read it through the API server's pod proxy. The default install stays on HTTPS. `config/manager/kustomization.yaml` is restored when the script ends.
3. Deploys SeaweedFS as the S3 store (it creates the bucket at start, and the script then makes one signed request to prove the credentials work) and a Postgres 16 with a `users` table of three rows (`config/samples/demo/`), then applies `config/samples/demo/policy.yaml`.
4. **Healthy case.** Waits for `RestoreVerified=True`, then checks `lastVerification.result=Passed`, a stored object key and checksum, the run returning to `Idle`, the metric `backupvouch_verification_success 1` and a `VerificationPassed` event. If the backup or the verification fails here, the script stops at once and prints the reason.
5. **Sabotage.** Overwrites the stored dump with text with a signed S3 `PUT` from a one-off `curlimages/curl` Pod, annotates the policy with `backupvouch.dev/reverify=<unix seconds>`, waits for `RestoreVerified=False`, and checks reason `ChecksumMismatch`, `lastVerification.result=Failed` for the same object, `BackupReady` still `True`, a `VerificationFailed` event and the metric at 0.
6. Exit code 0 only if every check holds. On any failure it prints the policy YAML, Jobs, Pods, recent events, the operator log and the runner logs, then deletes the cluster (unless `E2E_KEEP_CLUSTER=true`, or the cluster already existed).

The steps are functions in `hack/lib.sh` (`stage_preflight`, `stage_images_and_cluster`, `stage_operator`, `stage_fixtures`, `stage_healthy`, `stage_sabotage`, `stage_final`). `hack/e2e.sh` is only the list of calls, and `hack/demo.sh` calls the same functions with commentary, so the demo cannot drift away from the test.

Variables: `E2E_CLUSTER`, `E2E_KEEP_CLUSTER`, `CONTROLLER_IMG`, `CURL_IMG`, `KIND`. Debugging a failure: `E2E_KEEP_CLUSTER=true make e2e`, then use `kubectl --context kind-backupvouch-e2e`.

If the corruption step fails on your machine, do it by hand while the cluster is kept (`E2E_KEEP_CLUSTER=true`). Replace `<objectKey>` with `status.lastBackup.objectKey` of the policy: `kubectl --context kind-backupvouch-e2e run s3-curl --rm -i --restart=Never --image=curlimages/curl:8.12.1 --command -- curl -sS -X PUT --aws-sigv4 aws:amz:us-east-1:s3 --user e2eaccesskey:e2esecretkey123 -H 'x-amz-content-sha256: UNSIGNED-PAYLOAD' --data-binary garbage http://s3.default.svc:8333/backups/<objectKey>`. If the S3 store itself does not start, read its Pod: `kubectl --context kind-backupvouch-e2e get pods,events -n default` (an `ImagePullBackOff` means the image tag in `config/samples/demo/s3.yaml` is gone from Docker Hub; pick a current SeaweedFS 4.42+ tag).

## The demo

    make demo                 # about 5 to 10 minutes the first time (image builds and pulls)
    make demo-clean           # delete the demo cluster afterwards

`hack/demo.sh` runs the same stages as the e2e test in its own kind cluster (`backupvouch-demo`) and narrates them. It prints the policy it applies, and after each result the `kubectl get backuppolicy` line, the status block (conditions, last backup, last verification, history) and the events. Result 1 is `RestoreVerified=True` after a real backup and restore. Result 2 is the same policy after the stored dump was overwritten with garbage and a re-verification was requested: `RestoreVerified=False`, reason `ChecksumMismatch`, a `VerificationFailed` event, while `BackupReady` is still `True`.

Differences from `make e2e`: the cluster is kept afterwards (`DEMO_KEEP_CLUSTER=false` deletes it), an old demo cluster is deleted first, and `DEMO_PAUSE=true` waits for Enter after each stage when presenting. Needs docker, kind and kubectl, like the e2e test. The images are built locally and loaded into kind, so nothing is pulled from ghcr.io.

## Running tests

    make verify      # = make manifests generate fmt vet test (unit and envtest)
    make lint        # golangci-lint
    make e2e         # kind end-to-end, needs docker, kind and kubectl (about 5 to 10 minutes)
    make demo        # the narrated version of the e2e story, keeps its cluster

The scaffold's Ginkgo e2e suite (`test/e2e`, `test/utils`) was removed in step 6, see D17.

## CI

GitHub Actions, free for public repositories, on every push and pull request:

| Workflow | Runs |
|---|---|
| `lint.yml` | `make lint-config`, `make lint` |
| `test.yml` | `make verify` (generate, fmt, vet, unit and envtest tests), then `git diff --exit-code` so generated code, CRDs, RBAC and formatting must be committed |
| `test-e2e.yml` | installs a pinned kind, then `make e2e` (30 minute limit) |
| `publish-images.yml` | on a `vX.Y.Z` tag (or by hand, without pushing): builds the operator and runner images and pushes them to ghcr.io. The tag must equal the tag of `DefaultRunnerImage` |

`go mod tidy` is no longer run in CI (the scaffold ran it before the tests, which hides a go.mod that needs tidying). Run `go mod tidy` yourself when you change dependencies.

## Demo transcript

<!-- DEMO-TRANSCRIPT-PLACEHOLDER: replace this whole block with real output. Record it with `make demo-transcript` (saved to dist/demo-transcript.txt). Do not edit the output; image build lines may be cut. -->

**Not recorded yet.** Run `make demo-transcript`, then paste the applied policy, "Result 1", "Result 2" and the final `DEMO FINISHED` line here, inside a code block. To find this spot again: `grep -n DEMO-TRANSCRIPT docs/TESTING.md`.

## Dashboard tests (step 8)

- `go test ./internal/dashboard/...`: verdict and run-state tables, ordering, next run, no connection details in the JSON, read-only methods, headers, no directory listing, error without detail, and file checks (brand copies identical, no remote URLs, no inline code, no `innerHTML`, referenced fonts present).
- `make dashboard-js-test`: node tests for the scale (including minute steps and the 30 minute minimum window), formatting, headline text and the failure wording by reason. Not covered: drawing; look at it in a browser at 1280 px and 375 px (`make demo` creates three runs a few minutes apart).
