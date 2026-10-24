# PLAN.md: build BackupVouch in free Claude chats, step by step

BackupVouch (written `backupvouch` in the repo, Go module, images, API group and metric prefix) is a Kubernetes operator in Go (kubebuilder) that runs database backups on a schedule, restores each one into a scratch environment, runs checks, and vouches for the backup only if it is actually usable. Keep the name the same in every step.

## How this works

1. One step = one chat. Open a new chat on any account, upload the latest zip, paste the **Global rules** block, then paste that step's prompt.
2. Every step ends with a zip of the whole repo (`backupvouch-stepN.zip`). If a free limit hits, switch accounts, upload that zip, paste the **Resume prompt**, and carry on.
3. Claude's chat sandbox usually has no Go toolchain and cannot download Go modules, so it cannot compile or test this project. Code is written carefully but **unverified until you run it locally**. After each step, run the verify command on your machine and paste any errors into the chat (use the **Fix prompt**) before moving on.
4. `docs/CONTEXT.md` is the handoff file. It always says what is done, what is next, and what is broken.

Local verify command (run inside the repo after unzipping):

    make manifests generate fmt vet test

## Documents the repo ends up with (10 core)

| File | Purpose | Written in step |
|---|---|---|
| README.md | what it is, 5-command quickstart, diagram, comparison, limits | 1 (skeleton), 9 (final) |
| AGENTS.md | rules any AI assistant must follow in this repo | 1 |
| docs/CONTEXT.md | handoff state between chats and accounts | every step |
| docs/SPEC.md | problem, users, goals and non-goals, v1 scope | 1 |
| docs/ARCHITECTURE.md | components, reconcile flow, state machine (Mermaid) | 1 |
| docs/API.md | BackupPolicy spec, status, conditions, examples | 1 |
| docs/DECISIONS.md | short records of design choices and why | 1, then as needed |
| docs/TESTING.md | test layers, how to run, the sabotage demo | 6, 7 |
| docs/RUNBOOK.md | what to do when a backup or verification fails | 9 |
| SECURITY.md | threat model, RBAC, secrets handling | 9 |
| docs/BRAND.md | logo concept, palette, usage rules (plus the files in assets/brand/) | 1b |

All ten were drafted for you and ship in `backupvouch-docs.zip`; later steps update them as the code lands. Optional: CONTRIBUTING.md, CHANGELOG.md (step 9), docs/DESIGN.md (only if you build the dashboard in step 8).

---

## Global rules (paste at the start of EVERY chat)

~~~text
I'm building "BackupVouch" (lowercase `backupvouch` in code, repo and images), a Kubernetes operator in Go (kubebuilder v4) that runs
database backups on a schedule, restores each backup into a scratch environment, runs
my checks, and reports whether the backup is really usable. v1 supports PostgreSQL only.
It's a public portfolio project for backend/DevOps roles.

Hard constraints
- Zero cost: kind cluster locally, MinIO in-cluster as S3 storage, no cloud accounts,
  no paid APIs.
- Keep the kubebuilder scaffold layout (api/, internal/controller/, cmd/, config/).
- CRD: group backup.backupvouch.dev, version v1alpha1, kind BackupPolicy.
- Security basics: credentials only via Kubernetes Secrets and never logged, least-privilege
  RBAC, non-root containers, pinned image tags (no :latest).

Working rules
1. First read AGENTS.md and docs/CONTEXT.md in the uploaded zip. Do only the step I name.
2. Go is probably NOT installed in your sandbox and Go module hosts are probably
   unreachable. Check with `which go` and don't assume. Never claim code compiles or tests
   pass unless you actually ran them. Write small, conservative code that uses APIs you are
   sure exist, and mark anything uncertain with a `// VERIFY:` comment.
