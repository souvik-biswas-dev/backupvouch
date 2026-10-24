#!/usr/bin/env bash
# Shared helpers and stages for hack/e2e.sh and hack/demo.sh. Source it, do not run it.
# shellcheck shell=bash
#
# The two scripts run the same story (see docs/TESTING.md):
#   stage_preflight, stage_images_and_cluster, stage_operator, stage_fixtures, stage_healthy,
#   stage_sabotage, stage_final
# hack/e2e.sh runs them silently and exits non-zero on a wrong result. hack/demo.sh sets
# NARRATE=true, which adds commentary and prints the BackupPolicy status and events after each
# stage (show_state). The code is the same, so the demo cannot drift away from the test.
#
# Variables a caller may set before sourcing (all optional):
#   KIND, CLUSTER, KEEP_CLUSTER, CONTROLLER_IMG, CURL_IMG, NARRATE, PASS_MESSAGE, FAIL_MESSAGE
# The fixtures (an S3 store, Postgres with seed data, the BackupPolicy) live in config/samples/demo/.

cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 1

: "${KIND:=kind}"
: "${CLUSTER:=backupvouch-e2e}"
: "${KEEP_CLUSTER:=false}"
: "${CONTROLLER_IMG:=backupvouch-controller:e2e}"
# Used only for one-off signed S3 requests (create the bucket, corrupt the dump). curl 7.75+ can sign with --aws-sigv4.
# VERIFY: tag exists on Docker Hub, and the image has curl as its entrypoint's command.
: "${CURL_IMG:=curlimages/curl:8.12.1}"
: "${NARRATE:=false}"
: "${PASS_MESSAGE:=E2E PASSED: healthy backup verified, sabotaged backup rejected.}"
: "${FAIL_MESSAGE:=E2E FAILED}"
# The runner image must carry the tag the operator uses by default, so kind never pulls it.
RUNNER_IMG="$(sed -n 's/.*DefaultRunnerImage = "\(.*\)"/\1/p' internal/jobs/backup.go)"

FIXTURES=config/samples/demo
NS=default
POLICY=app-db
OP_NS=backupvouch-system
OP_DEPLOY=backupvouch-controller-manager
S3_HOST="s3.${NS}.svc:8333"
S3_ACCESS_KEY=e2eaccesskey
S3_SECRET_KEY=e2esecretkey123
BUCKET=backups

CREATED_CLUSTER=false
KUSTOMIZATION=config/manager/kustomization.yaml
KUSTOMIZATION_BACKUP="$(mktemp)"

if [[ -t 1 ]]; then BOLD=$'\033[1m'; RESET=$'\033[0m'; else BOLD=""; RESET=""; fi

# fd 3 keeps a copy of stderr, so FAIL messages stay visible even when poll silences a probe.
exec 3>&2

k() { kubectl --context "kind-${CLUSTER}" "$@"; }
log() { printf '\n==> %s\n' "$*"; }
fail() { printf '\nFAIL: %s\n' "$*" >&3; exit 1; }

