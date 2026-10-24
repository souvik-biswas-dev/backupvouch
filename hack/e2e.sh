#!/usr/bin/env bash
# End-to-end test for BackupVouch in a throwaway kind cluster. See docs/TESTING.md.
#
#   1. build the operator and runner images and load them into kind
<<<<<<< HEAD
#   2. deploy the operator, an S3 store (SeaweedFS) and a seeded Postgres, apply a BackupPolicy
=======
#   2. deploy the operator, MinIO and a seeded Postgres, apply a BackupPolicy
>>>>>>> origin/main
#   3. HEALTHY CASE: expect RestoreVerified=True
#   4. SABOTAGE: corrupt the stored dump, ask for a re-verification, expect RestoreVerified=False
#      with reason ChecksumMismatch, a VerificationFailed event and backupvouch_verification_success 0
#
# Exit code 0 only if both expectations hold. Needs docker, kind, kubectl and make.
<<<<<<< HEAD
# The steps live in hack/lib.sh and are shared with hack/demo.sh; the fixtures in config/samples/demo/.
=======
>>>>>>> origin/main
#
# Environment:
#   E2E_CLUSTER        kind cluster name (default backupvouch-e2e)
#   E2E_KEEP_CLUSTER   "true" keeps a cluster this script created (default: delete it on exit)
#   CONTROLLER_IMG     operator image tag (default backupvouch-controller:e2e)
<<<<<<< HEAD
#   CURL_IMG           curl image used only for one-off signed S3 requests (create bucket, corrupt dump)
#   KIND               kind binary (default kind)

set -euo pipefail

CLUSTER="${E2E_CLUSTER:-backupvouch-e2e}"
KEEP_CLUSTER="${E2E_KEEP_CLUSTER:-false}"

# shellcheck source=hack/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

stage_preflight
stage_images_and_cluster
stage_operator
stage_fixtures
stage_healthy
stage_sabotage
stage_final
=======
#   MC_IMG             MinIO client image used only to corrupt the dump
#   KIND               kind binary (default kind)
#
# VERIFY: written without being run. The first local run may need small fixes, paste the output back.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

KIND="${KIND:-kind}"
CLUSTER="${E2E_CLUSTER:-backupvouch-e2e}"
KEEP_CLUSTER="${E2E_KEEP_CLUSTER:-false}"
CONTROLLER_IMG="${CONTROLLER_IMG:-backupvouch-controller:e2e}"
# VERIFY: tag exists on Docker Hub.
MC_IMG="${MC_IMG:-minio/mc:RELEASE.2025-04-16T18-13-26Z}"
# The runner image must carry the tag the operator uses by default, so kind never pulls it.
RUNNER_IMG="$(sed -n 's/.*DefaultRunnerImage = "\(.*\)"/\1/p' internal/jobs/backup.go)"

NS=default
POLICY=app-db
OP_NS=backupvouch-system
OP_DEPLOY=backupvouch-controller-manager
MINIO_ACCESS_KEY=e2eaccesskey
MINIO_SECRET_KEY=e2esecretkey123
BUCKET=backups

CREATED_CLUSTER=false
KUSTOMIZATION=config/manager/kustomization.yaml
KUSTOMIZATION_BACKUP="$(mktemp)"

# fd 3 keeps a copy of stderr, so FAIL messages stay visible even when poll silences a probe.
exec 3>&2

k() { kubectl --context "kind-${CLUSTER}" "$@"; }
log() { printf '\n==> %s\n' "$*"; }
fail() { printf '\nFAIL: %s\n' "$*" >&3; exit 1; }

diagnostics() {
  echo
  echo "---------------- diagnostics ----------------"
  k -n "${NS}" get backuppolicies -o yaml 2>&1 || true
  k -n "${NS}" get jobs,pods -o wide 2>&1 || true
  k -n "${NS}" get events --sort-by=.lastTimestamp 2>&1 | tail -n 40 || true
  k -n "${OP_NS}" logs "deploy/${OP_DEPLOY}" --tail=150 2>&1 || true
  k -n "${NS}" logs -l app.kubernetes.io/managed-by=backupvouch --all-containers --tail=80 --prefix 2>&1 || true
  echo "---------------------------------------------"
}

