# Spec stub: Frappe the entire Grafana chrome

**Status:** backlog / stub
**Seeded:** 2026-04-12
**Parent:** Phase 3a shipped panel-level theming; this is the deferred Phase 3b.

## Problem

After Phase 3a, panel interiors render Frappe (series colors, thresholds, overrides), but the Grafana chrome around them is still stock: sidebar, navbar, panel headers/borders, menu hover states, buttons, inputs, focus rings, tooltips. The contrast is jarring — Frappe panels sit inside a Grafana-blue/gray cage. User wants the whole surface cohesive, no "system colors" anywhere.

## Proposed approach

Two viable paths, pick after measuring how much surface area actually needs overrides:

**Path A — nginx `sub_filter` + stylesheet injection.** Front Grafana with a local nginx that injects a `<link rel=stylesheet>` pointing at a hand-rolled Frappe override CSS. Target stable `data-testid` attrs (Grafana's supported theming hook) for sidebar, panel header, menu hover, focus ring, scrollbar. Zero Grafana patching; survives upgrades better than forked themes.

**Path B — Grafana theme plugin (`grafana/grafana-theme-catppuccin-frappe` if one exists, otherwise fork).** Native theming via Grafana's internal theming primitives. Cleanest but pins us to whatever the plugin author maintains; out-of-box plugins for Catppuccin specifically were sparse as of April 2026 research.

Bias: start with Path A — we already know the surface, CSS-level override is surgical, and we can land it without leaving the dev/otel stack.

## Candidate surface (non-exhaustive, audit-driven)

- Sidebar (background, icon tint, active item)
- Top navbar + breadcrumbs
- Panel header bar + title text
- Panel menu (kebab dropdown)
- Legend table rows (hover/selected state)
- Tooltip chrome (border, arrow, background)
- Form inputs (time picker, variable dropdowns, search)
- Focus rings and selection highlights
- Scrollbars (webkit + firefox)
- Annotation / alert markers

## Open questions

- Are there user-settable base colors in `grafana.ini` that get us 60% without CSS (e.g., `ui.theme.primary`)? Check before writing overrides.
- Login page — do we bother? (Admin-only, rarely seen.)
- Dark/light toggle — Frappe is a dark palette; do we force theme to `dark` in `defaults.ini` and skip light styling entirely?
- Does nginx + Grafana's `root_url` rewriting cooperate when serving through a subpath?
- Print/export (PDF reports) — do they honor injected CSS?

## References

- Phase 3a plan: `docs/backlog/frappe-phase-3a.md` ("Not in scope" → "Chrome theming (phase 3b)")
- Research: `docs/enterprise-otel/Grafana 11 12 Theming  State of the Art for Catppuccin Frappé (April 2026).md`
- Frappe hex palette already codified in `dev/otel/scripts/frappify.py`