3. Touch only the files the step needs, and list them before you start.
4. Finish every step the same way:
   a. update docs/CONTEXT.md (done, next, known issues, decisions, what I must verify locally);
   b. create /mnt/user-data/outputs/backupvouch-stepN.zip with the whole repo, excluding
      .git, bin/, testbin/ and cover.out;
   c. call present_files with the zip;
   d. end with at most 10 lines: what changed, the exact commands I should run locally,
      and what to paste back.
5. If you notice I'm near my free usage limit (about 90%), stop adding anything. Finish the
   file you're on, then do rule 4 immediately with "Status: partial" and a precise list of
   what remains.
6. If a step is too big for one chat, split it into Na and Nb and say so in CONTEXT.md.
~~~

## Resume prompt (use after switching accounts or starting a new chat)

~~~text
Resume BackupVouch. The attached zip is the latest repo state. Read AGENTS.md and
docs/CONTEXT.md first, then do Step N from docs/PLAN.md. My local verify result:
<paste output, or write "not run yet">.
~~~

## Fix prompt (use when the local verify command fails)

~~~text
Here is the output of `make manifests generate fmt vet test` on my machine. Fix only what
these errors point to, keep the change minimal, don't refactor, update docs/CONTEXT.md,
and give me a new zip backupvouch-stepN-fix.zip.
<paste errors>
~~~

---

## Step 0: scaffold the project (you, locally, about 10 minutes)

Claude can't run kubebuilder in its sandbox, so you create the scaffold. Prerequisites: Go, Docker, kubectl, kind, kubebuilder.

    mkdir backupvouch && cd backupvouch
    git init
    kubebuilder init --domain backupvouch.dev --repo github.com/souvik-biswas-dev/backupvouch
    kubebuilder create api --group backup --version v1alpha1 --kind BackupPolicy --resource --controller
    make manifests generate
    go build ./...
    unzip -o ../backupvouch-docs.zip -d .
    git add -A && git commit -s -m "Scaffold project and add design docs"
    zip -r ../backupvouch-step0.zip . -x ".git/*" "bin/*" "testbin/*" "cover.out"

`backupvouch-docs.zip` contains the ten drafted documents (README.md, AGENTS.md, SECURITY.md and docs/). Unzipping it with `-o` replaces the scaffold's README.md. Answer `y` to both kubebuilder prompts. Keep `backupvouch-step0.zip`; every later chat starts from the newest zip.

---

## Step 1: review the drafted documents (no Go code)

The ten documents were already drafted in `backupvouch-docs.zip`. This step is a consistency review, so later steps can trust them.

~~~text
Step 1. Upload: backupvouch-step0.zip. The design documents are already written. Review
them for consistency; do not rewrite them from scratch.

Check and fix:
- Field names, enum values, condition types and reasons, metric names, Job names and
  termination message formats are identical in docs/API.md, docs/ARCHITECTURE.md,
  docs/RUNBOOK.md, docs/SECURITY.md and docs/TESTING.md.
- Every RBAC verb in SECURITY.md is justified by the architecture, and nothing the
  architecture needs is missing (for example reading Pod status for termination messages).
- docs/DECISIONS.md matches what ARCHITECTURE.md actually describes.
- Anything that is a claim about Velero, CloudNativePG, K8up or FireDrill is marked "(verify)", and nothing claims BackupVouch is the first tool of its kind.
- README.md does not claim anything works yet.
- Point out design risks you see (for example the 4 KB termination message limit, backup
  Job idempotency, scratch Postgres on a non-root image) and propose fixes as edits to the
  documents.

Make the edits, update docs/CONTEXT.md, and give me the zip backupvouch-step1.zip.
~~~

---

## Step 1b: brand identity (vector logo and favicon)

~~~text
Step 1b. Upload: the latest zip. Create the BackupVouch brand identity as hand-authored
vector files and wire it into the repo.

Name and meaning: BackupVouch. To vouch is to give a trusted guarantee that something is
genuine. The product vouches that a backup really restores. Audience: DevOps and platform
engineers. Tone: precise, calm, trustworthy; not corporate-generic, not cartoonish.

