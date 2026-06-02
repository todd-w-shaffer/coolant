package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// ActiveSessionWindow defines how stale a session file's mtime can be before
// the file is considered inactive and ignored by the collector.
const ActiveSessionWindow = 60 * time.Second

// rediscoveryInterval throttles the glob+stat scan for active session files.
// Sessions don't appear or expire faster than this — running the full
// projects/*/*.jsonl scan every slow tick would burn syscalls for no gain on
// machines with many historical session dirs.
const rediscoveryInterval = 5 * time.Second

// cacheWindowSize is the number of slow-loop ticks (1s each) over which the
// displayed cache hit ratio is averaged. Long enough to absorb single-message
// noise; short enough that a fresh cache miss visibly moves the readout.
const cacheWindowSize = 30

// cacheDelta is one tick's contribution to the rolling cache-hit ratio.
type cacheDelta struct {
	read  int64 // cache_read_input_tokens delta over the tick
	total int64 // (input + cache_create + cache_read) delta over the tick
}

// cacheWindow is a ring buffer of recent per-tick deltas used to compute the
// rolling cache hit ratio displayed on the dashboard. The lifetime average
// stabilizes too high on long sessions ("locked at 100%"); a window of recent
// activity lets a cache miss show up in real time.
type cacheWindow struct {
	deltas [cacheWindowSize]cacheDelta
	pos    int
	full   bool
}

func (w *cacheWindow) add(d cacheDelta) {
	w.deltas[w.pos] = d
	w.pos++
	if w.pos == cacheWindowSize {
		w.pos = 0
		w.full = true
	}
}

func (w *cacheWindow) ratio() float64 {
	n := w.pos
	if w.full {
		n = cacheWindowSize
	}
	var sumRead, sumTotal int64
	for i := 0; i < n; i++ {
		sumRead += w.deltas[i].read
		sumTotal += w.deltas[i].total
	}
	if sumTotal == 0 {
		return 0
	}
	return float64(sumRead) / float64(sumTotal)
}

// Usage holds the four token counters emitted by Claude Code's transcript
// rows. Matches Anthropic's usage object shape.
type Usage struct {
	Input       int64 `json:"input_tokens"`
	Output      int64 `json:"output_tokens"`
	CacheCreate int64 `json:"cache_creation_input_tokens"`
	CacheRead   int64 `json:"cache_read_input_tokens"`
}

// TokenAccumulator tracks cumulative totals. Dedupe-by-message-id is the
// caller's responsibility — the collector enforces it per-file in scanFile.
// The accumulator is purely additive.
type TokenAccumulator struct {
	Stats TokenStats
}

func NewTokenAccumulator() *TokenAccumulator {
	return &TokenAccumulator{}
}

// Apply adds a usage to the running totals. CacheHitRatio is NOT updated here;
// it's derived from the recent-tick window in TokenCollector.Tick.
func (a *TokenAccumulator) Apply(u Usage) {
	a.Stats.InputTotal += u.Input
	a.Stats.OutputTotal += u.Output
	a.Stats.CacheCreateTotal += u.CacheCreate
	a.Stats.CacheReadTotal += u.CacheRead
}

// transcriptRow is the minimal shape we need from each assistant JSONL row.
type transcriptRow struct {
	Type    string `json:"type"`
	Message *struct {
		ID    string `json:"id"`
		Usage *Usage `json:"usage"`
	} `json:"message"`
}

// parseTranscriptLine returns (message_id, usage, true) for an assistant row
// carrying a usage object. Anything else returns ok=false.
func parseTranscriptLine(line []byte) (string, Usage, bool) {
	var row transcriptRow
	if err := json.Unmarshal(line, &row); err != nil {
		return "", Usage{}, false
	}
	if row.Type != "assistant" || row.Message == nil || row.Message.Usage == nil {
		return "", Usage{}, false
	}
	return row.Message.ID, *row.Message.Usage, true
}

// OTELTokenView is the OTEL fan-in surface TokenCollector consumes when
// configured via WithOTELView. Satisfied by *otel/cc.Adapter — declared
// here as a local interface (duck-typed) so the collector package
// doesn't gain a dependency on the cc package. Tests provide a fake.
type OTELTokenView interface {
	OTELTokens(now time.Time) (input, output, cacheCreate, cacheRead int64, ok bool)
	IsOTELLive(now time.Time) bool
	ObserveTokenSchemaDrift(field, ccVersion string)
}

