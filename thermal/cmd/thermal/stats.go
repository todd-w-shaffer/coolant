package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"syscall"

	"github.com/charmbracelet/x/term"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/stats"
	"github.com/toddwshaffer/coolant/thermal/internal/stats/format"
)

// Exit codes — see spec §0.4.
const (
	statsExitOK      = 0
	statsExitIOError = 1
	statsExitFlag    = 2
)

// validColorModes lists the accepted --color values. Unknown values
// are flag-misuse → exit 2 with this list printed to stderr.
var validColorModes = []string{"auto", "never", "always"}

// staticWindowChoices is the literal set of windows users can pin
// via --window. The aggregator's VisibleWindows() output is dynamic
// (install-age-dependent) — we accept the union of every possible
// VisibleWindows() return plus the literals "today" and "lifetime".
var staticWindowChoices = []string{"today", "7d", "30d", "60d", "90d", "alltime", "lifetime"}

// statsFlags is the parsed shape of `thermo stats` flag input. Pulled
// out of runStats so flag parsing is independently testable and the
// no-Checkpoint contract has a clear input boundary.
type statsFlags struct {
	json   bool
	window string // empty = show all visible windows + today + lifetime
	color  string // "auto" / "never" / "always"
	top    int    // clamped to [1, RecordListCap]; clampedNote set when adjusted
}

// parseStatsFlags resolves args into statsFlags and any user-facing
// notes (e.g., "--top clamped to 5"). usageOut is the destination for
// `--help` body when the parser surfaces flag.ErrHelp.
//
// Returns (flags, clampNote, exitCode):
//   - exitCode == -1 means "continue execution"
//   - exitCode == 0  means "print and exit OK" (e.g., --help)
//   - exitCode == 2  means "flag misuse, exit 2"
func parseStatsFlags(args []string, stdout, stderr io.Writer) (statsFlags, string, int) {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	fs.SetOutput(stderr)
	// Suppress flag pkg's auto-Usage on -h. The `flag.ErrHelp` branch
	// below routes help to stdout per §0.7; on real parse errors we
	// print usage ourselves so stderr ordering is "error then usage."
	fs.Usage = func() {}

	var f statsFlags
	fs.BoolVar(&f.json, "json", false, "Emit raw Snapshot JSON for `jq` piping; suppresses stderr warnings.")
	fs.StringVar(&f.window, "window", "", "Restrict the windows section to one of: today, 7d, 30d, 60d, 90d, lifetime.")
	fs.StringVar(&f.color, "color", "auto", "Color mode: auto, never, always.")
	fs.IntVar(&f.top, "top", stats.RecordListCap, "Top-N rows for records and distributions; clamped to [1, RecordListCap].")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stdout, statsUsageText())
			return f, "", statsExitOK
		}
		// flag.Parse already wrote the error to stderr (via SetOutput).
		// Append usage so the user sees both pieces.
		fmt.Fprintln(stderr, statsUsageText())
		return f, "", statsExitFlag
	}

	// Normalize --window: lowercase + trim. Then validate against the
	// static choice set. Empty stays empty (= "all visible").
	f.window = strings.ToLower(strings.TrimSpace(f.window))
	if f.window != "" && !slices.Contains(staticWindowChoices, f.window) {
		fmt.Fprintf(stderr, "stats: unknown --window=%q (valid: %s)\n",
			f.window, strings.Join(staticWindowChoices, ", "))
		return f, "", statsExitFlag
	}

	if !slices.Contains(validColorModes, f.color) {
		fmt.Fprintf(stderr, "stats: unknown --color=%q (valid: %s)\n",
			f.color, strings.Join(validColorModes, ", "))
		return f, "", statsExitFlag
	}

	// Clamp --top loud-but-non-fatal per §0.8.
	var note string
	if f.top < 1 {
		note = fmt.Sprintf("stats: --top=%d clamped to 1\n", f.top)
		f.top = 1
	} else if f.top > stats.RecordListCap {
		note = fmt.Sprintf("stats: --top=%d clamped to %d\n", f.top, stats.RecordListCap)
		f.top = stats.RecordListCap
	}

	return f, note, -1
}

