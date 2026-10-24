# RUNBOOK

What to do when BackupVouch reports a problem. Status: planned, to be updated with real output after the first end-to-end run.

First look:

    kubectl get backuppolicies
    kubectl describe backuppolicy <name>      # conditions and events
    kubectl get jobs -l backupvouch.dev/policy=<name>

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
| `BackupReady=False`, `SecretMissing` | a Secret named in the policy does not exist in the namespace | create it or fix `credentialsSecretRef`; the run is failed after about 60 seconds, not at the deadline |
| `BackupReady=False`, `InvalidRunnerResult` | the Job succeeded but its result was missing or invalid | read the Pod logs, check the runner image version |
| `BackupReady=False`, `BackupTimeout` | backup exceeded `backupTimeoutSeconds` | raise the timeout or investigate database size and load |
| `RestoreVerified=False`, `ChecksumMismatch` | the downloaded object differs from what was uploaded | treat the stored backup as corrupted, check storage and network, run a new backup |
| `RestoreVerified=False`, `RestoreFailed` | `pg_restore` failed | read the verification Pod logs, check extensions (the runner image only has the standard ones), the Postgres major version (the runner bundles 16), and that `verify.scratchSize` is big enough |
| `RestoreVerified=False`, `CheckFailed` | a check returned the wrong answer | the failing check is named in `status.lastVerification.failedChecks`, investigate the data |
| `RestoreVerified=False`, `StorageUnreachable` | download failed | check the endpoint, bucket, credentials Secret, and TLS setting |
| `RestoreVerified=False`, `SecretMissing` | the storage Secret does not exist | create it or fix `storage.credentialsSecretRef` |
| `RestoreVerified=False`, `VerificationTimeout` | restore plus checks exceeded the deadline | raise `verify.timeoutSeconds` or investigate size, this is also your real restore time warning |
| `RestoreVerified=False`, `InvalidRunnerResult` | the Job ended without a valid result (for example an OOM kill) | read the Pod logs, check the runner image version |

## Other conditions and events

| Signal | Meaning | What to do |
|---|---|---|
| `SpecValid=False`, `InvalidSchedule` or `InvalidTimeZone` | the cron expression or time zone is not valid | fix the spec; no runs start until then |
| `SpecValid=False`, `RunnerImageNotAllowed` | `runnerImage` is not in the operator's allow-list | use the default image or ask the operator admin to allow it |
| `RestoreVerified=Unknown`, `VerificationDisabled` | `verify.enabled` is false | expected |
| event `RetentionFailed` | the prune Job failed | nothing was deleted that must be kept; check the storage credentials allow deletes |

## Re-checking the newest backup

    kubectl annotate backuppolicy <name> backupvouch.dev/reverify="$(date +%s)" --overwrite

## A real restore

BackupVouch verifies backups but never restores into production. To restore, download the object named in `status.lastBackup.objectKey`, and use `pg_restore` into a new database. `status.lastVerification.restoreDurationSeconds` tells you roughly how long it will take.

## Pausing

Set `spec.suspend: true` to stop new runs. Running Jobs finish.
