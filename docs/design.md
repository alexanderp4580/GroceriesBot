# GroceriesBot migration design

Date: 2026-10-12. Status: proposed, awaiting user review.

## Outcome and scope

Replace the existing n8n Groceries Bot with one Go application and MySQL on the existing Coolify server. Preserve the useful Telegram experience while removing workflow orchestration, the second AI agent, and Google Sheets as the live store.

The user explicitly requested Go and MySQL. Retaining Telegram and OpenRouter, preserving the current feature set, and using long polling are proposed choices for review. The inspected active workflow, summarized in [discovery.md](discovery.md), is the behavior reference.

No website, account system, local model, Redis, or general workflow engine is in scope. The inactive Dutch workflow is outside this port and must remain recoverable.

## Approaches considered

1. **Recommended: Go and MySQL, Telegram long polling, one bounded AI tool loop.** Few deployment components; outbound Telegram traffic removes the need for a public webhook route. Code validates all mutations. This requires a deliberate switch away from the existing n8n webhook.
2. **Go and MySQL with Telegram webhooks.** Preserves the delivery approach but needs an HTTPS route, webhook authentication, and durable intake before acknowledging updates. Suitable if public webhook hosting is preferred.
3. **Port each n8n node and agent directly.** Closest internal resemblance, but retains unnecessary orchestration and makes model-driven side effects harder to validate. It does not achieve the requested simplification as well.

The recommended deployment has one Go container and one dedicated MySQL service with a persistent volume. The live Coolify inventory has no existing MySQL service to reuse.

## Application boundaries and flow

`cmd/groceriesbot` runs the service. Small internal packages own configuration, Telegram transport/rendering, OpenRouter requests, grocery rules, and MySQL persistence. A separate `cmd/import-groceries` imports an explicit CSV export; SQL schema changes live under `migrations/`.

One polling process receives Telegram message updates and saves them in MySQL before advancing the polling offset. An inbox processor handles updates in order, checks the allowed chat, loads recent conversation context and relevant grocery history, and passes text, reply context, or image content to OpenRouter.

The model chooses from a narrow tool set: read groceries/history, add, edit, mark purchased, remove, propose/confirm clearing, and request clarification. Tool arguments are typed and validated; no model SQL or arbitrary HTTP requests are accepted. The model cannot supply authoritative timestamps or override chat authorization. Use one model configuration, initially the existing `anthropic/claude-sonnet-4.5`, with configurable timeouts and a maximum of 10 tool rounds per update.

Application code renders the final message, keyboard, or category list, escaping untrusted text. Keep concise replies and emojis. List messages use bounded chunks rather than one HTTP request for every item. Select the largest available photo within configured byte limits, preserving its caption. Do not retain photo bytes after processing.

## Preserved behavior and enforced rules

- Add items with inferred categories and a default quantity of one. A quantity belongs only to its item; clarification state cannot leak it into the next item.
- Suggest variants from historical groceries and accept consecutive clarifying replies.
- Ask for confirmation of proposed items inferred from photos or broad ingredient requests.
- List pending groceries in the existing category order: Quick / ready food, Vegetables, Fruits, Vegan, Meat, Fish, Bread, Pantry, Candy, Bakery, Dairy, Household, Pharmacy, Drinks. Preserve unknown imported category text and list it after known categories.
- Edit pending items only. Adding or editing cannot assign a purchase timestamp or change the original added timestamp.
- Mark specified pending IDs or all pending groceries purchased using application time. Retain purchased rows for suggestions and history.
- Delete specified groceries by database ID, never by physical row position.
- Clearing all groceries requires an explicit confirmation associated with the initiating chat and user, expires after five minutes, and is invalidated by intervening grocery mutations. The model cannot invent confirmation. This operation includes purchased history, matching the existing clear-everything tool.
- Provide best-effort Netherlands price estimates in euros, clearly marked as estimates. Price estimation cannot mutate groceries.

Store timestamps in UTC and present them in Europe/Amsterdam. Preserve the current group-only authorization; optional allowed user IDs can further restrict access. Scope conversation context by chat and clarification/confirmation state by chat plus user to avoid one group member completing another member's request.

## MySQL persistence

Use MySQL 8.4 with InnoDB and utf8mb4, a dedicated database, and a restricted application user. Keep the database on a private container network with no public port.