// TokenCollector polls Claude Code session transcript files, accumulates token
// usage, and computes a smoothed throughput rate. Designed for the slow loop —
// session files do not update at sub-second cadence.
//
// Dedupe is per-file via lastMsgIDs: Claude Code emits one row per content
// block within a single response, all sharing one message.id and repeating
// the same usage object. Only the first row per id is counted. Tracking
// only the LAST id per file is sufficient because rows from one response
// arrive consecutively.
//
// When WithOTELView is supplied, Tick fans in OTEL throughput via an
// either-or merge: when OTELTokenView reports live + ok, OTEL totals
// replace the transcript baseline for the returned TokenStats; when
// stale or missing, transcript stays authoritative.
type TokenCollector struct {
	acc             *TokenAccumulator
	offsets         map[string]int64
	lastMsgIDs      map[string]string
	projects        string // ~/.claude/projects/ (overridable for tests)
	lastInput       int64
	lastOutput      int64
	lastCacheCrt    int64
	lastCacheRead   int64
	lastTick        time.Time
	cachedFiles     []string
	lastDiscov      time.Time
	window          cacheWindow
	lastActiveBytes int64   // sum of transcript file sizes (from offsets), last tick — drives PrettyTokensPerSec
	lastIOPerS      float64 // raw last per-tick input+output rate, carried across no-sample / source-flip ticks
	lastPrettyPerS  float64 // raw last per-tick chars÷4 rate from transcript byte growth

	otel             OTELTokenView
	lastSourceOTEL   bool   // tracks whether prior tick rendered OTEL — flip clamps deltas
	otelEverProduced bool   // sticky: once OTEL returned ok=true, lookup miss becomes drift
	driftFired       bool   // one-shot per process — never spam ObserveTokenSchemaDrift
	lastOTELDay      string // UTC "2006-01-02" of the last ok=true OTEL read — date roll vs. same-day miss
}

// TokenCollectorOption configures NewTokenCollector. Variadic so callers
// can pass zero options for the transcript-only baseline and tests get
// the same nil-safe path that runs when the OTEL receiver fails to bind.
type TokenCollectorOption func(*TokenCollector)

// WithOTELView attaches an OTEL fan-in source. When the view reports
// IsOTELLive + ok, its cumulative totals are authoritative for the
// emitted TokenStats; when stale, transcript is authoritative.
func WithOTELView(v OTELTokenView) TokenCollectorOption {
	return func(tc *TokenCollector) { tc.otel = v }
}

func NewTokenCollector(opts ...TokenCollectorOption) *TokenCollector {
	tc := &TokenCollector{
		acc:        NewTokenAccumulator(),
		offsets:    make(map[string]int64),
		lastMsgIDs: make(map[string]string),
		projects:   defaultProjectsDir(),
	}
	for _, opt := range opts {
		opt(tc)
	}
	return tc
}

func defaultProjectsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

