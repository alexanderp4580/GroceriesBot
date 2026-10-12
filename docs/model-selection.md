# Model selection

On 2026-10-12, the user selected `anthropic/claude-haiku-5.5` through OpenRouter with medium effort. The project SQLite settings and importer defaults use this ID and `llm.reasoning_effort="medium"`. The legacy n8n workflow remains on Sonnet 4.5.

## Benchmark comparison

Artificial Analysis reports these scores on Intelligence Index v4.3.2, a composite of ten evaluations. Higher scores indicate better results on that suite.

| Model and setting | Score |
| --- | ---: |
| Sonnet 4.5, highest reported variant | 21 |
| Haiku 5.5, low effort | 29 |
| Haiku 5.5, medium effort | 34 |
| Haiku 5.5, max effort | 43 |

Source: [Artificial Analysis, Anthropic models](https://artificialanalysis.ai/models/creators/anthropic), checked 2026-10-12. This supports trying Haiku for the grocery workflow; it does not establish grocery-specific correctness. Scores depend on reasoning configuration and token budget.

Anthropic's Haiku launch comparison uses Sonnet 5.5 and newer benchmark versions. Do not compare its OSWorld 2.1 offline-subset score with Sonnet 4.5's older OSWorld score as a controlled comparison.

## Cost and integration

OpenRouter lists Haiku's base rates at $0.10 per million input tokens and $0.50 per million output tokens, versus Sonnet 4.5 at $3 and $15. Haiku's short-prompt rates are 30 times lower per token; reasoning, tokenization, and retries affect total cost. Haiku's rates increase above 100,000 input tokens, beyond this bot's planned context.

Haiku supports images, tools, and structured output. Adaptive thinking is on by default with medium effort. The implementation must verify complete tool arguments and replies under the configured 800-token output bound, and measure cost and latency with the reasoning settings it actually sends. Omit unsupported sampling parameters rather than copying old Sonnet options.

References: [OpenRouter Haiku](https://openrouter.ai/anthropic/claude-haiku-5.5), [OpenRouter Sonnet](https://openrouter.ai/anthropic/claude-sonnet-4.5), [Anthropic model overview](https://platform.claude.com/docs/en/models/haiku-5-5/overview).