cleanup() {
  local status=$?
  set +e
  if [[ ${status} -ne 0 ]]; then
    diagnostics
  fi
  # `make deploy` rewrites the image in config/manager/kustomization.yaml, put it back.
  if [[ -s "${KUSTOMIZATION_BACKUP}" ]]; then
    cp "${KUSTOMIZATION_BACKUP}" "${KUSTOMIZATION}"
  fi
  rm -f "${KUSTOMIZATION_BACKUP}"
  k -n "${NS}" delete pod mc-corrupt --ignore-not-found --wait=false >/dev/null 2>&1
  if [[ "${CREATED_CLUSTER}" == "true" && "${KEEP_CLUSTER}" != "true" ]]; then
    log "deleting kind cluster ${CLUSTER}"
    "${KIND}" delete cluster --name "${CLUSTER}"
  elif [[ "${CREATED_CLUSTER}" == "true" ]]; then
    echo "kind cluster ${CLUSTER} kept (E2E_KEEP_CLUSTER=true), delete it with: ${KIND} delete cluster --name ${CLUSTER}"
  fi
  if [[ ${status} -eq 0 ]]; then
    echo
    echo "E2E PASSED: healthy backup verified, sabotaged backup rejected."
  else
    echo
    echo "E2E FAILED (exit ${status})"
  fi
  exit "${status}"
}
trap cleanup EXIT

# poll <description> <timeout seconds> <command...>: run the command every 3 seconds until it succeeds.
poll() {
  local desc=$1 timeout=$2
  shift 2
  local end=$((SECONDS + timeout))
  until "$@" >/dev/null 2>&1; do
    if ((SECONDS >= end)); then
      fail "timed out after ${timeout}s waiting for: ${desc}"
    fi
    sleep 3
  done
  echo "ok: ${desc}"
}

bp_field() { k -n "${NS}" get backuppolicy "${POLICY}" -o "jsonpath={$1}"; }
cond_field() { bp_field ".status.conditions[?(@.type==\"$1\")].$2"; }
cond_is() { [[ "$(cond_field "$1" "$2")" == "$3" ]]; }
field_is() { [[ "$(bp_field "$1")" == "$2" ]]; }
field_not_empty() { [[ -n "$(bp_field "$1")" ]]; }

manager_metrics() {
  local pod
  pod="$(k -n "${OP_NS}" get pods -l control-plane=controller-manager \
    -o jsonpath='{range .items[?(@.status.phase=="Running")]}{.metadata.name}{"\n"}{end}' | tail -n 1)"
  [[ -n "${pod}" ]] || return 1
  # The API server proxies the request to the pod, so no port-forward and no token are needed.
  k get --raw "/api/v1/namespaces/${OP_NS}/pods/http:${pod}:8443/proxy/metrics"
}
metric_is() { # metric_is <metric name> <value>
  manager_metrics | grep -E "^$1\{[^}]*policy=\"${POLICY}\"[^}]*\} $2\$"
}
event_exists() { # event_exists <reason>
  [[ -n "$(k -n "${NS}" get events --field-selector "involvedObject.name=${POLICY},reason=$1" -o name)" ]]
}

# ---------------------------------------------------------------- preflight
log "preflight"
for tool in docker kubectl make "${KIND}"; do
  command -v "${tool}" >/dev/null 2>&1 || fail "${tool} is required but was not found"
done
[[ -n "${RUNNER_IMG}" ]] || fail "could not read DefaultRunnerImage from internal/jobs/backup.go"
echo "controller image: ${CONTROLLER_IMG}"
echo "runner image:     ${RUNNER_IMG}"

# ---------------------------------------------------------------- images and cluster
log "building images"
make docker-build IMG="${CONTROLLER_IMG}"
make docker-build-runner RUNNER_IMG="${RUNNER_IMG}"

log "creating kind cluster ${CLUSTER}"
if "${KIND}" get clusters | grep -qx "${CLUSTER}"; then
  echo "cluster ${CLUSTER} already exists, reusing it (it will not be deleted)"
else
  "${KIND}" create cluster --name "${CLUSTER}" --wait 120s
  CREATED_CLUSTER=true
fi
kubectl config use-context "kind-${CLUSTER}" >/dev/null

log "loading images into kind"
"${KIND}" load docker-image --name "${CLUSTER}" "${CONTROLLER_IMG}" "${RUNNER_IMG}"

# ---------------------------------------------------------------- operator
log "deploying the operator"
cp "${KUSTOMIZATION}" "${KUSTOMIZATION_BACKUP}"
make deploy IMG="${CONTROLLER_IMG}"
# Serve metrics over plain HTTP so the test can read them without a token. The default install stays on HTTPS.
k -n "${OP_NS}" patch deployment "${OP_DEPLOY}" --type=json \
  -p '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--metrics-secure=false"}]'
