# TESTING

Status: planned, nothing here has been run yet.

## Layers

| Layer | What it covers | Tooling | Where |
|---|---|---|---|
| Unit | cron scheduling, defaulting and validation helpers, Job builders, check evaluation | Go tests, table-driven | next to the code |
| Controller | state transitions, idempotency, failure paths | envtest (real API server and etcd, no kubelet) | `internal/controller/` |
| End-to-end | the whole story in a real cluster, including sabotage | kind, MinIO, seeded Postgres | `hack/e2e.sh` |

envtest does not run Jobs, so controller tests set Job and Pod status by hand and assert what the controller does next. Recent Kubernetes versions validate Job status strictly (for example a completed Job needs a completion time and matching conditions), so the test helper must build a consistent status; check this against the Kubernetes version of the scaffold (VERIFY in step 6).

## What the controller tests must prove

- A due schedule creates exactly one backup Job, even if reconcile runs several times.
- A successful backup result leads to a verification Job.
- A passing verification sets `RestoreVerified=True` and updates metrics.
- Each failure mode in `ARCHITECTURE.md` produces the right condition reason and event.
- A restart in the middle of a run resumes without duplicates.
- Deleting the policy removes the Jobs through the finalizer. envtest has no garbage collector, so tests check the finalizer logic and the owner references, not Kubernetes' cascading delete.
- A missing Secret (Pod waiting in `CreateContainerConfigError`) fails the run with `SecretMissing`.
- Missed schedules produce one run, and an overlapping schedule is skipped.
- Retention creates a prune Job only after a passed verification and passes the protected object.
- The re-verify annotation is acted on once.

## The sabotage demo

The point of the project: show that a bad backup is caught.

1. Deploy MinIO and a Postgres with seed data, apply a BackupPolicy.
2. Wait for `RestoreVerified=True`.
3. Corrupt the stored dump (truncate it, or overwrite bytes).
4. Re-verify the corrupted object with the annotation `backupvouch.dev/reverify=<unix seconds>`. Waiting for the next scheduled run would not work, because that run takes a fresh, good backup.
5. Expect `RestoreVerified=False` with a reason such as `ChecksumMismatch` or `RestoreFailed`, an event, and `backupvouch_verification_success` at 0.

`hack/e2e.sh` exits non-zero if either the healthy case or the sabotaged case behaves differently from this.

## Running tests

    make manifests generate fmt vet test     # unit and envtest
    make e2e                                 # needs Docker and kind

## CI

GitHub Actions (free for public repositories) runs formatting, vet, lint, unit and envtest tests, and the kind end-to-end script on every push and pull request. The kubebuilder scaffold already ships `lint.yml`, `test.yml` and `test-e2e.yml`; step 6 adapts those rather than adding a parallel `ci.yml`, and either wires `make test-e2e` to `hack/e2e.sh` or removes the scaffold's `test/e2e` suite.

## Demo transcript

To be added after the first successful end-to-end run, with real output.
