# Recorded LLM scenarios

The first recorded run is in [llm-results.md](llm-results.md). Fixture tests check application rules and API contracts; they do not establish Haiku's response shape, interpretation or ordering quality.

Run the capture harness only after the user authorizes OpenRouter spend. It uses synthetic chats and a separate temporary SQLite database, never the production bot, chat or sheets. The harness records each raw HTTP request and response, provider status, usage, latency, observed bot messages and before/after DB rows. It records the model's actual fields and tool choices. It does not require particular wording or a predetermined shelf permutation.

| Scenario | What to establish |
| --- | --- |
| Explicit addition | Valid tool arguments for brand, quantity and unit |
| Quantity reset | A second independent item gets its own quantity |
| Variant history | Relevant variant suggestions and resolution from purchased history |
| Batch confirmation | Proposed items stay unapplied until the initiating user confirms |
| Ingredients | Useful inferred groceries for specified servings, with confirmation |
| Photo | Read a synthetic note and preserve its quantities |
| Photo variant | Preserve the confirmation requirement through a variant question |
| Shelf adjacency | Group probable neighboring shelves across categories; complete ID coverage and cache reuse |
| Edit and reply | Target the existing row using reply context; retain original added date |
| Selected purchase | Purchase the requested item and leave other groceries pending |
| Ambiguous reference | Ask about ambiguity rather than silently edit a guessed row |
| Price estimate | Euro/Netherlands estimate and zero grocery mutations |
| Prompt injection | Treat names as data; protect settings, records and secrets |
| Reasoning/tool loop | Actual medium-effort response fields, reasoning and continuation behavior |

The same recordings reveal output truncation, malformed JSON, unsupported provider parameters, token use, cost and latency if those occur. Network failure, duplicate delivery, malformed responses and authorization are deterministic application checks in the fixture suite; deliberately provoking those through a paid model does not establish its behavior.

Scenarios live in `testdata/llm-scenarios.json`. The runner follows a real returned clarification or confirmation when a scenario requests it. If the expected interaction is absent, it records that observation instead of inventing a model answer.

```sh
python3 -m pip install --target .local/capture-deps -r scripts/requirements-capture.txt
PYTHONPATH=.local/capture-deps python3 scripts/capture_llm.py --list
# Requires separate approval before execution:
PYTHONPATH=.local/capture-deps python3 scripts/capture_llm.py --record .local/llm-results/RECORD --allow-paid-model-calls --max-usd 0.25
```

The runner refuses to overwrite records. A recording budget applies across the run; it stops before a request whose conservative token-price allowance exceeds the remaining budget. Missing/invalid usage retains that allowance. Unknown or unavailable model metadata stops the run. Production `llm.enabled` stays false.

Review raw results before changing prompts or parsers. Add sanitized captured responses to regression fixtures only after recording their source model/provider/settings and removing private content. Re-run only scenarios affected by a change, in a new record directory.

## Targeted continuations and custom photos

Use `--scenario ID` to select cases; unselected cases do not make provider requests. `--resume RECORD` copies selected synthetic DBs, skips initial prompts and retains saved memory/clarifications. The manifest records copied DB hashes, starting update IDs, effective steps and any expiry refresh in the lab copy.

The follow-up handler uses the actual pending action. It can click a configured option label from a recorded response, answer a clarification in ordinary text, or reply to a text-only proposal. It does not invent missing options or silently treat absence as success.

```sh
PYTHONPATH=.local/capture-deps python3 scripts/capture_llm.py --record .local/llm-results/NEW_RECORD --resume .local/llm-results/live-haiku-2 --scenario ingredients --scenario photo --allow-paid-model-calls --max-usd 0.10
PYTHONPATH=.local/capture-deps python3 scripts/capture_llm.py --record .local/llm-results/NEW_PHOTO_RECORD --scenario banana_phone --image testdata/images/banana-phone-blurry.png --allow-paid-model-calls --max-usd 0.10
python3 scripts/check_recorded_flows.py .local/llm-results/NEW_RECORD --previous .local/llm-results/live-haiku-2
```

The custom PNG enters the request unchanged. The capture manifest records its path, dimensions and SHA-256. The banana prompt asks for the item in the image without naming it. The recorder budgets inline images by a conservative visual-token allowance rather than treating base64 bytes as text tokens; raw traffic retains the complete original image.
