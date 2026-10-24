# TESTING

Status: planned, nothing here has been run yet.

## Layers

| Layer | What it covers | Tooling | Where |
|---|---|---|---|
| Unit | cron scheduling, defaulting and validation helpers, Job builders, check evaluation | Go tests, table-driven | next to the code |
| Controller | state transitions, idempotency, failure paths | envtest (real API server and etcd, no kubelet) | `internal/controller/` |
| End-to-end | the whole story in a real cluster, including sabotage | kind, MinIO, seeded Postgres | `hack/e2e.sh` |

envtest does not run Jobs, so controller tests set Job and Pod status by hand and assert what the controller does next.

## What the controller tests must prove

- A due schedule creates exactly one backup Job, even if reconcile runs several times.
- A successful backup result leads to a verification Job.
- A passing verification sets `RestoreVerified=True` and updates metrics.
- Each failure mode in `ARCHITECTURE.md` produces the right condition reason and event.
- A restart in the middle of a run resumes without duplicates.
- Deleting the policy removes the Jobs.

## The sabotage demo

The point of the project: show that a bad backup is caught.

1. Deploy MinIO and a Postgres with seed data, apply a BackupPolicy.
2. Wait for `RestoreVerified=True`.
3. Corrupt the stored dump (truncate it, or overwrite bytes).
4. Trigger the next run, or verify the corrupted object.
5. Expect `RestoreVerified=False` with a reason such as `ChecksumMismatch` or `RestoreFailed`, an event, and `backupvouch_verification_success` at 0.

`hack/e2e.sh` exits non-zero if either the healthy case or the sabotaged case behaves differently from this.

## Running tests

    make manifests generate fmt vet test     # unit and envtest
    make e2e                                 # needs Docker and kind

## CI

GitHub Actions (free for public repositories) runs formatting, vet, unit and envtest tests, and the kind end-to-end script on every push and pull request.

## Demo transcript

To be added after the first successful end-to-end run, with real output.
