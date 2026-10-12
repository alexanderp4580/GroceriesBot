# GroceriesBot Implementation Plan

Read [architecture-review.md](architecture-review.md) before execution. It proposes corrections to package dependencies, token budgeting, import validation, and recovery. Those proposals have not yet changed this plan's interfaces or runtime settings.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans for native execution, or superpowers:subagent-driven-development if the user selects delegated execution. Track steps with the checkboxes below.

**Goal:** Replace the active n8n groceries workflow with a Go Telegram bot that stores groceries, LLM memory, settings, and delivery state in SQLite and uses less host memory and idle CPU.

**Architecture:** One Go process runs Telegram polling, ordered update processing, and reply delivery. One SQLite connection owns persistent state. The model proposes typed actions; application code validates and commits them.

**Tech Stack:** Go 1.27.2, standard-library HTTP and JSON, SQLite through `github.com/mattn/go-sqlite3 v1.14.52`, `github.com/joho/godotenv v1.5.1` for local configuration, and `golang.org/x/sys v0.49.0` for Linux file locking. These versions came from the Go module proxy during project setup. Pin dependencies in `go.mod` and `go.sum` when adding the Go packages.

**Spec:** [design.md](design.md). Also read [database.md](database.md) and [deployment.md](deployment.md).

**Selected model:** `anthropic/claude-haiku-5.5` with `llm.reasoning_effort="medium"`, stored in SQLite settings. The user selected it on 2026-10-12 to test its value for this workflow.
Read [model-selection.md](model-selection.md) before configuring reasoning and sampling parameters. Verify usable tool responses within the 800-token output limit.

## Global constraints

- One process, one replica, one SQLite connection. Hold the instance lock before contacting Telegram.
- Production DB path: `/data/groceriesbot.sqlite3`; mount the full `/data` directory.
- Container memory: 128 MiB; CPU: 0.5; `GOMEMLIMIT=64MiB`; `GOMAXPROCS=1`.
- WAL, full synchronous writes, 5,000 ms busy timeout, about 2 MiB cache, no mmap, 256-page automatic checkpoint.
- Six tool rounds, 800 output tokens, 30-second model timeout, 120-second processing deadline.
- Twenty retained messages per chat, at most 12,000 context characters, 30-day age limit.
- Photos: at most 4 MiB. Reuse outbound HTTP transports and cap response bodies.
- Secrets stay in process environment or the private local `.env`. Editable behavior settings stay in SQLite.
- `RUN_MODE=paused` permits local checks and health endpoints; `active` starts polling after startup validation.
- Coolify handles backups. Add no backup service or scheduler.
- Keep real rows, credentials, `.local/`, and `data/` outside Git and the image.
- The runtime retains the observed Telegram behavior and protects timestamp, confirmation, and authorization rules in code.

## Review focus

- A group member must not complete another member's clarification or clear confirmation. Task 3 tests user binding.
- A crash after a model response or mutation must resume without adding duplicate groceries. Task 4 tests each persistence boundary.
- A malformed or oversized image must fail within its byte limit. Task 4 tests missing photo sizes and bounded streaming.
- A redeployment may overlap containers despite a one-replica setting. Task 5 tests the process lock with two processes.
- History, retries, and list output can grow while a bot idles. Tasks 2, 4, and 6 test pagination, pruning, and resource ceilings.

## File layout

| Path | Responsibility |
| --- | --- |
| `cmd/groceriesbot/main.go` | Command dispatch and runtime lifecycle |
| `internal/config/config.go` | Environment and optional `.env`; startup validation |
| `internal/grocery/types.go` | Items, operations, results and action keys |
| `internal/grocery/service.go` | Input rules, authorization, confirmation and typed tools |
| `internal/store/sqlite.go` | Connection setup, embedded migrations, lock-free DB lifecycle |
| `internal/store/groceries.go` | Indexed reads and atomic grocery actions |
| `internal/store/state.go` | Settings, memory, confirmations, inbox, model turns and outbox |
| `internal/telegram/client.go` | Polling, downloads, sends and rate limits |
| `internal/telegram/render.go` | Escaped HTML, keyboards and bounded list chunks |
| `internal/openrouter/client.go` | Requests, typed model responses and body limits |
| `internal/bot/processor.go` | Durable processing and replay of saved turns |
| `internal/bot/runtime.go` | Poller, processor, outbox and hourly pruning |
| `internal/runtime/lock_linux.go` | Lifetime advisory file lock |
| `internal/runtime/health.go` | Liveness, readiness and CLI probe |
| `migrations/embed.go` | SQL migration embedding alongside the existing schema |
| `Dockerfile`, `toolchain/run`, `toolchain/versions.env` | Pinned CGO build and local container toolchain |
| `deploy/compose.yaml` | Coolify limits, credentials and persistent volume |

