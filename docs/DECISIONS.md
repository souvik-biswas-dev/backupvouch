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
- **Consequences:** nothing listens on the network (Unix socket only), cleanup is automatic when the Pod ends, and no extra resources can be left behind. Cost: the runner image contains Postgres server binaries and is larger.
- **Details (step 1 review):** `initdb` needs a UID that has a passwd entry, so the image fixes a non-root UID (70 on the Alpine Postgres image) and the Pod sets `runAsUser` and `fsGroup` to it. A read-only root filesystem is kept, with writable `emptyDir` mounts for the data directory, the socket directory and `/tmp`. The data `emptyDir` has a size limit (`verify.scratchSize`).

## D4: Results are returned through the container termination message

- **Context:** the controller needs a structured result from a Job without extra infrastructure.
- **Decision:** the runner writes a small JSON document to `/dev/termination-log`, and the controller reads it from the terminated container status.
- **Consequences:** no extra storage or API needed, and the message size limit (4 KB) keeps results compact. If a result does not fit or is invalid, the run is marked failed with reason `InvalidRunnerResult`.
- **Amended (step 1 review):** failures use the same channel (`{"error":{...}}`), but timeouts are read from the Job condition because a killed runner writes nothing. Messages are limited to 120 characters per check, at most 20 checks exist, and the runner trims to 3500 bytes. The Jobs use `terminationMessagePolicy: File` and no `ttlSecondsAfterFinished`. See `ARCHITECTURE.md`.

## D5: Deterministic Job names and owner references

- **Context:** reconcile can run repeatedly and the controller can crash at any point.
- **Decision:** Job names derive from the policy name and the scheduled run time, and Jobs are owned by the BackupPolicy.
- **Consequences:** creating a Job twice is a no-op, Kubernetes garbage-collects Jobs when the policy is deleted, and recovery needs no extra state.

## D6: The operator never reads Secrets

- **Context:** the controller does not need credential values, only the runners do.
- **Decision:** Jobs reference Secrets through `secretKeyRef` environment variables. The operator has no RBAC for Secrets.
- **Consequences:** a compromised controller cannot read credentials, but a missing Secret is only discovered when the Job starts. A Pod with a missing Secret does not fail, it waits in `CreateContainerConfigError`, so the controller reads that Pod state (it has `pods` read access) and fails the run early with reason `SecretMissing` after 60 seconds instead of waiting for the deadline.

## D7: Retention runs after verification and protects the newest verified backup

- **Context:** deleting old backups before a new one is proven good could leave the team with nothing usable.
- **Decision:** pruning happens only after a successful verification (or after a successful backup when verification is disabled, otherwise storage would grow forever), and the newest verified backup, named in `status.lastVerification.objectKey`, is never deleted. Pruning is done by a third Job (`runner prune`), because the controller never touches storage (see D12).
- **Consequences:** storage may briefly hold more than `keepLast` objects when verification keeps failing, which is intended.

## D8: No admission webhook in v1

- **Context:** a webhook gives early validation but adds certificates and operational weight.
- **Decision:** validation is done with CRD schema markers (including CEL rules and list-map keys) only.
- **Consequences:** unique check names come from `listType=map` with `listMapKey=name`, and the name length limit from a CEL rule, so the API server enforces both. What the schema cannot check (cron semantics, time zone, runner image allow-list) is validated in the controller and reported in the `SpecValid` condition. A webhook can be added later.

## D9: Project name BackupVouch

- **Context:** the working name `restoreguard` appears to be used by an existing backup auditing tool (verify), and a similar name causes confusion in searches.
- **Decision:** the project is called BackupVouch (`backupvouch` in code, repository, images, and metrics). To vouch is to give a trusted guarantee, which is what the operator does for each backup.
- **Consequences:** the API group is `backup.backupvouch.pages.dev` and metrics start with `backupvouch_`. Availability of the name on GitHub, pkg.go.dev, and Artifact Hub must be checked before the first release.

## D10: One run at a time, one catch-up, no automatic retry of a failed run

- **Context:** schedules can overlap a slow run, and a controller outage can span several schedules.
- **Decision:** at most one run per policy is active, and a schedule that arrives meanwhile is skipped. After downtime only the most recent missed schedule runs. A failed run is not retried before the next schedule. `runID` always comes from the scheduled time.
- **Consequences:** no pile-ups and no duplicate Jobs after a crash. A transient failure waits for the next schedule, unless the Job's own `backoffLimit` (1) retries it first.

## D11: Failures travel through the termination message too

- **Context:** the controller must tell `SourceUnreachable` from `StorageUnreachable` without reading Pod logs, and a Job that merely "failed" carries no reason.
- **Decision:** the runner writes `{"error":{"reason":...,"message":...}}` on failure and a `reason` in the verify result. The controller trusts only reasons from a fixed list, and maps anything else to `BackupJobFailed` or `InvalidRunnerResult` for backups, and to `RestoreFailed` for an unknown verify reason (`isValidVerifyFailureReason`).
- **Consequences:** reasons are precise, but the runner must classify errors carefully (step 3 and 4).

## D12: Retention is a third Job, `runner prune`

