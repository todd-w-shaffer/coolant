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

// TokenCollector polls Claude Code session transcript files, accumulates token
// usage, and computes a smoothed throughput rate. Designed for the slow loop —
// session files do not update at sub-second cadence.
//
// Dedupe is per-file via lastMsgIDs: Claude Code emits one row per content
// block within a single response, all sharing one message.id and repeating
// the same usage object. Only the first row per id is counted. Tracking
// only the LAST id per file is sufficient because rows from one response
// arrive consecutively.
type TokenCollector struct {
	acc           *TokenAccumulator
	offsets       map[string]int64
	lastMsgIDs    map[string]string
	projects      string // ~/.claude/projects/ (overridable for tests)
	lastTotal     int64
	lastInput     int64
	lastCacheCrt  int64
	lastCacheRead int64
	lastTick      time.Time
	cachedFiles   []string
	lastDiscov    time.Time
	window        cacheWindow
}

func NewTokenCollector() *TokenCollector {
	return &TokenCollector{
		acc:        NewTokenAccumulator(),
		offsets:    make(map[string]int64),
		lastMsgIDs: make(map[string]string),
		projects:   defaultProjectsDir(),
	}
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
func (tc *TokenCollector) Tick(now time.Time) TokenStats {
	if now.Sub(tc.lastDiscov) >= rediscoveryInterval {
		tc.cachedFiles = tc.activeSessionFiles(now)
		tc.lastDiscov = now
	}
	for _, path := range tc.cachedFiles {
		tc.scanFile(path)
	}

	s := &tc.acc.Stats
	total := s.InputTotal + s.OutputTotal + s.CacheCreateTotal + s.CacheReadTotal

	if !tc.lastTick.IsZero() {
		dt := now.Sub(tc.lastTick).Seconds()
		if dt > 0 {
			sample := float64(total-tc.lastTotal) / dt
			if sample < 0 {
				sample = 0 // defensive — should never happen within a stable source
			}
			// Raw per-tick rate. EMA smoothing was removed once the
			// sparkline landed: smoothing here would force the renderer
			// to display a decay tail (color decays via spring, amplitude
			// would not) because each history sample is the smoothed
			// scalar of that tick. The gauge spring handles visual easing
			// of step changes; the source signal needs to actually drop
			// to 0 between bursts for the bars to fall away.
			s.TokensPerSec = sample
		}

		readDelta := s.CacheReadTotal - tc.lastCacheRead
		totalDelta := (s.InputTotal - tc.lastInput) +
			(s.CacheCreateTotal - tc.lastCacheCrt) +
			readDelta
		tc.window.add(cacheDelta{read: readDelta, total: totalDelta})
		s.CacheHitRatio = tc.window.ratio()
	}

	tc.lastTotal = total
	tc.lastInput = s.InputTotal
	tc.lastCacheCrt = s.CacheCreateTotal
	tc.lastCacheRead = s.CacheReadTotal
	tc.lastTick = now
	s.ActiveSessions = len(tc.cachedFiles)
	return *s
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
