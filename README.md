# GroceriesBot

Port the existing n8n Groceries Bot to a simpler Go service backed by MySQL.

Project location: `/home/deck/SketchBook/Projects/GroceriesBot`.

## Scope

- Preserve the Telegram grocery assistant and its useful existing behavior.
- Replace n8n orchestration with Go.
- Replace Google Sheets storage with MySQL, including migration of existing grocery data.
- Inspect the existing Coolify deployment before deciding deployment and cutover details.
- Use TokenCrypt for credential access; never commit credentials or raw credential-bearing workflow exports.

## Status

Project created on 2026-10-12. Coolify and the active n8n workflow have been inspected. The migration design is ready for review; application implementation and data migration have not started.

See the [proposed design](docs/design.md), [discovery](docs/discovery.md), and [sanitized active workflow snapshot](docs/workflow-snapshot.json).