Work in two passes.

Pass 1, write docs/BRAND.md first, containing:
- the concept: ONE idea grounded in vouching, proof, or restoring, and why it fits;
- the palette: 4-5 named hex colours, with how they work on light and dark backgrounds;
- the wordmark typeface (open licence) and why;
- construction notes: grid, stroke widths, corner treatment, minimum size, clear space;
- do and don't rules.
Directions worth considering (reject any that feel like defaults): a notary-style seal
built from concentric arcs; a check mark formed from stacked backup layers; a stamped
impression that completes only when the restore succeeds. Invent a better one if you can.
Avoid these clichés: a shield with a check mark, a padlock, a cloud, a database cylinder with
a tick, a letter in a circle or rounded square, gradient blobs, "sparkle" shapes, 3D or
glossy effects, and more than three colours in the mark. Review your concept against this
list and revise anything generic before drawing.

Pass 2, build the files in assets/brand/:
- logo-mark.svg: the mark alone, square viewBox, flat vector.
- logo.svg: horizontal lockup of mark and wordmark. Convert the text to outlines (for
  example with fonttools and an open-licence font taken from an npm @fontsource package)
  or construct the letters geometrically, so the SVG never depends on an installed font.
- logo-mono.svg: single colour. Handle dark backgrounds either with a second file
  logo-dark.svg or with prefers-color-scheme inside the SVG; pick one approach and use it
  everywhere.
- favicon.svg: a simplified version of the mark that stays legible at 16 pixels, with
  thicker strokes and fewer details.
- Rasters rendered from the SVGs: favicon.ico (16, 32 and 48 pixels), favicon-32.png,
  apple-touch-icon.png (180 pixels), and social-preview.png (1280 by 640, for GitHub's
  social preview setting). If you can render in your sandbox (for example with cairosvg
  installed through pip), do it and look at the 16, 32, 64 and 256 pixel renders; fix
  anything muddy. If you cannot render, say so and give me the exact commands to run
  locally.
- All SVGs must be well-formed XML with a valid viewBox, no scripts, no external
  references, and no embedded raster images.

Wire it in:
- README.md uses a <picture> element with the light and dark logo at the top.
- docs/BRAND.md shows the <link rel="icon"> snippets and how the dashboard (step 8) will
  use favicon.svg and the palette.
Update docs/CONTEXT.md and give me the zip backupvouch-step1b.zip.
~~~

---

## Step 2: API types

~~~text
Step 2. Upload: the latest zip. Implement api/v1alpha1/backuppolicy_types.go exactly as
docs/API.md describes.

- kubebuilder markers: validation (required fields, enums, minimums, cron pattern),
  defaults, status subresource, printer columns, short name.
- A sample CR in config/samples/ and an updated kustomization.
- Pure helper functions in api/v1alpha1 (defaulting, validation of check definitions) with
  table-driven unit tests next to them.
- Do not write controller logic yet.

Remind me to run `make manifests generate` and the verify command.
~~~

---

## Step 3: scheduling and the backup Job

~~~text
Step 3. Upload: the latest zip. Implement scheduled backups.

- internal/schedule: compute the next run and whether a run is due, using
  github.com/robfig/cron/v3 (tell me to run `go get` for it). Keep it pure and fully
  unit-tested, including time zones and missed runs.
- internal/jobs: a builder that turns a BackupPolicy into a backup batch/v1 Job (runner
  image, args, env from Secret references, resource limits, non-root securityContext,
  backoffLimit, activeDeadlineSeconds, owner reference). Unit-test the builder output.
- cmd/runner with the `backup` subcommand: pg_dump (custom format) streamed to S3-compatible
  storage with github.com/minio/minio-go/v7, printing a small JSON result (object key, size,
  sha256) to /dev/termination-log. No secrets in logs.
