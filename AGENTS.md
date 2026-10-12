# GroceriesBot

Read `docs/design.md` and `docs/implementation.md` before changing the runtime.

The user chose Go and SQLite to reduce server resource use. SQLite stores groceries, conversation memory, settings, and delivery state. Keep one application process and one database connection unless measured behavior requires a change.

Keep `.env`, `data/`, `.local/`, credentials, and real sheet rows out of Git and container build contexts. Use synthetic fixtures in tests. Do not print secret values. Coolify handles backups; do not add a backup service or scheduler.

Coolify must mount the full `/data` directory. Keep the DB, WAL, SHM, and process lock there. Run one replica and prevent overlap during redeploys. Production Telegram switching is a separate action from project setup.

The user authorized implementation, GitHub push, Coolify deployment and cutover. Production AI must stay disabled until the user authorizes OpenRouter spend. Use fixture providers for verification; do not perform live model probes. Keep the old n8n workflow recoverable.

Use the stop-slop skill for prose. Verify changes before claiming success. Migration checks: `python3 -m unittest discover -s tests -v`. Add Go checks as runtime packages become available.
