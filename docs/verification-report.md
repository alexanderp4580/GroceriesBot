# Verification results

Release candidate `436cfe8`, 2026-10-12. Ran the recorded verification once in `.local/verification/release-candidate-1/`: tracked-file secret scan, seven Python migration tests, Go race tests, vet, compilation, pinned image build and synthetic container checks all passed.

The container checks verified UID 10001, a read-only root, a 128 MiB limit, persistence of groceries/memory/settings across recreation, and rejection of an overlapping process through the data lock. Fixture providers made no production model requests.

## Coolify and migration

Coolify deployed the release and started active Telegram polling. The first deployment failed because its Compose build context resolved outside the repository; context `.` corrected that failure. Later deployments completed. Runtime variables hold credentials; the image contains no credentials or imported rows.

The final Sheets snapshot imported 29 groceries, including six pending and 23 purchased, from 30 source rows. Snapshot compression/digest checks, SQLite integrity and foreign keys passed, with zero import issues. Private source and DB files remain in `.local/`. Production uses `1uqewh4rudhzcxwbvg8suqkq_groceriesbot-data` mounted at `/data`, UID 10001, mode 0600.

The n8n grocery workflow was unpublished and its service stopped after confirming zero unfinished grocery executions. Its data and workflow version remain available for rollback. Telegram webhook removal preserved queued updates. Production AI is enabled with Haiku 5.5, medium effort, and $1/day and $5/month bot caps.

Administrative inbox smoke commands `/list` and `/usage` delivered actual Telegram messages 771 and 772 to RAGroceries. They used the bot's own actor identity, not a fabricated human Telegram update. Grocery rows remained unchanged. The list required one shelf-order request costing $0.000248; `/usage` fetched live key usage and account credits without a model completion.

After restarting the live container, all 29 groceries, four conversation messages, two update records, two delivery records and the settings digest persisted. The readiness endpoint returned HTTP 200. Records: `.local/verification/active-initial.log`, `readonly-smoke.log`, `restart-measure.log` and remote `/root/groceriesbot-verification/`.

The user requested removal of the old n8n suggestion keyboard. Telegram accepted a `remove_keyboard` message, ID 773.

## Resource measurements

A fixture workload ran on the Coolify host under the production 128 MiB / 0.5 CPU limits. It processed 76 updates, delivered 83 messages, retained 20 conversation messages and ended with 123 pending items. The four-MiB photo buffer used the recorded banana response through a local fixture provider. Peak process RSS was 54.68 MiB. The fixture made zero actual OpenRouter requests. Record: `.local/verification/resource-workload-1.log`.

The ten-minute active polling measurement is running. This workload measurement measures resource use; the separate recorded banana test establishes actual model behavior.

## Model testing cost

Recorded live scenarios cost $0.0129009 for 42 requests. The deployed read-only list smoke added $0.000248, giving **$0.0131489** in test/model verification cost. Provider ledger amounts retain more precision than the bot's micro-dollar ledger. No additional model probes were run for deployment. See [LLM results](llm-results.md).

These are individual recorded flows, not a repeated-run reliability estimate. The final independent release review is pending.
