# Data migration

Imported on 2026-10-12. See [import-report.json](import-report.json) for counts, checksum, and verification results.

## Source coverage

The active n8n groceries workflow contains six Sheets tools. They reference the same spreadsheet and `Groceries` tab. The read-only Google request fetched the entire spreadsheet with `includeGridData=true`, including its single tab and 1,005 grid rows. See Google's [spreadsheets.get reference](https://developers.google.com/workspace/sheets/api/reference/rest/v4/spreadsheets/get).

This import covers the spreadsheet used by GroceriesBot. It does not enumerate unrelated spreadsheets in the Google account.

| Data | Count |
| --- | ---: |
| Spreadsheets | 1 |
| Tabs | 1 |
| Content-bearing rows, including header | 30 |
| Normalized grocery records | 29 |
| Pending | 6 |
| Purchased | 23 |
| Rejected grocery rows | 0 |

The source columns are `ID`, `Name`, `Category`, `Quantity`, `Added DateTime`, and `Purchase DateTime`. The row positions contain gaps. The importer keeps those positions in `source_rows` and assigns stable SQLite IDs to groceries.

Google returned numeric date values, an `en_US` locale, and Europe/Amsterdam as the spreadsheet timezone. The importer converts the serial dates to UTC and leaves blank purchase dates null. It rejects ambiguous local times and malformed dates rather than guessing their meaning. Google's [date rendering reference](https://developers.google.com/workspace/sheets/api/reference/rest/v4/DateTimeRenderOption) defines the serial-date epoch.

## Destination

`data/groceriesbot.sqlite3` is the main database for this project. It contains the full compressed source response, relational source rows, normalized groceries, and runtime tables. The import seeded 18 settings. Conversation memory starts empty.

The database occupies 163,840 bytes after import. Both the database and `.env` have mode `0600`; their parent data directories have mode `0700`. Git and the container build context exclude them.

The source snapshot includes returned values, formulas, notes, cell formatting, and spreadsheet metadata. Decompressing the stored snapshot reproduces the fetched JSON. This preserves formatting-only grid rows without expanding them into grocery records.

## Credential transfer

TokenCrypt supplied SSH access. n8n exported the required credentials into a temporary memory-backed directory, which the export helper removed after transfer. The local setup saved these keys to `.env`:

```text
TELEGRAM_BOT_TOKEN
OPENROUTER_API_KEY
GOOGLE_CLIENT_ID
GOOGLE_CLIENT_SECRET
GOOGLE_REFRESH_TOKEN
GOOGLE_ACCESS_TOKEN
GOOGLE_SPREADSHEET_IDS
```

Startup values also include `DATABASE_PATH`, `RUN_MODE=paused`, and the original chat allowlist for bootstrapping an empty database. The inline Telegram token matched the referenced credential, so the setup needed no duplicate token key. Google accepted a refresh-token exchange before the read-only fetch.

The setup deleted the intermediate local credential bundle and kept secret values out of logs. The production Go container needs the Telegram and OpenRouter keys; the Google values serve migration work.

## Import behavior

Use the [import helper](../scripts/import_sheets.py) with a private JSON snapshot and a chat ID. It validates the header, quantities, names, and dates. Rejected rows remain in the raw snapshot and source tables with an issue code.

Repeating the same snapshot returns the existing counts without inserting duplicates or resetting settings. A different snapshot requires a new empty database. This prevents a later import from overwriting runtime groceries.

For the final production import, fetch a new snapshot after stopping the n8n grocery workflow. Import into a new database, verify it, then install it on the stopped replacement's persistent volume. Keep the current database until that step succeeds.

## Verification performed

- SQLite integrity check returned `ok`; foreign-key check returned no errors.
- Stored snapshot JSON matched the fetched JSON, and its SHA-256 matched the recorded digest.
- Each of the 29 normalized groceries links to its original source row.
- Import tests covered numeric dates, source row gaps, additional hidden tabs, formulas and notes, malformed quantities and dates, repeat import, changed snapshots, and runtime tables.

The source sheet and n8n delivery remain unchanged. No production cutover occurred.