Use colocated `_test.go` files and synthetic fixtures under `testdata/`. Avoid frameworks and provider SDKs for endpoints covered by standard HTTP requests.

### Task 1: Prepare the imported database and project

**Files:** Existing SQL, Python import helper and tests, `.env`, `.env.example`, docs, `go.mod`, and Compose contract.

**Interfaces:** The resulting database implements schema version 1. It seeds settings without overwriting existing values. The runtime must open this database rather than create a parallel store.

- [x] Inspect the active workflow and record its behavior.
- [x] Recover the required credentials into a protected `.env`.
- [x] Fetch the full referenced spreadsheet through a read-only request.
- [x] Import its source snapshot and 29 groceries into SQLite.
- [x] Seed settings and create the memory and processing tables.
- [x] Verify source equality, integrity, relationships, import replay, and rejected-row preservation.
- [x] Write the design, deployment, database, migration, and implementation docs.

Current result: six pending groceries, 23 purchased, zero import issues. The Python helper is a migration tool, not a deployed dependency.

### Task 2: Implement configuration, SQLite access, and settings CLI

**Files:** `internal/config/config.go`, `internal/store/sqlite.go`, `internal/store/state.go`, `migrations/embed.go`, `cmd/groceriesbot/main.go`, and their tests.

**Interfaces:**

- `config.Load(envFile string) (config.Config, error)` returns DB path, run mode, and provider keys. Process variables take precedence over `.env`.
- `store.Open(ctx context.Context, path string) (*store.Store, error)` applies migrations and initializes one connection.
- `(*Store).Close() error`, `(*Store).Check(ctx context.Context) error`, `(*Store).Settings(ctx context.Context) (map[string]json.RawMessage, error)`, and `(*Store).SetSetting(ctx context.Context, key string, value json.RawMessage) error` support the CLI and later runtime.
- `(*Store).RecentMessages(ctx context.Context, chatID int64, limit int) ([]store.Message, error)` returns ordered context. `store.Message` contains ID, chat/user IDs, role, content, update ID and creation time.
- `(*Store).Prune(ctx context.Context, now time.Time) error` enforces retention without deleting pending work.

- [ ] Write tests that open a copy of the imported schema, apply migrations twice, preserve changed settings, reject an invalid setting type, and read at most 20 recent messages. Test environment precedence and treat shell syntax in `.env` as data.
- [ ] Run `go test ./internal/config ./internal/store`; confirm the missing behavior fails.
- [ ] Implement the interfaces and `doctor`, `settings list`, `settings get KEY`, and `settings set KEY JSON_VALUE`. Run migrations transactionally and reject schemas newer than the executable supports.
- [ ] Test that read queries use bounded results and pruning retains pending updates, model turns, actions, and replies while removing eligible completed records in foreign-key order.
- [ ] Run the package tests and `go test ./...`, then commit the configuration and store work.

Example store assertion after seeding 30 messages and reading with limit 20:

```go
if len(messages) != 20 || messages[0].Content != "message 11" {
    t.Fatalf("wrong retained context: %v", messages)
}
```

### Task 3: Implement grocery rules and atomic actions

**Files:** `internal/grocery/types.go`, `internal/grocery/service.go`, `internal/store/groceries.go`, and tests.

**Interfaces:**

