# Spec stub: what fills the headline top-row real estate

**Status:** backlog / stub
**Seeded:** 2026-04-12

## Problem

Quips are being removed from the headline top row (matured out — the bloom layer replaces the personality function). This frees valuable real estate: the full width of the top row minus the right-anchored cluster (SESS/BLD stack, agents count, double-height LCD).

Initial reflex was to fill it immediately with stat clusters (CTX/WK/$/hr/TOK/s type displays). User pushback: this needs its own brainstorm round. The top row is prime summary territory — whatever goes there should be the *single most calculated, useful thing* to see at a glance, not a dashboard dump. Activity-level detail (spawn rate, decomp count, raw throughput) is zoom-in territory — users pull *up* into the app to see that, not down at the always-on strip.

## Constraints already decided

- **Two-row layout stays.** Top row stays reserved even if blank initially.
- **Right cluster unchanged.** Sessions/build-shell stack, agents count, double-height LCD all keep their current positions.
- **Bottom row stays runtimes.** `37m · 1.4M ctx · 12 agents · bld:2 sh:1` pattern is locked.
- **Not T2 material.** ΔLOC / SPAWN / DECOMP / NET-style activity indicators are explicitly off the table — too granular for always-on real estate.

## Candidate directions (for the real brainstorm)

- **Budget / economic signal** — something that answers "am I burning money I shouldn't be?" at a glance. Cost velocity, budget headroom, weekly trajectory.
- **Narrative state summary** — a calculated one-liner about current state: "Building rapidly · low risk" or "Stalling · recoverable." Not a quip, a derived signal.
- **Goal/intent marker** — what the session is currently FOR. "Feature: headline-bloom" or a git-branch-adjacent identifier.
- **Enterprise/team signal** — once Thermal Cloud lands: team load, active collaborators, queued handoffs.

## Open questions for the real brainstorm

- What's the single most-calculated thing the user wants to see? ("Calculated" = derived, not raw.)
- How often does the value meaningfully change? (Once per session? Per minute? Per tick? Drives whether it's a "set it and forget it" slot or a live display.)
- Does this slot belong to the OSS tool or to Thermal Cloud's enterprise overlay? If cloud-only, it stays blank in the plugin.
- Should the slot be *bound* to the bloom's intensity signal, or is the bloom one thing and the stat another?

## References

- `thermal/internal/widgets/headline.go` — two-row layout, right cluster rendering
- This spec should be brainstormed after the current thermographic-accent-layer spec lands, since the bloom's reading depends on knowing what it's washing over
- Adjacent surface: `claude-statusline/` already renders CTX / session / weekly bars in braille — top-row content should NOT duplicate that signal