- **Context:** the first draft said the controller never touches storage, yet retention has to delete objects.
- **Decision:** a Prune Job (`<policy>-prune-<runID>`) lists objects under the prefix, keeps the newest `keepLast`, and skips the protected object.
- **Consequences:** the controller keeps its "never touches storage" property. The storage credentials used by Jobs must be allowed to delete, so users should enable bucket versioning or object lock if they fear a compromised credential (see `SECURITY.md`).

## D13: Manual re-verification by annotation

- **Context:** the sabotage demo corrupts a stored dump. If the demo waited for the next scheduled run, a fresh good backup would be taken and nothing would fail. Re-checking old backups for bit rot is also useful.
- **Decision:** the annotation `backupvouch.pages.dev/reverify=<unix seconds>` makes an idle policy verify `status.lastBackup` again. The handled value is stored in `status.lastReverifyID`.
- **Consequences:** a small API surface that is not part of the spec, and the demo can run in seconds. The alternative (a `spec` field such as `verify.recheckInterval`) is left for later.

## D14: Runner images are allow-listed

- **Context:** `runnerImage` plus `secretKeyRef` lets whoever creates a BackupPolicy run any image with any Secret of the namespace mounted, which is more power than creating a BackupPolicy suggests.
- **Decision:** the operator takes a list of allowed runner image patterns (flag or environment variable). The default allows only the operator's own pinned default image. Other images give `SpecValid=False`, reason `RunnerImageNotAllowed`.
- **Consequences:** safer by default; users who want a custom image must configure the operator.

## D15: One Postgres major version per runner image

- **Context:** `pg_dump` refuses to dump a server newer than itself, and a restore into an older server can fail.
- **Decision:** the runner image bundles Postgres 16 tools and server. `pg_restore` runs with `--no-owner --no-privileges` so missing roles do not fail a restore. Sources newer than 16 are unsupported in v0.1; other versions need another runner image (built from the same Dockerfile with a build argument, to be decided in step 3).
- **Consequences:** documented in the README limitations and the runbook. Extensions that are not in the image (for example PostGIS) make a restore fail with `RestoreFailed`.

## D16: API type details that follow from Go and controller-gen

- **Context:** writing `backuppolicy_types.go` (step 2) showed three places where a literal reading of `API.md` would not work.
- **Decision:** (1) `verify.enabled` is a `*bool`, because with `omitempty` a plain `bool` set to false is dropped and the server default `true` would silently win. `scratchSize` is a `*resource.Quantity` for the same reason (a zero Quantity serialises as `"0"`). (2) `status.lastVerification.restoreDurationSeconds` is an `int64` of whole seconds, because controller-gen refuses floating-point fields unless `allowDangerousTypes` is set; the runner still reports a fractional value in its termination message and the controller rounds it. (3) `runID`, `currentRunID`, `lastRunID` and `lastReverifyID` are `int64` Unix timestamps. Optional structs (`retention`, `verify`) use `omitzero` and `default={}` so their inner defaults apply when they are omitted.
- **Consequences:** `BackupPolicySpec.SetDefaults()` mirrors the schema defaults in Go, so the controller does not depend on server-side defaulting. Restore time resolution is one second.


## D17: End-to-end test is a shell script, the scaffold's Ginkgo e2e suite is removed

- **Context:** kubebuilder ships `test/e2e` (Ginkgo, cert-manager, a metrics curl Pod) that tests the empty scaffold. The sabotage story needs an S3 store, a seeded Postgres, a corrupted object and a re-verification, which is a sequence of `kubectl` steps more than a set of independent specs.
- **Decision:** `hack/e2e.sh` (with fixtures in `config/samples/demo/`) is the end-to-end test, run by `make e2e` and by `test-e2e.yml`. `test/e2e`, `test/utils` and the Makefile targets `setup-test-e2e`, `test-e2e`, `cleanup-test-e2e` were deleted. cert-manager is not installed because v0.1 has no webhook (D8).
- **Consequences:** the demo script (step 7) can reuse the same fixtures and steps. The e2e checks are plain bash, so they are less structured than Ginkgo specs. If a Go e2e suite is wanted later, add it next to the script.

## D18: Events use the `events.k8s.io` API, and failed Jobs are deleted with background propagation

- **Context:** the reconciler records events with `mgr.GetEventRecorder`, which writes through the `events.k8s.io` API, while the RBAC marker (step 5) granted the core `events` resource. Envtest runs as an administrator, so the mismatch would only show on a real cluster, as events that never appear. Separately, `batch/v1` Jobs default to orphaning their Pods when deleted, so failing a run early (`SecretMissing`) left the stuck Pod behind.
- **Decision:** the marker is `groups=events.k8s.io,resources=events,verbs=create;patch;update`. The unused `create` and `delete` verbs on `backuppolicies` were removed to match SECURITY.md. `failBackup` and `failVerify` delete the Job with `DeletePropagationBackground`, as the finalizer and the cleanup already did.
- **Consequences:** `hack/e2e.sh` asserts the `VerificationPassed` and `VerificationFailed` events, which is the check that settles the events RBAC question (VERIFY). `config/rbac/role.yaml` was edited by hand to match what `make manifests` should produce; run `make manifests` and commit any difference.

