# SPEC

Status: draft v0.1

## Problem

Most teams back up their databases and almost none of them test the restore. A backup job can exit successfully while the dump is truncated, the credentials have rotated, a table is missing, or the restore takes six hours instead of ten minutes. Teams find out during an incident.

## Users

Small teams running PostgreSQL on Kubernetes (including k3s and kind) who want evidence, not hope, that their latest backup works.

## Goals

1. Run PostgreSQL backups on a schedule and store them in S3-compatible storage.
2. After every backup, restore it into a throwaway Postgres and run user-defined SQL checks.
3. Report the result in the resource status, Kubernetes events, and Prometheus metrics, so it can be alerted on.
4. Measure restore duration, so the team knows its real recovery time.
5. Be free to run and demo locally (kind and an in-cluster S3 store), with no cloud account.

## Non-goals (v1)

- Backing up cluster objects or volumes (tools such as Velero cover that, verify).
- Point-in-time recovery or WAL archiving.
- Engines other than PostgreSQL (MongoDB and Redis are later work).
- Restoring into production automatically.
- Multi-cluster management.
- A required UI (a read-only dashboard is optional).

## v1 scope

- One CRD, `BackupPolicy`, see `API.md`.
- A controller that runs backup and verification as Kubernetes Jobs, see `ARCHITECTURE.md`.
- One runner image with `backup` and `verify` subcommands.
- Retention: keep the last N backups, never deleting the newest verified one, done by a prune Job.
- Manual re-verification of the newest backup with an annotation.
- Status conditions `BackupReady` and `RestoreVerified`, events, and four metrics.
- A demo that backs up, verifies, sabotages the stored backup, re-verifies it, and shows verification failing.
- One Postgres major version (16) per runner image.

## User stories

- As an operator of a small cluster, I declare a schedule and checks, and I can see at a glance whether last night's backup restored correctly.
- As an on-call engineer, I get an alert when the newest backup is too old or its verification failed, with the failing check named.
- As a team lead, I can tell auditors how long a real restore takes.
- As a security reviewer, I can see that the operator never reads secrets, and I can enforce with a NetworkPolicy that verification reaches nothing but storage.

## Success criteria

- Corrupting a stored backup leads to `RestoreVerified=False` at the next verification (scheduled, or triggered with the re-verify annotation), with the cause visible in status and events.
- A controller crash at any point creates no duplicate Jobs and leaves no scratch resources behind.
- The whole demo runs on a laptop in under ten minutes without a cloud account.

## Comparison with similar tools

Checked on 2026-10-25 against each project's current documentation (README, pkg.go.dev page or documentation site). "Not found" means the documentation I read does not describe the feature; it does not prove the feature is absent. Re-check before quoting this table elsewhere.

| Tool | What it is | Takes the backup itself | Restores it on a schedule and checks the data |
|---|---|---|---|
| Velero | Backup and restore of Kubernetes objects and persistent volumes. Backups, schedules and restores are custom resources; restore can remap namespaces and run restore hooks | yes (cluster objects and volume snapshots, not a database dump) | not described in its documentation; a restore is something you run |
| CloudNativePG | An operator that runs PostgreSQL on Kubernetes. Base backups (object store or volume snapshots), scheduled backups and recovery into a new cluster, including point-in-time recovery | yes, for the clusters it runs | not described in its documentation; recovery is a bootstrap method you apply, and its docs leave verifying the result to you |
| K8up | A restic-based backup operator for PVCs and applications, with `Backup`, `Schedule`, `Restore`, `Check`, `Prune` and `Archive` resources and a Prometheus endpoint | yes (restic snapshots) | not found. Its `Check` job tests the integrity of the restic repository, which is not a restore of your data |
| FireDrill (github.com/kirilurbonas/FireDrill) | A verification layer over backups made by other tools. Restores a real backup into a disposable sandbox, checks the data, measures restore time against RTO and RPO targets, and writes signed evidence. Postgres, MySQL and Velero drills; a CLI and a Kubernetes operator with scheduled drills. Apache-2.0 | no, "it does not back anything up" by design | yes, this is its purpose |

FireDrill is clearly further along on verification than BackupVouch: signed audit evidence, RTO and RPO targets, a ransomware canary check, compliance-control export and Slack notifications. BackupVouch does not try to match that. Its narrower idea is **one Kubernetes resource that takes the PostgreSQL backup and proves it**, with a small, readable codebase and no other backup tool needed. Pick FireDrill if you already have backups from another tool and need audit-grade evidence. Pick BackupVouch if you want one small operator that does both steps for PostgreSQL and nothing else.

## Open questions

- Whether any part of the work is better contributed to FireDrill than built here (the comparison above is the current answer: they solve different halves).
- Whether to support retention by age as well as count.
- Whether the dashboard (optional) is worth including in v0.1.