# Narration helpers, all no-ops unless NARRATE=true (the demo).
narrating() { [[ "${NARRATE}" == "true" ]]; }
say() { narrating || return 0; printf '\n%s# %s%s\n' "${BOLD}" "$*" "${RESET}"; }
pause() {
  narrating && [[ "${DEMO_PAUSE:-false}" == "true" ]] || return 0
  read -r -p "Press Enter to continue... " _
}
show_cmd() { narrating || return 0; printf '\n%s$ %s%s\n' "${BOLD}" "$*" "${RESET}"; }
# show_state <title>: the status and the events of the policy, as the demo's evidence.
show_state() {
  narrating || return 0
  say "$1"
  show_cmd "kubectl get backuppolicy ${POLICY}"
  k -n "${NS}" get backuppolicy "${POLICY}" || true
  show_cmd "kubectl get backuppolicy ${POLICY} -o yaml   # status only"
  k -n "${NS}" get backuppolicy "${POLICY}" -o yaml | sed -n '/^status:/,$p' || true
  show_cmd "kubectl get events --field-selector involvedObject.name=${POLICY}"
  k -n "${NS}" get events --field-selector "involvedObject.name=${POLICY}" --sort-by=.lastTimestamp || true
  pause
}
show_file() { # show_file <path>: print a manifest, demo only
  narrating || return 0
  show_cmd "cat $1"
  cat "$1"
}

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
  k -n "${NS}" delete pod -l app=s3-curl --ignore-not-found --wait=false >/dev/null 2>&1
  if [[ "${CREATED_CLUSTER}" == "true" && "${KEEP_CLUSTER}" != "true" ]]; then
    log "deleting kind cluster ${CLUSTER}"
    "${KIND}" delete cluster --name "${CLUSTER}"
  elif [[ "${CREATED_CLUSTER}" == "true" ]]; then
    echo "kind cluster ${CLUSTER} kept, delete it with: ${KIND} delete cluster --name ${CLUSTER}"
    echo "inspect it with: kubectl --context kind-${CLUSTER} -n ${NS} get backuppolicy ${POLICY} -o yaml"
  fi
  if [[ ${status} -eq 0 ]]; then
    echo
    echo "${PASS_MESSAGE}"
  else
    echo
    echo "${FAIL_MESSAGE} (exit ${status})"
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

# s3_call <method> <path> [curl args...]: one signed request to the in-cluster S3 store, from a short-lived Pod.
# Prints the HTTP status code (or nothing if the Pod never finished). The Pod is created, polled and read
# with plain kubectl commands, with no attach or piped stdin, so it behaves the same on a laptop and in CI.
s3_call() {
  local method=$1 path=$2 pod phase="" i
  shift 2
  pod="s3-curl-${RANDOM}${RANDOM}"
  k -n "${NS}" run "${pod}" --restart=Never --labels=app=s3-curl --image="${CURL_IMG}" --command -- \
    curl -s -o /dev/null -w '%{http_code}' -X "${method}" \
    --aws-sigv4 "aws:amz:us-east-1:s3" --user "${S3_ACCESS_KEY}:${S3_SECRET_KEY}" \
    -H "x-amz-content-sha256: UNSIGNED-PAYLOAD" "$@" "http://${S3_HOST}/${path}" >/dev/null 2>&1 || return 1
  for ((i = 0; i < 90; i++)); do
    phase="$(k -n "${NS}" get pod "${pod}" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    [[ "${phase}" == "Succeeded" || "${phase}" == "Failed" ]] && break
    sleep 2
  done
  k -n "${NS}" logs "${pod}" 2>/dev/null || true
  k -n "${NS}" delete pod "${pod}" --ignore-not-found --wait=false >/dev/null 2>&1 || true
}

# The bucket is normally created by the store at start (-bucket). This call proves that signed requests work
# with our credentials and makes sure the bucket exists. 200 = created, 409 = it was already there.
ensure_bucket() {
  local code
  code="$(s3_call PUT "${BUCKET}" -T /dev/null)"
  [[ "${code}" == "200" || "${code}" == "409" ]]
}

# ---------------------------------------------------------------- stages

stage_preflight() {
  log "preflight"
  local tool
  for tool in docker kubectl make "${KIND}"; do
    command -v "${tool}" >/dev/null 2>&1 || fail "${tool} is required but was not found"
  done
  [[ -n "${RUNNER_IMG}" ]] || fail "could not read DefaultRunnerImage from internal/jobs/backup.go"
  echo "controller image: ${CONTROLLER_IMG}"
  echo "runner image:     ${RUNNER_IMG}"
}

stage_images_and_cluster() {
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
}

