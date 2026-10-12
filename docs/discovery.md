# Migration discovery — 2026-10-12

## User requirements

Create GroceriesBot alongside the other SketchBook projects. Migrate and port the existing n8n groceries project to Go and MySQL, simplifying its architecture. Check the Coolify server and request its API token through TokenCrypt.

## Evidence available before the live check

The earlier read-only assessment is at `/home/deck/Documents/Codex/2026-10-11-check-current-state-of-sa3-browser/n8n-groceries-assessment.md`. It reported:

- A single active Groceries Bot workflow with 22 nodes; an inactive Daily Dutch Phrases workflow also exists.
- Telegram input for a configured group; Claude Sonnet 4.5 through OpenRouter.
- Google Sheets groceries with ID, Name, Category, Quantity, Added DateTime, and Purchase DateTime.
- Add, edit, list by category, mark purchased, delete, clear with confirmation, variant suggestions, reply context, image input, and approximate price estimates.
- An n8n queue deployment with a worker, external task runners, Redis, and PostgreSQL.
- Simple Memory in queue mode, fragile sheet row/ID mapping, and an assumption that every photo has a third size.

The older functional specification at `/home/deck/Documents/Notes/AstraBot/Groceries/AstraBot Groceries Specification.md` is useful context but differs from the later live assessment. The active workflow must be the source of truth.

These are previous observations, not a fresh inspection in this session.

## Proposed direction for review

One Go application owns Telegram input, bounded OpenRouter requests, validated grocery operations, and replies. MySQL persists grocery records, chat context, confirmation state, and processed Telegram update IDs. Use stable database IDs instead of sheet row numbers. Enforce purchase timestamps and clear confirmation in application code.

MySQL replaces Google Sheets as requested. Redis, n8n workers, task runners, and the second purchase-completion AI agent should not be necessary for the replacement. Preserve image handling and conversational clarification unless the user chooses a smaller scope.

The implementation design must define retry and duplicate-update behavior, chat authorization, data import validation, tests, and a rollback path before changing Telegram delivery.

## Fresh Coolify inspection

On 2026-10-12, authenticated read-only API calls to `https://coolify.tail.liveleds.io/api/v1/` confirmed:

- Project: Personal Server (`h8z19ucshyj91wms6smk1seq`), server: localhost (`v3odz6rd56ro7lpnh3xoot27`).
- n8n service (`fwcmxt6ry1vt7p5scuhzwizj`) is running and healthy.
- Main n8n and worker use `n8nio/n8n:2.10.4`; task runners use `n8nio/runners:2.10.4`. All are running and healthy.
- Its bundled databases are PostgreSQL 16 Alpine and Redis 6 Alpine, both healthy.
- A separate N8N Postgres DB uses PostgreSQL 18 Alpine and is healthy. Its role relative to the bundled PostgreSQL has not been established.
- No MySQL service appeared in either the services or standalone database inventory returned by the API.
- Other services include an exited AI WebUI, a healthy CouchDB, and a Tailscale sidecar.

The version endpoint returned a non-JSON response; the Coolify version was not established. No resources were created or changed on Coolify.

## Pending inspection

- Actual Google Sheets rows and their data quality; no grocery data has been fetched or migrated.
- Current Telegram webhook metadata; the active n8n Telegram trigger establishes the workflow entry point, but Telegram itself has not been queried.

TokenCrypt request `767880c8e89f8eb5c2240e28380833d5` was approved and used for the API inventory. Credentials were supplied through `tokencrypt run`, without printing or saving the token.

The API does not expose the n8n workflow. TokenCrypt request `6ce816e3fbfd6e8b78aaf72b669e7b96` was approved for `deck_ssh_passphrase_hetzner`. SSH through the known Tailscale host on port 22666 succeeded. Credentials were injected into the existing SSH askpass helper and were not saved.

The active workflow was read directly from n8n's PostgreSQL database, joining its active version to workflow history. [workflow-snapshot.json](workflow-snapshot.json) preserves selected parameters and connections; it deliberately excludes credentials and URL fields and is not a restorable workflow export.

Fresh workflow findings:

- Groceries Bot (`SLffBV8YkETQcXcF`) is active and has 22 nodes.
- The trigger restricts messages to one Telegram group, with no user restriction.
- Both agents use `anthropic/claude-sonnet-4.5`; the main agent permits 15 iterations.
- Memory uses `=static_memory`, not a per-chat session key.
- The output parser routes a message, options, or individual grocery list messages.
- Photo retrieval selects `message.photo[2]`; images require confirmation before adding the proposed item.
- Delete clears by a model-supplied row number and describes row number as ID + 1.
- Clear confirmation is described in the tool prompt. Purchase timestamps and item IDs are also model supplied.
- Pending item edits are restricted by tool instructions rather than demonstrated application validation.

Fresh host snapshot: 3,819 MiB total RAM, 1,688 MiB available, and 106 MiB swap used. The groceries n8n stack used approximately 514 MiB across main (299.7), worker (161.3), runners (6.668), PostgreSQL (41.13), and Redis (5.621). This is one snapshot, not a peak or a forecast for Go plus MySQL.

## Cutover constraints

Keep the existing workflow recoverable and grocery source data intact. Validate imports and replacement behavior before switching Telegram delivery. Retiring the entire n8n stack also requires preserving the inactive Dutch workflow. Production cutover is separate from creating this local project and inspecting the server.

## Fresh credential availability check

The user subsequently asked whether all tokens could be fetched from n8n. A read-only inventory found five stored credential records, and each could be decrypted by n8n's own credential export command:

| Credential type | Count | Populated authentication fields |
| --- | --- | --- |
| OpenRouter | 1 | API key |
| Telegram | 2 | Bot access token in each record |
| Google Sheets OAuth2 | 1 | Access token, refresh token, client ID, client secret |
| PostgreSQL | 1 | User and password, with host/database configuration |

The groceries workflow references one OpenRouter record, one Telegram record, and the Google Sheets record. An inline Telegram bot URL is also present in the active workflow, so migration must account for that additional token location. Whether the inline token matches a stored Telegram credential has not been checked.

For each credential, the export lived in a mode-0700 temporary directory under `/dev/shm` in the n8n container and was removed in a `finally` block. Only credential types, IDs, and field-presence flags were returned. No token values were printed or copied into the project, and no plaintext exports were retained. Retrievability and field presence were verified; provider acceptance, expiry, and OAuth refresh were not tested.