- Dockerfile.runner (postgres:16-alpine base plus the runner binary, non-root).
- Do not wire the full controller yet.
~~~

---

## Step 4: restore verification

~~~text
Step 4. Upload: the latest zip. Implement verification.

- cmd/runner `verify` subcommand: download the backup object, check its sha256 against the
  recorded value, start a throwaway Postgres (initdb and pg_ctl as a non-root user, data in
  an emptyDir), pg_restore into it, run each configured SQL check, measure restore duration,
  and write a JSON result to /dev/termination-log: per-check pass/fail with a short message,
  overall result, duration. Always shut Postgres down and clean up.
- A checks package: define how a check passes (the query must return a single boolean true,
  or a value that matches an expected value) and unit-test it.
- internal/jobs: the verification Job builder (same hardening as the backup Job, plus a
  NetworkPolicy suggestion in docs/SECURITY notes).
- Update docs/DECISIONS.md if you deviate from the plan.
~~~

---

## Step 5: the controller

~~~text
Step 5. Upload: the latest zip. Wire everything into the reconciler.

- Reconcile implements the state machine from docs/ARCHITECTURE.md: decide whether a run
  is due, create the backup Job, read its result, create the verification Job, read its
  result, update status and conditions, and requeue sensibly.
- Be idempotent: reconcile may run twice or after a crash at any point, and must not
  create duplicate Jobs. Use deterministic Job names and owner references.
- Failure handling: Job failure, timeout, missing or invalid termination message, deleted
  Secret. Each ends in a clear condition and an Event.
- Finalizer and cleanup of Jobs and temporary resources on delete, honouring retention.
- Prometheus metrics registered with controller-runtime: last backup timestamp, last
  successful verification timestamp, verification success (0/1), restore duration seconds.
- RBAC markers: the minimum set of verbs. Explain each in docs/SECURITY notes.
~~~

---

## Step 6: tests and CI

~~~text
Step 6. Upload: the latest zip. Add the test layers described in docs/TESTING.md.

- envtest-based controller tests. envtest doesn't run Jobs, so the tests set Job status
  by hand and assert the state transitions, idempotency, and failure paths.
- hack/e2e.sh: create a kind cluster, deploy MinIO and a seeded Postgres, build and load
  the operator and runner images, apply a BackupPolicy, wait for Verified=True. Then
  SABOTAGE: corrupt the stored dump and confirm the next verification becomes Failed with
  the right condition. Exit non-zero if either expectation is wrong.
- .github/workflows/ci.yml (free for public repos): fmt, vet, unit and envtest tests, and
  the e2e script using a kind action.
- Makefile targets: e2e and verify.
- docs/TESTING.md describing all of the above.
~~~

---

## Step 7: packaging and the demo

~~~text
Step 7. Upload: the latest zip. Make it easy to install and demo.

- config/default and an install kustomization; image build targets (docker-build,
  docker-build-runner).
- config/samples/demo/: MinIO, a Postgres with seed data, and a BackupPolicy.
- hack/demo.sh: one command that spins up kind and runs the whole story: backup, verify
  OK, sabotage, verify FAILED, and prints the status and the events each time.
- Optional GitHub Actions job that publishes images to ghcr.io (free for public repos).
- Add the demo transcript to docs/TESTING.md.
~~~

---

## Step 8 (optional): status dashboard with a distinctive design

The operator itself has no UI. If you want a visual piece for the portfolio, add a small read-only dashboard served by the operator.

~~~text
Step 8. Upload: the latest zip. Build a small READ-ONLY status dashboard.

