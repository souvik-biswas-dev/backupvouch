# Monitoring

BackupVouch exports four gauges per policy (labels `namespace` and `policy`) on the operator's metrics endpoint, and they are recomputed on every reconcile, so they come back after an operator restart.

| Metric | Meaning |
|---|---|
| `backupvouch_verification_success` | 1 if the last verification passed, 0 if it failed |
| `backupvouch_last_verified_timestamp_seconds` | when the last verification passed |
| `backupvouch_last_backup_timestamp_seconds` | when the last backup succeeded |
| `backupvouch_restore_duration_seconds` | how long the last restore took, in whole seconds (rounded up) |

## Alerts

`docs/monitoring/prometheusrule.yaml` has four rules: a failed restore check (critical), no passed check or no successful backup for 36 hours (warning), and a restore slower than 30 minutes (info). Apply it with `kubectl apply -f` if you run prometheus-operator, or copy the `groups:` block into your own rules. The 36 hour limits assume a daily schedule; use about 1.5 times your schedule interval. What to do when an alert fires is in [RUNBOOK.md](RUNBOOK.md).

## Dashboard

`docs/monitoring/grafana-dashboard.json` has six panels (newest backup state, time since the last passed check, last restore time, and three time series). In Grafana choose Dashboards, New, Import, upload the file and pick your Prometheus data source.

## Scraping

The metrics endpoint is protected (see `config/prometheus/` for the ServiceMonitor and the TLS patch, and SECURITY.md). Nothing here has been tested against a live Prometheus yet: if a rule or panel is wrong, fix it and tell the project.
