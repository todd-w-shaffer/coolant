package main

import (
	"fmt"
	"io"
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

func renderRecords(w io.Writer, snap stats.Snapshot, f statsFlags, r format.Renderer, now time.Time) {
	fmt.Fprintln(w, r.Style(format.StyleSectionTitle, "records"))
	for _, b := range format.Boards() {
		rows := format.FormatLeaderboard(b.Kind, b.Pick(snap.Records), f.top, now)
		renderRows(w, b.Label, rows, r)
	}
	rows := format.FormatBurstLeaderboard(snap.Records.BiggestBurst, f.top, now)
	renderRows(w, format.BurstBoardLabel, rows, r)
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
	return format.WindowKeys(agg.VisibleWindows())
}

func windowCounters(agg *stats.Aggregator, snap stats.Snapshot, key string, now time.Time) stats.Counters {
	switch kind, days := format.ParseWindowKey(key); kind {
	case format.WindowToday:
		// Anchor on `now` (Renderer-injected) rather than LastUpdated
		// so a snapshot captured before midnight UTC still routes to
		// the current day, and pinned-clock tests can fix the bucket.
		return snap.Daily[stats.DayKey(now)]
	case format.WindowLifetime:
		return snap.Lifetime()
	case format.WindowDays:
		return agg.Window(days)
	default:
		return stats.Counters{}
	}
}

func renderWindows(w io.Writer, agg *stats.Aggregator, snap stats.Snapshot, f statsFlags, r format.Renderer, now time.Time) {
	fmt.Fprintln(w, r.Style(format.StyleSectionTitle, "windows"))
	for _, key := range windowsToShow(agg, f.window) {
		c := windowCounters(agg, snap, key, now)
		label := r.Style(format.StyleWindowLabel, format.FormatWindowLabel(key))
		fmt.Fprintf(w, "  %-12s %s\n", label, format.FormatWindowCounters(c))
	}
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
		rows := format.DistRows(group.lifetime, group.last30d)
		shown := f.top
		if shown > len(rows) {
			shown = len(rows)
		}
		for i := 0; i < shown; i++ {
			label, counts := format.FormatDistributionRow(rows[i].Key, rows[i].Lifetime, rows[i].Last30)
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
