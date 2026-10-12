# GroceriesBot

GroceriesBot will replace the n8n grocery assistant with one Go process and a SQLite database. Reducing server memory and background CPU use is the main reason for the migration.

## Current state

The project lives in `/home/deck/SketchBook/Projects/GroceriesBot`.

- Imported the complete spreadsheet referenced by the active n8n workflow: one tab, 29 groceries, six pending and 23 purchased.
- Created `data/groceriesbot.sqlite3` with the grocery data, full source snapshot, settings, and tables for LLM memory and delivery state.
- Recovered the bot's Telegram, OpenRouter, and Google OAuth credentials into `.env`, mode `0600`.
- Added the schema, tested import helper, implementation plan, and Coolify deployment contract.

The Go Telegram runtime still needs implementation. The existing n8n bot remains active. SQLite is the database for the replacement; the imported data represents a point-in-time copy until cutover.

## Files

| Path | Purpose |
| --- | --- |
| `data/groceriesbot.sqlite3` | Main database, including imported groceries and runtime tables |
| `.env` | Local credentials and startup configuration; excluded from Git |
| `.env.example` | Configuration names with empty credentials |
| `migrations/001_initial.sql` | Initial SQLite schema |
| `scripts/import_sheets.py` | One-time Sheets JSON importer; Python standard library only |
| `scripts/export_n8n.py` | Remote credential transfer helper; its stdout contains secrets |
| `deploy/compose.yaml` | Single-container Coolify deployment with a persistent `/data` volume |
| `go.mod` | Go module; runtime dependencies follow the implementation plan |

Keep the database, source data, and credentials outside Git and the image. The container needs only the Telegram token and OpenRouter key. Google credentials serve the import.

## Checks you can run now

```sh
python3 -m unittest discover -s tests -v
sqlite3 -readonly data/groceriesbot.sqlite3 'PRAGMA integrity_check;'
sqlite3 -readonly data/groceriesbot.sqlite3 'SELECT count(*), sum(purchased_at IS NULL), sum(purchased_at IS NOT NULL) FROM groceries WHERE deleted_at IS NULL;'
```

The database check should print `ok`; the counts from this import are `29|6|23`. Runtime implementation will add Go build and test commands.

The importer accepts a private Google Sheets API JSON snapshot:

```sh
python3 scripts/import_sheets.py .local/sheets-snapshot.json --chat-id=-5138495076
```

Repeating the same import preserves existing groceries and settings. A different snapshot requires a new empty database; the importer refuses to overwrite a working database.

## Coolify

Mount the named `groceriesbot-data` volume at `/data`. Set `DATABASE_PATH=/data/groceriesbot.sqlite3`, run one replica, and inject credentials through Coolify runtime variables. Start with `RUN_MODE=paused` while preparing the deployment. Coolify handles backups.

`deploy/compose.yaml` requires `GROCERIESBOT_IMAGE` to reference a tested image of the future Go runtime. It is a deployment contract, not a deployed service. See [deployment.md](docs/deployment.md) for volume ownership, resource limits, data transfer, and cutover checks.

## Documentation

- [Design](docs/design.md): runtime behavior and resource budget.
- [Implementation plan](docs/implementation.md): files, interfaces, tests, and work order.
- [Deployment](docs/deployment.md): Coolify configuration and persistent storage.
- [Database](docs/database.md): tables, settings, and SQLite connection rules.
- [Migration report](docs/migration.md): source coverage and verification.
- [Discovery](docs/discovery.md): server and active-workflow findings.
- [Earlier n8n assessment](docs/n8n-assessment.md), [historical workflow specification](docs/legacy-workflow-spec.md), and [sanitized active workflow](docs/workflow-snapshot.json).