- Define `grocery.Item` with ID, chat ID, name, category, quantity, added/purchase dates and row version.
- Define `grocery.ActionKey` with `UpdateID int64`, `RoundIndex int`, and `ActionIndex int`.
- Define `grocery.Operation` with kind, chat/user identity, target IDs, item fields, and confirmation reference. Keep authoritative timestamps outside model-supplied fields.
- Define `grocery.Result` with changed items and clarification/confirmation state.
- `(*Store).ListPending(ctx context.Context, chatID int64, limit, offset int) ([]grocery.Item, error)` and `(*Store).History(ctx context.Context, chatID int64, search string, limit int) ([]grocery.Item, error)` provide context.
- `(*Store).Apply(ctx context.Context, key grocery.ActionKey, op grocery.Operation, now time.Time) (grocery.Result, error)` commits validation-sensitive state, mutation, revision, and saved result together. Existing action keys return the saved result only if arguments match.
- `grocery.NewService(db *store.Store, clock func() time.Time) *grocery.Service` and `(*Service).Execute(ctx context.Context, key grocery.ActionKey, chatID, userID int64, toolName string, arguments json.RawMessage) (grocery.Result, error)` expose validated tools.

- [ ] Write tests for default quantity, quantity reset, pending-only edits, immutable added time, selected/all purchase completion, ID gaps, shop-route category order and read-only price estimates. Use the seeded route for both text and button rows; assert Bread precedes Dairy and alias `Vegtables` sorts with Vegetables.
- [ ] Add tests for unauthorized chats, mixed-chat target IDs, empty IDs, nonfinite/negative quantity, a second user's confirmation, expiry after 300 seconds, and confirmation invalidation after a mutation.
- [ ] Run `go test ./internal/grocery ./internal/store`; confirm the new cases fail.
- [ ] Implement typed validation and transactional actions. Persist photo/multi-item proposals as pending actions and require an explicit user response before adding them. Save state and result before returning success.
- [ ] Assert that applying the same action twice creates one item and that altered arguments under the same action key fail without a mutation. Test rollback on an injected database failure.
- [ ] Run package tests and `go test ./...`, then commit the grocery service.

### Task 4: Implement Telegram, OpenRouter, and restart recovery

**Files:** `internal/telegram/*`, `internal/openrouter/client.go`, `internal/bot/processor.go`, `internal/bot/runtime.go`, additions to `internal/store/state.go`, and tests.

**Interfaces:**

- `telegram.NewClient(token string, httpClient *http.Client) *telegram.Client` provides `Poll(ctx context.Context, offset int64, timeoutSeconds int) ([]telegram.Update, error)`, `Send(ctx context.Context, message telegram.Outgoing) (int64, error)`, and `DownloadPhoto(ctx context.Context, photos []telegram.Photo, maxBytes int64) ([]byte, string, error)`.
- `openrouter.NewClient(key string, httpClient *http.Client) *openrouter.Client` provides `Complete(ctx context.Context, request openrouter.Request) (openrouter.Response, error)`. Request contains model, messages, typed tool definitions and max output tokens; response contains assistant content and tool calls.
- `bot.NewProcessor(db *store.Store, service *grocery.Service, model *openrouter.Client, telegramClient *telegram.Client) *bot.Processor` and `(*Processor).Process(ctx context.Context, updateID int64) error` drive one persisted update.
- Store methods `AcceptUpdates`, `NextUpdate`, `LoadTurn`, `SaveTurn`, `CompleteUpdate`, `NextReply`, and `MarkReplySent` own inbox, saved turns and outbox transactions. Use the schema's unique IDs rather than an in-memory deduplication map.

- [ ] Write HTTP fixture tests for polling offsets, replies, captions, keyboard clarification, photo arrays with one entry, HTML escaping, rate limits, and downloads over 4 MiB. Test body limits even when a server omits `Content-Length`.
- [ ] Write processor tests for saved context after restart, six-round exhaustion, 30-second request timeout, 120-second processing deadline, malformed model calls, and an empty/unauthorized chat allowlist.
- [ ] Run `go test ./internal/telegram ./internal/openrouter ./internal/bot`; confirm the missing transport and recovery behavior fails.
- [ ] Implement reusable HTTP clients and the three loops. Persist accepted updates before advancing the poll offset. Save the accepted model response before applying its tools. Commit final outbox messages and update completion together.
- [ ] Test crashes after intake, saved model response, committed mutation, and queued reply. Reopen the same database after each crash and assert that the grocery count and purchase timestamps remain correct.
- [ ] Model an uncertain Telegram send outcome and verify that any duplicate reply cannot repeat the mutation. Test ordered pending work, bounded retries, and restart-safe pruning.
- [ ] Run `go test ./...` and `go test -race ./...`, then commit transport and orchestration.

