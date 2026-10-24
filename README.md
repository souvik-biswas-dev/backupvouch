<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/brand/logo-dark.svg">
    <img alt="BackupVouch" src="assets/brand/logo.svg" width="320">
  </picture>
</p>

# BackupVouch

A Kubernetes operator that proves your database backups can be restored.

BackupVouch runs PostgreSQL backups on a schedule, restores each one into a throwaway Postgres, runs the SQL checks you define, and reports **Verified** or **Failed** through status conditions, events, and Prometheus metrics. A backup only counts once it has been restored and checked.

> **Status: design phase.** The documents in this repository describe what v0.1 will do. The code is being built step by step, see `docs/PLAN.md` and `docs/CONTEXT.md`. Nothing below is claimed to work yet.

## Why

Backup jobs usually report success when the command exits with code 0. A truncated dump, a rotated credential, or a missing table only shows up during an incident. BackupVouch turns "we have backups" into "we restored last night's backup and these checks passed".

## Planned quickstart

    hack/demo.sh        # creates a kind cluster, then: backup, verify OK, sabotage, verify FAILED
    kubectl get backuppolicies

The exact five commands will be fixed in step 7 and match `hack/demo.sh`.

## Planned example

    apiVersion: backup.backupvouch.dev/v1alpha1
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
        endpoint: minio.default.svc:9000
        bucket: backups
        credentialsSecretRef: { name: minio-creds }
      verify:
        checks:
          - name: users-not-empty
            sql: "SELECT count(*) > 0 FROM users"

## Documents

| Document | Contents |
|---|---|
| [docs/SPEC.md](docs/SPEC.md) | problem, goals, non-goals, v1 scope |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | components, reconcile flow, state machine |
| [docs/API.md](docs/API.md) | BackupPolicy reference |
| [docs/DECISIONS.md](docs/DECISIONS.md) | design decisions and reasons |
| [docs/TESTING.md](docs/TESTING.md) | test layers and the sabotage demo |
| [docs/RUNBOOK.md](docs/RUNBOOK.md) | what to do when something fails |
| [SECURITY.md](SECURITY.md) | threat model and permissions |
| [docs/BRAND.md](docs/BRAND.md) | logo concept, palette, usage rules |
| [AGENTS.md](AGENTS.md) | rules for AI assistants working in this repo |

## How this differs from other tools

See the comparison in [docs/SPEC.md](docs/SPEC.md). In short: many tools appear to focus on taking backups and restoring on demand (verify); BackupVouch focuses on automatically proving that the latest backup restores. FireDrill appears to be an existing project in the same space (verify), so BackupVouch does not claim to be first; its difference is one resource that takes the backup and proves it. Claims about other projects must be checked against their current documentation before release.

## Limitations (planned v0.1)

PostgreSQL only, and one Postgres major version (16) per runner image. No point-in-time recovery. Verification checks are SQL you write. No automatic restore into production.

## License

To be decided (Apache-2.0 suggested).
