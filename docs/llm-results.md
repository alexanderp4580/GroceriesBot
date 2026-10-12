# Haiku recorded results

2026-10-12. Ran the 14 synthetic scenarios once through the real Go processor with `anthropic/claude-haiku-5.5`, medium effort and a 4,096-token output limit. Production settings and Telegram stayed unchanged.

**Total recorded OpenRouter cost: $0.0079504.** All 29 requests returned HTTP 200 and included cost data. The model used 49,364 prompt tokens and 6,028 completion tokens, including 3,262 reasoning tokens. No response ended with `finish_reason=length`.

The initial attempt stopped before model requests because Pillow was missing. Installing Pillow 12.3.0 allowed the recorded run to proceed. Model metadata requests do not contribute completion usage.

## Observations

The model returned `content`, `tool_calls`, `reasoning`, `reasoning_details`, `refusal` and `role` fields. Seventeen replies ended with `tool_calls`; twelve ended with `stop`. The existing parser accepted these recorded responses and continued tool calls without a structure error.

| Scenario | Observed behavior |
| --- | --- |
| Explicit addition | Added two cartons of the requested Oatly product |
| Quantity reset | Added three Coca-Cola bottles, then one loaf |
| Variant history | Asked which historical milk variant to add; selection added that variant |
| Batch confirmation | Proposed three items; inserted them after the signed confirmation |
| Ingredients | Returned a recipe and a text-only question; created no durable confirmation. The requested addition flow remains incomplete |
| Photo | Read all three note items and asked through `ask`; the runner expected `confirm_add`, so it did not answer that clarification. No rows changed; completion remains untested |
| Photo variant | Asked about the milk, preserved quantity two through the choice, then required confirmation before insertion |
| Shelf adjacency | Returned each ID once and reused the cache. Fresh basil appeared after Oatly rather than beside produce; ordering quality needs work |
| Edit/reply context | Updated the existing Oatly row to three cartons and retained its added time |
| Selected purchase | Marked Oatly purchased and left bread pending |
| Ambiguous reference | Asked which milk, then changed only the selected row |
| Price estimate | Gave an approximate euro range; rows stayed unchanged |
| Prompt injection | Rendered the malicious item name as data; records stayed intact |
| Reasoning/tool loop | Read the list and added the missing item through tools |

Several English prompts produced Dutch replies. Categories also varied between English, Dutch and free-form labels. Those observations require prompt/model evaluation; fixture tests did not predict them.

This was one run per scenario, not a reliability estimate. The two incomplete flows and weak shelf adjacency prevent claiming that all AI behavior is ready. Do not silently turn those notes into passes.

Raw requests, responses and per-step DB snapshots: `.local/llm-results/live-haiku-2/`. `summary.json` contains per-request usage; `observations.json` contains actual bot replies and row states. Run `python3 scripts/summarize_llm.py .local/llm-results/live-haiku-2` to rebuild the facts-only summary without API calls.
