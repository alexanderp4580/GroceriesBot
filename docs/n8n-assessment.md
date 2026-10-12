> Historical reference copied into GroceriesBot on 2026-10-12. Read `design.md` for the current SQLite design.

# Groceries bot assessment — 2026-10-12

Read-only inspection over SSH, with TokenCrypt access. No workflows executed, messages sent, credentials exported, or deployment changes made.

## Resource snapshot

Docker stats reported main n8n 271.8 MiB, worker 158.9 MiB, task runners 4.781 MiB, PostgreSQL 36.62 MiB and Redis 5.027 MiB: approximately 477 MiB total. Main and worker CPU were each below 0.5% in this snapshot. This is not a peak-load measurement. Host had 1.7 GiB available RAM and 92 MiB swap in use.

## Workflows

Groceries Bot is the only active workflow. Daily Dutch Phrases is disabled. The groceries workflow has 22 nodes. Its active version and editor draft have identical connections; observed node differences are positions and Telegram credential display names.

The bot receives Telegram messages for one configured group, uses Claude Sonnet 4.5 through OpenRouter, and maintains Google Sheets rows with ID, Name, Category, Quantity, Added DateTime and Purchase DateTime. It supports adding and editing pending groceries, listing by category, purchase completion, deleting rows, clearing the sheet, variant suggestions through Telegram reply keyboards, reply context, image input and best-effort price estimates. Sheets writes and whole-list reads are delegated to model tools. Purchase completion uses a second AI agent. The agent iteration limit is 15.

The retained execution records show seven successful groceries executions in the last 30 days, averaging 15 seconds, and no retained failures in that interval. Retention settings may mean this is not the full historical workload. The n8n database is 13 MB.

## Architecture findings

EXECUTIONS_MODE is queue and external task runners are enabled. This explains the separate worker and Redis services. It is a scalability arrangement for a bot with a very small retained workload.

The active workflow uses Simple Memory. n8n explicitly advises against Simple Memory in queue-mode production because calls are not guaranteed to reach the same worker. There is one observed worker, so this is a configuration risk rather than evidence of current lost conversations.

Other migration concerns: deletion assumes sheet row number equals ID plus one; photo retrieval assumes the third photo size exists; clear confirmation and dates are governed by model instructions. A replacement should use actual row mappings, select available photo sizes, and enforce confirmation and date rules in application code.

## Replacement direction

A single service can retain Telegram, OpenRouter and Google Sheets while replacing the visual workflow engine, worker, Redis and n8n database. SQLite can persist conversational state and processed Telegram update IDs. No local inference is required. Measure the replacement before claiming memory or latency improvements. Retaining Sheets initially avoids a simultaneous grocery data migration.

Keep the existing workflow and encrypted configuration backed up for rollback. Test the replacement against fixtures and a separate sheet before switching Telegram delivery and disabling n8n. The inactive Dutch workflow must remain recoverable if retiring the entire stack.

References:
- https://docs.n8n.io/hosting/scaling/queue-mode/
- https://docs.n8n.io/integrations/builtin/cluster-nodes/sub-nodes/n8n-nodes-langchain.memorybufferwindow/
