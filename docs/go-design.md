# Go thermal dashboard — design reference

Pull this in when working on widgets, rendering, or animation internals. The conventions that prevent compilation errors (bubbletea v2 API, lipgloss v2 types) live in CLAUDE.md — this doc covers *how the subsystems work*.

## Keyboard shortcuts

- `h` — toggle help overlay (replaces sparklines with plain-language explainers)
- `c` — collapse/expand the notification bar (top row with [i] install, etc.)
- `q` / `ctrl+c` — quit

## UI layers

- **Notification bar** (top, collapsible via `c`) — transient chrome: install CTA, future version alerts
- **Rates bar** (bottom, always visible) — system stats with colored dots + spawn/death/net + permanent `[h] help`
- **Status messages** — loaded from `thermal/internal/model/data/messages.csv` via `go:embed`, prefixed with `:: `

## Sparklines

Double-resolution braille: both columns of each character pack two time samples (left=N, right=N+1), doubling visible history. Raw data is linearly interpolated (midpoint insertion) before rendering to turn step-function transitions into visible ramps. Shared helpers `prepareSparkData()`/`prepareSparkMask()` handle zero-padding, interpolation, and visible window slicing.

**Height** is proportional (auto-scaled to visible window peak). **Color** encodes severity via go-colorful `BlendHcl()` (green->yellow->red, 24-bit truecolor). Values below 2% of peak render as invisible (noise floor).

**Edge fades** dim the outermost 3 characters on both left and right edges (35%->60%->82% brightness ramp) so data fades in on entry and fades out on exit. Uses `dimmedFg()` which blends severity color toward black via Lab space.

## Gauges

Harmonica spring physics (critically damped, 30fps) for smooth numeric readout easing. **Sparklines scroll at animation rate (30fps)**, not collector rate: each `AnimTick` pushes the spring-interpolated value into a per-gauge `renderHistory` buffer, and sparklines render from this buffer. This decouples scroll speed from data collection. Peak per gauge tracks the visible render-history window only with fast decay (0.982/tick at 30fps, ~1.3s half-life) — spikes that scroll off screen release the scale within ~1s.

## Render architecture

Collector samples at 150ms, animation tick runs at 30fps (~33ms). Springs interpolate between data arrivals (~4.5 frames per sample). `snapshotMsg` updates spring targets; `animTickMsg` advances springs AND pushes interpolated values into render history, driving sparkline scroll. The two rates are fully decoupled via separate bubbletea messages.

## CPU sampling

Caches the mach host port (avoids port leak) and holds the last computed CPU% when tick deltas are zero (avoids false 0% gaps at 150ms).
