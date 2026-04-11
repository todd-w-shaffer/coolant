# OTEL Self-Reporting: Product Design Review

## 1. Configuration UX

The existing config system is beautifully lenient — bare units, partial configs, forgiving coercion. OTEL config should inherit that spirit. The simplest possible enablement is a single key. The complex case grows naturally from there.

### Config block design

```toml
[otel]
endpoint = "localhost:4317"
```

That's it. That's the flip. One line enables gRPC OTLP export with sensible defaults (insecure transport, 15s export interval, no custom labels). Everything else is progressive disclosure:

```toml
[otel]
endpoint = "localhost:4317"
# protocol = "grpc"              # "grpc" (default) or "http"
# interval = "15s"               # export batch interval
# insecure = true                # default true; set false for TLS
# cert = "/path/to/client.pem"   # mTLS client cert
# key = "/path/to/client-key.pem"
# ca = "/path/to/ca.pem"

[otel.labels]
team = "platform"
environment = "staging"
host = "build-42"
```

The `[otel.labels]` sub-table is flat string-to-string. No arrays, no nesting. These become OTEL resource attributes attached to every metric. The platform team managing 200 machines sets `host` and `team` per-machine in config; the individual developer never touches this section.

### Env var precedence

Follow the existing pattern exactly: `OTEL_EXPORTER_OTLP_ENDPOINT` (the OTEL standard env var) > `config.toml [otel] endpoint` > disabled. This is critical — it means a platform team can set the endpoint via environment without touching config files at all. Container deployments, systemd units, launchd plists all work without generating per-host TOML.

Do NOT invent custom env var names. The OTEL spec already defines `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_CERTIFICATE`, `OTEL_EXPORTER_OTLP_HEADERS`, etc. Honor those. Users who already have OTEL-instrumented environments will have these set. Thermal should just work.

The only Coolant-specific env var worth supporting: `COOLANT_OTEL=0` as a kill switch. If a platform team sets the endpoint via environment but a developer needs to disable it on their machine temporarily, `COOLANT_OTEL=0` overrides everything. Without this, the developer would have to unset the env var, which might break other tools.

### Why config-file-first is right

OTEL configuration is inherently "set once, forget" — not something you toggle per-invocation. The config file is the correct home. Env vars exist for infrastructure override, not daily use.

## 2. CLI Integration

### No new flags for export configuration

Adding `--otel-endpoint`, `--otel-labels`, `--otel-cert` would be a mistake. The flag namespace is already clean and focused on visual behavior (`--theme`, `--animation`, `--demo`, `--kitt-highscore`). OTEL configuration is operational plumbing — it belongs in the config file or environment, not in CLI flags that would need to be typed (or scripted) on every launch.

### One diagnostic flag: `--otel-status`

```
$ thermo --otel-status
otel: exporting to localhost:4317 (grpc, insecure)
      labels: team=platform, environment=staging
      last export: 3s ago (14 metrics, 0 errors)
      status: healthy
```

This is a print-and-exit command (like `--list-themes`). It resolves the full config + env precedence chain, attempts a connection, and reports what it found. For the platform team deploying to 200 machines, this is the `curl -I` equivalent — you SSH in, run `thermo --otel-status`, and immediately know if metrics are flowing.

If OTEL is not configured:

```
$ thermo --otel-status
otel: disabled (no endpoint configured)
      set [otel] endpoint in ~/.config/coolant/config.toml
      or export OTEL_EXPORTER_OTLP_ENDPOINT
```

Helpful, not cryptic. Points you at the next step.

## 3. Dashboard Indicator

The thermal strip is 244x10 characters of carefully composed information density. Adding an OTEL indicator must cost nearly nothing.

### Proposal: single glyph in the rates line

The rates line currently ends with `[h] help`. When OTEL is active and healthy, prepend a single dimmed glyph before the help hint:

```
...  ⊙ Chrome  ↑ [h] help
```

The `↑` (upwards arrow) is dimmed to the same level as `[h] help`. It means "data is flowing up." It's one character. It uses the same dim styling as existing ambient indicators (`⊞ Desktop`, `⊙ Chrome`). A developer who doesn't know about OTEL will never notice it. A developer who just enabled OTEL will glance at the rates line and see confirmation.

When OTEL is configured but unhealthy (endpoint unreachable), the arrow dims further or changes to a muted warning color — but NEVER red. Red means thermal meltdown. OTEL export failure is not a thermal event.

When OTEL is not configured, nothing appears. Zero visual cost for the default case.

### What NOT to do

