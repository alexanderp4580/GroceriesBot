# Recorded LLM scenarios

No live model results exist yet. Fixture tests check application rules and API contracts; they do not establish Haiku's response shape, interpretation or ordering quality.

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
python3 scripts/capture_llm.py --list
# Requires separate approval before execution:
python3 scripts/capture_llm.py --record .local/llm-results/RECORD --allow-paid-model-calls --max-usd 0.25
```

The runner refuses to overwrite records. A recording budget applies across the run; it stops before a request whose conservative token-price allowance exceeds the remaining budget. Missing/invalid usage retains that allowance. Unknown or unavailable model metadata stops the run. Production `llm.enabled` stays false.

Review raw results before changing prompts or parsers. Add sanitized captured responses to regression fixtures only after recording their source model/provider/settings and removing private content. Re-run only scenarios affected by a change, in a new record directory.
