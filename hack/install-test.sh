#!/usr/bin/env bash
# Installs BackupVouch the way a user would, from the released manifest, into a fresh kind cluster, and
# runs the same healthy and sabotage checks as hack/e2e.sh. See docs/RELEASE.md, step 5.
#
#   make build-installer           # writes dist/install.yaml
#   bash hack/install-test.sh
#
# Unlike hack/e2e.sh it builds nothing and loads nothing into kind: the operator and runner images named in
# the manifest are pulled from ghcr.io, so they must be public (docs/RELEASE.md, step 3). Only the demo
# fixtures (S3 store, Postgres, policy) come from this repository.
#
# Environment:
#   INSTALLER             manifest to apply (default dist/install.yaml)
#   INSTALL_CLUSTER       kind cluster name (default backupvouch-install)
#   INSTALL_KEEP_CLUSTER  "true" keeps the cluster afterwards (default: delete it)
#   KIND                  kind binary (default kind)

set -euo pipefail

CLUSTER="${INSTALL_CLUSTER:-backupvouch-install}"
KEEP_CLUSTER="${INSTALL_KEEP_CLUSTER:-false}"
INSTALLER="${INSTALLER:-dist/install.yaml}"
PASS_MESSAGE="INSTALL TEST PASSED: the released manifest installs, a healthy backup is verified, a sabotaged one is rejected."
FAIL_MESSAGE="INSTALL TEST FAILED"

# shellcheck source=hack/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

stage_preflight
[[ -f "${INSTALLER}" ]] || fail "${INSTALLER} not found, run: make build-installer"
grep -E '^\s*image: ' "${INSTALLER}" | sort -u

log "creating kind cluster ${CLUSTER}"
if "${KIND}" get clusters | grep -qx "${CLUSTER}"; then
  fail "cluster ${CLUSTER} already exists, delete it first: ${KIND} delete cluster --name ${CLUSTER}"
fi
"${KIND}" create cluster --name "${CLUSTER}" --wait 120s
CREATED_CLUSTER=true
kubectl config use-context "kind-${CLUSTER}" >/dev/null

log "applying ${INSTALLER}"
k apply -f "${INSTALLER}"
# The only change to the install: serve metrics over plain HTTP so this script can read them without a token.
k -n "${OP_NS}" patch deployment "${OP_DEPLOY}" --type=json \
  -p '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--metrics-secure=false"}]'
k -n "${OP_NS}" rollout status "deployment/${OP_DEPLOY}" --timeout=300s
k wait --for=condition=Established crd/backuppolicies.backup.backupvouch.pages.dev --timeout=60s

stage_fixtures
stage_healthy
stage_sabotage
stage_final