- `groceries`: generated stable ID, chat ID, name, category, positive quantity, added timestamp, nullable purchase timestamp, and row version. Use soft deletion for recoverable removals; deleted rows are excluded from lists and suggestions.
- `conversation_messages`: chat ID, user ID, role, message content, and timestamps. Keep the latest 20 turns per chat and prune older content.
- `pending_actions`: chat/user identity, action type, proposed items or target IDs, list revision, and expiry. These records survive restarts.
- `telegram_updates`: unique update ID, chat/user IDs, normalized payload, processing state, attempts, and error classification.
- `tool_actions`: unique update/round/action identity and recorded mutation result, preventing a replay from applying the same operation twice.
- `outbox`: formatted replies and delivery state, saved in the same transaction as completed changes.
- `import_rows`: source-file hash and row number mapped to the generated grocery ID, enabling repeatable imports and preserving legacy IDs as metadata.
- `schema_migrations`: applied migration versions.

Keep grocery mutations, action records, confirmation consumption, and outbox insertion transactional. Do not hold database transactions open during model or Telegram network calls.

## Failure and recovery behavior

Retry transient Telegram and OpenRouter failures with bounded exponential backoff and rate-limit handling. Invalid model arguments receive a structured tool error, with no database mutation. Stop after the tool-round limit and queue a concise failure reply that reports any changes already committed.

Persist input and action progress so restarts do not recreate groceries or apply purchase completion twice. Every mutation result must be durable before continuing the model conversation. A repeated update returns previously recorded results.

The Telegram outbox prevents lost replies after committed changes. Telegram message delivery has an ambiguity if a network timeout follows a successful send; a retry may duplicate a reply. Do not claim exactly-once message delivery. Grocery mutations remain deduplicated independently.

Use structured logs with update IDs, durations, action names, and error classes. Never log API keys, token-bearing URLs, raw image data, or full grocery conversations by default. Provide health/readiness endpoints and graceful shutdown. Measure resource usage in staging before making savings claims.

## Data migration and cutover

Import Google Sheets CSV rather than maintain ongoing Sheets synchronization. Validate headers, retain raw source values in the protected migration archive, and import pending and purchased groceries. Blank purchase dates remain null. Preserve valid legacy IDs as metadata while assigning new stable IDs. Reject invalid quantities, ambiguous dates, duplicate legacy IDs, and malformed rows with a row-level report; do not silently skip or guess them. Run a dry import first and compare row counts and normalized values.

Develop and validate with synthetic data and a separate test Telegram bot. Credential availability has been verified through temporary decrypted exports in the n8n container's memory-backed filesystem, followed by deletion. No secret values were printed or retained. Transferring production credentials into the replacement's protected configuration and reading actual spreadsheet data are later migration steps.

Before production switch: preserve a restorable workflow backup and encrypted credential backup, record current Telegram webhook settings, stop the active n8n grocery workflow, take the final sheet export, validate/import it, then remove the webhook without dropping pending updates and start the single polling instance. Long polling and webhooks cannot consume the same bot simultaneously, as documented by [Telegram](https://core.telegram.org/bots/api#getupdates).

Keep n8n recoverable while verifying the replacement. If rollback is needed, stop polling, reconcile all post-cutover mutations back into the source sheet from the protected action history, and restore the previous workflow/webhook. Do not retire unrelated Coolify databases, shared infrastructure, or the inactive Dutch workflow as part of this port.

## Verification and acceptance

Use deterministic model/Telegram fixtures for conversational behavior and MySQL integration checks for transactions, replay handling, and import behavior. Cover generic item clarification, per-item quantity reset, multi-item confirmation, photo arrays of different sizes, captions/replies, category ordering, pending-only edits, purchase completion, delete by ID, clear confirmation expiry/user binding, unauthorized chats, malformed model output, network failures, and crash/restart replay.

Verify CSV import twice produces no duplicates and that invalid rows fail visibly. Verify a crash between a committed mutation and Telegram delivery retains the reply without repeating the mutation. Run staging smoke tests with the separate bot before any production switch.

Acceptance: the observed grocery behavior works through Telegram; MySQL stores groceries and durable state; no n8n/Redis dependency remains in the replacement; original data is reconciled; rollback is documented and tested; secrets are absent from committed artifacts. This document does not authorize a production cutover by itself.

## Reference material

- [Selected active workflow parameters and connections](workflow-snapshot.json), inspected 2026-10-12. Sanitized discovery artifact, not a backup.
- [Telegram Bot API](https://core.telegram.org/bots/api) for polling, updates, photos, and message rendering.
- [MySQL InnoDB transaction model](https://dev.mysql.com/doc/refman/8.4/en/innodb-transaction-model.html) for transaction design.
