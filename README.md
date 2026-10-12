# GroceriesBot

A Go Telegram grocery assistant with SQLite storage, deployed on Coolify. It replaces the groceries n8n workflow and Sheets storage with one process and a persistent `/data` volume.

Use `/list` for purchase buttons. The bot updates the same list after a purchase. IDs shown beside items work with `/done` and `/remove`.

| Command | Action |
| --- | --- |
| `/add 2 carton Oat milk` | Add an item with quantity and unit |
| `/list` | Show the shopping list |
| `/done 12 18` or `/done all` | Mark pending items purchased |
| `/remove 12` | Remove a pending item |
| `/undo` | Undo your last mutation if nobody changed those items afterward |
| `/clear` | Confirm clearing pending items and history |
| `/cancel` | Cancel your pending question |
| `/save usual` | Save pending items as a template |
| `/usual` or `/usual NAME` | Confirm adding a saved template |
| `/templates` | List templates |
| `/alias milk = Oatly Barista` | Approve a name alias |
| `/usage` | Show model cost, tokens and unresolved reservations |

AI text, photos, variants and shelf ordering use Haiku 5.5 with medium effort. The route guides item placement, including neighboring categories. AI starts disabled to respect the user's OpenRouter credit restriction; exact commands and buttons still work. `/list` labels its fallback ordering while AI is disabled.

## Local development

Copy `.env.example` to a private `.env`, supply credentials and set a test chat allowlist. Keep production credentials and data outside Git. Process variables override the optional file. The default mode is `paused`.

```sh
toolchain/run go test ./...
toolchain/run go run ./cmd/groceriesbot doctor
toolchain/run go run ./cmd/groceriesbot settings get llm.enabled
python3 scripts/verify.py --record .local/verification/RECORD --containers
```

`toolchain/run` uses the pinned Go container through Podman. Tests use fixtures and do not need credentials. The recorded verification script refuses an existing output directory.

## Deployment and docs

Coolify builds `Dockerfile` using `deploy/compose.yaml`, with one replica, 128 MiB memory and a persistent full-directory mount. Runtime secrets belong in Coolify environment variables. Coolify handles backups.

Read [implementation](docs/implementation.md), [design](docs/design.md), [deployment](docs/deployment.md), [migration](docs/migration.md), and [verification results](docs/verification-report.md). The selected behavior settings live in SQLite; use the CLI to inspect or change them.

Recorded live model behavior and total cost: [LLM results](docs/llm-results.md). Two flows need further verification; see the report before enabling production AI.
