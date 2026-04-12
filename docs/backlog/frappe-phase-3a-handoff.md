# Frappe Phase 3a — Handoff (per-series color binding)

**Status:** in-progress, uncommitted. Changes on disk, Grafana reloads them.
**Parent:** `docs/backlog/frappe-phase-3a.md`

## What's wired (uncommitted)

- `dev/otel/dashboards/claude-techdebt.json` panel #301:
  - datasource → `{type: "datasource", uid: "-- Mixed --"}`
  - target A (Prometheus) + target B (Infinity CSV, `parser: "backend"`)
  - `configFromData` transform, `configRefId: "B"`, mappings `color→color` and `name→field.name`
- `dev/otel/provisioning/datasources/infinity.yml` — `parser: backend` added to both global_queries
- `dev/otel/dashboards/claude-cfo.json` panel #401 — **not yet touched** (clone of #301 once #301 works)

Current result: both `coolant` and `shootsfilm` render mauve (#ca9ee6). The transform fires, but per-series matching fails, so the first row's color gets applied to all series as a fallback.

## Three discoveries (don't re-derive)

1. **Infinity `parser: "backend"`** is required for panel transforms. Default `"simple"` parses client-side in Explore but returns empty frames to backend consumers. Without this, `configFromData` can't see refId B at all — dropdown shows only A twice.
2. **Panel datasource must be `-- Mixed --`** (`{type: "datasource", uid: "-- Mixed --"}`) to allow per-query datasources. Otherwise Grafana silently drops queries whose datasource doesn't match the panel's. Query B never reaches the backend; no error surfaces.
3. **`configFromData` handler key for "Field name"** is literally `"field.name"` (confirmed by grepping Grafana 12 JS bundle). Matches against field NAME only, **not** `displayNameFromDS`. Prometheus timeseries-multi frames have field name = `"Value"` universally, so `field.name` matching never resolves to individual repos.

## The wall

`configFromData` + `field.name` handler requires the target field's literal `name` to equal the lookup key. Prometheus gives us `name: "Value", displayNameFromDS: "coolant"`. We need to reshape A so each series's data field is named `coolant` / `shootsfilm` before `configFromData` runs.

## Two untried paths

### Path 1: `prepareTimeSeries` transform (try first, cheap)
Grafana stock transform. Add before `configFromData`:
```json
{ "id": "prepareTimeSeries", "options": { "format": "multi" } }
```
"Multi-frame time series" is supposed to reshape so each series becomes its own frame with the field name derived from labels. If field name becomes `coolant` / `shootsfilm`, `field.name` matching resolves and each series gets its proper color. 5-minute test.

### Path 2: `byRegexp` override fed by CSV data (fallback, hackier)
Skip `configFromData`. In `fieldConfig.overrides`, add one override per row of the CSV using a `byRegexp` matcher against `displayNameFromDS`. Generate these overrides at dashboard-gen time from the CSV via `frappify.py`. Loses the "pure dashboard JSON, no recompile" elegance but guaranteed to work.

## Kickoff prompt for next session

```
Finish docs/backlog/frappe-phase-3a.md. Uncommitted changes are staged
on disk — Grafana hot-reloads them via file provisioning. Start by
reading docs/backlog/frappe-phase-3a-handoff.md for the three
discoveries and the per-series matching wall.

First attempt: add `prepareTimeSeries` transform with `format: "multi"`
before `configFromData` in panel #301 of
dev/otel/dashboards/claude-techdebt.json. Wait ~2s for Grafana's 10s
file poll to reload, then use playwright to render the panel and
grep the legend DOM for swatch colors.

Verification script already exists at /tmp/zoom_legend.mjs (uses
cached playwright at ~/.npm/_npx/e41f203b7505f1fb). Login is
admin/coolant at localhost:3000.

Success looks like:
  title="coolant"   → rgb(202, 158, 230)  (#ca9ee6 mauve)
  title="shootsfilm" → rgb(239, 159, 118) (#ef9f76 peach)

If path 1 fails, fall back to path 2 (byRegexp overrides generated
by frappify.py from the CSV). Once #301 works, clone the pattern
to panel #401 in claude-cfo.json (swap repo→organization_id,
repo_colors.csv→org_colors.csv).

After both panels bind, update frappify_audit.py to recognize
configFromData (or byRegexp overrides, per chosen path) as "themed",
re-run audit, commit via /commit.
```

## Verification commands

```bash
# Re-check target state on disk
curl -s -u admin:coolant http://localhost:3000/api/dashboards/uid/coolant-techdebt \
  | python3 -c 'import json,sys; d=json.load(sys.stdin)["dashboard"]; [print(json.dumps(p,indent=2)) for p in d["panels"] if p["id"]==301]'

# Test Infinity query directly (parser=backend should return fields, not empty)
curl -s -u admin:coolant -H 'Content-Type: application/json' \
  -X POST http://localhost:3000/api/ds/query \
  -d '{"queries":[{"refId":"B","datasource":{"type":"yesoreyeram-infinity-datasource","uid":"infinity"},"type":"csv","source":"url","parser":"backend","format":"table","url":"http://localhost:9091/repo_colors.csv","url_options":{"method":"GET"},"columns":[{"selector":"name","text":"name","type":"string"},{"selector":"color","text":"color","type":"string"}]}]}'

# Render + inspect legend (headless)
cd /tmp && node zoom_legend.mjs
```
