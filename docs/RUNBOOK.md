# RUNBOOK

What to do when BackupVouch reports a problem. Status: reasons and checks below match the controller after step 6b and a real `make demo` run.

First look:

    kubectl get backuppolicies
    kubectl describe backuppolicy <name>      # conditions and events
    kubectl get jobs -l backupvouch.pages.dev/policy=<name>

## Alerts (examples)

Backup is stale (no new backup for more than 26 hours):

    time() - backupvouch_last_backup_timestamp_seconds > 26 * 3600

Verification failed:

    backupvouch_verification_success == 0

Verification is stale (the newest backup has not been proven good for more than 26 hours):

    time() - backupvouch_last_verified_timestamp_seconds > 26 * 3600

## Where to read the details

Three places hold the reason, from short to long:

1. `kubectl get backuppolicies` shows only `VERIFIED` (True, False or Unknown).
2. `status.lastVerification` has the detail: `failedChecks` names each failing check, and `message` says why. With failed checks, `message` holds their messages. Without any (for example `ChecksumMismatch`, where nothing was restored), `message` is a fixed text for the reason. The same text is in the `VerificationFailed` event (`kubectl describe backuppolicy <name>`).
3. The Pod logs of the verify Job (`kubectl logs -l backupvouch.pages.dev/role=verify`) have the full output. The runner keeps messages short and free of text or row contents (a failed check may show a boolean or a short number), so go to the logs for the rest.

`Restore took: Not recorded` in the dashboard is normal for a checksum failure: no restore happened.

## Conditions and what to do

| Condition and reason | Meaning | What to do |
|---|---|---|
| `BackupReady=False`, `SourceUnreachable` | the backup Job could not connect to PostgreSQL | check host, port, credentials Secret, NetworkPolicy, and that the role can connect |
| `BackupReady=False`, `StorageUnreachable` | upload failed | check the endpoint, bucket, credentials Secret, and TLS setting |
| `BackupReady=False`, `BackupJobFailed` | the Job failed for another reason | read the Job's Pod logs |
| `BackupReady=False`, `SecretMissing` | a Secret named in the policy does not exist in the namespace | create it or fix `credentialsSecretRef`; the run is failed after about 60 seconds, not at the deadline |
| `BackupReady=False`, `InvalidRunnerResult` | the Job succeeded but its result was missing or invalid | read the Pod logs, check the runner image version |
| `BackupReady=False`, `BackupTimeout` | backup exceeded `backupTimeoutSeconds` | raise the timeout or investigate database size and load |
| `RestoreVerified=False`, `ChecksumMismatch` | the stored object does not match the checksum recorded when the backup was taken, so nothing was restored | treat the stored backup as corrupted, check storage and network, run a new backup |
| `RestoreVerified=False`, `RestoreFailed` | `pg_restore` failed | read the verification Pod logs, check extensions (the runner image only has the standard ones), the Postgres major version (the runner bundles 16), and that `verify.scratchSize` is big enough |
| `RestoreVerified=False`, `CheckFailed` | a check returned the wrong answer | the failing check is named in `status.lastVerification.failedChecks` and its detail is in `status.lastVerification.message`; investigate the data |
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

    kubectl annotate backuppolicy <name> backupvouch.pages.dev/reverify="$(date +%s)" --overwrite

## A real restore

BackupVouch verifies backups but never restores into production. To restore, download the object named in `status.lastBackup.objectKey`, and use `pg_restore` into a new database. `status.lastVerification.restoreDurationSeconds` tells you roughly how long it will take.

## Pausing

Set `spec.suspend: true` to stop new work. A run in progress finishes. While a policy is suspended and idle it also ignores the `backupvouch.pages.dev/reverify` annotation: set `suspend` back to `false` to re-check a backup.

## A Job was deleted by hand

If you delete the Job of a run that is in progress, the controller recreates it (a verify Job from `status.lastBackup`, a backup Job from the same run ID), so deleting Jobs by hand only restarts that stage.
