# GroceriesBot design

Updated 2026-10-12. The user chose SQLite for groceries, LLM memory, settings, and delivery state. This replaces the initial MySQL proposal.

## Goal

Run the existing Telegram grocery assistant with one Go process and one local SQLite file on Coolify. Reduce the server cost of the n8n main process, worker, runners, PostgreSQL, and Redis. The inspected stack used about 514 MiB in one snapshot; that figure is a baseline, not a peak measurement.

Preserve Telegram, OpenRouter, photos, variant suggestions, reply context, grocery lists, edits, purchase completion, deletion, clearing, and approximate price estimates. Use `anthropic/claude-haiku-5.5` as the model, selected by the user on 2026-10-12. Keep model selection in SQLite settings. The legacy n8n workflow uses Sonnet 4.5.

## Runtime

Use Telegram long polling. Persist incoming updates before advancing the offset; Telegram supports polling once the bot's webhook has been removed. See the [Telegram API](https://core.telegram.org/bots/api#getupdates).

Run one polling loop, one ordered update processor, and one outbox loop in the same process. Use one SQLite connection and short transactions. Hold an exclusive OS lock at `/data/groceriesbot.lock` for the process lifetime so a second container cannot consume the same bot or database during redeployment. A second instance exits before it contacts Telegram.

Call OpenRouter through a bounded tool loop. Application code checks each tool argument and performs the database mutations. Replace the purchase-completion sub-agent with a SQL update. Reuse HTTP connections. Store accepted model responses before executing their tool calls, then persist each mutation and result in one transaction. On restart, resume the saved response and return saved results for completed actions.

Use the same SQLite file for durable chat history, clarification state, settings, updates, model turns, and replies. Load only the context needed for the current request. Photos stay in bounded memory buffers during processing.

## Resource budget

These are targets to verify after implementing the Go runtime:

| Measure | Target or configured limit |
| --- | --- |
| Application RSS while idle | At most 40 MiB after warmup |
| Application RSS during fixture workload | At most 96 MiB |
| Container memory limit | 128 MiB |
| Go memory target | `GOMEMLIMIT=64MiB` |
| CPU limit | 0.5 CPU; `GOMAXPROCS=1` |
| Idle CPU | Below 0.5% of one core over ten minutes |
| SQLite page cache | About 2 MiB per connection |
| SQLite connections | One |
| Message context | Latest 20 messages, at most 12,000 characters |
| Grocery context | At most 200 pending items and 20 history suggestions |
| Tool rounds | Six per update |
| Model output | 800 tokens per request |
| Model request timeout | 30 seconds |
| Total update processing deadline | 120 seconds |
| Photo download | At most 4 MiB |
| Telegram polling timeout | 50 seconds |

Use SQLite WAL, `synchronous=FULL`, `busy_timeout=5000`, `cache_size=-2048`, `mmap_size=0`, and `wal_autocheckpoint=256`. Read the limits from the database where they describe user behavior; keep container memory and CPU limits in deployment configuration. Use indexed queries and pagination for full-list operations rather than loading an unbounded history into an LLM prompt. SQLite documents WAL and cache behavior in its [WAL guide](https://www.sqlite.org/wal.html) and [pragma reference](https://www.sqlite.org/pragma.html).

Prune message history to 20 messages per chat and a maximum age of 30 days. Prune completed updates, model turns, and sent replies after 30 days; preserve records with pending work. Keep mutation results while their update remains. Trim large completed model payloads after delivery. Run pruning on startup and hourly, not in a busy loop. Coolify handles backups.

## Grocery behavior

Generate stable database IDs and retain legacy sheet IDs as import metadata. Add pending groceries with a quantity of one unless the user supplies a quantity. Scope clarifications by chat and user; quantities belong to the current item.

Use history to suggest variants. Confirm items inferred from a photo or broad ingredient request before inserting them. Preserve captions and Telegram reply context. Choose the largest available photo size within the download limit; handle one-photo arrays without indexing a nonexistent third entry.

List pending groceries in the category order stored in settings. Normalize the legacy spelling `Vegtables` to `Vegetables` for display while preserving the source text. Put unknown categories after known ones. Render escaped HTML and bounded message chunks with concise action summaries.

Allow edits to pending rows. Application code owns timestamps and cannot change an item's original added date during an edit. Mark selected pending IDs or all pending groceries purchased using the current time. Retain purchased history for suggestions. Delete by database ID and soft-delete rows so historical mutations remain traceable.

Clear the full list, including purchased history, only after the initiating user confirms in the same chat. Confirmation expires after five minutes and becomes invalid after an intervening grocery mutation. The model cannot supply proof of confirmation. Increment the chat's list revision in the same transaction as each mutation.

Price estimates use euros and Netherlands prices. Label them as estimates and keep them read-only. Store timestamps as UTC text and present them in Europe/Amsterdam.

## Settings and secrets

Store editable behavior settings as JSON values in `settings`. Seed defaults on an empty database, validate updates, and preserve existing values across restarts and imports. Use a CLI for settings in the first version; keep settings administration outside the LLM tool set.

Keep `TELEGRAM_BOT_TOKEN` and `OPENROUTER_API_KEY` in `.env` for local work and Coolify runtime variables in production. Load the optional local `.env` without executing shell syntax; process environment values take precedence. Keep `DATABASE_PATH` and `RUN_MODE` as startup variables. Only `RUN_MODE=active` starts Telegram polling. `paused` permits database checks and health endpoints.

The imported database restricts access to the current Telegram group. User restrictions default to an empty list, matching the active workflow. Runtime authorization comes from SQLite settings. `ALLOWED_CHAT_IDS` seeds a new empty database only; reject active startup with an empty allowlist.

## Failure handling

Deduplicate update IDs, persist each accepted model response, and identify tool actions by update, round, and action index. Commit a grocery mutation together with its saved tool result. On replay, check the saved arguments before returning that result. Resume ordered work after a crash without repeating a committed mutation.

Persist final Telegram messages in the outbox before marking processing complete. Honor rate limits and retry transient network errors with bounded backoff. Telegram may accept a message before a timeout reaches the caller; a retry can duplicate a reply. The application must preserve grocery state despite that ambiguity.

Stop after the round limit or processing deadline. Queue a failure reply that reports any actions already committed. Reject malformed tool output without changing grocery rows. Log update IDs, durations, and error codes, excluding credentials and raw conversations.

## Coolify and cutover

Use one container with the full `/data` directory on persistent local storage. Keep the SQLite database, WAL, SHM, and instance lock on that mount. Deploy with the compose contract and stop the old instance before starting the replacement. The lock remains the overlap guard.

The user manages backups through Coolify. Confirm that Coolify's configured backup includes the data volume. Do not add backup containers or schedules.

The imported database is a snapshot while n8n remains active. Before switching Telegram, stop the grocery workflow, fetch a final read-only sheet snapshot into a fresh database, verify it, and install that database on the volume. Remove the webhook without dropping queued updates, then start the replacement in active mode. Retain the old workflow for rollback and reconcile post-cutover changes before restoring it. Preserve the inactive Dutch workflow when retiring the old stack.

## Acceptance

Pass the conversation, authorization, replay, import, and resource checks in the implementation plan. Verify that a container recreation preserves groceries, memory, and settings, and that a second instance fails before polling. Measure idle and workload resource use on the Coolify host. Report those measurements against the baseline rather than assuming the replacement meets its targets.
