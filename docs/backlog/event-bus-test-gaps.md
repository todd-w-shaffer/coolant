# Event Bus Test Gaps

Observations from the 2026-04-20 enrichment session. None are urgent — all code works and is integration-tested — but direct unit coverage would catch regressions earlier.

## Soon

1. **`--kitt-highscore` flag default and env var opt-out have no test.**
   `thermal/cmd/thermal/main.go:210,260`. No test verifies the default is `true` or that `COOLANT_KITT_HIGHSCORE=0` opts out.

2. **`_extract_escaped` has no direct unit test.**
   `scripts/common.sh:98-103`. Only integration-tested via agent-start/stop. Missing: backslash escaping, newline/tab, empty field, missing field.

3. **`project` derivation untested for edge cases.**
   `scripts/common.sh:114`. `${_agent_cwd##*/}` is tested for happy path but not: empty cwd, root `/`, trailing slash `/apps/coolant/`.

4. **`EventParallelEngaged` is a dead constant.**
   `thermal/internal/collector/events.go:17`. Defined in Go, never emitted by bash, has unreachable alert handler in `state.go:252`. Either implement or remove.

## Later

5. **`KITTMaxDots` cap not independently verified in prepareDots.**
   `thermal/internal/widgets/breathedots.go:142-144`. Test checks render width but doesn't verify sweep position is also capped.

6. **Epoch race window.**
   `thermal/internal/model/state.go:80` captures `time.Now()` at init, but event tailer starts ~100ms later in `main.Init()`. Near-zero practical risk but a historical stop landing in the gap would miscount by 1.
