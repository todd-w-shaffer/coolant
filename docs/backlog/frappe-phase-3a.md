# Frappe Phase 3a — Infinity + CSV lookups for dynamic-series theming

**Status:** planned, not started
**Depends on:** commits `0e7b0d8` (phase 2 model panels) and `cff0cc1` (v12 alignment)
**Related:** `dev/otel/frappe-audit.md` (coverage report), `docs/enterprise-otel/Grafana 11 12 Theming...md` (research)

## Why

After phase 2, four panels still fall back to Grafana's classic rainbow:

- `claude-cfo` #401 "Cost by Organization" — `by (organization_id)`
- `claude-techdebt` #301 "Cost per Line Added — Daily Trend" — `by (repo)`
- `claude-insights` #400 "Cost Rate by Session" — `by (session_id)`, **unbounded**
- `claude-spend` #30, #41 — turn out to be mislabeled (see scope below)

`byName` overrides don't scale when series names are data. The durable pattern per the April 2026 research is: static CSV lookup of name→color, joined via Grafana's native **"Config from query results"** transform, served by the **Infinity** data source plugin. Pure dashboard JSON, no recompile, survives Grafana upgrades.

## Scope refinements after inspecting panels

- **cfo #401** (organization_id) — **Infinity target.**
- **techdebt #301** (repo) — **Infinity target.**
- **spend #30** (model) — NOT Infinity. Model-keyed but our `MODEL_KEYED_PANELS` registry missed it. Just add to frappify.py and re-run.
- **spend #41** (single `sum(...)`, no `by`) — NOT Infinity. Has byName overrides for "input"/"output" with Frappe colors already; default mode is `palette-classic`. Flip to `palette-classic-by-name` like the other bare panels.
- **insights #400** (session_id, truly unbounded) — **Skip 3a.** Session IDs are ephemeral UUIDs; a CSV lookup is nonsense. Accept the rainbow or revisit under 3c (ECharts).

**Real Infinity scope:** 2 panels.

## Architecture

1. Install Infinity data source plugin into the Homebrew Grafana install.
2. Seed two CSV files under version control: `dev/otel/lookups/org_colors.csv`, `dev/otel/lookups/repo_colors.csv`. Columns: `name,color`. Rotate through 8 Frappe accents: mauve, lavender, teal, sapphire, peach, green, sky, pink.
3. Provision Infinity data source via YAML pointing at the CSVs (`file://` URLs).
4. Modify the two target panels: add a second query (refId B) against Infinity returning `(name, color)`, then add a "Config from query results" transform routing the `color` column into `fieldConfig.color.fixedColor` by matching `name` against the series label.
5. Update `start.sh` preflight to ensure Infinity plugin is installed.
6. Fix the two bonus panels (spend #30, #41) via frappify.py at the same time.

## Files touched

- `dev/otel/lookups/org_colors.csv` — create
- `dev/otel/lookups/repo_colors.csv` — create
- `dev/otel/provisioning/datasources/infinity.yml` — create
- `dev/otel/start.sh` — add Infinity plugin preflight
- `dev/otel/scripts/frappify.py` — add `("claude-spend.json", 30)` to `MODEL_KEYED_PANELS`; add `("claude-spend.json", 41): "palette-classic-by-name"` to `BARE_COLOR_PANELS`
- `dev/otel/dashboards/claude-cfo.json` — rewrite panel #401 (second query + transform)
- `dev/otel/dashboards/claude-techdebt.json` — rewrite panel #301 (second query + transform)
- `dev/otel/scripts/frappify_audit.py` — recognize `configFromData` transform as "themed"

## Steps

1. **Bonus fixes first** (cheap, validates frappify.py still works end-to-end).
   - Edit the two registries in frappify.py.
   - Run `python3 dev/otel/scripts/frappify.py`, eyeball diff, re-run for idempotency.
   - Re-run audit; confirm count drops.

2. **Seed CSVs.** 8-color Frappe rotation. Seed `repo_colors.csv` with repos already appearing in local data (`coolant`, `claude-code`) plus placeholder rotation rows. Seed `org_colors.csv` with just a header + one placeholder since the dogfood data has no real orgs yet.

3. **Write Infinity provisioning YAML** pointing at both CSVs via `file://` URLs.

4. **Update `start.sh` preflight** — check `grafana cli plugins ls` for `yesoreyeram-infinity-datasource`, install to the Homebrew plugin dir if absent. Brew plugin dir is at `$(brew --prefix grafana)/share/grafana/data/plugins` — verify before committing.

5. **Wire up cfo #401.** Add refId B query against Infinity's `org_colors`, add "Config from query results" transform with `color` column → `color.fixedColor`, matched on `organization_id`. Written directly as JSON; dashboards are file-provisioned, not UI-edited.

6. **Wire up techdebt #301.** Same pattern with `repo_colors.csv`.

7. **Teach frappify_audit.py** to detect `configFromData` transforms and treat those panels as themed regardless of default `color.mode`.

8. **Run frappify.py + audit end-to-end.** Expected totals: ~80 themed / 2 classic (session + something leftover) / 10 default (row headers).

9. **Commit. User Ctrl-Cs stack and re-runs `start.sh`** — preflight installs Infinity, provisioning picks up the new data source, dashboards hot-reload, panels render Frappe colors.

## Known unknowns / risks

- **Infinity + `file://` URLs.** Grafana may block `file://` for security. Might need `allow_loading_unsigned_plugins` or a plugin-specific URL allowlist in `grafana.ini`. Will discover during implementation; fallback is inline CSV in the provisioning YAML itself (Infinity supports `source: inline`).
- **Config-from-query transform JSON shape.** Conceptually described; exact JSON will be pulled from a Grafana reference dashboard and iterated. Expect 1–2 tries to get the matcher right.
- **Brew plugin directory path.** May differ from default; will source correctly during step 4.

## Not in scope (defer)

- **insights #400 "Cost Rate by Session."** Truly unbounded; either accept rainbow or revisit via phase 3c (Business Charts / ECharts migration).
- **Chrome theming** (sidebar, navbar, panel backgrounds). That's phase 3b (nginx sub_filter + data-testid CSS). Gated on whether chrome clashes visibly after 3a lands.

## Verification

- Eyeball cfo #401 and techdebt #301 in browser — Frappe colors per org/repo.
- `frappify-audit.md` shows both panels as ✅ themed.
- Stack restart is clean; no Grafana errors about missing plugin or data source.
