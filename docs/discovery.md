# Discovery

Inspection date: 2026-10-12.

## User requirements

Create GroceriesBot beside the other SketchBook projects. Replace n8n with Go to reduce resource use. The user first named MySQL, then chose SQLite as the main database for groceries, LLM memory, settings, and delivery state. Deploy on Coolify with persistent storage. Coolify handles backups. Use the stop-slop skill for project prose.

## Coolify host

Authenticated read-only calls to `https://coolify.tail.liveleds.io/api/v1/` found the Personal Server project and a healthy n8n service (`fwcmxt6ry1vt7p5scuhzwizj`). Main n8n and its worker use `n8nio/n8n:2.10.4`; runners use `n8nio/runners:2.10.4`. The service includes PostgreSQL 16 Alpine and Redis 6 Alpine.

A separate N8N Postgres DB uses PostgreSQL 18 Alpine. Its relationship to the bundled database remains unknown. Other services include an exited AI WebUI, CouchDB, and a Tailscale sidecar. The API inventory contained no MySQL service; the replacement does not need one.

SSH inspection succeeded through the known Tailscale host and port. The host had 3,819 MiB RAM, 1,688 MiB available, and 106 MiB swap used. Docker reported these memory figures for the groceries stack:

| Component | MiB |
| --- | ---: |
| Main n8n | 299.7 |
| Worker | 161.3 |
| Runners | 6.668 |
| PostgreSQL | 41.13 |
| Redis | 5.621 |
| Total | 514.419 |

This is a single snapshot. The replacement's resource figures still need measurement. The API version endpoint returned a non-JSON response, so this inspection did not establish the Coolify version. No Coolify resources changed.

## Active workflow

The active version of Groceries Bot (`SLffBV8YkETQcXcF`) has 22 nodes. [workflow-snapshot.json](workflow-snapshot.json) contains selected parameters and connections with credential and URL fields omitted. It is a discovery record, not a restorable export.

The workflow restricts input to one Telegram group and has no user restriction. Both agents use `anthropic/claude-sonnet-4.5`; the main agent allows 15 iterations. It supports adding, editing, purchase completion, deletion, clearing, variant suggestions, category lists, photo input, reply context, and price estimates.

The memory node uses `=static_memory`. Photo retrieval selects `message.photo[2]`. Delete instructions equate the sheet row number to ID plus one. Prompts govern clear confirmation, pending-only edits, purchase timestamps, and generated IDs. These are the rules the Go runtime must enforce in code.

The older [assessment](n8n-assessment.md) and [workflow specification](legacy-workflow-spec.md) provide historical context. The active workflow takes precedence where they differ.

## Credentials

n8n could decrypt each of its five stored credential records:

| Type | Count | Authentication fields present |
| --- | ---: | --- |
| OpenRouter | 1 | API key |
| Telegram | 2 | Bot token |
| Google Sheets OAuth2 | 1 | Access token, refresh token, client ID and secret |
| PostgreSQL | 1 | User and password |

GroceriesBot uses one OpenRouter record, one Telegram record, and the Sheets record. Its embedded Telegram token matches the referenced stored token. The second Telegram credential and PostgreSQL credential belong outside this runtime's configuration.

TokenCrypt supplied Coolify and SSH grants. n8n exported credentials under `/dev/shm` and removed the temporary export after transfer. The local migration saved the required values to `.env` with mode `0600`, then deleted the intermediate credential bundle. It did not print token values or commit secrets.

Google accepted a refresh-token exchange and the read-only spreadsheet request. This confirms Google access for the import; no Telegram or OpenRouter request tested their tokens.

## Imported data

The six Sheets tools reference one spreadsheet and the `Groceries` tab. The spreadsheet contains one tab, 1,005 grid rows, and six columns. The import captured its full API grid response, including cell values, formulas, notes, and formatting returned by Google.

The data contains one header and 29 grocery rows, with gaps in physical row numbers. All quantities are one. Twenty-three groceries have purchase timestamps; six remain pending. Google stores the dates as serial numbers and configures the spreadsheet timezone as Europe/Amsterdam.

See [migration.md](migration.md) for the SQLite import and verification results. The source sheet remains live under n8n until cutover.
