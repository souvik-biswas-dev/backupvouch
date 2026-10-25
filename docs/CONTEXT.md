# CONTEXT (handoff)

Last updated: 2026-10-25, after the dashboard fixes and the step 9 documents were written (not yet run locally).
Project: BackupVouch, a Kubernetes operator in Go (kubebuilder v4) that runs PostgreSQL backups and verifies them by restoring into a scratch Postgres.
Repo: github.com/souvik-biswas-dev/backupvouch, local path `~/oss/backupvouch`. Author: Souvik. Free Claude plan, so work is done in small steps and handed over with this file.

## Where things stand

| Step | What | Status |
|---|---|---|
| 0 to 8 | scaffold, docs, brand, API, Jobs, verification, controller, tests, demo, dashboard | done and merged (check PR #7 on GitHub) |
| 6b | controller fixes a to f | merged |
| 8b | dashboard failure wording and crowded track | written, **not run locally yet** (JS tests pass in the sandbox, Go not compiled) |
| 9 | final docs | written, **needs the demo transcript and local checks** |

## What the latest zip changed

Dashboard (`internal/dashboard`):
- `view.go`: `Verification.Reason` (JSON `reason`), the reason of the `RestoreVerified` condition, only while that condition is False and the last verification failed (`failureReason`). Test: `TestVerificationReasonComesFromTheFalseCondition`.
- `web/js/copy.js`: `verdictText(verdict, reason)`, `runText(state, reason)`, `failureSummary(lv)`. A checksum or storage failure no longer says "restored". Without failed checks, the message is the main line, with a fallback "The restore check failed and no reason was recorded."
- `web/js/render.js`: uses the three functions above. Only the newest run gets the reason-specific sentence.
- `web/js/scale.js`: minute steps (5, 10, 15, 30), `MINUTE`, `MIN_SPAN` = 30 minutes.
- `web/js/timeline.js`: the window is built from now and the runs. A next run further than 3 window widths away is drawn as an arrow at the right edge labelled "next in 22 h" (100 px of room is kept at the right); a close one is included in the window as before.
- `web/app.css`: `.next-arrow`.
- `jstest/scale.test.mjs`, `jstest/copy.test.mjs`: updated and extended.

Docs:
- `README.md` rewritten (no more "design phase").
- `docs/API.md`: `suspend` behaviour (item g, decided: intended, a suspended idle policy does nothing, including re-verify), the non-False condition reasons (item i), the verify reason whitelist, restore time rounded up, `lastVerification.message` wording.
- `docs/ARCHITECTURE.md`: prune counts are not read by the controller (item h, decided: dropped the claim instead of changing Go code), the 6b behaviour (rule 8).
- `docs/DECISIONS.md` D11, `docs/RUNBOOK.md` (where to read details, deleted Jobs, pausing), `docs/TESTING.md`, `docs/PLAN.md` progress table.
- `docs/TESTING.md` has a marked placeholder `DEMO-TRANSCRIPT-PLACEHOLDER` for the real demo output.

## Do now (in this order)

1. Apply the zip, then run the checks in "Before every commit" below.
2. `make demo && make dashboard-enable && make dashboard`, open http://localhost:9090 at 1280 px and 375 px. Look at: the sabotage policy text (should say the backup could not be trusted and was not restored, with the checksum message as the main line), and the track with 3 runs a few minutes apart (minute ticks, "next in 22 h" arrow at the right edge, nothing on top of anything). Not checked by anyone yet: 375 px layout, keyboard use of the track (arrows, Home, End, Enter), reduced motion.
3. `make demo-transcript`, then paste the real output into `docs/TESTING.md` in place of the placeholder block.
4. Commit on a branch, push, open a PR, wait for Tests, Lint and E2E, merge.

## Still unchecked before calling it finished
- The comparisons with Velero, CloudNativePG, K8up and FireDrill in `docs/SPEC.md` are from memory. The README only says they are being checked. Check them against each project's current README.
- There is no `LICENSE` file. The README says Apache-2.0 is intended. Add the Apache-2.0 text as `LICENSE` before the first release.
- Name availability on GitHub, pkg.go.dev and Artifact Hub (D9).
- `.github/workflows/publish-images.yml` has never run (needs a tag push). `dist/install.yaml` was never applied to a cluster.
- `config/manager/manager.yaml` has no `--allowed-runner-images` (D14).
- Prune counts: the controller does not read `{deleted, kept}`. Implement only if wanted: read the prune Pod termination message and put the counts in the `RetentionPruned` event.
- Coverage gaps: `cmd/runner` about 31%, `internal/checks` about 48%, `api/v1alpha1` about 36%, `cmd/main.go` none.
- The demo cluster may still be running: `kind delete cluster --name backupvouch-demo`.

## How to work (what worked so far)
1. Upload this file plus the files to change (or the repo zip); the assistant edits and returns a zip or a patch.
2. Apply: `cd ~/oss/backupvouch && unzip -o ~/Downloads/<zip>`.
3. Before every commit: `make fmt && make verify`, `make dashboard-js-test`, then `git status --short config/ api/` (must be empty), `go mod tidy && git diff --stat go.mod go.sum` (must be empty), `grep -rIn '^<<<<<<<' . --exclude-dir=.git --exclude-dir=bin` (must find nothing).
4. `make lint` fails locally with a Go toolchain mismatch (golangci-lint built for an older Go). That is not a code finding. If the Lint job fails on GitHub, paste the red lines into a chat. Lint rules (`.golangci.yml`): no `sort` package (use `slices`/`cmp`), `new(expr)` instead of a `ptr` helper (Go 1.26), preallocate slices appended to in a plain loop, no variable names that shadow an import.
5. Branch per step, `git commit -s`, PR, wait for Tests, Lint and E2E, then merge.
6. The assistant's sandbox has no Go and no module access, so nothing it writes in Go is compiled until you run `make verify`. It can run the node tests.

## Prompt to start the next chat
"Read CONTEXT.md. The dashboard fixes and step 9 docs were written but not run. Here is the output of `make verify` and `make dashboard-js-test` [paste]. Fix anything red, then help me with [the demo transcript / the SPEC comparison check / the LICENSE file]." Upload this file and, if something is red, the files named in the error.
