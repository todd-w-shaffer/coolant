# Thermal Enterprise v2 — Platform Architect Findings

**Reviewer:** Independent platform architecture review
**Date:** 2026-04-11
**Inputs:** CLAUDE.md, origin-story.md, spec.md, cost-attribution.md, Perplexity data-sources reference, progressive value ladder (provided inline)

---

## 0. Summary of Position

The original spec was designed around a single assumption that is now
provably wrong: that Thermal would be the sole OTEL emitter on the
machine and that token/cost data was unreachable. The Perplexity research
demolishes both. Claude Code already emits per-request token counts, cost
in USD, model attribution, and session correlation via standard OTEL env
vars. Local JSONL session files carry the same data with zero
configuration.

This is not a setback — it is a massive architectural gift. Thermal does
not need to become a token-counting authority. It needs to become the
**enrichment layer** that fuses Claude Code's authoritative token/cost
signals with system state, gate intelligence, and agent lifecycle
semantics that Claude Code cannot observe.

The daemon architecture, the two-emitter model, and the progressive value
ladder are sound in principle. What follows are the specific risks,
design gaps, and concrete recommendations I found.

---

## 1. Progressive Value Ladder — Component Mapping

This is the architectural backbone. Every design decision should be
tested against: "does this break a rung?" If adding Rung 3 requires
backfilling something that should have existed at Rung 1, the ladder has
a gap.

### Rung 0: Free Thermal (plugin + dashboard)

**Components:** Existing open-source repo. Plugin hooks, gate system,
thermo TUI, process/system collector. Zero enterprise code.

**Status:** Ships today. No changes needed.

### Rung 1: Cost statusline (ambient per-session cost)

**Components:** Claude Code statusline (`claude-statusline/`) reads
session JSONL from `~/.claude/projects/<encoded-cwd>/<session-id>.jsonl`,
parses `usage` objects from assistant turns, multiplies by configurable
rates.

**Critical finding: This rung requires zero enterprise code.** The
session JSONL files are readable by any process. The statusline is
already a shell script. Adding cost display is a pure free-tier feature.

**Rate configuration:** A TOML block in `~/.config/coolant/config.toml`:

```toml
[cost]
# Defaults ship with the binary, updated each release.
# User overrides for enterprise contract rates.
input_per_mtok = 3.00
output_per_mtok = 15.00
cache_read_per_mtok = 0.30
cache_creation_per_mtok = 3.75
```

**Risk: Session JSONL path is not a stable API.** The path
`~/.claude/projects/<encoded-cwd>/<session-id>.jsonl` is undocumented as
a contract. Schema changes, path changes, or encoding changes will break
tailing. Mitigations:

1. Wrap JSONL discovery in a single function (`resolveSessionPath`) that
   can be patched independently.
2. Validate the `usage` schema on first parse; log a structured warning
   if fields are missing or renamed. Do not crash — degrade to "cost
   unavailable."
3. Ship a `CLAUDE_CODE_SESSION_DIR` override env var so users can point
   at a non-default location.
4. Pin the known-good schema version in code comments with the date it
   was verified.

**Risk: Rate staleness.** Anthropic changes pricing. If rates are baked
into a binary, users on old versions see wrong numbers silently.
Mitigations:

1. Defaults in the binary (updated each release).
2. Config file overrides (enterprise contract rates).
3. A `[cost] source = "config"` field that makes it explicit the user is
   overriding. When not set, the binary defaults apply.
4. Future: an optional HTTP fetch for current rates from a Thermal
   endpoint. Not needed for v1 — manual config + release updates are
   sufficient for launch.

**Recommendation:** Do not gate Rung 1 behind enterprise. It is the PLG
hook. Every developer who sees their session cost becomes a buyer for
fleet visibility. The cost statusline should ship in the open repo.

### Rung 2: Claude Code OTEL (fleet visibility, no Thermal enterprise)

**Components:** Standard Claude Code OTEL configuration. Two env vars:
`CLAUDE_CODE_ENABLE_TELEMETRY=1` and `OTEL_LOGS_EXPORTER=otlp` (plus
endpoint). Enterprise managed settings can push these via MDM.

