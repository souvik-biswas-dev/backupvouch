# Changelog

All notable changes are listed here. The format follows Keep a Changelog. The API is `v1alpha1`: until it reaches `v1beta1`, any release may contain breaking changes, and they are called out here.

## Unreleased

- The API domain is `backupvouch.pages.dev` (API group `backup.backupvouch.pages.dev`, same prefix for the annotation, labels and finalizer). See decision D23. Anything created under an earlier group name is not migrated.
- Project site in `site/` (Cloudflare Pages), alert rules and a Grafana dashboard in `docs/monitoring/`, `CONTRIBUTING.md`, issue and pull request templates, Dependabot, a scheduled security scan, and image signing and SBOM in the publish workflow.

## 0.1.0 (not released yet)

First release: scheduled PostgreSQL backups to S3, restore verification into a scratch Postgres with SQL checks, status conditions, events, Prometheus metrics, retention pruning, manual re-verification, and an optional read-only dashboard.
