package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/toddwshaffer/coolant/thermal/internal/stats"
)

// runStatsdump folds the entire JSONL file into an aggregator (loaded
// from cache, then incrementally extended) and writes the resulting
// snapshot as pretty-printed JSON to w. Hidden dev/debug subcommand —
// the user-facing `thermo stats` shares foldSnapshot but renders text
// (and optional JSON via its own flag).
//
// Returns the number of events folded plus any I/O error. A zero fold
// count against a non-empty JSONL is the skewed-install signal —
// caller surfaces it to stderr.
func runStatsdump(w io.Writer, cfg stats.Config) (folded int, err error) {
	agg, folded, err := foldSnapshot(cfg)
	if err != nil {
		return 0, err
	}

	snap := agg.Snapshot()
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(snap); err != nil {
		return folded, err
	}

	if cfg.JSONLPath != "" && folded == 0 {
		if info, statErr := os.Stat(cfg.JSONLPath); statErr == nil && info.Size() > 0 {
			fmt.Fprintln(os.Stderr,
				"warning: stats engine sees 0 schema:1 events in JSONL — "+
					"bash hooks may be pre-schema. Run install.sh --upgrade to refresh.")
		}
	}

	return folded, nil
}