**Critical finding: This rung delivers value with zero Thermal
enterprise involvement.** Claude Code's `api_request` log events carry
per-request `input_tokens`, `output_tokens`, `cache_read_tokens`,
`cache_creation_tokens`, `cost_usd`, `model`, `session.id`, and
`user.account_id`. A platform team can deploy an OTEL collector, push
two env vars, and have fleet-wide Claude usage in Grafana today.

**Thermal's role at Rung 2:** None, mechanically. But Thermal can add
value here through documentation, recommended Grafana dashboards for
Claude Code OTEL data, and collector configuration templates. This is
content, not code — ship it in the open repo as
`docs/fleet-otel-quickstart/`.

**Why this matters architecturally:** If Rung 2 works without Thermal
enterprise, then enterprise's value proposition must be clearly
differentiated. "We also emit OTEL" is not a differentiator when Claude
Code already does it. Enterprise's value is the *enrichment* — the
system-state + gate-intelligence signals that Claude Code cannot emit.

### Rung 3: Daemon fleet-wide (first moment enterprise code is required)

**This is the first rung that requires enterprise code.** The daemon
collects system state, tails JSONL for token enrichment, and emits
fused OTEL metrics that correlate token cost with machine state.

**Components:**
- Headless daemon binary (`thermald` or `thermo --daemon`)
- launchd plist / systemd unit for lifecycle management
- Session JSONL tailer (reuses Rung 1 discovery logic)
- Claude Code OTEL consumer (receives or tails Claude Code's signals)
- Enriched OTEL emitter (Thermal metrics + correlated token data)
- Unix domain socket for TUI connection

**This is the enterprise paywall.** Free thermo collects and displays.
Enterprise thermald collects, enriches, and exports. The MetricSink
interface in the open repo is the seam.

### Rung 4: Enterprise rates + fleet labels

**Components:** Config TOML `[otel.labels]` + `[cost]` blocks. Rate
overrides per enterprise contract. Fleet labels (`team`, `project`,
`cost_center`) attached as OTEL resource attributes.

**No new binary components.** This is configuration depth on top of Rung
3. Ship as documentation and config examples.

### Rung 5: Full integration (compliance, alerting, dashboards)

**Components:** Grafana dashboard JSONs, Prometheus alerting rules,
compliance documentation package. All shipped as artifacts in the
private repo.

---

## 2. Daemon Architecture

### What it runs

The daemon is a headless thermo: same collector goroutines (fast 150ms +
slow 1s + event tailer 500ms), same `AppState` computation, but with
the TUI replaced by:

1. **MetricSink** — OTEL export (the existing spec's `Record()` path)
2. **Session JSONL tailer** — new collector loop that watches active
   Claude Code session files for token/cost data
3. **IPC listener** — Unix domain socket serving `Snapshot` + enriched
   state to connected TUI clients

### Session JSONL tailing mechanics

This is the most architecturally delicate component. The daemon needs
to discover and tail active session JSONL files without Claude Code's
cooperation.

**Discovery:** Scan `~/.claude/projects/` for directories, then within
each find the most recently modified `.jsonl` file. A session is
"active" if its file was modified within the last 60 seconds (tunable).

**Tailing:** Same pattern as `events.go` — poll at interval, seek past
previously-read bytes, parse new lines, handle truncation. The existing
`TailEvents` pattern is directly reusable. Extract the core
offset-tracking logic into a generic `JsonlTailer` that both the coolant
event tailer and the session tailer use.

**Schema extraction:** Parse each line as JSON. Look for objects with a
`usage` key containing `input_tokens`/`output_tokens`. Ignore lines
that don't match. This is defensive — if Claude Code adds new event
types, the tailer ignores them.

**Multi-session:** The daemon must tail multiple session files
simultaneously (one per active Claude Code instance). Use a goroutine
per active session, managed by a session-discovery goroutine that
periodically scans the directory tree.

**File rotation:** Session files are per-session, not rotated. But a
developer may start/stop many sessions. The daemon should track which
files it has fully consumed and only actively tail files modified
recently.

**Risk: Encoded-cwd path format.** The `<encoded-cwd>` component of the
path is an implementation detail of Claude Code. If the encoding changes,
discovery breaks. Mitigation: discover by glob pattern
(`~/.claude/projects/*/*.jsonl`), not by reconstructing the encoding.

**Risk: Filesystem permission.** The daemon runs as the user. Session
files are user-owned. No permission issues on macOS. But if an admin
deploys the daemon as a different user (launchd system daemon vs. user
agent), it loses access. **Recommendation: Always deploy as a launchd
user agent (`~/Library/LaunchAgents/`), never a system daemon
(`/Library/LaunchDaemons/`).** Document this prominently.

### Process lifecycle

**launchd plist (macOS):**
```xml
<key>KeepAlive</key>
<true/>
<key>ThrottleInterval</key>
<integer>5</integer>
```

`KeepAlive` handles crash recovery — launchd restarts automatically.
`ThrottleInterval` prevents crash-loop storms (5 second minimum between
restarts).

**Log rotation:** Write to `~/Library/Logs/coolant/thermald.log`. Use
`os.O_CREATE|os.O_WRONLY|os.O_APPEND` with a size check at startup —
if the log exceeds 10MB, rotate to `.1` (keep one backup). Or better:
use `os.Stdout` and let launchd handle it via `StandardOutPath` +
`newsyslog.d` config.

**Graceful shutdown:** Trap SIGTERM (launchd's stop signal). Call
`MeterProvider.Shutdown()` with 5s context deadline. Flush final OTEL
batch. Close IPC listener. Exit 0.

**Health check:** The daemon should write a heartbeat timestamp to a
well-known path (`$TMPDIR/coolant-$USER.daemon.pid` or similar). The
TUI checks this to decide whether to connect to the daemon or collect
directly.

### systemd unit (Linux, future)

Same pattern. `Restart=on-failure`, `RestartSec=5s`.
`StandardOutput=journal`. `User=%i` for template instantiation.

---

## 3. IPC Between Daemon and TUI

### Protocol

Unix domain socket at `$TMPDIR/coolant-$USER.daemon.sock`.

**Why UDS over TCP:** No port conflicts, filesystem permission model,
zero network stack overhead, automatic cleanup on process exit (if using
abstract namespace on Linux; on macOS, explicit cleanup needed).

**Wire format:** Length-prefixed JSON. Each message is a 4-byte
big-endian length prefix followed by that many bytes of JSON.

**Why not gRPC:** Binary size (+8MB), schema ceremony, no benefit for a
single-machine, single-client IPC path. Length-prefixed JSON is
debuggable with `socat` and trivial to implement.

**Why not bare Snapshot struct:** The IPC payload should be a superset
of Snapshot. The daemon adds enrichment data (token counts, cost
accumulator, session attribution) that doesn't belong in the collector
layer. Define a `DaemonState` struct:

```go
type DaemonState struct {
    Snapshot   collector.Snapshot
    AppState   AppStateSummary  // threat, headroom, rates, agent counts
    Sessions   []SessionCost    // per-session token/cost accumulators
    OTELStatus OTELHealth       // last export time, error count
    DaemonUptime time.Duration
}
```

The TUI deserializes this and populates its `AppState` from the summary
rather than computing it locally. This means the TUI in daemon mode does
no collection and no computation — it is a pure renderer.

### Versioning

Include a `version` field in every IPC message. The TUI checks
compatibility:
- Same major version: proceed.
- Different major version: log warning, fall back to standalone mode.

**Recommendation:** Start at version 1. Reserve version 0 for the
development period. Bump major version only when fields are removed or
semantics change. Adding fields is backward-compatible.

### Fallback

When the daemon is not running (no socket, connection refused, stale
heartbeat):
1. TUI logs "daemon not detected, collecting directly" once at startup.
2. TUI runs the existing standalone collector path — identical to
   current free-tier behavior.
3. If the daemon comes online while the TUI is running, the TUI does
   NOT hot-switch. It continues in standalone mode until restarted.
   Hot-switching is complex and unnecessary — the TUI lifecycle is
   short (user launches it, looks, closes).

### Is Snapshot sufficient as the seam?

**No.** `Snapshot` is the collector's output, not the model's output.
The TUI needs `AppState` derivatives (threat level, smoothed counts,
headroom, agent fresh/stale split, personality quips). If the daemon
sends raw Snapshots, the TUI must run its own `AppState.Update()` loop
— duplicating computation and potentially diverging from the daemon's
view.

**The seam should be `DaemonState`** (or similar) that includes both
raw Snapshot data and computed AppState summaries. The TUI becomes a
thin renderer in daemon mode, thick standalone in solo mode.

---

## 4. Two-Emitter Model

### The scenario

Both Claude Code and Thermal emit OTEL to the same collector endpoint.
Claude Code emits `claude_code.*` metrics/logs. Thermal emits
`thermal.*` metrics.

### What correlates them

`session.id` is the join key. Claude Code attaches it to every signal.
Thermal's daemon can extract it from session JSONL file paths (the
filename is the session ID) and attach it to `thermal.*` metrics as a
resource attribute.

**Risk: session.id semantics.** Claude Code's `session.id` is a UUID
per conversation session. Thermal's `SessionTree.RootPID` is a process
tree. These are different abstractions. A single Claude Code session
spawns one root process which may have many descendants. The mapping is
1:1 in practice but not guaranteed — Claude Code could change its
process model. **Recommendation:** Correlate via `session.id` string
match, not PID derivation. The daemon learns session IDs from JSONL
filenames and from hook events (which carry `session_id` on stdin).

### Metric naming collisions

**No collisions exist.** Claude Code uses the `claude_code.*` namespace.
Thermal uses the `thermal.*` namespace. As long as Thermal never emits
metrics in the `claude_code.*` namespace, there is zero collision risk.

**Recommendation:** Add a unit test in the enterprise repo that asserts
all registered metric names start with `thermal.`. This is a
belt-and-suspenders guard against a developer accidentally duplicating a
Claude Code metric.

### What breaks

1. **Cardinality explosion if both emit `session.id` on metrics.**
   Claude Code's `OTEL_METRICS_INCLUDE_SESSION_ID` defaults to true.
   Thermal should NOT include `session.id` as a metric attribute — it
   should appear only on log events used for correlation queries. Metrics
   with unbounded session IDs cause Prometheus storage issues.

2. **Collector resource limits.** Two emitters means double the
   ingestion volume. For a typical developer machine this is negligible
   (Claude Code emits ~15 metrics at 60s intervals + log events at 5s;
   Thermal emits ~15 metrics at 10s intervals). At fleet scale (200
   devs), this is roughly 400 additional time series from Thermal. Well
   within any production Prometheus/Mimir capacity.

3. **Attribution confusion in dashboards.** A Grafana dashboard showing
   "cost by team" must know which emitter is authoritative for cost.
   Claude Code's `cost_usd` is ground truth. Thermal's
   `thermal.agents.duration_seconds` is a proxy. The dashboard must not
   accidentally sum both. **Recommendation:** Thermal should never emit
   a metric named `thermal.cost.usd` that could be confused with Claude
   Code's `claude_code.cost.usage`. Instead, Thermal's enrichment adds
   system-context dimensions to queries over Claude Code's cost data —
   via Grafana join queries, not duplicate metrics.

### Revised cost metric strategy

The original spec reserves `thermal.tokens.*` and `thermal.cost.usd` as
forward-compatible names. With the discovery that Claude Code already
emits these signals, **Thermal should not duplicate them.**

**Drop the reserved `thermal.tokens.*` and `thermal.cost.usd` metrics.**
Replace with:

| Metric | Type | Purpose |
|--------|------|---------|
| `thermal.session.cost_usd` | Float64Gauge | Latest accumulated cost from session JSONL — a convenience mirror for the statusline, NOT authoritative |
| `thermal.agents.duration_seconds` | Float64Counter | Agent-hours (unchanged, Thermal-authoritative) |
| `thermal.gate.time_saved_seconds` | Float64Counter | Gate ROI (unchanged, Thermal-authoritative) |

The `thermal.session.cost_usd` gauge is explicitly documented as derived
from Claude Code's session JSONL — a convenience for Thermal-only
dashboards. Fleet cost dashboards should query `claude_code.cost.usage`
directly.

---

## 5. Session JSONL Tailing — Risk Analysis

### The core risk

`~/.claude/projects/<encoded-cwd>/<session-id>.jsonl` is a local
artifact, not a documented API. Anthropic can change the path, the
encoding, the schema, or the existence of these files at any time.

### Severity: Medium-High

The Perplexity research confirms these files exist and carry token data
as of April 2026. Multiple third-party tools (fazm.ai, community
scripts) already parse them. But Anthropic has no obligation to maintain
compatibility.

### Mitigations (defense in depth)

1. **Prefer OTEL over JSONL.** The daemon should consume Claude Code's
   OTEL signals as the primary token data path when both are available.
   OTEL is a documented, supported integration surface. JSONL is the
   fallback for environments where OTEL is not configured.

2. **Schema validation on first parse.** If the `usage` object schema
   changes, log a structured error and stop attempting to parse token
   data from JSONL. Do not crash. Degrade to proxy-only metrics
   (agent-hours, gate ROI).

3. **Feature flag.** `[daemon] jsonl_tailing = true` in config. Allows
   disabling JSONL tailing entirely if it becomes problematic, without
   a binary update.

4. **Version-pin the known schema.** In code comments, document exactly
   which fields are expected and when they were last verified. Include
   a link to the authoritative reference.

### Alternative: Hook-based token ingestion

Instead of tailing JSONL directly, register a `Stop` hook that posts
session token totals to the daemon's local endpoint. This is a
*documented* integration surface.

**Pros:** Uses the official hooks API. Receives `transcript_path` and
`session_id` on stdin. Can parse the session JSONL at that moment
(single read, not continuous tail). Hook execution is synchronous and
reliable.

**Cons:** Only fires at end-of-turn (not per-request). The statusline
needs real-time cost, so hook-based is insufficient for Rung 1. For
the daemon (Rung 3+), end-of-turn granularity is acceptable for OTEL
export (10s batches anyway).

**Recommendation:** Use both.
- **Statusline (Rung 1):** Tail JSONL directly (real-time, per-request).
- **Daemon (Rung 3):** Prefer Claude Code OTEL signals. Fall back to
  hook-based ingestion (`Stop` hook → daemon HTTP endpoint). JSONL
  tailing as third fallback.

This gives three independent paths to token data, ordered by API
stability.

---

## 6. Build Pipeline

### Two repos, independent versioning

| Repo | Visibility | Versioning | Artifacts |
|------|-----------|-----------|-----------|
| `coolant` (open) | Public | Semantic versions: `v1.x.y` | `thermo-darwin-{arm64,amd64}` on GitHub Releases |
| `thermal-enterprise` (private) | Private | Semantic versions: `v1.x.y` (independent) | `thermald-darwin-{arm64,amd64}` on private Releases |

**The two version numbers are independent.** The open repo moves faster
(community contributions, theme additions, animation tweaks). The
private repo moves on enterprise customer cadence.

**Compatibility contract:** The `MetricSink` interface in the open repo
is the seam. Changing `MetricSink` is a breaking change that requires
coordinated releases. Additive changes (new fields on `AppState`,
new `Snapshot` fields) are backward-compatible.

**Go module dependency:** `thermal-enterprise` imports
`github.com/toddwshaffer/coolant/thermal` as a Go module. This pins
a specific version of the open repo. Enterprise builds test against a
pinned coolant version; upgrades are explicit.

### Integration tests

**Problem:** The enterprise repo imports the open repo as a module. How
do you test enterprise behavior against open-repo changes before they
merge?

**Solution:** CI in the open repo runs `go test ./...` (existing). CI
in the enterprise repo runs:

1. Standard tests: `go test -tags enterprise ./...`
2. Interface compliance: a test that instantiates `MetricSink` from the
   enterprise implementation and calls `Record()` with a synthetic
   `AppState` + `Snapshot`. This catches interface drift.
3. Cross-repo smoke: a GitHub Action in the enterprise repo that, on a
   schedule (nightly) or on-demand, checks out the latest `coolant/main`,
   replaces the module dependency, and runs the full test suite. This
   catches breaking changes in the open repo before they hit a release.

### Binary size

| Build | Estimated size | Key deps |
|-------|---------------|----------|
| Free (`thermo`) | ~15MB | bubbletea, lipgloss, harmonica, go-colorful |
| Enterprise (`thermo -tags enterprise`) | ~18-20MB | + OTEL SDK, protobuf |
| Daemon (`thermald`) | ~18-20MB | Same as enterprise, minus bubbletea (headless) |

**Recommendation:** The daemon should be a separate binary (`thermald`),
not a flag on thermo (`thermo --daemon`). Separate binaries have cleaner
lifecycle management (launchd runs a specific binary, not a binary with
a flag) and allow the daemon to exclude bubbletea/lipgloss dependencies
entirely.

---

## 7. Specific Risks and Recommendations

### Risk: Claude Code OTEL schema changes (beta traces)

The trace span schema (`claude_code.llm_request` attributes) is
explicitly beta. Span names and attributes may change between releases.
**Recommendation:** Do not depend on trace spans for any production data
path. Use log events (`api_request`) as the primary OTEL signal — these
are fully documented with enumerated field names.

### Risk: Max/Pro customers have no Admin API

The Claude Code Analytics API (`/v1/organizations/usage_report/claude_code`)
requires an Admin API key available only to Enterprise/API orgs. Max/Pro
stacked plans get nothing server-side.

**Implication:** For Startup/SMB customers, Thermal's daemon with JSONL
tailing or local OTEL consumption may be the *only* way to get fleet-wide
cost data. This is a strong value proposition — document it explicitly in
sales materials.

### Risk: OTEL collector as single point of failure

If the enterprise OTEL collector goes down, both Claude Code and Thermal
lose export capability. Claude Code drops batches silently. Thermal
drops batches with the `↑` glyph going amber.

**Recommendation:** The daemon should buffer to local disk on export
failure. A small WAL (write-ahead log) — 10MB cap, circular — that
replays on reconnection. This is a differentiator over Claude Code's
fire-and-forget export.

### Risk: Daemon socket cleanup on crash

On macOS, Unix domain sockets are filesystem entries. If the daemon
crashes without cleanup, the socket file persists. The next daemon
instance fails to bind.

**Mitigation:** On startup, check if the socket file exists. If it does,
attempt to connect — if connection refused, the previous daemon is dead.
Unlink the stale socket and proceed. This is a standard pattern.

### Risk: Multiple Claude Code instances with overlapping sessions

A developer may run multiple Claude Code instances in different terminal
tabs, each with their own session. The daemon must handle multiple
simultaneous session JSONL files. The current `TailEvents` pattern
handles a single file.

**Recommendation:** The session discovery goroutine maintains a map of
active session file paths → tailer goroutines. New files spawn new
tailers. Files not modified for 5 minutes have their tailers shut down.

---

## 8. Implementation Sequence (Revised)

Mapped to progressive value rungs. Each phase is a shippable checkpoint.

### Phase 0: Rung 1 Prerequisites (open repo, free tier)

1. Generic `JsonlTailer` extracted from `events.go` pattern.
2. Session JSONL discovery function (`resolveActiveSessions`).
3. Rate config TOML block in `config.toml` parser.
4. Statusline cost display (reads session JSONL, applies rates).
5. Tests: JSONL tailing with schema changes, rate calculation, multi-
   session discovery.

**Ships as:** Open repo release. Every Thermal user gets ambient cost.

### Phase 1: Rung 2 Content (open repo, documentation only)

6. `docs/fleet-otel-quickstart/` — Claude Code OTEL setup guide.
7. Grafana dashboard JSONs for Claude Code native OTEL (not Thermal
   metrics — pure Claude Code `api_request` events).
8. Collector configuration templates (Grafana Alloy, Datadog Agent,
   vanilla OTEL Collector).

**Ships as:** Open repo documentation. Platform teams can deploy
fleet-wide Claude observability with zero Thermal enterprise code.

### Phase 2: Enterprise Plumbing (private repo, Rung 3)

9. `MetricSink` interface in open repo (`internal/model/sink.go`).
10. Nil stub in open repo (`cmd/thermal/otel_stub.go`).
11. Enterprise `MetricSink` implementation in private repo.
12. System metrics + agent lifecycle + threat level + gate counters.
13. Daemon binary (`thermald`) — headless collector + OTEL emitter.
14. launchd plist template with KeepAlive.
15. Session JSONL tailer in daemon (reuses Phase 0 code).
16. Token enrichment: fuse session token data with Thermal system state.
17. Build tag isolation (`enterprise` tag).

### Phase 3: IPC + TUI Integration (private repo, Rung 3)

18. `DaemonState` struct definition.
19. UDS IPC server in daemon.
20. UDS IPC client in TUI (daemon mode).
21. Fallback logic: detect daemon, connect or collect standalone.
22. `--otel-status` diagnostic (reads daemon state if connected).

### Phase 4: Security Hardening (private repo, Rung 3-4)

23. Closed attribute enforcement with unit test.
24. Transport security (TLS enforcement, localhost exception, mTLS).
25. Config file permission checks.
26. Endpoint allowlisting via env.
27. Kill switch (`COOLANT_OTEL=0`).
28. Metric namespace unit test (all names start with `thermal.`).

### Phase 5: Fleet Polish (private repo, Rung 4-5)

29. Enterprise Grafana dashboards (Fleet Overview, Cost & Efficiency,
    Deep Dive) — these join `thermal.*` and `claude_code.*` metrics.
30. Alerting rule templates.
31. Fleet labels (`team`, `project`, `cost_center`) as resource attrs.
32. Enterprise rate overrides in config.
33. `↑` glyph indicator with failure states.
34. Compliance documentation package.

### Phase 6: Resilience (private repo, Rung 5)

35. Local WAL for export failure buffering.
36. Daemon health monitoring (heartbeat file, self-diagnostics).
37. Log rotation.
38. Nightly cross-repo integration test in CI.

---

## 9. What the Spec Should Change

1. **Drop `thermal.tokens.*` and `thermal.cost.usd` reserved metrics.**
   Claude Code is authoritative on tokens and cost. Thermal should not
   duplicate. Replace with `thermal.session.cost_usd` as an explicit
   convenience mirror, documented as derived.

2. **Add daemon architecture section.** The spec mentions "Invisible
   Thermal" but does not specify lifecycle management, IPC protocol,
   or the relationship between daemon state and TUI rendering.

3. **Add session JSONL tailing as a documented data path** with explicit
   risk analysis and fallback chain (OTEL > hooks > JSONL).

4. **Revise the cost attribution section.** The "blocked on external
   data source" framing is obsolete. Token data is available now via
   Claude Code OTEL and session JSONL. The question is not "can we get
   it" but "should we re-emit it or join against it in Grafana."

5. **Add two-emitter correlation section.** Document `session.id` as
   the join key, the cardinality risk of putting it on metrics, and the
   dashboard query patterns for joining `thermal.*` with
   `claude_code.*`.

6. **Split the implementation sequence by rung.** The current Phase 1-4
   sequence is organized by engineering concern (plumbing, security,
   polish, cost). It should be organized by customer value delivery
   (Rung 1, Rung 2, ...) so each phase ships independently useful value.

7. **Separate daemon binary from TUI.** `thermald` is not `thermo
   --daemon`. Different binary, different lifecycle, different release
   cadence.

8. **Add Startup/SMB value narrative.** The spec underweights Profile B.
   For Max/Pro customers, Thermal's daemon may be the only path to
   fleet-wide cost visibility. This is a selling point, not a
   limitation.

---

## 10. Open Questions for the Next Review

1. **Should the daemon consume Claude Code OTEL directly?** It could
   run a local OTEL receiver endpoint and have Claude Code export to
   `localhost:4318`, then re-export enriched signals to the fleet
   collector. This centralizes the token data path through the daemon.
   Pro: single emitter to fleet collector, simpler firewall rules. Con:
   adds a proxy hop, daemon becomes SPOF for token data.

2. **What is the daemon's behavior when Claude Code is not running?**
   It should still collect system stats and emit OTEL. The token
   enrichment fields are simply empty. This establishes a baseline for
   "machine health when Claude is idle."

3. **How does the install script change?** Today `install.sh` installs
   the thermo binary. Enterprise install needs to also install thermald,
   the launchd plist, and configure OTEL. Should this be a separate
   `install-enterprise.sh` or a flag on the existing script?

4. **What is the upgrade path?** When a new enterprise release ships,
   how does thermald get updated on 200 machines? MDM push of the new
   binary + `launchctl unload/load`? A self-updater? This is an
   operational concern that should be designed before Rung 3 ships.

5. **Should Rung 1 (statusline cost) also support Claude Code OTEL as
   a data source?** If Claude Code is already exporting OTEL locally,
   the statusline could query the local collector's Prometheus endpoint
   instead of tailing JSONL. This would be more stable but adds a
   dependency on a running collector.