// Tick scans active session files and returns updated TokenStats. Safe to call
// when no projects directory exists — returns zero-valued stats and the
// previous EMA rate decays toward zero.
//
// Active-file discovery is throttled to rediscoveryInterval; in between, the
// cached set is reused. This keeps the per-second cost bounded even when
// ~/.claude/projects/ holds hundreds of historical session directories.
//
// When WithOTELView is configured and the view reports IsOTELLive + ok,
// OTEL cumulative totals are authoritative — the transcript scan still
// runs (so the baseline is ready if OTEL goes silent), but the emitted
// TokenStats reports OTEL numbers. Naive sum would 2× foreground turns
// since both sources report the same assistant message.
func (tc *TokenCollector) Tick(now time.Time) TokenStats {
	if now.Sub(tc.lastDiscov) >= rediscoveryInterval {
		tc.cachedFiles = tc.activeSessionFiles(now)
		tc.lastDiscov = now
	}
	for _, path := range tc.cachedFiles {
		tc.scanFile(path)
	}

	src := &tc.acc.Stats

	// Sum transcript file sizes from the offsets map — scanFile already
	// advanced offsets[path] to the post-tail size for every active file,
	// so no separate os.Stat syscalls are needed. Drives PrettyTokensPerSec,
	// which is always transcript-derived, independent of the OTEL fan-in.
	var activeBytes int64
	for _, path := range tc.cachedFiles {
		activeBytes += tc.offsets[path]
	}

	// Either-or source select: OTEL authoritative when live + ok, else
	// transcript. A naive sum would 2× every foreground turn since both
	// sources report the same assistant message.
	useOTEL := false
	var out TokenStats
	if tc.otel != nil && tc.otel.IsOTELLive(now) {
		in, op, cC, cR, ok := tc.otel.OTELTokens(now)
		day := now.UTC().Format("2006-01-02")
		switch {
		case ok:
			tc.otelEverProduced = true
			tc.lastOTELDay = day
			useOTEL = true
			out.InputTotal = in
			out.OutputTotal = op
			out.CacheCreateTotal = cC
			out.CacheReadTotal = cR
		case tc.otelEverProduced && !tc.driftFired && day == tc.lastOTELDay:
			// Live receiver, lookup miss, previously produced, SAME UTC day —
			// strong signal of an attribute rename. Fire the drift gate once.
			// An empty lookup on a NEW UTC day is the per-day cumulative bucket
			// rolling over at midnight (a gap / no-data-yet), not a rename:
			// treat it as a baseline reset and leave the one-shot gate
			// available so a genuine later rename can still fire. Either way,
			// fall back to transcript for this tick.
			tc.otel.ObserveTokenSchemaDrift("token_lookup_miss", "")
			tc.driftFired = true
		}
	}
	if !useOTEL {
		out.InputTotal = src.InputTotal
		out.OutputTotal = src.OutputTotal
		out.CacheCreateTotal = src.CacheCreateTotal
		out.CacheReadTotal = src.CacheReadTotal
	}

	sourceFlipped := useOTEL != tc.lastSourceOTEL

	if !tc.lastTick.IsZero() {
		dt := now.Sub(tc.lastTick).Seconds()
		if dt > 0 {
			// PRTY: chars÷4 from transcript byte growth. Source-independent
			// (OTEL has no byte stream), so it is computed every tick — a
			// source flip doesn't disturb the transcript file growth. Includes
			// JSONL envelope overhead, so it overestimates pure-text tokens by
			// a constant factor — fine for a "visual feel" sparkline.
			bytesDelta := activeBytes - tc.lastActiveBytes
			if bytesDelta < 0 {
				bytesDelta = 0 // file rotated / deleted between ticks
			}
			tc.lastPrettyPerS = float64(bytesDelta) / 4.0 / dt

			// IO rate + cache window come from the authoritative source. IO
			// excludes cache_create + cache_read on purpose — the sparkline and
			// "io N/s" want fresh model traffic, not cached prefix re-reads. Raw
			// per-tick rate (no EMA): the gauge spring handles visual easing.
			// Skip on a source flip so a cross-source delta doesn't pollute the
			// rate or the cache window.
			if !sourceFlipped {
				ioSample := float64((out.InputTotal-tc.lastInput)+(out.OutputTotal-tc.lastOutput)) / dt
				if ioSample < 0 {
					ioSample = 0 // defensive — should never happen within a stable source
				}
				tc.lastIOPerS = ioSample

				readDelta := out.CacheReadTotal - tc.lastCacheRead
				if readDelta < 0 {
					readDelta = 0
				}
				inDelta := out.InputTotal - tc.lastInput
				if inDelta < 0 {
					inDelta = 0
				}
				ccDelta := out.CacheCreateTotal - tc.lastCacheCrt
				if ccDelta < 0 {
					ccDelta = 0
				}
				tc.window.add(cacheDelta{read: readDelta, total: inDelta + ccDelta + readDelta})
			}
		}
	}

	// Cumulative readouts are the since-launch TRANSCRIPT totals, not the
	// authoritative-source totals. The transcript accumulator only ever grows,
	// so these are naturally monotonic — they never step backward when the rate
	// source flips OTEL↔transcript, and never freeze when OTEL's per-day bucket
	// rolls empty at UTC midnight (OTEL is per-day cumulative; transcript is
	// since-launch). OTEL feeds the live RATE/sparkline above; the cumulative
	// glance-readout stays on the transcript so it can't regress or stall.
	out.WorkTotal = src.InputTotal + src.OutputTotal + src.CacheCreateTotal
	out.BillTotal = out.WorkTotal + src.CacheReadTotal

	out.IOTokensPerSec = tc.lastIOPerS
	out.PrettyTokensPerSec = tc.lastPrettyPerS
	out.CacheHitRatio = tc.window.ratio()
	out.ActiveSessions = len(tc.cachedFiles)

	tc.lastInput = out.InputTotal
	tc.lastOutput = out.OutputTotal
	tc.lastCacheCrt = out.CacheCreateTotal
	tc.lastCacheRead = out.CacheReadTotal
	tc.lastActiveBytes = activeBytes
	tc.lastTick = now
	tc.lastSourceOTEL = useOTEL
	return out
}

// activeSessionFiles returns session JSONL files modified within
// ActiveSessionWindow.
func (tc *TokenCollector) activeSessionFiles(now time.Time) []string {
	if tc.projects == "" {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(tc.projects, "*", "*.jsonl"))
	if err != nil {
		return nil
	}
	out := matches[:0]
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) <= ActiveSessionWindow {
			out = append(out, m)
		}
	}
	return out
}

// scanFile reads new lines from a session file. On first encounter it seeds
// the offset to the file's current size so pre-existing history is not
// replayed as live activity.
func (tc *TokenCollector) scanFile(path string) {
	off, seen := tc.offsets[path]
	if !seen {
		info, err := os.Stat(path)
		if err != nil {
			return
		}
		tc.offsets[path] = info.Size()
		return
	}
	tc.offsets[path] = TailFile(path, off, func(line []byte) {
		id, usage, ok := parseTranscriptLine(line)
		if !ok {
			return
		}
		if tc.lastMsgIDs[path] == id {
			return // multi-block response: same msg.id repeats with identical usage
		}
		tc.lastMsgIDs[path] = id
		tc.acc.Apply(usage)
	}, nil)
}