Functionality
- The operator serves JSON at /api/v1/policies (read from the manager's cache; no writes)
  and the static dashboard from embedded files (go:embed) on a separate port.
- The dashboard shows each BackupPolicy: last backup, last verified restore, pass/fail
  history, restore duration, and the failing check when verification fails.
- No build step: plain HTML, CSS and ES modules. Everything is vendored and embedded.
  No CDN links, because clusters may be offline. Fonts are self-hosted (open-licence
  fonts, e.g. via @fontsource packages from npm), as woff2 files in the repo.

Brand (from step 1b)
- Use assets/brand/favicon.svg as the page favicon (embedded in the binary) and the logo mark
  in the page header. Derive the colour tokens from docs/BRAND.md. The dashboard may extend
  the palette but must not contradict the brand.

Design brief (this is the point of the step)
Do NOT produce a generic AI-looking dashboard. Work in two passes.

Pass 1, a short design plan before any code:
- Pick ONE concept grounded in this product's subject (backups, proof that a restore
  worked, time). Possible directions, to be rejected if they feel like defaults: a
  chain-of-custody ledger where each verification is a stamped entry; strata, with each
  backup as a sediment layer and failed verifications as fractures; a tide-table style time
  axis. Choose or invent one and say why it fits.
- Tokens: 4-6 named hex colours, one or two typefaces with clear roles, a type scale, and a
  layout described in prose plus an ASCII wireframe.
- Review the plan against the list of generic tells below. If any part of it is a default
  rather than a choice made for this product, revise it and say what you changed.

Generic tells to avoid unless I explicitly ask for them:
- warm cream background with a high-contrast serif and a terracotta accent;
- near-black background with one acid-green or vermilion accent;
- newspaper-style hairline rules with zero border radius;
- content chopped into identical rounded cards with the same soft grey shadow and gradient
  washes;
- tracked-out ALL-CAPS eyebrow labels above every heading, "A · B · C" meta strings, "WORD
  - fragment" labels, arrows appended to every link, monospace for every small label;
- accenting a single word in a headline, numbered 01/02/03 markers where the content is
  not a sequence, fade-and-slide-up entrances on every section, hover effects on every card.

Pass 2, build it:
- Spend boldness in ONE memorable element (for example a custom SVG timeline of
  verifications drawn with d3-scale and d3-shape, vendored). Keep everything else quiet.
- At most two small libraries, each justified in docs/DESIGN.md. No Tailwind or component-kit
  defaults; use custom CSS with CSS variables.
- Motion only for one orchestrated moment on load and for responses to the user's actions.
- Quality floor: responsive down to a phone, visible keyboard focus, respects
  prefers-reduced-motion, accessible contrast, and a useful empty state and error state in
  plain language (say what happened and what to do).
- Critique your own result, remove one decoration, and write docs/DESIGN.md with the
  tokens, the concept, and the decisions.
~~~

---

## Step 9: finish the documents and polish

~~~text
Step 9. Upload: the latest zip. Finish the documentation and repository polish.

- README.md: one-sentence pitch, a diagram, a 5-command quickstart that matches
  hack/demo.sh, the sabotage demo shown as terminal output, a "How this differs from
  Velero, CloudNativePG and K8up" section (verify claims against their current docs),
  limitations and roadmap (MongoDB and Redis engines next).
- docs/RUNBOOK.md: for each alert or failure condition, what it means and what to do.
- SECURITY.md: threat model, RBAC rationale, secrets handling, how to report issues.
- CONTRIBUTING.md and CHANGELOG.md (v0.1.0).
- Make every doc consistent with the code as it now exists, and list any gaps honestly in
  the README limitations.
- Suggest a GitHub repository description and topics, and remind me to upload assets/brand/social-preview.png in the repository settings (Social preview).
- Check the README logo renders on GitHub in both light and dark mode.
~~~

---

## Before you publish

- Run the verify command and the e2e script on your own machine, and fix whatever fails. Don't publish code that has never compiled.
- Read the code and docs once end to end. Be ready to explain the reconcile loop, why Jobs, and what happens if the controller crashes halfway through a restore.
- Say honestly in the README that you built it with AI assistance if you did. Flux and many other projects ask for it, and it's the better habit.
- Pin the repo on your GitHub profile.