Do not add a status bar. Do not add text like "OTEL: OK". Do not use the notification bar (that's for plugin CTA). Do not put it in the headline. The dashboard's design language is "information IS color" — a colored glyph, not a label.

## 4. Error UX

### The cardinal rule: export failure never degrades the dashboard

OTEL export is a side-channel. If the endpoint goes down, the dashboard continues rendering at full fidelity. No error modals. No blocking retries. No jitter from failed network calls.

### Failure indication

When the OTEL exporter fails to connect or receives errors:

1. **First failure**: the `↑` glyph changes from dim white to dim amber (the theme's warn color, NOT the crit/red color). This is a "heads up," not an alarm.

2. **Sustained failure (>60s)**: the `↑` glyph pulses at the stale-breath rate (the same slow dim-bright cycle used for ghost agent dots). This reuses existing animation infrastructure and communicates "this thing is not dead, but it's not healthy either."

3. **Recovery**: the glyph returns to steady dim white. No fanfare.

### Alert log integration

On first failure and on recovery, push a single line to the scrolling alert log at the bottom of the dashboard:

```
otel: export failed (connection refused)
otel: export recovered
```

These scroll away naturally. They don't persist. They don't flash. They're the same visual weight as spawn-burst alerts.

### Logging

Write detailed OTEL errors to the coolant log file (`$TMPDIR/coolant-$USER.log`), not to stderr. The dashboard owns stderr for bubbletea rendering. Diagnostic details go where `thermo --otel-status` and log-file inspection can find them.

## 5. The "Flip a Switch" Moment

### Zero to metrics in three steps

**Step 1**: Start an OTLP collector. (This is outside Thermal's scope, but the docs should include a one-liner: `docker run -p 4317:4317 otel/opentelemetry-collector`.)

**Step 2**: Add one line to config:

```
mkdir -p ~/.config/coolant
echo '[otel]\nendpoint = "localhost:4317"' >> ~/.config/coolant/config.toml
```

**Step 3**: Restart thermo. (Or, if hot-reload ships — see below — just wait 5 seconds.)

The developer looks at the rates line, sees `↑`, and knows it's working. They open Grafana, import the pre-built dashboard, and see their machine's thermal data. Total elapsed time: under 2 minutes.

### Hot-reload: worth it, but not for v1

The existing `config.Load()` runs once at startup. Hot-reload means watching the config file (fsnotify or polling) and re-merging on change. This is valuable — restarting thermo means losing sparkline history and agent state. But it adds complexity to the initial implementation.

Ship v1 as restart-required. Add hot-reload in the next cycle. When hot-reload ships, the `↑` glyph appearing without a restart will be a delightful moment. But don't block the feature on it.

### What the developer feels

The experience should be: "I added a line, restarted, and it just worked." No key generation. No registration endpoint. No SDK initialization ceremony. The TOML config already proved this pattern — partial configs, lenient parsing, sane defaults. OTEL enablement should feel like setting `warm_pct = 70` feels: change a value, see the effect.

## 6. Grafana Dashboard Design Philosophy

The pre-built Grafana dashboards must feel like they belong to the same product as the terminal strip. This is not "we export metrics and someone else visualizes them." This is Thermal's web face.

### Dark background, always

Grafana's dark theme is the only option. The terminal dashboard renders on dark terminals. The Grafana dashboard should feel like looking at the same data through a different window, not a different product.

### Severity color palette from the active theme

The Classic theme's traffic-light gradient (green to amber to red) maps directly to Grafana's threshold coloring. Iron's blackbody (purple to magenta to amber) would require custom color overrides. Ship the Classic-derived palette first. Include the hex values in the dashboard JSON so they match exactly. A Grafana panel turning amber should be the same amber as a sparkline turning amber.

### Sparkline-heavy, not number-heavy

Grafana's default stat panels show a big number with a tiny sparkline. Invert that. Use time-series panels with the sparkline fill, minimal axis labels, and severity-colored thresholds. The developer should be able to glance at the Grafana dashboard from across the room and read the thermal state from color alone — the same "peripheral vision first" principle.

### Panel layout mirrors the terminal strip

The terminal strip has a clear visual hierarchy: headline (top), gauges (middle), rates (bottom). The Grafana dashboard should follow the same top-to-bottom flow: overall threat level at top, CPU/MEM/SWAP sparklines in the middle row, process category breakdown and agent activity at the bottom. A developer who knows the terminal layout should feel immediately oriented in the web layout.

### What to avoid

Do not add panels just because Grafana makes it easy. Every panel must answer a question the terminal strip already answers. The Grafana dashboard is not "more data" — it is "the same data, with history." The terminal shows the last 90 seconds. Grafana shows the last 24 hours. Same metrics, different time window. If a panel doesn't have a corresponding element in the terminal strip, question whether it belongs.

### Fleet view (the 200-machine case)

For the platform team, add a second dashboard: the fleet overview. This one IS different from the terminal strip — it shows aggregate threat levels across machines, sorted by severity. Think of it as a grid of thermal strips, each reduced to a single row: hostname, threat level, CPU, MEM. Color-coded. The platform engineer scans the grid, spots the red row, clicks through to that machine's detail dashboard. This is the enterprise value proposition: "which of my 200 build machines is about to melt?"
