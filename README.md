<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/brand/logo-dark.svg">
    <img alt="BackupVouch" src="assets/brand/logo.svg" width="320">
  </picture>
</p>

# BackupVouch

Project site: https://backupvouch.pages.dev

A Kubernetes operator that proves your database backups can be restored.

BackupVouch runs PostgreSQL backups on a schedule, restores each one into a throwaway Postgres, runs the SQL checks you define, and reports the result through status conditions, events, Prometheus metrics and an optional read-only dashboard.

**A backup only counts once it has been restored and checked.** A backup job that exits with code 0 proves very little: a truncated dump, a rotated credential or a missing table usually shows up during an incident. BackupVouch turns "we have backups" into "we restored last night's backup and these checks passed".

> **Status: early (API `v1alpha1`).** The operator, the runner and the dashboard work in a kind cluster (`make demo`, `make e2e`). It has not been run in production, the API may still change, and no release has been published yet.

## Quickstart

You need docker, kind and kubectl.

    make demo           # builds the images, creates a kind cluster, then: backup, verify OK, sabotage, verify FAILED
    make demo-clean     # delete the demo cluster afterwards

The demo takes a real backup of a small Postgres, restores it and passes the checks. It then overwrites the stored backup with garbage and asks for a re-check, which must fail. It prints the policy status and events after each stage. See [docs/TESTING.md](docs/TESTING.md).

To install the operator in your own cluster, see [docs/INSTALL.md](docs/INSTALL.md).

## Example

    apiVersion: backup.backupvouch.pages.dev/v1alpha1
    kind: BackupPolicy
    metadata:
      name: app-db
    spec:
      schedule: "0 2 * * *"
      source:
        host: postgres.default.svc
        database: app
        credentialsSecretRef: { name: pg-creds }
      storage:
        endpoint: s3.default.svc:8333
        bucket: backups
        credentialsSecretRef: { name: s3-creds }
      verify:
        checks:
          - name: users-not-empty
            sql: "SELECT count(*) > 0 FROM users"

## What you see

    kubectl get backuppolicies        # short name: bp
    kubectl describe backuppolicy app-db

| Where | What it tells you |
|---|---|
| `status.conditions` | `SpecValid`, `BackupReady` and `RestoreVerified`, each True, False (with a reason) or Unknown |
| `status.lastBackup` | time, result, stored object key, size and SHA-256 |
| `status.lastVerification` | result, restore time in whole seconds, `failedChecks`, and a `message` saying why it failed |
| `status.history` | the last 10 runs |
| Events | `BackupSucceeded`, `VerificationPassed`, `VerificationFailed` (with the reason), `RetentionPruned` and others |
| Metrics | `backupvouch_*` series, for example `backupvouch_verification_success` and `backupvouch_last_verified_timestamp_seconds` |

Every field and reason is in [docs/API.md](docs/API.md). Alert examples are in [docs/RUNBOOK.md](docs/RUNBOOK.md).

## Dashboard (optional, off by default)

A small read-only page that answers one question per policy: can the newest backup be trusted? It is unauthenticated and listens on the loopback address inside the operator Pod, so you reach it with `kubectl port-forward`.

    make dashboard-enable
    make dashboard         # then open http://localhost:9090

Details and the security notes are in [docs/INSTALL.md](docs/INSTALL.md) and [SECURITY.md](SECURITY.md).

## Security

The operator runs Jobs with a fixed non-root user, a read-only root filesystem and no capabilities. Only allow-listed runner images are used, and failure messages are kept short and never carry text or row contents (a failed check may show a boolean or a short number, see [ARCHITECTURE.md](docs/ARCHITECTURE.md)). The threat model, the operator's RBAC and a recommended NetworkPolicy are in [SECURITY.md](SECURITY.md). To report a vulnerability, follow the instructions there.

## Documents

| Document | Contents |
|---|---|
| [docs/SPEC.md](docs/SPEC.md) | problem, goals, non-goals, v1 scope |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | components, reconcile flow, state machine |
| [docs/API.md](docs/API.md) | BackupPolicy reference |
| [docs/DECISIONS.md](docs/DECISIONS.md) | design decisions and reasons |
| [docs/INSTALL.md](docs/INSTALL.md) | install, runner image allow-list, dashboard, uninstall |
| [docs/TESTING.md](docs/TESTING.md) | test layers, the sabotage demo, CI |
| [docs/RUNBOOK.md](docs/RUNBOOK.md) | what to do when something fails |
| [docs/MONITORING.md](docs/MONITORING.md) | metrics, alert rules and a Grafana dashboard |
| [docs/UPGRADING.md](docs/UPGRADING.md) | how to upgrade, and what is not tested yet |
| [docs/RELEASE.md](docs/RELEASE.md) | the release steps, image signing, the project site |
| [SECURITY.md](SECURITY.md) | threat model and permissions |
| [docs/BRAND.md](docs/BRAND.md) | logo concept, palette, usage rules |
| [AGENTS.md](AGENTS.md) | rules for AI assistants working in this repo |
| [CONTRIBUTING.md](CONTRIBUTING.md), [CHANGELOG.md](CHANGELOG.md) | how to contribute, what changed |

## How this differs from other tools

BackupVouch is not the first tool in this space. Velero, CloudNativePG and K8up take or restore backups, but their documentation does not describe a scheduled restore-and-check of the data (K8up's `Check` tests the integrity of its restic repository, not a restore). FireDrill verifies backups made by other tools, and does much more on the verification side (signed evidence, RTO and RPO targets), but it does not take backups. BackupVouch's narrower idea is one Kubernetes resource that takes the PostgreSQL backup and proves it, with a small codebase and no other backup tool. The table, with the date it was checked, is in [docs/SPEC.md](docs/SPEC.md).

## Limitations

PostgreSQL only, and one Postgres major version (16) per runner image. No point-in-time recovery. Verification checks are SQL you write. BackupVouch never restores into production. The API is `v1alpha1`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). In short: `make fmt && make verify` before a pull request, and sign your commits.

## License

Apache-2.0, see [LICENSE](LICENSE).
