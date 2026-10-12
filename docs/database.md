# Database

SQLite is the main store for this project. The local database is `data/groceriesbot.sqlite3`; production uses `/data/groceriesbot.sqlite3` on Coolify's persistent volume.

The migration created schema version 1 from [001_initial.sql](../migrations/001_initial.sql). It imported 29 grocery records and seeded runtime settings. Memory and processing tables start empty; importing a sheet does not reconstruct previous n8n conversations.

## Tables

| Table | Contents |
| --- | --- |
| `groceries` | Stable ID, chat, legacy ID, item, category, quantity, UTC dates, deletion state and version |
| `settings` | Behavior settings as JSON, with update timestamps |
| `chat_state` | List revision per chat |
| `conversation_messages` | User, assistant and tool messages, scoped by chat |
| `pending_actions` | Clarifications and confirmations scoped by chat and user |
| `telegram_updates` | Durable input queue, unique update ID, attempts and state |
| `llm_turns` | Saved model requests and responses by update and round |
| `tool_actions` | Validated arguments and committed results by update, round and action |
| `outbox` | Pending and sent Telegram messages |
| `source_snapshots` | SHA-256 digest and zlib-compressed full Sheets API response |
| `source_sheets` | Spreadsheet and tab identity, locale, timezone and grid size |
| `source_rows` | Content-bearing rows with original positions and cell values |
| `import_issues` | Rows that need review before normalization |
| `schema_migrations` | Applied schema versions |

The full compressed snapshot preserves the returned grid response, including formatting-only rows. The relational source rows preserve headers, gaps, values, formulas and notes for inspection. `groceries.source_row_id` links each imported grocery to its source. Raw data stays in the source tables when normalization rejects a row.

An imported `legacy_id` is metadata. Use `groceries.id` for edits and deletion. The schema gives each item a generated ID and permits a nullable purchase timestamp. For new groceries, application code sets `added_at` and leaves `purchased_at` null.

## Connection rules

The Go runtime must configure its single connection with these values:

```sql
PRAGMA foreign_keys=ON;
PRAGMA journal_mode=WAL;
PRAGMA synchronous=FULL;
PRAGMA busy_timeout=5000;
PRAGMA cache_size=-2048;
PRAGMA mmap_size=0;
PRAGMA wal_autocheckpoint=256;
```

Use `SetMaxOpenConns(1)` and `SetMaxIdleConns(1)`. Apply connection-specific pragmas through the connection setup or driver DSN so they remain in force if Go replaces the connection. SQLite persists WAL mode; cache, timeout, and foreign-key settings need connection initialization. See the [SQLite pragma reference](https://www.sqlite.org/pragma.html).

Do not hold a transaction open while waiting for Telegram or OpenRouter. Commit a mutation and its tool result together; store the accepted model response before applying its tools. Use a separate transaction to queue the final replies and complete the update.

## Settings

The importer seeded the existing model, allowed Telegram group, category order, and resource bounds from [design.md](design.md). The Go runtime should expose these CLI operations:

```text
groceriesbot settings list
groceriesbot settings get KEY
groceriesbot settings set KEY JSON_VALUE
```

These commands are planned; the Go CLI does not exist yet. Validate keys and JSON types before saving. Preserve existing settings when applying defaults. Read settings at the start of each update so a CLI change takes effect without a restart.

Settings include model selection, tool-round and output limits, memory retention, context size, category order, polling timeout, image size, confirmation expiry, and chat/user authorization. Secrets belong in runtime environment variables. The database must not store API keys.

## Checks

```sh
sqlite3 -readonly data/groceriesbot.sqlite3 'PRAGMA integrity_check;'
sqlite3 -readonly data/groceriesbot.sqlite3 'PRAGMA foreign_key_check;'
sqlite3 -readonly data/groceriesbot.sqlite3 'SELECT key,value_json FROM settings ORDER BY key;'
```

The first query should print `ok`; the second should print no rows. Protect the full `/data` directory and keep it on a local filesystem. WAL uses companion files beside the database, as described in the [SQLite WAL guide](https://www.sqlite.org/wal.html).
