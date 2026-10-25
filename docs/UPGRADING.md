# Upgrading

BackupVouch is `v1alpha1`. Until the API reaches `v1beta1`, any release can change or remove fields. Every breaking change is listed in [CHANGELOG.md](../CHANGELOG.md) before you upgrade.

## How to upgrade

1. Read the CHANGELOG entries between your version and the new one.
2. Apply the new manifest: `kubectl apply -f https://github.com/souvik-biswas-dev/backupvouch/releases/download/<version>/install.yaml` (or your own kustomize build). `kubectl apply` updates the CRD in place and keeps your BackupPolicies.
3. Check the rollout: `kubectl -n backupvouch-system rollout status deploy/backupvouch-controller-manager`.
4. Check that every policy still reports: `kubectl get backuppolicies -A`. The `status` is recomputed on the first reconcile, and the metrics come back with it.

Do not delete the CRD to "reinstall": deleting a CRD deletes every BackupPolicy. Stored backups in your bucket are not touched by BackupVouch either way, but the history in `status` would be gone.

## Runner image

The operator starts each Job with one runner image, by default the one with the same version tag as the operator, and only images on the `--allowed-runner-images` list are accepted (see [INSTALL.md](INSTALL.md)). When you set a custom runner image, upgrade it together with the operator.

## Not tested yet

There is no automated upgrade test (install version N, upgrade to N+1, check that a policy keeps working). Until there is one, try an upgrade on a non-critical policy first.