// runStats MUST NOT call Aggregator.Checkpoint(). A concurrent
// `thermo` dashboard already checkpoints every 30s; a second writer
// would race the os.Rename and corrupt the cache. The fold path
// below loads the cache via stats.New(cfg) (read-only) and replays
// JSONL on top in-memory.
func runStats(stdout, stderr io.Writer, args []string, cfg stats.Config) int {
	f, clampNote, code := parseStatsFlags(args, stdout, stderr)
	if code >= 0 {
		return code
	}
	if clampNote != "" {
		// Clamp note is a flag-input issue, not data; --json must NOT
		// suppress it.
		fmt.Fprint(stderr, clampNote)
	}

	agg, folded, err := foldSnapshot(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "stats: %v\n", err)
		return statsExitIOError
	}

	// Skewed-install: JSONL has bytes but zero schema:1 events. Surface
	// to stderr per §0.10. Suppressed under --json (§0.3) so machine
	// consumers get a clean stream.
	if !f.json && cfg.JSONLPath != "" && folded == 0 {
		if info, statErr := os.Stat(cfg.JSONLPath); statErr == nil && info.Size() > 0 {
			fmt.Fprintln(stderr,
				"warning: stats engine sees 0 schema:1 events in JSONL — "+
					"bash hooks may be pre-schema. Run install.sh --upgrade to refresh.")
		}
	}

	snap := agg.Snapshot()
	if f.json {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(snap); err != nil {
			// SIGPIPE on stdout is normal Unix flow (`thermo stats |
			// head -5`). Per §0.4 those exit 0; only genuine I/O
			// failure exits 1.
			if errors.Is(err, syscall.EPIPE) {
				return statsExitOK
			}
			fmt.Fprintf(stderr, "stats: %v\n", err)
			return statsExitIOError
		}
		return statsExitOK
	}
	r := format.Renderer{Plain: resolvePlainMode(f.color, stdout)}
	renderStats(stdout, agg, snap, f, r)
	return statsExitOK
}

// resolvePlainMode picks color-vs-plain per --color flag. `always`
// forces ANSI even when piped; `never` forces plain; `auto` honors
// NO_COLOR / TERM=dumb / non-TTY stdout.
func resolvePlainMode(mode string, stdout io.Writer) bool {
	switch mode {
	case "never":
		return true
	case "always":
		return false
	}
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return true
	}
	if f, ok := stdout.(*os.File); ok {
		return !term.IsTerminal(f.Fd())
	}
	return true
}

// foldSnapshot loads the durable cache (read-only via stats.New) and
// replays JSONL on top in-memory. Mirrors runStatsdump.go:23-49 but
// returns the live aggregator so the caller can render either text or
// JSON without re-folding. Per §0.2 / §3.4 the helper MUST NOT call
// Checkpoint.
func foldSnapshot(cfg stats.Config) (*stats.Aggregator, int, error) {
	agg := stats.New(cfg)
	if cfg.JSONLPath == "" {
		return agg, 0, nil
	}
	f, err := os.Open(cfg.JSONLPath)
	if err != nil {
		if os.IsNotExist(err) {
			return agg, 0, nil
		}
		return agg, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var folded int
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev collector.GateEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Schema >= 1 && ev.Schema <= stats.MaxKnownSchema {
			folded++
		}
		agg.Fold(ev, 0)
	}
	// scanner.Err is non-nil for token-too-long (>1MB line) or read
	// faults — surface so callers exit 1 rather than silently
	// truncating the dump.
	if err := scanner.Err(); err != nil {
		return agg, folded, err
	}
	return agg, folded, nil
}

func statsUsageText() string {
	return `Usage: thermo stats [flags]

Print a one-shot summary of cross-session aggregates from
~/.coolant/stats.json.

Flags:
  --json            Emit Snapshot as JSON (for jq); stable across runs.
  --window=W        Restrict windows section to one of:
                    today, 7d, 30d, 60d, 90d, lifetime.
  --color=MODE      auto (default), never, always.
                    auto: color when stdout is a TTY; honors NO_COLOR
                    and TERM=dumb. always: color even when piped (use
                    with less -R). never: plain regardless.
  --top=N           Top-N rows in records & distributions.
                    Clamped to [1, RecordListCap].
  --help            Print this message.`
}
