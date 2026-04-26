// Package format renders stats values for human-readable surfaces.
// The package is the cross-consumer formatting contract — `thermo
// stats`, the future scoreboard widget, and the OTEL exporter all
// build their output through these helpers so unit tiers, tier
// breakpoints, and zero-state glyphs stay coherent across surfaces.
//
// All time math is UTC (per stats engine convention). Formatters are
// pure: no globals, no I/O, no env reads. Color and width concerns
// flow through the `Renderer` struct (see `color.go`).
package format

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/stats"
)

// dashGlyph is the canonical zero-state. Surfaces a missing/empty
// value without misleading "0" semantics (a real zero is meaningful;
// missing data isn't).
const dashGlyph = "—"

// billionCapThreshold is the value at or above which FormatCount
// emits "999B+". Anything ≥ 999.5B rounds to "1000.0B" with one
// decimal, which would need a fourth integer digit we don't render.
const billionCapThreshold int64 = 999_500_000_000

// FormatCount renders an integer with tier contraction. Tier table:
//
//	<1_000        → "0".."999" (no contraction)
//	<1_000_000    → "1.0K" / "12K" / "999K"
//	<1_000_000_000 → "1.0M" / "12M" / "999M"
//	>=1B          → "1.0B" / "12B" / "999B"
//	>=999.5B      → "999B+"
//
// Decimal slot only at single-digit prefix (1.5K). At ≥10 the prefix
// truncates to integer (12K, 999K). Rounded half-away-from-zero via
// Go's %.1f.
func FormatCount(n int64) string {
	if n < 0 {
		n = 0
	}
	if n < 1000 {
		return strconv.FormatInt(n, 10)
	}
	if n >= billionCapThreshold {
		return "999B+"
	}
	switch {
	case n < 1_000_000:
		return contractTier(n, 1_000.0, "K")
	case n < 1_000_000_000:
		return contractTier(n, 1_000_000.0, "M")
	default:
		return contractTier(n, 1_000_000_000.0, "B")
	}
}

func contractTier(n int64, divisor float64, suffix string) string {
	q := float64(n) / divisor
	if q >= 10 {
		return fmt.Sprintf("%d%s", int64(q), suffix)
	}
	return fmt.Sprintf("%.1f%s", q, suffix)
}

// FormatDuration renders seconds as the most-significant two units.
// Tier breakpoints: 60s → m, 60m → h, 24h → d. Zero-remainder cases
// drop the second unit (60 → "1m", not "1m 0s").
func FormatDuration(seconds int64) string {
	if seconds <= 0 {
		return "0s"
	}
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		m := seconds / 60
		s := seconds % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm %ds", m, s)
	}
	if seconds < 86400 {
		h := seconds / 3600
		m := (seconds % 3600) / 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh %dm", h, m)
	}
	d := seconds / 86400
	h := (seconds % 86400) / 3600
	if h == 0 {
		return fmt.Sprintf("%dd", d)
	}
	return fmt.Sprintf("%dd %dh", d, h)
}

// FormatBytes renders byte counts as binary-prefixed compact strings.
// Always one decimal place above 1 KB; integer-only at the B tier.
func FormatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	const (
		kb = 1024
		mb = 1024 * 1024
		gb = 1024 * 1024 * 1024
	)
	switch {
	case n < mb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	case n < gb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	}
}

// FormatTokens renders a (tokens_in, tokens_out) pair as a compact
// dual-arrow string. Both zero collapses to the dash glyph.
func FormatTokens(in, out int64) string {
	if in == 0 && out == 0 {
		return dashGlyph
	}
	return fmt.Sprintf("%s↑ · %s↓", FormatCount(in), FormatCount(out))
}

// FormatDayCompare renders a "today vs best day" comparator. bestDate
// is a dayKey() string ("2006-01-02"); the helper formats it for
// display. When today exceeds best, the "new best" framing flips the
// preposition so the user sees their own progress.
func FormatDayCompare(today, best int64, bestDate string) string {
	if today == 0 && best == 0 {
		return dashGlyph
	}
	dateStr := bestDate + " (UTC)"
	if t, err := time.Parse("2006-01-02", bestDate); err == nil {
		dateStr = t.Format("Jan 2") + " (UTC)"
	}
	if today > best {
		return fmt.Sprintf("%d today (new best, prev: %d, %s)", today, best, dateStr)
	}
	return fmt.Sprintf("%d today (best: %d, %s)", today, best, dateStr)
}

// RecordKind switches the value formatter inside FormatRecord. A
// typed enum (rather than a magic string) makes call sites
// compile-check the kind and the IDE autocomplete the available
// options.
type RecordKind int

const (
	KindCount RecordKind = iota
	KindDuration
)

func shortSessionID(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[:4]
}

func formatRecordValue(kind RecordKind, v int64) string {
	if kind == KindDuration {
		return FormatDuration(v)
	}
	return FormatCount(v)
}

