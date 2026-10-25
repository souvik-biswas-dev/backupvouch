#!/usr/bin/env bash
# Renames the API domain of the project, for example from backupvouch.pages.dev to backupvouch.pages.dev.
#
#   bash hack/rename-domain.sh backupvouch.pages.dev
#
# It changes every tracked text file that mentions the old domain (API group, annotation, labels,
# finalizer, RBAC markers, samples, docs, scripts) and renames the generated CRD file. Run
# `make manifests generate fmt verify` afterwards: controller-gen must produce the same result.
# The old domain is read from PROJECT, so running the script twice with the same name does nothing.
set -euo pipefail

new="${1:-}"
[ -n "${new}" ] || { echo "usage: $0 <new-domain>   e.g. backupvouch.pages.dev" >&2; exit 2; }
case "${new}" in
  *[!a-z0-9.-]*|.*|*.|*..*) echo "'${new}' is not a plain lower-case domain name" >&2; exit 2;;
esac
case "${new}" in *.*) ;; *) echo "'${new}' needs at least one dot" >&2; exit 2;; esac

cd "$(git rev-parse --show-toplevel)"
old="$(sed -n 's/^domain: //p' PROJECT | head -n 1)"
[ -n "${old}" ] || { echo "could not read the domain from PROJECT" >&2; exit 1; }
if [ "${old}" = "${new}" ]; then echo "already using ${new}, nothing to do"; exit 0; fi

# Escape the dots for sed (only dots are special in a domain name).
old_re="${old//./\\.}"
files="$(git grep -Il --untracked -- "${old}" || true)"
count=0
for f in ${files}; do
  sed -i "s/${old_re}/${new}/g" "${f}"
  count=$((count + 1))
done

# controller-gen names the CRD file <group>_<plural>.yaml, so it moves with the group.
oldcrd="config/crd/bases/backup.${old}_backuppolicies.yaml"
newcrd="config/crd/bases/backup.${new}_backuppolicies.yaml"
if [ -f "${oldcrd}" ]; then git mv "${oldcrd}" "${newcrd}"; fi

# Record the decision once.
if ! grep -q '^## D23:' docs/DECISIONS.md; then
  cat >> docs/DECISIONS.md <<DEC

## D23: The API domain is ${new}

- **Context:** a Kubernetes API group should be a DNS name the project controls. The first choice, \`${old}\`, would have cost money for a project with no users yet.
- **Decision:** the API group is \`backup.${new}\`, a free Cloudflare Pages name for the project site (\`site/\`). The same domain prefixes the annotation \`${new}/reverify\`, the labels \`${new}/policy\`, \`${new}/run-id\` and \`${new}/role\`, and the finalizer.
- **Consequences:** the rename breaks any BackupPolicy created under the old group (there were no users). A later move to a registered domain is another breaking change, so do it before a first stable release, never after.
DEC
fi

echo "renamed ${old} -> ${new} in ${count} files; CRD file moved if it existed"
echo "next: make manifests generate fmt verify && make dashboard-js-test, then git status --short"