## D19: One set of steps and one set of fixtures for the e2e test and the demo

- **Context:** the demo tells the same story as the e2e test (backup, verify, sabotage, verify fails). Two copies would drift apart, and a demo that no test covers breaks silently.
- **Decision:** the steps are functions in `hack/lib.sh`. `hack/e2e.sh` calls them silently and exits non-zero on a wrong result; `hack/demo.sh` sets `NARRATE=true` and calls the same functions, which then add commentary and print the status and events. The fixtures (S3 store, seeded Postgres, the BackupPolicy) moved from `hack/e2e/` to `config/samples/demo/`, with a `kustomization.yaml` for applying them at once. The demo keeps its cluster, and deletes an older demo cluster first, because a kept cluster still holds the previous run's policy status.
- **Also decided:** the default operator image is `ghcr.io/souvik-biswas-dev/backupvouch:$(VERSION)`, not `controller:latest`; `make build-installer` refuses a missing or `:latest` tag and checks the result; `dist/` is git-ignored (the installer is attached to a release, not committed). Images are published only from a tag equal to the `DefaultRunnerImage` tag, amd64 only for now.
- **Consequences:** a change to the story is made once. The refactor of `hack/e2e.sh` moved code without changing it, but both scripts are still unrun on a real cluster, so the first run may need fixes.

## D20: SeaweedFS replaces MinIO in the e2e test and the demo; curl replaces the `mc` client

- **Context:** the first CI run of step 6 failed at "deploying MinIO": `ImagePullBackOff`, `pull access denied` for `minio/minio:RELEASE.2025-04-22T22-12-26Z`. MinIO removed its Docker Hub repositories in September 2026, and `quay.io/minio` stopped allowing anonymous pulls shortly after. The same applied to `minio/mc`, which the sabotage step used. The MinIO community edition is no longer maintained, so pinning another MinIO tag would only move the problem.
- **Decision:** the fixture S3 store is SeaweedFS (`chrislusf/seaweedfs`, `weed mini`, pinned to 4.42) in `config/samples/demo/s3.yaml`, with Service `s3` on port 8333 and Secret `s3-creds`. The runner is unchanged: it uses the `minio-go` client library, which speaks plain S3 and is independent of the MinIO server. The `mc` pod is replaced by `s3_call` in `hack/lib.sh`: a one-off `curlimages/curl` Pod that sends a SigV4-signed request (`--aws-sigv4`), read back with `kubectl wait`-style polling and `kubectl logs` instead of an attached, piped stdin. The same helper first proves that signed requests and the bucket work, so a broken store fails early with a clear message.
- **Consequences:** no dependency on MinIO images anywhere. The server is a test fixture only; users keep any S3-compatible store. The tags `chrislusf/seaweedfs:4.42` and `curlimages/curl:8.12.1` are VERIFY items until the first green CI run, and can be bumped in one place each (`s3.yaml`, `CURL_IMG` in `hack/lib.sh`).


## D21: Read-only dashboard, off by default, loopback only

- **Context:** step 8 adds a status page. It exposes policy names, schedules, object keys and failure messages, and the operator has no authentication layer to put in front of it.
- **Decision:** the dashboard is a separate listener enabled by `--dashboard-bind-address` (default `0`, disabled). `make dashboard-enable` sets `127.0.0.1:9090` so it is reachable only through `kubectl port-forward` (Kubernetes authenticates that). It accepts GET and HEAD only, reads BackupPolicies from the manager's cache (no new RBAC), runs on every replica (no leader election), omits connection details from the JSON, and sets a CSP that allows only same-origin files.
- **Consequences:** binding it to a pod IP or exposing it through a Service or Ingress makes status readable by anyone who can reach it; do that only behind an authenticating proxy. See SECURITY.md.

## D22: The dashboard has no JavaScript dependencies and no CDN

- **Context:** the plan allowed up to two small libraries and required everything to be vendored and embedded.
- **Decision:** none. The time scale, ticks and shapes are small plain ES modules; fonts are self-hosted woff2. Files are embedded with `go:embed`, and `.dockerignore` re-includes them.
- **Consequences:** nothing to update or audit, and it works offline. Pure logic (scale, formatting, headline text) has node tests (`make dashboard-js-test`); drawing has none and needs a manual look.

## D23: The API domain is backupvouch.pages.dev

- **Context:** a Kubernetes API group should be a DNS name the project controls. The first choice, `backupvouch.dev`, would have cost money for a project with no users yet.
- **Decision:** the API group is `backup.backupvouch.pages.dev`, a free Cloudflare Pages name for the project site (`site/`). The same domain prefixes the annotation `backupvouch.pages.dev/reverify`, the labels `backupvouch.pages.dev/policy`, `backupvouch.pages.dev/run-id` and `backupvouch.pages.dev/role`, and the finalizer.
- **Consequences:** the rename breaks any BackupPolicy created under the old group (there were no users). A later move to a registered domain is another breaking change, so do it before a first stable release, never after.
