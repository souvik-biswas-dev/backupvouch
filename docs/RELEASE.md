# Releasing

The steps for a release, in order. The first release (`v0.1.0`) is also the first time several of these run, so expect to fix small things and re-tag.

## 0. Before tagging

- `main` is green: Tests, Lint and E2E. `make fmt && make verify` passes locally.
- `CHANGELOG.md`: move "Unreleased" under the new version with the date.
- The tag must equal the tag of `DefaultRunnerImage` in `internal/jobs/backup.go` (the publish workflow checks this and fails otherwise). `config/manager` and `make build-installer` use the same tag.

## 1. Dry run of the image build

On GitHub: Actions, "Publish images", Run workflow on `main`. It builds both images and pushes nothing. Fix any failure before tagging.

## 2. Tag and publish

    git switch main && git pull --ff-only
    git tag -s v0.1.0 -m "v0.1.0"      # use -a instead of -s if you have no signing key
    git push origin v0.1.0

The workflow pushes `ghcr.io/souvik-biswas-dev/backupvouch:v0.1.0` and `ghcr.io/souvik-biswas-dev/backupvouch-runner:v0.1.0`, attaches an SBOM and provenance, and signs both by digest.

## 3. Make the packages public

New ghcr.io packages are private. On GitHub open your profile, Packages, each of the two packages, Package settings, Change visibility, Public. Then check from a machine that is not logged in:

    docker logout ghcr.io
    docker pull ghcr.io/souvik-biswas-dev/backupvouch:v0.1.0
    docker pull ghcr.io/souvik-biswas-dev/backupvouch-runner:v0.1.0

## 4. Verify the signature

    cosign verify ghcr.io/souvik-biswas-dev/backupvouch:v0.1.0 \
      --certificate-oidc-issuer https://token.actions.githubusercontent.com \
      --certificate-identity-regexp '^https://github.com/souvik-biswas-dev/backupvouch/\.github/workflows/publish-images\.yml@refs/tags/v'

Do the same for the runner image.

## 5. Install from the released manifest into an empty cluster

This is the test nobody has run yet: it proves a stranger can install the operator. The script creates a fresh kind cluster, applies `dist/install.yaml` (the images are pulled from ghcr.io, nothing is built or loaded), and then runs the same healthy and sabotage checks as `make e2e`.

    make build-installer
    bash hack/install-test.sh

It ends with `INSTALL TEST PASSED` and deletes its cluster (`INSTALL_KEEP_CLUSTER=true` keeps it). If an image cannot be pulled, step 3 (public packages) is not done. The only change it makes to the install is `--metrics-secure=false`, so the script can read metrics without a token.

## 6. GitHub release

    gh release create v0.1.0 dist/install.yaml --title v0.1.0 --notes-file <(sed -n '/^## 0.1.0/,/^## /p' CHANGELOG.md | sed '$d')

`docs/INSTALL.md` links to `releases/download/v0.1.0/install.yaml`, so this file must be attached.

## 7. Project site

The site in `site/` is plain files, no build step. Two ways to publish it as `backupvouch.pages.dev`:

- **Upload:** Cloudflare dashboard, Workers & Pages, Create, Pages, Upload assets. Project name `backupvouch`, drag the `site/` folder in. If the name is taken Cloudflare tells you; choose another and run `bash hack/rename-domain.sh <new-name>.pages.dev` before the first release (the API group follows the site name, decision D23).
- **Git:** connect the GitHub repository, no build command, build output directory `site`. Every push to `main` then updates the site.

## After the release

Register the package on pkg.go.dev by opening `https://pkg.go.dev/github.com/souvik-biswas-dev/backupvouch@v0.1.0` once. Announce nothing until the install test in step 5 has passed.
