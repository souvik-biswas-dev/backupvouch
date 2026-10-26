# Testing BackupVouch on a real database

The demo database has one table and three rows. Real databases break things the demo cannot: roles and ownership, extensions, large objects, size, time. This page is the checklist for the first real runs, and the table to fill in so a failure can be fixed quickly. Never test against production: use a copy, or a read-only replica.

## Before you start

- PostgreSQL 16 only for now (the runner image has the 16 tools). A 14, 15 or 17 source may work for plain dumps but is not tested.
- A Kubernetes cluster: kind is fine. S3-compatible storage the cluster can reach (the demo's SeaweedFS is fine for a first run).
- Secrets: the source database credentials and the S3 keys. The keys the Secrets must contain are in `docs/API.md`. Use a database user that can read everything it should dump.
- Start from `config/samples/backup_v1alpha1_backuppolicy.yaml`, set `source`, `storage` and `verify.checks`.

## Checks to write

SQL checks run against the **restored** copy. Write checks that match your data, and always include one that must fail:

1. A row count that you can compare, for example `SELECT count(*) > 0 FROM <important table>` with `expect: "true"`.
2. A recent-data check, for example `SELECT max(created_at) > now() - interval '2 days' FROM <table>`.
3. A deliberately wrong check (`SELECT count(*) FROM <table>` with `expect: "-1"`). The run must report `CheckFailed` and name the check. If it passes, tell the project.

## Things to try, one at a time

| # | Try | What should happen |
|---|---|---|
| 1 | The plain first run | `RestoreVerified=True`, a restore time |
| 2 | A database with several schemas, extensions (for example `pgcrypto`, `uuid-ossp`, `postgis`) and custom roles that own tables | restore passes, or fails with a message that names the problem (missing extension, role does not exist) |
| 3 | A larger database (hundreds of MB or more) | note the backup time, the restore time and whether `scratchSize` was enough; raise `verify.timeoutSeconds` if needed |
| 4 | Wrong source password | `BackupReady=False` with a reason, no hung run |
| 5 | Wrong S3 key or endpoint | a clear failure reason, no hung run |
| 6 | The deliberately wrong check from above | `CheckFailed`, the check named |
| 7 | Delete the running Job (`kubectl delete job ...`) | the controller recreates it; no second run |
| 8 | Restart the operator while a run is in progress | the run resumes or finishes; metrics come back |
| 9 | `kubectl annotate backuppolicy NAME backupvouch.pages.dev/reverify=$(date +%s)` | the stored backup is checked again |

## What to write down

Copy this table into an issue or into a chat, one row per try:

| # | Postgres version | DB size | Backup time | Restore time | Result | Message or error (copy it) |
|---|---|---|---|---|---|---|

And for any failure paste:

    kubectl describe backuppolicy NAME
    kubectl get backuppolicy NAME -o yaml          # the status part
    kubectl logs -l backupvouch.pages.dev/role=verify --all-containers --tail=100
    kubectl logs -n backupvouch-system deploy/backupvouch-controller-manager --tail=100

Remove hostnames and secrets first. Failure messages never include text or row contents, but a failed check can show a boolean or a short number (for example `got "3"`), so check before you share.

## What counts as done

Three different real databases (different size or content) each ran: a passing run, a deliberately failing check, and one broken-credential run, with clear messages every time. Until then call the project an early alpha.

## Results (2026-10-25)

Run on a kind cluster with the v0.1.0 `install.yaml`, PostgreSQL 16 sources, SeaweedFS as S3. The sample databases are public datasets (neondatabase/postgres-sample-dbs), not production data. Times are wall-clock Job durations.

| Database | Source size | Dump size | Backup | Restore | Checks |
|---|---|---|---|---|---|
| Demo (`app`, 3 rows) | tiny | 2.9 kB | 11 s | 1 s | 2 passed |
| Synthetic (4 extensions, 2 custom roles, 6M-row table) | 1006 MB | 158 MB | 17 s | 13 to 26 s | 5 passed |
| Chinook (media store) | 9.9 MB | 163 kB | 4 s | 1 s | 2 passed |
| Lego (580k-row table) | 43 MB | 3.1 MB | 4 s | 1 s | 2 passed |
| Pagila (partitions, enums, full text) | 17 MB | 801 kB | 4 s | 1 s | 2 passed |

For the demo, Chinook, Lego and Pagila databases: a deliberately wrong check gave `Failed` and named `must-fail`, and a wrong source password gave `BackupReady=False` with no hung Job. A wrong S3 key (demo database) gave `BackupReady=False` with a clear message. Deleting a running backup Job recreated it with no duplicate run, and restarting the operator mid-run finished the run (synthetic database). All recovered when the fault was removed. See `docs/KNOWN-LIMITATIONS.md` for what these runs showed.
