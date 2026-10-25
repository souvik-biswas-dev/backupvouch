# API reference: BackupPolicy

Group `backup.backupvouch.pages.dev`, version `v1alpha1`, kind `BackupPolicy`, short name `bp`, namespaced. Field names here are final for v0.1.

## Example

```yaml
apiVersion: backup.backupvouch.pages.dev/v1alpha1
kind: BackupPolicy
metadata:
  name: app-db
  namespace: default
spec:
  schedule: "0 2 * * *"
  timeZone: UTC
  suspend: false
  source:
    engine: postgres
    host: postgres.default.svc
    port: 5432
    database: app
    credentialsSecretRef:
      name: pg-creds            # keys: username, password
  storage:
    endpoint: s3.default.svc:8333
    bucket: backups
    prefix: app/
    useTLS: false
    credentialsSecretRef:
      name: s3-creds            # keys: accessKey, secretKey
  retention:
    keepLast: 7
  backupTimeoutSeconds: 1800
  verify:
    enabled: true
    timeoutSeconds: 900
    scratchSize: 5Gi
    checks:
      - name: users-not-empty
        sql: "SELECT count(*) > 0 FROM users"
        expect: "true"
  runnerImage: ghcr.io/souvik-biswas-dev/backupvouch-runner:v0.1.0
  resources:
    limits: { cpu: "1", memory: 1Gi }
```

## Spec

| Field | Type | Default | Notes |
|---|---|---|---|
| `schedule` | string | required | five-field cron expression |
| `timeZone` | string | `UTC` | IANA time zone name |
| `suspend` | bool | `false` | stops all new work. A run that is already in progress finishes. While suspended and idle, the controller does nothing else: no scheduled runs, no manual re-verification (the `backupvouch.pages.dev/reverify` annotation is ignored until `suspend` is false again), and `BackupReady` and `RestoreVerified` are not created for a policy that never ran (`SpecValid` still is). Metrics and the dashboard keep showing the last status |
| `source.engine` | enum | `postgres` | only `postgres` in v1 |
| `source.host` | string | required | host or service name |
| `source.port` | int | `5432` | 1-65535 |
| `source.database` | string | required | database to dump |
| `source.credentialsSecretRef.name` | string | required | Secret with `username` and `password` |
| `storage.endpoint` | string | required | `host:port` of S3-compatible storage |
| `storage.bucket` | string | required | bucket name |
| `storage.prefix` | string | empty | object key prefix |
| `storage.useTLS` | bool | `false` | |
| `storage.credentialsSecretRef.name` | string | required | Secret with `accessKey` and `secretKey` |
| `retention.keepLast` | int | `7` | minimum 1 |
| `backupTimeoutSeconds` | int | `1800` | Job `activeDeadlineSeconds` for backup |
| `verify.enabled` | bool | `true` | if false, a run ends at `BackedUp` |
| `verify.timeoutSeconds` | int | `900` | Job deadline for verification |
| `verify.scratchSize` | quantity | `5Gi` | size limit of the `emptyDir` that holds the downloaded dump and the scratch Postgres data; must exceed the dump plus the restored database |
| `verify.checks` | list | empty | at most 20 entries, a map keyed by `name` (the API server enforces unique names) |
| `verify.checks[].name` | string | required | DNS-label style |
| `verify.checks[].sql` | string | required | one statement that returns exactly one row and one column, at most 2000 characters |
| `verify.checks[].expect` | string | `"true"` | expected text of the single value, compared after normalisation (see below) |
| `runnerImage` | string | operator default | must match the operator's allow-list of runner images (default: only the operator's own default image), see D14 |
| `resources` | ResourceRequirements | modest defaults | applied to runner containers |

The name of a BackupPolicy is at most 45 characters, because Job names (`<policy>-verify-<runID>`) and label values must stay within 63 characters. A CEL rule on the type enforces this.

### How a check result is compared

The runner runs each check inside a read-only transaction with a statement timeout, on the restored copy only. The single returned value is turned into text: booleans become `true` or `false` (not `t` or `f`), everything else uses the driver's text form. The check passes when that text equals `expect` exactly. Multi-statement SQL is rejected.

## Status

