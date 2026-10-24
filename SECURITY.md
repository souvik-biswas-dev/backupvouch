# Security

## Reporting a vulnerability

Please open a private security advisory on the GitHub repository (Security tab) instead of a public issue. Include the version, a description, and steps to reproduce.

## Threat model

BackupVouch handles database credentials, storage credentials, and copies of production data. The main risks and the planned mitigations:

| Risk | Mitigation |
|---|---|
| Credentials leak through logs, events, or status | Secrets are only referenced by name and injected into Jobs as environment variables from `secretKeyRef`. The operator never reads Secret values. The runner never prints them. |
| A verification run exposes production data | The scratch Postgres runs inside the verification Job's own container, with data on an `emptyDir`. Nothing is exposed on the network. |
| Verification Job reaches other services | Recommended NetworkPolicy: allow egress only to the storage endpoint and DNS. |
| Over-privileged operator | RBAC limited to what the controller needs (table below). It does not read Secrets. |
| Compromised container escalates | Jobs run as non-root, with a read-only root filesystem where possible, no privilege escalation, dropped capabilities, and resource limits. |
| Malicious SQL in checks | Checks are written by whoever can create a BackupPolicy. They run only against the throwaway restored copy, never the source database. Treat permission to create BackupPolicies as sensitive. |
| Tampered or truncated backup | The runner records a SHA-256 on upload and verification refuses a mismatch. |
| Source database is damaged by backup load | The dump uses a read-only role. Backup Jobs have timeouts and resource limits. |

## Operator RBAC (planned)

| Resource | Verbs | Why |
|---|---|---|
| backuppolicies | get, list, watch, update, patch | main reconcile object |
| backuppolicies/status | get, update, patch | report results |
| jobs (batch) | get, list, watch, create, delete | run backup and verification Jobs, clean up |
| pods | get, list, watch | read the termination message of Job pods |
| events | create, patch | emit events |
| leases (coordination.k8s.io) | get, list, watch, create, update, patch, delete | leader election |

No access to Secrets, nodes, or anything cluster-wide beyond this.

## Recommended hardening for users

- Give BackupVouch a dedicated read-only database role for dumps.
- Use storage credentials limited to one bucket or prefix.
- Apply the egress NetworkPolicy from `docs/ARCHITECTURE.md`.
- Pin the runner image by tag or digest.
