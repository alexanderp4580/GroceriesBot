# Architecture review and proposals

Reviewed 2026-10-12. Scope: repository, SQL schema, import helper, deployment contract, and planned Go runtime. The repository has no Go runtime source yet. These are proposed changes, not implemented features.

## Recommendation

Keep one Go process, SQLite, Telegram long polling, and Haiku 5.5 with medium effort. Correct the package dependencies, import validation, reasoning budget, and recovery details before implementing the runtime. Add deterministic commands and purchase buttons to the first version; schedule undo and usage reporting next.

The current five import tests pass. SQLite integrity checks pass and the database contains 29 groceries. Two additional synthetic probes exposed a validation gap; they did not touch the real database.

## Findings

### 1. The planned packages create an import cycle

[implementation.md](implementation.md) places `Service` in `internal/grocery`, makes it depend on `*store.Store`, and makes the store consume `grocery.Operation` and return `grocery.Result`. Implementing those signatures in those packages requires both packages to import each other. Go rejects package import cycles; see [import declarations in the Go specification](https://go.dev/ref/spec#Import_declarations).

Keep grocery types and pure rules independent of storage. Put workflows and small consumer interfaces in `internal/bot`. Let SQLite import grocery types, and wire concrete dependencies in the command. Neither SQLite nor the grocery package should import the bot package.

### 2. Medium reasoning needs a larger total output allowance

The project combines medium effort with an 800-token output cap. OpenRouter counts reasoning within the completion allowance. Its documented Anthropic budget mapping has a 1,024-token minimum reasoning allocation on applicable paths, exceeding the current total cap. Adaptive model/provider behavior needs a direct compatibility check; the current settings have not passed such a check. See [OpenRouter reasoning documentation](https://openrouter.ai/docs/guides/best-practices/reasoning-tokens).

Propose 4,096 total completion tokens as a starting ceiling, retaining medium effort and concise user replies. Test complete tool calls, finish reasons, provider errors, actual usage, and latency. A ceiling is not a target: the model can finish sooner. Tune the ceiling from measured requests instead of relying on the benchmark scores.

Keep the reply-length policy separate from the model's total reasoning/completion budget. Return a controlled error on truncated tool arguments; do not execute incomplete actions.

### 3. The importer can accept the wrong source

`scripts/import_sheets.py` recognizes grocery data only when a tab is named `Groceries`. If no tab matches, it archives data and returns a zero-grocery success report without an issue. An empty snapshot also returns success.

Reproduction results:

| Synthetic input | Current result |
| --- | --- |
| Empty snapshot array | Zero groceries, zero issues, no error |
| Grocery tab renamed, with its records intact | Zero groceries, zero issues, no error |

Validate expected spreadsheet/tab identities and required headers before importing. Accept a confirmed header-only grocery sheet as an empty list, but reject a missing expected tab, empty response, or duplicate required header. Preserve the full source snapshot after validation.

The helper currently calls `os.umask()` inside its library entry point and leaves that process-wide setting changed. Move the mask to the CLI boundary and use explicit permissions for files created by the importer. Keep library calls free of global process configuration changes.

### 4. Photo persistence contradicts the memory-only photo policy

The plan saves full LLM requests in `llm_turns.request_json` but says photo bytes leave memory after processing. A request containing a base64 image would persist those bytes and repeat them across tool rounds.

Persist a normalized message, Telegram file ID, and content digest. Download and encode the image for the network request in bounded memory. Store model/tool response records needed for replay, excluding image bytes and token-bearing download URLs. If an unfinished turn needs its image after restart, fetch it again or fail with a retryable user-facing response.

Keep complete tool-call/result groups together when trimming context. A fixed count of individual messages can leave orphaned tool results or required reasoning records. Test the provider's replay requirements and route continuity before enabling cross-provider failover within a turn.

### 5. A single SQL connection requires explicit ownership

One connection fits this workload, but code can deadlock if it opens a transaction and calls `sql.DB` again, or starts another query while rows retain that connection. Go's [connection guide](https://go.dev/doc/database/manage-connections) and [transaction guide](https://go.dev/doc/database/execute-transactions) describe this risk.

Use transaction methods throughout an action, close row iterators before further queries, and bound database contexts. Keep provider calls outside transactions. Use cheap health checks; reserve full integrity scans for the doctor command and startup validation.

Recover `processing` updates after acquiring the exclusive process lock. Use a persisted intake sequence for queue order and a separate transport cursor for polling progress. Save intake and cursor advancement together. Give pending outbox work a clear retry policy so a failed reply cannot vanish between states.

### 6. Defaults and migration tools need a single source

Defaults now live in Python, while the planned Go runtime will also seed them. Keep default values in one embedded JSON artifact that the importer and runtime can read. Validate them into a typed Go settings structure before processing updates.

The sheet-fetch helper used for this migration lives under ignored `.local/`. Commit a reusable fetch/refresh utility under `scripts/migration/`; keep tokens and captured data under ignored paths. Parameterize workflow/container identities in migration tools instead of hardcoding deployment identifiers.

Use numbered migrations with recorded checksums and a transactional migration runner. Keep the existing imported database usable through forward migrations. A synthetic insert also showed that the current `REAL` quantity column and `CHECK(quantity > 0)` accept the text `many`. Add database type enforcement through a forward migration, using SQLite `STRICT` tables or explicit storage-class constraints. Keep typed application validation too. [SQLite documents STRICT typing here](https://www.sqlite.org/stricttables.html).

## Proposed Go layout

```text
cmd/groceriesbot/              entry point, CLI and dependency wiring
internal/
  bot/                        handlers, tools, orchestration and recovery
  grocery/                    item types and pure validation rules
  sqlite/                     SQL, transactions and migrations
    migrations/               embedded numbered SQL files
  telegram/                   API client and message rendering
  openrouter/                 model API client and response mapping
  config/                     environment parsing and typed settings
scripts/migration/            Python fetch/import and n8n transfer tools
deploy/                       Coolify Compose configuration
docs/                         current design, plan and operations
  archive/                    legacy workflow material
```

Put Go tests beside their packages and fixtures in each package's `testdata/`. Keep the Python importer as a one-time migration tool. Go's [module layout guide](https://go.dev/doc/modules/layout) recommends `cmd` and `internal` for a self-contained server.

Import direction:

```text
cmd -> bot, sqlite, telegram, openrouter, config
bot -> grocery, config
sqlite -> grocery, config
telegram and openrouter -> their own transport DTOs
grocery -> standard library
```

Define the store/model/message interfaces where the bot consumes them. Use command-level adapters when provider DTOs need translation; transport and storage packages do not import bot interfaces. Avoid a generic repository interface spanning unrelated operations.

Use standard-library HTTP, `context.Context`, `signal.NotifyContext`, and `log/slog`. Reuse clients, close response bodies, cap body reads, and propagate cancellation. Add `gofmt`, `go vet`, tests, race checks, and `govulncheck` to CI after runtime packages exist. Test transactions against temporary SQLite databases, HTTP behavior with `httptest`, and tool decoding with fuzz tests. References: [shutdown contexts](https://pkg.go.dev/os/signal#NotifyContext), [structured logging](https://pkg.go.dev/log/slog), [vulnerability checks](https://go.dev/doc/security/vuln/), [Go fuzzing](https://go.dev/doc/tutorial/fuzz).

Keep the chosen CGO SQLite driver unless measurements justify replacing it. Pin build dependencies and compatible container base images. Keep the existing single replica, `/data` mount, and Coolify-managed backups. Set the module path to the actual repository path when that repository exists.

## Proposed features

| Feature | Benefit | Scope and dependency |
| --- | --- | --- |
| Deterministic `/list`, `/done`, `/remove`, `/cancel` | Immediate responses for exact commands, no model cost | First version; reuse validated actions and authorization |
| Purchase buttons on a shopping-list message | Tap an item rather than type IDs | First version; validate callback identity, item state and revision; acknowledge callbacks and update the message |
| `/undo` | Recover from a mistaken add, edit, purchase or clear | Next; journal before/after state and refuse undo if later edits conflict |
| `/usage` and spend limits | See tokens, reasoning, cost, latency and retries | Next; persist provider usage and enforce a configured request/budget policy |
| `/usual` and saved shopping templates | Re-add chosen staples without another interpretation loop | Later; explicit item selection and confirmation, no automatic scheduler |
| Quantity units and variant aliases | Distinguish two cartons from two litres; remember preferred variants | Later; structured units plus user-approved aliases |

Undo should apply to the requesting user's last eligible action. Record the expected item versions and apply the inverse in one transaction. A conflict should preserve the newer edits and explain why undo cannot proceed.

Keep routine list and purchase actions deterministic even when natural-language requests use the model. Store callback and command results through the same deduplication path as AI tool actions.

## Work order

1. Amend the design and plan with the package direction, tested model budget, source validation, and recovery rules.
2. Build configuration, SQLite access and deterministic list/purchase commands as the first working slice.
3. Add Haiku interpretation, clarification and photo handling using the same action layer.
4. Add inline purchase controls, then verify restart and volume behavior.
5. Measure resource use before production cutover. Add undo and usage reporting after the core flow passes.

This review proposes changes to the existing plan. No runtime code, database schema, model limits, or production services changed during the review.
