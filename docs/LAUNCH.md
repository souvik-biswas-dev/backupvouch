# Launch plan

Do this **after** `docs/REAL-DB-TEST.md` is done. Everything you claim must be something you ran. Never claim "production ready", never state facts about other tools that you did not check (the table in `docs/SPEC.md` has the date it was checked).

## Is it worth launching?

Yes, as an early open-source project asking for feedback. The pain is real (backups that fail only when needed), the demo is a clear story, and the code is small and tested. It is a narrow tool: PostgreSQL dumps into S3, Postgres 16, no point-in-time recovery. FireDrill verifies backups from other tools and has more verification features; CloudNativePG, pgBackRest and Barman cover serious physical backup needs. The likely users are small teams running Postgres on Kubernetes with `pg_dump` backups. The goal of a first launch is not stars. It is **three people who ran it on a real database and told you what broke.**

## Before you post anything

- [ ] Real-database test done (three databases), the bugs it found fixed and released as `v0.1.1` or later
- [ ] GitHub release published, repository About filled in, social preview uploaded, site live
- [ ] A 2 minute screen recording of the demo (script below), uploaded as a GitHub release asset or to YouTube; linked from the README
- [ ] README top section says what it is, what it is not, and that it is alpha
- [ ] Issues are on, the issue templates work, and you have time to answer for a week

## The 2 minute demo (script)

1. (0:00) Title: "A backup only counts once it has been restored and checked." One sentence on the problem.
2. (0:15) Show the `BackupPolicy` YAML: schedule, source, bucket, two SQL checks.
3. (0:30) `make demo` output or `kubectl get backuppolicy -w`: backup, restore, `VERIFIED True`, restore time.
4. (1:00) "Now bit rot": overwrite the stored dump. Re-verify. `VERIFIED False`, reason `ChecksumMismatch`, the event, the metric at 0.
5. (1:40) The dashboard showing the failure in plain words.
6. (1:55) Repo and site links, "alpha, feedback wanted".

Record at a readable font size, no music, no speed-ups of the important parts.

## Posts (adapt, keep them short and honest)

**Show HN title:** `Show HN: BackupVouch, a Kubernetes operator that restores your Postgres backups to check them`

**Show HN first comment:**

> I built this because a backup job that exits 0 proves very little. BackupVouch takes a PostgreSQL backup on a schedule, restores it into a throwaway Postgres, runs SQL checks you wrote, and reports the result in status, events and Prometheus metrics. In the demo I corrupt the stored dump after the backup job succeeded, and it catches that.
>
> It is an early alpha: PostgreSQL 16 only, plain dumps to S3, no point-in-time recovery, tested in kind and on [N] real databases so far. FireDrill already verifies backups made by other tools; this is a smaller operator that takes the backup and checks it in one resource. I would like to hear what breaks on your database.
>
> Repo: https://github.com/souvik-biswas-dev/backupvouch  Site: https://backupvouch.pages.dev

**Reddit (r/kubernetes, r/PostgreSQL):** read each subreddit's self-promotion rules first; some want a text post that explains the problem, with the link at the end, or only allow it in a weekly thread. Lead with the sabotage story, not with the project name.

**Blog post idea:** "My backup job said success and the backup was garbage". Show the sabotage, explain `ChecksumMismatch`, and what a restore check catches that exit codes do not.

**Newsletters that take submissions:** Postgres Weekly, Kubernetes Weekly, Cloud Native Weekly (check each site for the current submission form).

## First month

- Answer every issue within a day or two, even if only to say you saw it.
- Ask the first few users for their `docs/REAL-DB-TEST.md` table.
- Publish a short release for every real bug fixed, with a CHANGELOG line.
- Write down what people ask for. If three people ask for the same thing (MySQL, other storage, encryption), that is the next feature. If nobody asks, do not build it.
- Track: issues from people who ran it on a real database, not stars.

## If the goal is more than a portfolio project

Open-source operators are hard to turn into income directly. The realistic routes are consulting for teams that want their backups verified, or a hosted service that checks backups for teams that do not want to run an operator. Decide which of these (if either) you want before spending a lot of time on promotion.
