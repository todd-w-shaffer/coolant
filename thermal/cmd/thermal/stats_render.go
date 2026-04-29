package main

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/stats"
	"github.com/toddwshaffer/coolant/thermal/internal/stats/format"
)

// renderStats writes the human-readable body to w. The aggregator is
// passed alongside the snapshot because Window(N), WindowByType /
// WindowByProject, and VisibleWindows aren't on Snapshot — every
// rendered surface needs the live aggregator handle in the same pass.
func renderStats(w io.Writer, agg *stats.Aggregator, snap stats.Snapshot, f statsFlags, r format.Renderer) {
	now := r.Clock()
	renderHeader(w, snap, r, now)
	fmt.Fprintln(w)
	renderRecords(w, snap, f, r, now)
	fmt.Fprintln(w)
	renderWindows(w, agg, snap, f, r, now)
	fmt.Fprintln(w)
	renderDistributions(w, agg, snap, f, r)
	if hasDiagnostics(snap) {
		fmt.Fprintln(w)
		renderDiagnostics(w, snap, r)
	}
}

func renderHeader(w io.Writer, snap stats.Snapshot, r format.Renderer, now time.Time) {
	first := format.FormatRelativeTime(snap.FirstSeen, now)
	last := format.FormatRelativeTime(snap.LastUpdated, now)
	title := r.Style(format.StyleHeader, "coolant stats")
	fmt.Fprintf(w, "%s — first seen %s · last update %s\n", title, first, last)
}

type recordSpec struct {
	label string
	kind  format.RecordKind
}

var recordBoards = []struct {
	spec recordSpec
	pick func(stats.Records) stats.RecordList
}{
	{recordSpec{"peak concurrent", format.KindCount}, func(r stats.Records) stats.RecordList { return r.PeakConcurrent }},
	{recordSpec{"longest agent", format.KindDuration}, func(r stats.Records) stats.RecordList { return r.LongestAgentS }},
	{recordSpec{"longest session", format.KindDuration}, func(r stats.Records) stats.RecordList { return r.LongestSessionS }},
	{recordSpec{"most agents/session", format.KindCount}, func(r stats.Records) stats.RecordList { return r.MostAgentsSession }},
	{recordSpec{"most tokens (agent)", format.KindCount}, func(r stats.Records) stats.RecordList { return r.MostTokensAgent }},
	{recordSpec{"most tool calls", format.KindCount}, func(r stats.Records) stats.RecordList { return r.MostToolCallsAgent }},
}

func renderRecords(w io.Writer, snap stats.Snapshot, f statsFlags, r format.Renderer, now time.Time) {
	fmt.Fprintln(w, r.Style(format.StyleSectionTitle, "records"))
	for _, b := range recordBoards {
		rows := format.FormatLeaderboard(b.spec.kind, b.pick(snap.Records), f.top, now)
		renderRows(w, b.spec.label, rows, r)
	}
	rows := format.FormatBurstLeaderboard(snap.Records.BiggestBurst, f.top, now)
	renderRows(w, "biggest burst", rows, r)
}

func renderRows(w io.Writer, label string, rows [][2]string, r format.Renderer) {
	if len(rows) == 0 {
		fmt.Fprintf(w, "  %-20s %s\n",
			label, r.Style(format.StyleDimmed, "(no records yet)"))
		return
	}
	for i, row := range rows {
		shownLabel := label
		if i > 0 {
			shownLabel = ""
		}
		fmt.Fprintf(w, "  %-20s %-7s %s\n",
			shownLabel,
			r.Style(format.StyleRecordValue, row[0]),
			r.Style(format.StyleRecordMeta, row[1]),
		)
	}
}

// windowsToShow resolves the user's --window flag into the list of
// window keys to render. Empty filter shows all visible + today +
// lifetime (§0.5 default).
func windowsToShow(agg *stats.Aggregator, filter string) []string {
	if filter != "" {
		return []string{filter}
	}
	out := []string{"today"}
	out = append(out, agg.VisibleWindows()...)
	hasAllTime := false
	for _, w := range out {
		if w == "alltime" || w == "lifetime" {
			hasAllTime = true
			break
		}
	}
	if !hasAllTime {
		out = append(out, "lifetime")
	}
	return out
}

func windowCounters(agg *stats.Aggregator, snap stats.Snapshot, key string, now time.Time) stats.Counters {
	switch key {
	case "today":
		// Anchor on `now` (Renderer-injected) rather than LastUpdated
		// so a snapshot captured before midnight UTC still routes to
		// the current day, and golden tests can pin the bucket.
		return snap.Daily[stats.DayKey(now)]
	case "alltime", "lifetime":
		return snap.Lifetime()
	case "7d":
		return agg.Window(7)
	case "30d":
		return agg.Window(30)
	case "60d":
		return agg.Window(60)
	case "90d":
		return agg.Window(90)
	default:
		return stats.Counters{}
	}
}