Use fixture providers for automated tests. Send live test messages only through a separate test bot and chat during an authorized smoke test.

### Task 5: Build the Coolify image and persistent runtime

**Files:** `internal/runtime/lock_linux.go`, `internal/runtime/health.go`, `cmd/groceriesbot/main.go`, `Dockerfile`, `toolchain/run`, `toolchain/versions.env`, and the existing Compose file.

**Interfaces:**

- `runtime.AcquireLock(path string) (io.Closer, error)` acquires a nonblocking exclusive Linux advisory lock. Hold its descriptor until shutdown; do not delete/recreate the lock inode during process lifetime.
- `runtime.HealthHandler(storeCheck func(context.Context) error, ready func() bool) http.Handler` serves `/healthz` and `/readyz` on port 8080. Liveness reports process/storage health; readiness reports active polling readiness. Paused mode has healthy liveness and false readiness.
- `groceriesbot healthcheck` probes local `/healthz` with a five-second bound and returns a nonzero exit code on failure.

- [ ] Write subprocess tests proving that a second process cannot acquire the same lock and can acquire it after the first exits. Test paused mode makes no provider requests, failed storage prevents active startup, and SIGTERM preserves pending work.
- [ ] Run `go test ./internal/runtime`; confirm the lifecycle cases fail.
- [ ] Implement startup: load environment, acquire lock, open/check DB, load/validate settings, start health server, then start loops only in active mode. Use a 30-second shutdown bound.
- [ ] Add a pinned multi-stage CGO build. Use a Go 1.27.2 builder with GCC and a compatible minimal runtime with CA certificates/timezone data. Record builder and runtime image digests in `toolchain/versions.env`. Create `/data` owned by `10001:10001`; copy the binary to `/app/groceriesbot` and run as that user.
- [ ] Build the image and run it with a temporary named volume using the Compose limits. Verify the read-only root, writable `/data`, ownership, health command, and credentials excluded from image layers.
- [ ] Create synthetic groceries, memory, and a changed setting in a test volume. Recreate the container with that same volume and assert all three survive. Attempt a second instance against the volume and assert it exits before a provider request.
- [ ] Validate the resolved Compose file with placeholder credentials without printing real values. Run Go tests and container checks, then commit the image setup.

The Compose file needs no public domain. Keep the production volume name stable and use a distinct volume for staging. Coolify owns backup configuration.

### Task 6: Measure resource use and prepare cutover

**Files:** `testdata/`, `scripts/measure_resources.py`, `docs/resource-results.md`, and updates to migration/deployment docs.

**Interfaces:** The measurement script accepts a container name and emits timestamped RSS, CPU and container status samples, excluding logs or environment. Store the source workload and sampling interval with the report.

- [ ] Run a ten-minute idle measurement after warmup, then a fixture workload covering text, clarification, lists, purchases, and a near-limit image. Confirm the 40 MiB idle and 96 MiB workload RSS targets and the 128 MiB hard container limit. Measure idle CPU against the 0.5% target.
- [ ] Exercise long history and repeated updates to show that memory, table retention and retries remain bounded. Fix any measured limit failures and rerun the relevant workload.
- [ ] Record measured results against the 514 MiB n8n snapshot, including test conditions and limits on the comparison.
- [ ] Verify a separate test bot's behavior and container recreation before production switching.
- [ ] Prepare the final read-only sheet import into a new DB after the old workflow stops. Verify its counts, raw snapshot, integrity and relationships.
- [ ] Follow the cutover order in [deployment.md](deployment.md): stop old grocery delivery, install the final DB, remove the webhook without dropping queued updates, activate one replacement, and verify behavior.
- [ ] Keep the old workflow recoverable until acceptance. Reconcile replacement mutations before a rollback. Preserve the inactive Dutch workflow if retiring n8n.

The current request covers project preparation, data import, configuration, and these docs. The Go runtime, resource measurements, and production switch remain future work.
