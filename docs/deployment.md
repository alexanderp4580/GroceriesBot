# Coolify deployment

Coolify builds the repository's pinned CGO Dockerfile through `/deploy/compose.yaml`. Use the Personal Server project, production environment and existing GitHub App. Disable automatic deployment until a new build has passed verification.

The app has no public domain or published port. Telegram polling uses outbound HTTPS; the health endpoint listens on container localhost port 8080. Compose sets one replica, UID 10001, read-only root, 128 MiB memory, 0.5 CPU, dropped capabilities and an 8 MiB `/tmp` filesystem. Go uses `GOMEMLIMIT=64MiB` and `GOMAXPROCS=1`.

Set `TELEGRAM_BOT_TOKEN` and `OPENROUTER_API_KEY` as runtime variables, never build arguments. Set `RUN_MODE=paused` during setup and `active` at cutover. Google credentials are migration-only and do not enter the deployed app. Production AI remains disabled in SQLite until the user authorizes credits.

## Persistence

Mount the named volume `groceriesbot-data` at `/data`. Verify the actual volume name after Coolify resolves Compose; retain it across redeployments. It contains the database, WAL, SHM and exclusive instance lock. Keep production and synthetic verification volumes separate. The image initializes `/data` for UID/GID 10001; existing volumes retain their ownership.

Coolify handles backups. This project adds no backup process or schedule. Backup coverage remains the user's Coolify configuration.

The bot acquires a nonblocking lifetime lock before opening SQLite or contacting Telegram. Stop the old instance before replacing it; the lock also prevents overlapping consumers. A paused instance checks storage and serves health without provider calls.

## Cutover

1. Pass the reviewed-build verification and synthetic container persistence checks.
2. Deploy paused through Coolify; verify limits, storage and health.
3. Unpublish the n8n grocery workflow. Stop its service after checking that queued/running grocery executions have finished. Preserve its DB/volumes and inactive Dutch workflow for rollback.
4. Fetch a final Sheets snapshot with `scripts/fetch_sheets.py`; import into a fresh DB with `scripts/import_sheets.py`. Require zero import issues and verify raw snapshot equality, integrity and row counts.
5. Stop the replacement; transfer the closed/checkpointed DB over SSH into its volume, owner 10001 and mode 0600. Keep real rows outside Git and image layers.
6. Remove the Telegram webhook with `drop_pending_updates=false` using the CLI. Set `RUN_MODE=active`, then deploy one instance through Coolify.
7. Check health, pending list rendering and outbox delivery. Record a ten-minute host resource measurement and a restart/persistence check.

Exact commands and buttons can be tested with AI disabled. Natural-language, image, and model shelf-placement validation require the recorded scenarios and separate credit authorization in [llm-scenarios.md](llm-scenarios.md). Do not describe fixture outcomes as live Haiku results.

## Operations

```sh
# Runs with the local tokencrypt grant and prints metadata, not secrets:
tokencrypt run --with coolify_api_root_token -- python3 scripts/coolify.py status --uuid APP_UUID
# Run inside the deployed container:
/app/groceriesbot doctor
/app/groceriesbot settings get llm.enabled
/app/groceriesbot settings list
```

`scripts/server` uses TokenCrypt's existing SSH passphrase grant. `scripts/coolify.py` provisions the app, uploads runtime secrets from the private `.env`, and requests deployment. It prints filtered metadata; HTTP error details stay in `.local/` for private inspection. Do not dump generated Compose files or container environments.

On rollback, stop the Go app, reconcile post-cutover grocery changes, restore n8n service and publish its original workflow version. Do not restart the old bot concurrently with polling.
