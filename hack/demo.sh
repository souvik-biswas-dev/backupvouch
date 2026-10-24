#!/usr/bin/env bash
# The BackupVouch demo: one command, the whole story, in a throwaway kind cluster.
#
#   1. build the operator and runner images, create a kind cluster, deploy the operator
#   2. deploy an S3 store (SeaweedFS) and a Postgres with seed data, apply a BackupPolicy
#   3. backup -> restore into a scratch Postgres -> checks pass: RestoreVerified=True
#   4. SABOTAGE: corrupt the stored backup and ask for a re-check: RestoreVerified=False
#   After each stage the BackupPolicy status and its events are printed.
#
# It runs the same steps as hack/e2e.sh (shared in hack/lib.sh), with commentary. Needs docker,
# kind, kubectl and make. Takes about 5 to 10 minutes the first time (image builds and pulls).
#
# Unlike the e2e test, the cluster is KEPT afterwards so you can look around. A previous demo
# cluster with the same name is deleted first. Remove it with: make demo-clean
#
# Environment:
#   DEMO_CLUSTER        kind cluster name (default backupvouch-demo)
#   DEMO_KEEP_CLUSTER   "false" deletes the cluster when the demo ends (default true)
#   DEMO_PAUSE          "true" waits for Enter after each stage, handy when presenting
#   CONTROLLER_IMG      operator image tag (default backupvouch-controller:demo)
#   CURL_IMG            curl image used only for one-off signed S3 requests (create bucket, corrupt backup)
#   KIND                kind binary (default kind)

set -euo pipefail

CLUSTER="${DEMO_CLUSTER:-backupvouch-demo}"
KEEP_CLUSTER="${DEMO_KEEP_CLUSTER:-true}"
CONTROLLER_IMG="${CONTROLLER_IMG:-backupvouch-controller:demo}"
NARRATE=true
PASS_MESSAGE="DEMO FINISHED: the healthy backup was verified and the sabotaged one was rejected."
FAIL_MESSAGE="DEMO FAILED"

# shellcheck source=hack/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

say "BackupVouch: a backup only counts once it has been restored and checked."
stage_preflight

# A kept demo cluster still holds the previous run's policy and status; start from a clean one.
if "${KIND}" get clusters 2>/dev/null | grep -qx "${CLUSTER}"; then
  say "Removing the previous demo cluster ${CLUSTER}."
  "${KIND}" delete cluster --name "${CLUSTER}"
fi

stage_images_and_cluster
stage_operator
stage_fixtures
stage_healthy
stage_sabotage
stage_final

say "What you saw: the backup job had succeeded, yet the stored copy could no longer be restored. BackupVouch found that out by restoring it, and reports it in status conditions, events and the backupvouch_verification_success metric."
