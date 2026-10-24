# DECISIONS

Short records of design choices. Add a new record when something changes.

## D1: Run backups and verification as Kubernetes Jobs

- **Context:** the controller could run `pg_dump` itself or delegate the work.
- **Decision:** the controller only creates and watches Jobs. The work happens in the runner image.
- **Consequences:** the controller stays small and restart-safe, workloads get their own resource limits and security context, and results are read from Job Pods. Cost: slightly more moving parts than an in-process exec.

## D2: PostgreSQL first

- **Context:** supporting several engines early spreads effort thin.
- **Decision:** v1 supports PostgreSQL only, with `pg_dump` custom format and `pg_restore`.
- **Consequences:** deterministic verification with plain SQL checks. The `source.engine` field exists so other engines can be added without breaking the API.

## D3: Scratch Postgres inside the verification Job's container

- **Context:** restoring needs a Postgres server. Options: a sidecar plus a Service, a separate Pod, or a process in the same container.
- **Decision:** the verify subcommand starts a throwaway Postgres with `initdb` and `pg_ctl` as a non-root user, on an `emptyDir`.
- **Consequences:** nothing listens on the network, cleanup is automatic when the Pod ends, and no extra resources can be left behind. Cost: the runner image contains Postgres server binaries and is larger.

## D4: Results are returned through the container termination message

- **Context:** the controller needs a structured result from a Job without extra infrastructure.
- **Decision:** the runner writes a small JSON document to `/dev/termination-log`, and the controller reads it from the terminated container status.
- **Consequences:** no extra storage or API needed, and the message size limit (4 KB) keeps results compact. If a result does not fit or is invalid, the run is marked failed with reason `InvalidRunnerResult`.

## D5: Deterministic Job names and owner references

- **Context:** reconcile can run repeatedly and the controller can crash at any point.
- **Decision:** Job names derive from the policy name and the scheduled run time, and Jobs are owned by the BackupPolicy.
- **Consequences:** creating a Job twice is a no-op, Kubernetes garbage-collects Jobs when the policy is deleted, and recovery needs no extra state.

## D6: The operator never reads Secrets

- **Context:** the controller does not need credential values, only the runners do.
- **Decision:** Jobs reference Secrets through `secretKeyRef` environment variables. The operator has no RBAC for Secrets.
- **Consequences:** a compromised controller cannot read credentials, but a missing Secret is only discovered when the Job starts, and shows up as a failed run.

## D7: Retention runs after verification and protects the newest verified backup

- **Context:** deleting old backups before a new one is proven good could leave the team with nothing usable.
- **Decision:** pruning happens only after a successful verification, and the newest verified backup is never deleted.
- **Consequences:** storage may briefly hold more than `keepLast` objects when verification keeps failing, which is intended.

## D8: No admission webhook in v1

- **Context:** a webhook gives early validation but adds certificates and operational weight.
- **Decision:** validation is done with CRD schema markers only.
- **Consequences:** cross-field rules (such as unique check names) are validated in the controller and reported in a condition. A webhook can be added later.

## D9: Project name BackupVouch

- **Context:** the working name `restoreguard` is already used by an existing backup auditing tool, and a similar name causes confusion in searches.
- **Decision:** the project is called BackupVouch (`backupvouch` in code, repository, images, and metrics). To vouch is to give a trusted guarantee, which is what the operator does for each backup.
- **Consequences:** the API group is `backup.backupvouch.dev` and metrics start with `backupvouch_`. Availability of the name on GitHub, pkg.go.dev, and Artifact Hub must be checked before the first release.
