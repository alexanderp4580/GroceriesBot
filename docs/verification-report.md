# Verification results

2026-10-12. Ran `python3 scripts/verify.py --record .local/verification/reviewed-build-1 --containers` once against runtime commit `fee07ec`.

| Check | Result |
| --- | --- |
| Tracked-file secret scan | Passed |
| Python migration tests | 7 passed |
| Go tests with race detection, cache disabled | Passed across packages |
| Go vet | Passed |
| Go compilation | Passed |
| Pinned container image build | Passed |
| Container policy | UID 10001, read-only root, 128 MiB limit |
| Container recreation | Groceries, memory and settings persisted |
| Overlapping instance | Second process rejected by the data lock |
| Paused mode | Network disabled for the container check; AI remained false |

Full stdout/stderr and exit codes: `.local/verification/reviewed-build-1/`. The image ID is `1b33b20a90075390d3bf646f6e0cfe3cdc30a78d740383903aa2ef47a7d64df1`. Podman reported that OCI format ignores the image-level health declaration; the Compose file supplies its own healthcheck, and the verification script executed the health command.

The independent review found five important issues. Regression tests reproduced and verified fixes for clarification provenance/reuse, large-list clearing/capacity, retry ordering and grocery-tool context bounds. Another regression verified confirmation replay after a committed mutation.

Actual Haiku tests cost $0.0079504 for 29 requests across 14 scenarios. See [llm-results.md](llm-results.md) for recorded behavior and incomplete flows. Fixture success does not establish live AI readiness.

Coolify deployment and ten-minute host resource measurements have not run. The user redirected the task to recorded tests and cost reporting. No production workflow, webhook or container changed.

## Follow-up verification

Recorded race-suite output for the runner fixes is in `.local/verification/flow-runner-fix/` and `.local/verification/banana-probe/`. Both suites passed. New regressions cover following actual clarification labels, replying to text-only proposals, selecting only requested scenarios, and preserving a full-size PNG while estimating image cost from visual tokens instead of base64 text bytes.

Two saved-conversation continuations and the generated banana photo completed with application confirmation. `scripts/check_recorded_flows.py` compares the final rows with the actual recorded proposal, checks zero insertions before confirmation and confirms consumed pending state. Total OpenRouter cost is $0.0103365. See [llm-results.md](llm-results.md).