stage_operator() {
  log "deploying the operator"
  cp "${KUSTOMIZATION}" "${KUSTOMIZATION_BACKUP}"
  make deploy IMG="${CONTROLLER_IMG}"
  # Serve metrics over plain HTTP so the script can read them without a token. The default install stays on HTTPS.
  k -n "${OP_NS}" patch deployment "${OP_DEPLOY}" --type=json \
    -p '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--metrics-secure=false"}]'
  k -n "${OP_NS}" rollout status "deployment/${OP_DEPLOY}" --timeout=180s
  k wait --for=condition=Established crd/backuppolicies.backup.backupvouch.dev --timeout=60s
}

stage_fixtures() {
  log "deploying an S3 store (SeaweedFS) and a seeded Postgres"
  k apply -f "${FIXTURES}/s3.yaml" -f "${FIXTURES}/postgres.yaml"
  k -n "${NS}" rollout status deployment/s3 --timeout=300s
  k -n "${NS}" rollout status deployment/postgres --timeout=180s
  poll "S3 store answers signed requests and bucket ${BUCKET} exists" 240 ensure_bucket
}

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

stage_healthy() {
  log "HEALTHY CASE: apply the BackupPolicy and wait for a verified restore"
  say "The source database has a users table with 3 rows. This policy takes a backup, restores it into a throwaway Postgres and runs two SQL checks against the restored copy."
  show_file "${FIXTURES}/policy.yaml"
  k apply -f "${FIXTURES}/policy.yaml"

  poll "RestoreVerified=True" 600 verified_or_failed

  field_is ".status.lastVerification.result" Passed || fail "lastVerification.result is not Passed"
  field_not_empty ".status.lastBackup.objectKey" || fail "lastBackup.objectKey is empty"
  field_not_empty ".status.lastBackup.sha256" || fail "lastBackup.sha256 is empty"
  cond_is BackupReady status True || fail "BackupReady is not True"
  poll "run finished (phase Idle after the prune Job)" 300 field_is ".status.phase" Idle
  poll "metric backupvouch_verification_success 1" 60 metric_is backupvouch_verification_success 1
  poll "event VerificationPassed" 60 event_exists VerificationPassed
  log "healthy case OK"
  show_state "Result 1: the backup was taken, restored, checked and verified (RestoreVerified=True)."
}

stage_sabotage() {
  log "SABOTAGE: corrupt the stored dump, then ask for a re-verification"
  say "Now we simulate bit rot: the stored backup is overwritten with garbage. The backup job succeeded earlier, so a tool that only checks the backup job would still say everything is fine."
  OBJECT_KEY="$(bp_field ".status.lastBackup.objectKey")"
  echo "overwriting s3://${BUCKET}/${OBJECT_KEY} with garbage"
  # A plain S3 PUT of the same key replaces the object. Fallback by hand: see docs/TESTING.md.
  CORRUPT_CODE="$(s3_call PUT "${BUCKET}/${OBJECT_KEY}" -H "Content-Type: application/octet-stream" \
    --data-binary "this is not a postgres dump")"
  [[ "${CORRUPT_CODE}" == "200" ]] || fail "overwriting the stored dump returned HTTP '${CORRUPT_CODE}', expected 200"

  say "We ask BackupVouch to check the stored backup again (annotation backupvouch.dev/reverify). A scheduled run would take a fresh good backup, so it would not show the problem."
  # A scheduled run would take a fresh good backup, so ask for a re-check of the stored one (D13).
  REVERIFY_ID="$(date +%s)"
  show_cmd "kubectl annotate backuppolicy ${POLICY} backupvouch.dev/reverify=${REVERIFY_ID}"
  k -n "${NS}" annotate backuppolicy "${POLICY}" "backupvouch.dev/reverify=${REVERIFY_ID}" --overwrite
  poll "controller acted on the annotation" 120 field_is ".status.lastReverifyID" "${REVERIFY_ID}"

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
  show_state "Result 2: the corrupted backup was caught (RestoreVerified=False, reason ${REASON}), a VerificationFailed event was emitted."
}

verification_failed() {
  cond_is RestoreVerified status False
}

stage_final() {
  log "final status"
  k -n "${NS}" get backuppolicy "${POLICY}"
}
