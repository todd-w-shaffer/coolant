package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/stats"
)

// runStatsdump folds the entire JSONL file into an aggregator (loaded
// from cache, then incrementally extended) and writes the resulting
// snapshot as pretty-printed JSON to w. Hidden dev/debug subcommand —
// distinct from a future user-facing `thermo stats` (separate spec).
//
// Returns the number of events folded plus any I/O error. A zero fold
// count against a non-empty JSONL is the skewed-install signal —
// caller surfaces it to stderr.
func runStatsdump(w io.Writer, cfg stats.Config) (folded int, err error) {
	agg := stats.New(cfg)

	if cfg.JSONLPath != "" {
		f, ferr := os.Open(cfg.JSONLPath)
		if ferr == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			// Match events.go default buffer size for parity.
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
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
		} else if !os.IsNotExist(ferr) {
			return 0, ferr
		}
	}

	snap := agg.Snapshot()
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(snap); err != nil {
		return folded, err
	}

	// Skewed-install detection: JSONL had bytes but zero schema:1
	// events. Per spec §3, surface to stderr; stays advisory, not
	// fatal.
	if cfg.JSONLPath != "" && folded == 0 {
		if info, statErr := os.Stat(cfg.JSONLPath); statErr == nil && info.Size() > 0 {
			fmt.Fprintln(os.Stderr,
				"warning: stats engine sees 0 schema:1 events in JSONL — "+
					"bash hooks may be pre-schema. Run install.sh --upgrade to refresh.")
		}
	}

	return folded, nil
}