// joinMeta joins non-empty parts with " · ". Empty parts collapse so
// a session-scoped record (no AgentType, no Project) doesn't render
// as "·  · session abcd".
func joinMeta(parts ...string) string {
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return dashGlyph
	}
	return strings.Join(out, " · ")
}

// FormatRecord renders one RecordEntry as (value, meta). The meta
// string drops empty optional fields cleanly so session-scoped
// records don't carry stub separators.
func FormatRecord(kind RecordKind, e stats.RecordEntry, now time.Time) (value, meta string) {
	value = formatRecordValue(kind, e.Value)
	var sessionPart string
	if e.SessionID != "" {
		sessionPart = "session " + shortSessionID(e.SessionID)
	}
	relTime := FormatRelativeTime(e.At, now)
	if relTime == dashGlyph {
		relTime = ""
	}
	meta = joinMeta(e.AgentType, e.Project, sessionPart, relTime)
	return value, meta
}

// FormatBurstRecord renders a BurstRecord as (count, meta). Meta
// includes the burst window ("in 2s") plus session + relative time.
func FormatBurstRecord(b stats.BurstRecord, now time.Time) (value, meta string) {
	value = FormatCount(b.Count)
	windowPart := fmt.Sprintf("in %ds", b.WindowS)
	var sessionPart string
	if b.SessionID != "" {
		sessionPart = "session " + shortSessionID(b.SessionID)
	}
	relTime := FormatRelativeTime(b.At, now)
	if relTime == dashGlyph {
		relTime = ""
	}
	meta = joinMeta(windowPart, sessionPart, relTime)
	return value, meta
}

// FormatLeaderboard renders top-n entries from a RecordList as
// (value, meta) pairs. Caller clamps `n` to RecordListCap; this
// helper only clamps to len(rl).
func FormatLeaderboard(kind RecordKind, rl stats.RecordList, n int, now time.Time) [][2]string {
	if n > len(rl) {
		n = len(rl)
	}
	if n <= 0 {
		return nil
	}
	rows := make([][2]string, n)
	for i := 0; i < n; i++ {
		v, m := FormatRecord(kind, rl[i], now)
		rows[i] = [2]string{v, m}
	}
	return rows
}

// FormatBurstLeaderboard is the BurstRecordList sibling of
// FormatLeaderboard. Same clamp semantics.
func FormatBurstLeaderboard(rl stats.BurstRecordList, n int, now time.Time) [][2]string {
	if n > len(rl) {
		n = len(rl)
	}
	if n <= 0 {
		return nil
	}
	rows := make([][2]string, n)
	for i := 0; i < n; i++ {
		v, m := FormatBurstRecord(rl[i], now)
		rows[i] = [2]string{v, m}
	}
	return rows
}

// FormatDistributionRow renders one entry from a by_type or
// by_project map as (label, "lifetime · last30d"). Caller pre-sorts
// and pre-clamps; this helper just shapes the row.
func FormatDistributionRow(key string, lifetime, last30d int64) (label, counts string) {
	return key, fmt.Sprintf("%d · %d", lifetime, last30d)
}

// FormatRelativeTime renders a past timestamp relative to now. Tier
// table:
//
//	t.IsZero()       → "—"
//	t > now          → "—" (records are always past; future means clock skew)
//	delta < 60s      → "just now"
//	delta < 60m      → "Nm ago"
//	delta < 24h      → "Nh ago"
//	delta < 7d       → "Nd ago"
//	delta < 30d      → "Nw ago"
//	delta >= 30d     → "Mon DD" (UTC date)
//
// All math uses .UTC() on both sides — caller-supplied `now` may be
// any zone; the renderer normalizes.
func FormatRelativeTime(t, now time.Time) string {
	if t.IsZero() {
		return dashGlyph
	}
	tu := t.UTC()
	nu := now.UTC()
	if tu.After(nu) {
		return dashGlyph
	}
	delta := nu.Sub(tu)
	switch {
	case delta < time.Minute:
		return "just now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm ago", int64(delta/time.Minute))
	case delta < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int64(delta/time.Hour))
	case delta < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int64(delta/(24*time.Hour)))
	case delta < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int64(delta/(7*24*time.Hour)))
	default:
		return tu.Format("Jan 2")
	}
}

// FormatWindowLabel maps a window key to its human label. Both
// "alltime" and "lifetime" map to "lifetime" so callers can pass
// either the aggregator's VisibleWindows() output or the literal
// "lifetime" sentinel without branching.
func FormatWindowLabel(key string) string {
	switch key {
	case "today":
		return "today (UTC)"
	case "7d":
		return "7 days"
	case "30d":
		return "30 days"
	case "60d":
		return "60 days"
	case "90d":
		return "90 days"
	case "alltime", "lifetime":
		return "lifetime"
	default:
		return key
	}
}
