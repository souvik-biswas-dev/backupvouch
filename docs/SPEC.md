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
5. Be free to run and demo locally (kind and MinIO), with no cloud account.

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

All entries below are from memory and must be checked against current documentation before the README makes any claim.

| Tool | Focus | Automatic restore verification |
|---|---|---|
| Velero | cluster resources and volume backup and restore | not built in (verify) |
| CloudNativePG | running PostgreSQL on Kubernetes, with backup and recovery | restore testing left to the user (verify) |
| K8up | restic-based backups of volumes and databases | not built in (verify) |
| FireDrill (github.com/kirilurbonas/FireDrill) | verification layer over backups made by other tools (Postgres, MySQL, Velero), with an operator that runs drills on a schedule and measures restore time | yes, this is its purpose (verify) |

BackupVouch's difference is that **verification is the product**: a backup is only counted once it has been restored and checked. FireDrill already works in this space, so BackupVouch does not claim to be the first. Its intended difference is narrower: one Kubernetes resource that **takes the backup and proves it**, with a small, readable codebase and no dependency on another backup tool. Compare features with FireDrill's current README before publishing any claim.

## Open questions

- Which license (Apache-2.0 suggested).
- How BackupVouch is positioned next to FireDrill (see the comparison above), and whether any part of the work is better contributed upstream.
- Whether to support retention by age as well as count.
- Whether the dashboard (optional) is worth including in v0.1.
