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

## Recorded continuations

Resumed only the ingredients and photo conversations from private copies of their saved synthetic databases. Preserved the original recordings. The runner refreshed the copied photo clarification's expiry for this lab continuation and recorded that adjustment in `capture-manifest.json`; production state stayed unchanged.

The ingredients reply was ordinary user text approving the earlier proposal. Haiku then created an 18-item pending batch, and the signed confirmation inserted those exact items. The photo test clicked the actual returned “Yes, add all 3” option, received a three-item proposal, and inserted two cartons oat milk, one sourdough loaf and six apples after the final confirmation.

Both flows completed with zero rows inserted before final confirmation. The issue in the initial run was the runner's missing follow-up handling. The grocery runtime needed no change for these continuations. Each flow still required a final application confirmation after the earlier conversational approval.

Two additional requests cost **$0.0015588**. Cumulative OpenRouter testing cost after these continuations: **$0.0095092** for 31 requests. Raw records and invariant checks: `.local/llm-results/flow-continuation-1/`.

## Generated banana phone photo

Used the built-in image-generation tool to create one yellow banana on a kitchen counter with soft focus and handheld phone-photo blur. The exact prompt and image hash are in `testdata/images/banana-phone-blurry.metadata.json`; the saved PNG is `testdata/images/banana-phone-blurry.png`. The capture manifest records its dimensions and source hash. The recorded request contains the identical PNG, with no crop, resize or compression.

The user caption was “Add the item shown in this photo to my shopping list.” It did not name the fruit. Haiku replied: “I see a banana in the photo. Should I add 1 banana (category: fruit) to your shopping list?” A recorded human reply led to a one-item application proposal. The final signed confirmation added exactly one `banana`, category `fruit`, quantity 1. Nothing entered the grocery table before that confirmation.

The two requests cost **$0.0008273**. Total OpenRouter testing cost across the initial scenarios, two continuations and banana photo: **$0.0103365** for 33 requests. This total covers OpenRouter calls; the built-in image tool did not expose a separate generation price. Raw records and checks: `.local/llm-results/banana-phone-1/`.

The ingredients and photo flows have now completed in recorded continuations. Shelf-adjacency quality and language/category consistency still need work; these results do not claim production deployment or repeated-run reliability.
