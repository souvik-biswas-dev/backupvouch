# Changelog

All notable changes are listed here. The format follows Keep a Changelog. The API is `v1alpha1`: until it reaches `v1beta1`, any release may contain breaking changes, and they are called out here.

## Unreleased

Nothing yet.

## 0.1.0 (2026-10-25, pre-release)

First release. Scheduled PostgreSQL backups to S3, restore verification into a scratch Postgres with SQL checks, status conditions, events, Prometheus metrics, retention pruning, manual re-verification, and an optional read-only dashboard. Images `ghcr.io/souvik-biswas-dev/backupvouch:v0.1.0` and `backupvouch-runner:v0.1.0` are signed with cosign and carry an SBOM and provenance.

- The API domain is `backupvouch.pages.dev` (API group `backup.backupvouch.pages.dev`, same prefix for the annotation, labels and finalizer). See decision D23.
- Project site in `site/`, alert rules and a Grafana dashboard in `docs/monitoring/`, `CONTRIBUTING.md`, issue and pull request templates, Dependabot, a scheduled security scan, and image signing and SBOM in the publish workflow.
- Proven so far: `make verify`, `make e2e`, `make demo`, and `hack/install-test.sh` (a fresh kind cluster installed from `dist/install.yaml` with the released images). Not yet proven: real databases other than the demo one, Postgres versions other than 16, a cluster other than kind.
