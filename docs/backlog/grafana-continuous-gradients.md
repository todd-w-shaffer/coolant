# Spec stub: Evaluate continuous-by-value gradients for "by X" panels

**Status:** backlog / stub
**Seeded:** 2026-04-12

## Problem

Phase 3a theming went all-in on `palette-classic-by-name` + CSV-driven byName overrides. That path works well when the series set is *identity* (these specific repos, these specific models), but it breaks down for:

- **Unbounded cardinality** — insights #400 "Cost Rate by Session" can't be CSV-seeded; session_id is per-run. Currently the sole 🌈 leftover in the audit.
- **Ranking semantics** — "Think-to-Ship Ratio by Repo", "Hottest Sessions", "Heavy-Tail Session Audit" etc. communicate magnitude, not identity. A gradient *encodes* the rank; a per-name hex wastes the channel.

Grafana ships built-in continuous gradients (`continuous-BlPu`, `continuous-YlBl`, `continuous-RdYlGr`, `continuous-GrYlRd`, etc.) that render natively in Frappe-adjacent palettes and require zero CSV maintenance. Screenshot of #200 with `continuous-BlPu` looked great — never surfaced during the prior theming push because the mental model anchored on "per-name overrides."

## Proposed approach

1. Audit every panel currently using `palette-classic-by-name` + CSV overrides. Classify each as **identity** (keep byName) or **ranking** (flip to continuous gradient).
2. For ranking panels, pick a Frappe-harmonious built-in: `continuous-BlPu` (lavender→mauve) and `continuous-GrYlRd` (green→yellow→red) are the two that visually land closest.
3. Extend `frappify.py` with a `CONTINUOUS_GRADIENT_PANELS` table keyed by panel id → gradient mode, analogous to `BARE_COLOR_PANELS`.
4. Resolves insights #400 without needing a topk hack or Business Charts plugin.

## Open questions

- Is `continuous-BlPu` the right default, or do we want different gradients per semantic (cost = red-scale, ratio = blue-scale, throughput = green-scale)?
- The "Value" row visible in the #200 screenshot — separate bug or symptom of a query missing `by (repo)`?
- Do we keep byName overrides for model panels (opus/sonnet/haiku) since those *are* identity, or flip those too for consistency?

## References

- Prior Phase 3a work: palette-classic + CSV overrides, see `dev/otel/scripts/frappify.py`
- Audit: `dev/otel/frappe-audit.md` — one 🌈 remaining (#400)
- Screenshot 2026-04-12 21:51 — #200 rendered with `continuous-BlPu`, the trigger for this note