func renderWindows(w io.Writer, agg *stats.Aggregator, snap stats.Snapshot, f statsFlags, r format.Renderer, now time.Time) {
	fmt.Fprintln(w, r.Style(format.StyleSectionTitle, "windows"))
	for _, key := range windowsToShow(agg, f.window) {
		c := windowCounters(agg, snap, key, now)
		label := r.Style(format.StyleWindowLabel, format.FormatWindowLabel(key))
		fmt.Fprintf(w, "  %-12s %d started · %d completed · %d orphaned · %d sessions",
			label, c.AgentsStarted, c.AgentsCompleted, c.AgentsOrphaned, c.Sessions)
		if c.TranscriptBytesTotal > 0 {
			fmt.Fprintf(w, " · %s transcripts", format.FormatBytes(c.TranscriptBytesTotal))
		}
		if c.GateCapEvents > 0 {
			fmt.Fprintf(w, " · %d gate.cap", c.GateCapEvents)
		}
		fmt.Fprintln(w)
	}
}

type distRow struct {
	key      string
	lifetime int64
	last30d  int64
}

// collectDist builds rows from the union keyset of the two maps,
// sorted by lifetime desc with key asc on ties. The asc-on-ties
// rule places "__other" at the bottom of any count tier without a
// special-case branch.
func collectDist(lifetime, last30d map[string]int64) []distRow {
	rows := make([]distRow, 0, len(lifetime)+len(last30d))
	seen := make(map[string]struct{}, len(lifetime)+len(last30d))
	for k, v := range lifetime {
		rows = append(rows, distRow{key: k, lifetime: v, last30d: last30d[k]})
		seen[k] = struct{}{}
	}
	for k, v := range last30d {
		if _, present := seen[k]; present {
			continue
		}
		rows = append(rows, distRow{key: k, lifetime: 0, last30d: v})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].lifetime != rows[j].lifetime {
			return rows[i].lifetime > rows[j].lifetime
		}
		return rows[i].key < rows[j].key
	})
	return rows
}

// renderDistributions pairs lifetime by_type / by_project with a
// hardcoded 30-day window. The --window flag controls only the
// "windows" section above.
func renderDistributions(w io.Writer, agg *stats.Aggregator, snap stats.Snapshot, f statsFlags, r format.Renderer) {
	fmt.Fprintln(w, r.Style(format.StyleSectionTitle, "distributions (lifetime · last 30 days)"))

	for _, group := range []struct {
		title    string
		lifetime map[string]int64
		last30d  map[string]int64
	}{
		{"by type", snap.ByType, agg.WindowByType(30)},
		{"by project", snap.ByProject, agg.WindowByProject(30)},
	} {
		fmt.Fprintf(w, "  %s\n", group.title)
		rows := collectDist(group.lifetime, group.last30d)
		shown := f.top
		if shown > len(rows) {
			shown = len(rows)
		}
		for i := 0; i < shown; i++ {
			label, counts := format.FormatDistributionRow(rows[i].key, rows[i].lifetime, rows[i].last30d)
			fmt.Fprintf(w, "    %-20s %s\n",
				r.Style(format.StyleRecordValue, label),
				r.Style(format.StyleRecordMeta, counts),
			)
		}
		if extra := len(rows) - shown; extra > 0 {
			fmt.Fprintf(w, "    %s\n",
				r.Style(format.StyleDimmed, fmt.Sprintf("(%d more)", extra)))
		}
	}
}

func hasDiagnostics(snap stats.Snapshot) bool {
	lt := snap.Lifetime()
	return lt.DegradedWritesTotal > 0 || lt.AgentsOrphaned > 0
}

func renderDiagnostics(w io.Writer, snap stats.Snapshot, r format.Renderer) {
	lt := snap.Lifetime()
	fmt.Fprintln(w, r.Style(format.StyleSectionTitle, "diagnostics"))
	if lt.DegradedWritesTotal > 0 {
		fmt.Fprintf(w, "  %s %d\n",
			r.Style(format.StyleRecordMeta, "degraded writes (lifetime)"),
			lt.DegradedWritesTotal)
	}
	if lt.AgentsOrphaned > 0 {
		fmt.Fprintf(w, "  %s %d\n",
			r.Style(format.StyleRecordMeta, "agents orphaned (lifetime)"),
			lt.AgentsOrphaned)
	}
}
