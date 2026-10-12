# Implementation

One Go executable runs Telegram polling, ordered update processing and outbox delivery. SQLite owns groceries, memory, settings, inbox, accepted model responses, action results and replies. Python handles the one-time Sheets migration; the deployed image contains no Python.

## Packages

| Package | Responsibility |
| --- | --- |
| `cmd/groceriesbot` | Startup, CLI, signals and health server |
| `internal/config` | Optional `.env`, process variables and typed settings |
| `internal/grocery` | Items, action keys and transport-neutral messages |
| `internal/sqlite` | Embedded migrations, bounded reads, transactions, replay and pruning |
| `internal/telegram` | Polling, bounded photo downloads and plain-text delivery |
| `internal/openrouter` | Bounded HTTP requests, tool responses and usage |
| `internal/bot` | Commands, signed buttons, confirmations, model loop and list ordering |
| `internal/runtime` | Exclusive instance lock and health endpoints |
| `migrations` | Shared import defaults and embedded versioned SQL |

Providers and persistence do not depend on the bot. The processor owns the provider interfaces. Tests use real temporary SQLite databases and fixture providers.

## Processing

1. Persist incoming updates and the next polling offset in one transaction.
2. Process updates in intake order. Check chat/user authorization before actions or model calls.
3. Handle exact commands and HMAC-signed buttons without a conversational model request.
4. For AI requests, check `llm.enabled` and reserve budget before contacting the provider. Save an accepted response before executing its tools.
5. Commit each mutation, revision increment, undo journal and result together. Replay keys contain update, round and action index; replay rejects changed arguments.
6. Commit final replies and bounded conversation memory with update completion. Deliver the outbox separately.

A Telegram timeout can duplicate a reply if Telegram accepted the first send. It cannot repeat the associated grocery mutation. Uncertain model requests retain conservative budget reservations, visible through `/usage`.

The processor allows six tool rounds, a 30-second request timeout and a 120-second update deadline. It caps responses at 256 KiB, downloads at 4 MiB and history at 20 messages / 12,000 characters. It reads at most 200 pending items into model context. Shopping lists paginate ten items per message; pending lists support up to 1,000 items.

## Grocery actions

Commands support additions, selected/all purchase completion, removal, undo, clearing, saved templates and approved aliases. Pending edits preserve added dates. Undo compares row versions and refuses to overwrite subsequent edits.

Clear and batch/photo additions require an initiating-user confirmation in the same chat. Confirmations expire and become invalid when the grocery revision changes. The model cannot produce a confirmation reference. Clarification buttons carry a signed user binding; another group member cannot answer them.

Haiku receives the existing shop route as guidance and returns existing item IDs in shelf order, including cross-category adjacency. Go validates the complete ID set and preserves that order in text and buttons. SQLite caches it by revision and route/model fingerprint. Failed or disabled ordering produces a complete list with a visible fallback notice.

The selected model is `anthropic/claude-haiku-5.5`, medium effort, with 4,096 total output tokens. This replaces the proposed 800-token limit so reasoning and tool arguments have room. Production starts with `llm.enabled=false` per the user's credit restriction. Fixture tests exercise AI behavior without OpenRouter requests.

## Storage and deployment

Migrations 002 and 003 upgrade the imported version-1 DB. The runtime validates migration checksums and rejects newer schemas. It keeps one connection, WAL, full synchronous writes, a five-second busy timeout, 2 MiB page cache and no mmap.

The container runs as UID 10001, with a read-only root, 128 MiB memory limit and 0.5 CPU. Coolify mounts the complete `/data` directory. Groceries, memory, settings, WAL, SHM and the exclusive process lock share that volume. A second instance exits before contacting Telegram. Coolify handles backups.

`RUN_MODE=paused` serves health without provider calls. Active startup requires a chat allowlist, token and absent webhook. Cutover explicitly stops the old workflow, fetches the final Sheets snapshot, installs the verified DB, removes the webhook without dropping updates and starts one polling instance. Preserve the original n8n workflow for rollback.

## Verification

Run `python3 scripts/verify.py --record .local/verification/RECORD --containers` once for the reviewed build. Each step saves its full output plus exit status. The script refuses to overwrite an existing record. It runs Python migration tests, Go race tests, vet, compilation, image build and a network-disabled container persistence/lock check.

Use `scripts/measure_resources.py` on the Coolify host for ten-minute idle measurements. Record live deployment checks separately; do not infer production readiness from fixture tests. See `docs/verification-report.md` for actual results once verification finishes.

Actual model behavior requires the opt-in recorder and scenario review in [llm-scenarios.md](llm-scenarios.md). Fixture checks do not verify the selected live model.