| Field | Meaning |
|---|---|
| `observedGeneration` | last spec generation reconciled |
| `phase` | `Idle`, `BackingUp`, `Verifying`, or `Pruning` |
| `currentRunID` | Unix timestamp (scheduled time) of the run in progress, empty when idle |
| `lastRunID` | `runID` of the most recent run that was started; used with the schedule to decide whether a run is due |
| `lastReverifyID` | the last value of the `backupvouch.pages.dev/reverify` annotation that was acted on |
| `lastBackup.time` | when the last backup finished |
| `lastBackup.result` | `Succeeded` or `Failed` |
| `lastBackup.objectKey`, `sizeBytes`, `sha256` | the stored artifact |
| `lastBackup.message` | short reason on failure |
| `lastVerification.time` | when the last verification finished |
| `lastVerification.result` | `Passed` or `Failed` |
| `lastVerification.objectKey` | the backup object that was verified (the newest verified backup is protected from retention) |
| `lastVerification.restoreDurationSeconds` | measured restore time in whole seconds (rounded up by the controller, so a restore that took 0.4 s shows 1; the runner measures with a fraction; CRDs avoid floating-point fields, see D16) |
| `lastVerification.failedChecks` | names of checks that failed |
| `lastVerification.message` | on failure, the messages of the failed checks, or a fixed text for the reason when no check ran (for example `ChecksumMismatch`: the stored backup does not match the checksum recorded when it was taken). Empty on success |
| `history` | the last 10 runs: `runID`, time, objectKey, backup result, verification result |
| `conditions` | see below |

### Conditions

| Type | True means | False reasons |
|---|---|---|
| `SpecValid` | the spec passed the checks the CRD schema cannot do | `InvalidSchedule`, `InvalidTimeZone`, `RunnerImageNotAllowed` |
| `BackupReady` | the last backup succeeded | `SourceUnreachable`, `StorageUnreachable`, `SecretMissing`, `BackupJobFailed`, `BackupTimeout`, `InvalidRunnerResult` |
| `RestoreVerified` | the last backup was restored and every check passed | `ChecksumMismatch`, `RestoreFailed`, `CheckFailed`, `StorageUnreachable`, `SecretMissing`, `VerificationTimeout`, `InvalidRunnerResult` |

Reasons while a condition is **not** False:

| Type | Status | Reason | When |
|---|---|---|---|
| `SpecValid` | True | `SpecValid` | the spec passed validation |
| `BackupReady` | True | `BackupSucceeded` | the last backup succeeded |
| `BackupReady` | Unknown | `NoRunYet` | no backup has finished yet |
| `RestoreVerified` | True | `VerificationPassed` | the last backup was restored and every check passed |
| `RestoreVerified` | Unknown | `NoRunYet` | no verification has finished yet |
| `RestoreVerified` | Unknown | `VerificationDisabled` | `verify.enabled` is false |

The controller only accepts a verification failure reason from the list above (`ChecksumMismatch`, `RestoreFailed`, `CheckFailed`, `StorageUnreachable`, `SecretMissing`, `VerificationTimeout`, `InvalidRunnerResult`). Any other reason sent by a runner is recorded as `RestoreFailed`.

`BackupReady` and `RestoreVerified` are `Unknown` until the first backup or verification finishes. When `verify.enabled` is false, `RestoreVerified` stays `Unknown` with reason `VerificationDisabled`. While `SpecValid` is False, no new run starts.

## Manual re-verification

To re-check the newest stored backup without taking a new one (for example to detect bit rot, or in the sabotage demo), set an annotation with any new Unix timestamp:

    kubectl annotate backuppolicy app-db backupvouch.pages.dev/reverify="$(date +%s)" --overwrite

When the policy is `Idle`, the controller creates a verification Job named `<policy>-verify-<value>` for `status.lastBackup`, records the value in `status.lastReverifyID`, and updates `lastVerification` and `RestoreVerified` as usual. The same value is never acted on twice.

## Printer columns

`kubectl get backuppolicies` shows: `SCHEDULE`, `PHASE`, `LAST BACKUP`, `VERIFIED` (the `RestoreVerified` condition status), `RESTORE TIME`, `AGE`.

## Validation rules

- `schedule` must be a valid five-field cron expression, and `timeZone` a valid IANA name. The schema checks the shape; the controller checks the rest and reports `SpecValid=False` (`InvalidSchedule`, `InvalidTimeZone`).
- `retention.keepLast` is at least 1.
- The BackupPolicy name is at most 45 characters.
- Check names are unique (list map key) and match `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`. At most 20 checks.
- `sql` must not be empty. Checks run only against the restored copy.
- Secret references name Secrets in the same namespace.
