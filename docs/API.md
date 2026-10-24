# API reference: BackupPolicy

Group `backup.backupvouch.dev`, version `v1alpha1`, kind `BackupPolicy`, short name `bp`, namespaced. Field names here are final for v0.1.

## Example

```yaml
apiVersion: backup.backupvouch.dev/v1alpha1
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
    endpoint: minio.default.svc:9000
    bucket: backups
    prefix: app/
    useTLS: false
    credentialsSecretRef:
      name: minio-creds         # keys: accessKey, secretKey
  retention:
    keepLast: 7
  backupTimeoutSeconds: 1800
  verify:
    enabled: true
    timeoutSeconds: 900
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
| `suspend` | bool | `false` | pauses scheduling, running Jobs finish |
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
| `verify.checks[].name` | string | required | unique within the policy, DNS-label style |
| `verify.checks[].sql` | string | required | must return exactly one row and one column |
| `verify.checks[].expect` | string | `"true"` | expected text of the single value |
| `runnerImage` | string | operator default | pinned tag recommended |
| `resources` | ResourceRequirements | modest defaults | applied to runner containers |

## Status

| Field | Meaning |
|---|---|
| `observedGeneration` | last spec generation reconciled |
| `phase` | `Idle`, `BackingUp`, or `Verifying` |
| `currentRunID` | Unix timestamp of the run in progress, empty when idle |
| `lastBackup.time` | when the last backup finished |
| `lastBackup.result` | `Succeeded` or `Failed` |
| `lastBackup.objectKey`, `sizeBytes`, `sha256` | the stored artifact |
| `lastBackup.message` | short reason on failure |
| `lastVerification.time` | when the last verification finished |
| `lastVerification.result` | `Passed` or `Failed` |
| `lastVerification.restoreDurationSeconds` | measured restore time |
| `lastVerification.failedChecks` | names of checks that failed |
| `lastVerification.message` | short reason on failure |
| `history` | the last 10 runs: time, objectKey, backup result, verification result |
| `conditions` | see below |

### Conditions

| Type | True means | False reasons |
|---|---|---|
| `BackupReady` | the last backup succeeded | `SourceUnreachable`, `StorageUnreachable`, `BackupJobFailed`, `BackupTimeout` |
| `RestoreVerified` | the last backup was restored and every check passed | `ChecksumMismatch`, `RestoreFailed`, `CheckFailed`, `VerificationTimeout`, `InvalidRunnerResult` |

`RestoreVerified` is `Unknown` until the first verification finishes.

## Printer columns

`kubectl get backuppolicies` shows: `SCHEDULE`, `PHASE`, `LAST BACKUP`, `VERIFIED` (the `RestoreVerified` condition status), `RESTORE TIME`, `AGE`.

## Validation rules

- `schedule` must be a valid five-field cron expression.
- `timeZone` must be a valid IANA name.
- `retention.keepLast` is at least 1.
- Check names are unique and match `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`.
- `sql` must not be empty. Checks run only against the restored copy.
- Secret references name Secrets in the same namespace.