k -n "${OP_NS}" rollout status "deployment/${OP_DEPLOY}" --timeout=180s
k wait --for=condition=Established crd/backuppolicies.backup.backupvouch.dev --timeout=60s

# ---------------------------------------------------------------- fixtures
log "deploying MinIO and a seeded Postgres"
k apply -f hack/e2e/minio.yaml -f hack/e2e/postgres.yaml
k -n "${NS}" rollout status deployment/minio --timeout=180s
k -n "${NS}" rollout status deployment/postgres --timeout=180s

# ---------------------------------------------------------------- healthy case
log "HEALTHY CASE: apply the BackupPolicy and wait for a verified restore"
k apply -f hack/e2e/policy.yaml

verified_or_failed() {
  if cond_is BackupReady status False; then
    fail "backup failed: reason=$(cond_field BackupReady reason) message=$(cond_field BackupReady message)"
  fi
  if cond_is SpecValid status False; then
    fail "spec rejected: reason=$(cond_field SpecValid reason) message=$(cond_field SpecValid message)"
  fi
  if cond_is RestoreVerified status False; then
    fail "verification failed in the healthy case: reason=$(cond_field RestoreVerified reason) message=$(cond_field RestoreVerified message)"
  fi
  cond_is RestoreVerified status True
}
poll "RestoreVerified=True" 600 verified_or_failed

field_is ".status.lastVerification.result" Passed || fail "lastVerification.result is not Passed"
field_not_empty ".status.lastBackup.objectKey" || fail "lastBackup.objectKey is empty"
field_not_empty ".status.lastBackup.sha256" || fail "lastBackup.sha256 is empty"
cond_is BackupReady status True || fail "BackupReady is not True"
poll "run finished (phase Idle after the prune Job)" 300 field_is ".status.phase" Idle
poll "metric backupvouch_verification_success 1" 60 metric_is backupvouch_verification_success 1
poll "event VerificationPassed" 60 event_exists VerificationPassed
log "healthy case OK"

# ---------------------------------------------------------------- sabotage
log "SABOTAGE: corrupt the stored dump, then ask for a re-verification"
OBJECT_KEY="$(bp_field ".status.lastBackup.objectKey")"
echo "overwriting s3://${BUCKET}/${OBJECT_KEY} with garbage"
# VERIFY: kubectl run with a piped stdin (-i) and the mc entrypoint. Fallback: run `mc pipe` by hand, see docs/TESTING.md.
printf 'this is not a postgres dump\n' | k -n "${NS}" run mc-corrupt --rm -i --quiet --restart=Never \
  --image="${MC_IMG}" \
  --env="MC_HOST_local=http://${MINIO_ACCESS_KEY}:${MINIO_SECRET_KEY}@minio.${NS}.svc:9000" \
  -- pipe "local/${BUCKET}/${OBJECT_KEY}"

# A scheduled run would take a fresh good backup, so ask for a re-check of the stored one (D13).
REVERIFY_ID="$(date +%s)"
k -n "${NS}" annotate backuppolicy "${POLICY}" "backupvouch.dev/reverify=${REVERIFY_ID}" --overwrite
poll "controller acted on the annotation" 120 field_is ".status.lastReverifyID" "${REVERIFY_ID}"

verification_failed() {
  cond_is RestoreVerified status False
}
poll "RestoreVerified=False" 600 verification_failed

REASON="$(cond_field RestoreVerified reason)"
[[ "${REASON}" == "ChecksumMismatch" ]] || fail "expected reason ChecksumMismatch, got ${REASON}"
field_is ".status.lastVerification.result" Failed || fail "lastVerification.result is not Failed"
field_is ".status.lastVerification.objectKey" "${OBJECT_KEY}" || fail "the failed verification names a different object"
cond_is BackupReady status True || fail "BackupReady should still be True, only the stored copy was corrupted"
poll "run finished (phase Idle)" 60 field_is ".status.phase" Idle
poll "event VerificationFailed" 60 event_exists VerificationFailed
poll "metric backupvouch_verification_success 0" 60 metric_is backupvouch_verification_success 0
log "sabotage OK: the corrupted backup was rejected with reason ${REASON}"

log "final status"
k -n "${NS}" get backuppolicy "${POLICY}"
>>>>>>> origin/main
