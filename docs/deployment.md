# Coolify deployment

Use [deploy/compose.yaml](../deploy/compose.yaml) once the Go runtime has a tested image. No service has been deployed for GroceriesBot yet.

## Resource configuration

Create a Docker Compose application in the existing Personal Server project. Use the repository root as the base directory and `deploy/compose.yaml` as the Compose location. Coolify supports repository-based Compose deployments and runtime variable references; see its [Compose documentation](https://coolify.io/docs/applications/builds/docker-compose).

Set these values in Coolify:

| Variable | Value |
| --- | --- |
| `GROCERIESBOT_IMAGE` | Tested image reference, pinned to a digest for production |
| `TELEGRAM_BOT_TOKEN` | Recovered token from the private local `.env` |
| `OPENROUTER_API_KEY` | Recovered key from `.env` |
| `RUN_MODE` | `paused` during setup, `active` at cutover |

The Compose file fixes `DATABASE_PATH=/data/groceriesbot.sqlite3`, `GOMEMLIMIT=64MiB`, and `GOMAXPROCS=1`. It limits the container to 128 MiB and 0.5 CPU, drops Linux capabilities, and uses a read-only root filesystem with an 8 MiB temporary filesystem.

Set credentials as runtime variables. They do not belong in build arguments or the image. The production bot does not need the Google OAuth credentials. Keep the service without a public domain or published port; polling uses outbound HTTPS. Its health endpoint stays on internal port 8080.

## Persistent volume

The Compose contract declares a named volume, `groceriesbot-data`, mounted at `/data`. Keep that name stable when redeploying. Coolify documents volume and directory mounts in its [persistent storage guide](https://coolify.io/docs/core/persistent-storage/storage-mounts/overview).

The Go image must create `/data` owned by UID/GID `10001:10001` before declaring or using the volume. The Compose service runs with that identity. Docker initializes a new empty named volume from the image directory; an existing volume retains its existing ownership. Check write access before starting the bot. If the volume has the wrong owner, stop the app and correct the owner on the volume to `10001:10001`.

Keep these files on the mount:

```text
/data/groceriesbot.sqlite3
/data/groceriesbot.sqlite3-wal
/data/groceriesbot.sqlite3-shm
/data/groceriesbot.lock
```

Mount the directory, rather than the SQLite file. Keep production and test volumes distinct. Run one replica. Stop the old container before starting the new one, and implement the exclusive lock in the Go runtime to catch accidental overlap.

Coolify handles backups. Confirm that its configured backup covers `groceriesbot-data`; this project adds no backup service or schedule.

## Initial database transfer

The local import is a snapshot. Before production cutover, stop the n8n grocery workflow and fetch the final sheet state into a new empty SQLite database. Verify row counts and import issues before using that file.

Close the importer and other database connections before transferring its database. Confirm that SQLite has checkpointed the WAL; use `PRAGMA wal_checkpoint(TRUNCATE)` while the source application is stopped if a WAL remains. Then transfer the verified database to the production volume over SSH while GroceriesBot is stopped. Preserve mode `0600` and owner `10001:10001`. Do not place real database data in an image layer or Git commit.

Start the new container in paused mode. Check schema, integrity, writable storage, and settings. Paused mode must not poll Telegram or change webhook settings. Test restart persistence before cutover: insert a synthetic record in a separate test database, recreate the container with the same test volume, and confirm that the record, settings, and memory remain. Keep the test volume separate from production.

## Image contract

Task 5 in [implementation.md](implementation.md) must produce a Go image that:

- Includes `/app/groceriesbot`, CA certificates, and timezone data.
- Creates `/data` with the required owner and runs as UID/GID 10001.
- Embeds SQL migrations and keeps the process working directory at `/app`.
- Implements `groceriesbot healthcheck` as a bounded request to its internal liveness endpoint.
- Opens the database, applies migrations, checks write access, and acquires the process lock before polling.
- Handles SIGTERM, finishes or persists current work, and closes SQLite within the 30-second stop grace period.

Build SQLite's CGO driver into the binary and use a runtime image compatible with its C library. Do not deploy the source tree through a generic buildpack that guesses the start command or omits CGO.

## Production cutover

Record current Telegram webhook settings. Stop the n8n grocery workflow, take and verify the final import, and install the database on the stopped replacement's volume. Remove the webhook with `drop_pending_updates=false`. Only then set `RUN_MODE=active` and start the single Go instance. Telegram does not allow polling while a webhook remains configured; see [getUpdates](https://core.telegram.org/bots/api#getupdates).

Check a pending list, an addition, a clarification, purchase completion, and a restart with the same volume. Measure idle and workload memory/CPU against [design.md](design.md). Keep the old workflow recoverable through the acceptance period. A rollback must reconcile post-cutover changes before restoring n8n delivery.

Retire the grocery stack after acceptance. Preserve the inactive Dutch workflow and inspect dependencies before stopping shared services. Coolify's own database and Redis belong to Coolify, not this replacement.
