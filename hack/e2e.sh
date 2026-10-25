#!/usr/bin/env bash
# End-to-end test for BackupVouch in a throwaway kind cluster. See docs/TESTING.md.
#
#   1. build the operator and runner images and load them into kind
#   2. deploy the operator, an S3 store (SeaweedFS) and a seeded Postgres, apply a BackupPolicy
#   3. HEALTHY CASE: expect RestoreVerified=True
#   4. SABOTAGE: corrupt the stored dump, ask for a re-verification, expect RestoreVerified=False
#      with reason ChecksumMismatch, a VerificationFailed event and backupvouch_verification_success 0
#
# Exit code 0 only if both expectations hold. Needs docker, kind, kubectl and make.
# The steps live in hack/lib.sh and are shared with hack/demo.sh; the fixtures in config/samples/demo/.
#
# Environment:
#   E2E_CLUSTER        kind cluster name (default backupvouch-e2e)
#   E2E_KEEP_CLUSTER   "true" keeps a cluster this script created (default: delete it on exit)
#   CONTROLLER_IMG     operator image tag (default backupvouch-controller:e2e)
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
