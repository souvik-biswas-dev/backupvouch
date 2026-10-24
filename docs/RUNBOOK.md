# RUNBOOK

What to do when BackupVouch reports a problem. Status: planned, to be updated with real output after the first end-to-end run.

First look:

    kubectl get backuppolicies
    kubectl describe backuppolicy <name>      # conditions and events
    kubectl get jobs -l backupvouch/policy=<name>

## Alerts (examples)

Backup is stale (no new backup for more than 26 hours):

    time() - backupvouch_last_backup_timestamp_seconds > 26 * 3600

Verification failed:

    backupvouch_verification_success == 0

Verification is stale (the newest backup has not been proven good for more than 26 hours):

    time() - backupvouch_last_verified_timestamp_seconds > 26 * 3600

## Conditions and what to do

| Condition and reason | Meaning | What to do |
|---|---|---|
| `BackupReady=False`, `SourceUnreachable` | the backup Job could not connect to PostgreSQL | check host, port, credentials Secret, NetworkPolicy, and that the role can connect |
| `BackupReady=False`, `StorageUnreachable` | upload failed | check the endpoint, bucket, credentials Secret, and TLS setting |
| `BackupReady=False`, `BackupJobFailed` | the Job failed for another reason | read the Job's Pod logs |
| `BackupReady=False`, `BackupTimeout` | backup exceeded `backupTimeoutSeconds` | raise the timeout or investigate database size and load |
| `RestoreVerified=False`, `ChecksumMismatch` | the downloaded object differs from what was uploaded | treat the stored backup as corrupted, check storage and network, run a new backup |
| `RestoreVerified=False`, `RestoreFailed` | `pg_restore` failed | read the verification Pod logs, check extensions and Postgres version compatibility |
| `RestoreVerified=False`, `CheckFailed` | a check returned the wrong answer | the failing check is named in `status.lastVerification.failedChecks`, investigate the data |
| `RestoreVerified=False`, `VerificationTimeout` | restore plus checks exceeded the deadline | raise `verify.timeoutSeconds` or investigate size, this is also your real restore time warning |
| `RestoreVerified=False`, `InvalidRunnerResult` | the Job ended without a valid result | read the Pod logs, check the runner image version |

## A real restore

BackupVouch verifies backups but never restores into production. To restore, download the object named in `status.lastBackup.objectKey`, and use `pg_restore` into a new database. `status.lastVerification.restoreDurationSeconds` tells you roughly how long it will take.

## Pausing

Set `spec.suspend: true` to stop new runs. Running Jobs finish.
