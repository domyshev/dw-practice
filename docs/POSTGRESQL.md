# PostgreSQL storage and one-time SQLite import

The normal API requires `DATABASE_URL` and PostgreSQL 17 or later. SQLite remains
available only for mock fixtures, fast regression tests and reading the old
migration snapshot. The API refuses to start without PostgreSQL or schema version 1.
Existing local SQLite journals are never opened or migrated automatically.

## Database roles and schema

Use a schema-owner role to initialize the database:

```sh
DATABASE_URL='postgres://schema-owner@database/dw_practice' go run ./backend --init-postgres
```

Run the API with a separate login role that has CONNECT, schema USAGE, DML on
users/codes/sessions/entries/create_requests, SELECT and INSERT on audit, SELECT
on schema_version and sequence USAGE. Do not grant schema creation, table
ownership, role administration or audit UPDATE/DELETE/TRUNCATE. An additional
trigger rejects audit changes even for the owner unless explicitly disabled.

The deployment provisions random passwords directly on the server. Connection
URLs and credentials must not be printed, copied into logs or committed.

## One-time production cutover

1. Disable the updater and hourly backup timers and wait for active jobs.
2. Stop only DW Practice. Use SQLite's backup API to make a fresh snapshot,
   including committed WAL writes; verify quick_check and foreign_key_check.
3. Start a dedicated PostgreSQL container on a private Docker network with no
   published host port. Preserve the old image, Compose configuration and SQLite files.
4. With the schema-owner URL, run the candidate image with
   `--import-sqlite /migration/source.db`, mounting only that snapshot read-only.
   The importer initializes the schema and copies all six tables in one transaction.
   It requires an empty destination and rejects a second import, even if tables
   were subsequently emptied. It compares row counts and SHA-256 fingerprints
   over every column of every table before committing and advances the OTP sequence.
5. Grant the runtime role, switch DW to its PostgreSQL URL, start the candidate
   image, and verify its commit, health, existing account/session and journal.
6. Replace SQLite-specific updater/backup scripts with their PostgreSQL versions.
   Verify a full dump restoration and a scheduled backup/report before re-enabling timers.

IDs, JSON bodies, timestamps, soft-deleted records, audit snapshots, idempotency
keys, OTP digests and session digests are copied without transformation. Keep
SESSION_SECRET unchanged to preserve sessions and code verification.

If cutover verification fails before reopening the application, stop the
candidate and restore the previous Compose configuration/image pointing at the
preserved SQLite database. After PostgreSQL accepts real writes, preserve a
PostgreSQL dump before any rollback; the original SQLite snapshot no longer
contains the newest changes.

## Backups after cutover

Use custom-format pg_dump archives with root-only permissions. A successful
backup must restore fully into a separate temporary database, validate schema
version and audit protection, and record restored table counts. Keep historical
SQLite backups separately. Scheduled PostgreSQL copies retain hourly/daily/weekly
recovery points; the existing weekly report reads only backup metadata.
