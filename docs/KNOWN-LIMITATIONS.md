# Known limitations (v0.1.0)

Found while testing on the databases in `docs/REAL-DB-TEST.md`. None of these lose data; they limit what a passing result proves or how clearly a failure is reported.

- **Ownership and grants are not verified.** The restore runs `pg_restore --no-owner --no-privileges` (DECISIONS.md), so a pass proves data, schemas, extensions, indexes and large objects restore, not roles, ownership or grants.
- **Wrong credentials are reported as unreachable.** A wrong password or S3 key gives the reasons `SourceUnreachable` / `StorageUnreachable` although the server answered.
- **Source error messages are cut at 120 characters**, which can remove the useful part (for example `password auth...`).
- **A failed backup still shows the last good verification.** `VERIFIED` stays `True`, and `history` entries for failed backups carry the earlier `verificationResult`. Check `lastBackup` and `BackupReady` as well.
- **`BackupReady` stays `False` after the cause is fixed** until the next scheduled backup (a manual re-verify checks the stored backup only).
- **The verify runner writes nothing to its log on success**, so restore warnings are not recorded.
- **A failed SQL check may show the actual value** if it is a boolean or a short number (ARCHITECTURE.md). Do not write checks that return sensitive numbers.
- **PostgreSQL 16 only.** The runner image has the 16 tools.
- **Tested on public sample databases and a synthetic 1 GB database**, not on production data.
