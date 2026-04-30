package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/toddwshaffer/coolant/thermal/internal/otel/cc"
	"github.com/toddwshaffer/coolant/thermal/internal/stats"
)

const (
	ccFindingsExitOK      = 0
	ccFindingsExitIOError = 1
	ccFindingsExitFlag    = 2
)

// ccFindingsFlags is the parsed shape of `thermo cc-findings` flags
// per spec §3.5. Pulled out for testability.
type ccFindingsFlags struct {
	severity string // "" = all
	signal   string // "metric" (default), "log", "trace"
	top      int
}

// runCcFindings is the v1 review surface for filing-grade findings
// (§3.5). Reads ~/.coolant/cc-otel-findings.jsonl plus the .1 rotation
// sibling, unmarshals into typed Finding, groups by
// (signal_type, finding_kind), prints top-N per category.
//
// Renders from typed fields ONLY — never from raw JSONL bytes (§0.13).
// _ stats.Config is accepted for parity with runStats so callers can
// compose them under productionStatsConfig() without conditionals;
// not used in v1 — findings live in ~/.coolant/, not stats config.
func runCcFindings(stdout, stderr io.Writer, args []string, _ stats.Config) int {
	fs := flag.NewFlagSet("cc-findings", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}

	var f ccFindingsFlags
	fs.StringVar(&f.severity, "severity", "", "Filter by severity: info, warn, error.")
	fs.StringVar(&f.signal, "signal", "metric", "Filter by signal type: metric (v1), log, trace.")
	fs.IntVar(&f.top, "top", 5, "Top-N findings per category.")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stdout, ccFindingsUsageText())
			return ccFindingsExitOK
		}
		fmt.Fprintln(stderr, ccFindingsUsageText())
		return ccFindingsExitFlag
	}

	validSeverities := map[string]bool{
		"":                       true,
		string(cc.SeverityInfo):  true,
		string(cc.SeverityWarn):  true,
		string(cc.SeverityError): true,
	}
	if !validSeverities[f.severity] {
		fmt.Fprintf(stderr, "cc-findings: unknown --severity=%q (valid: info, warn, error)\n", f.severity)
		return ccFindingsExitFlag
	}
	validSignals := map[string]bool{
		string(cc.SignalTypeMetric): true,
		string(cc.SignalTypeLog):    true,
		string(cc.SignalTypeTrace):  true,
	}
	if !validSignals[f.signal] {
		fmt.Fprintf(stderr, "cc-findings: unknown --signal=%q (valid: metric, log, trace)\n", f.signal)
		return ccFindingsExitFlag
	}
	if f.top < 1 {
		f.top = 1
	}

	if cc.SignalType(f.signal) != cc.SignalTypeMetric {
		fmt.Fprintf(stdout, "no %s-signal findings — log/trace axis ships in cc-otel-multisignal-adapter.spec.md.\n", f.signal)
		return ccFindingsExitOK
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "cc-findings: cannot resolve home: %v\n", err)
		return ccFindingsExitIOError
	}
	primary := filepath.Join(home, ".coolant", "cc-otel-findings.jsonl")
	rotated := primary + ".1"

	findings, err := readFindingsFiles(primary, rotated)
	if err != nil {
		fmt.Fprintf(stderr, "cc-findings: %v\n", err)
		return ccFindingsExitIOError
	}

	if f.severity != "" {
		filtered := findings[:0]
		for _, fnd := range findings {
			if string(fnd.Severity) == f.severity {
				filtered = append(filtered, fnd)
			}
		}
		findings = filtered
	}

	if len(findings) == 0 {
		fmt.Fprintln(stdout, "no findings.")
		return ccFindingsExitOK
	}

	groups := groupBySignalKind(findings)
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Fprintf(stdout, "cc-findings — %d total\n", len(findings))
	for _, k := range keys {
		bucket := groups[k]
		fmt.Fprintf(stdout, "\n[%s] %d finding(s)\n", k, len(bucket))
		max := f.top
		if len(bucket) < max {
			max = len(bucket)
		}
		for i := 0; i < max; i++ {
			renderFinding(stdout, bucket[i])
		}
		if len(bucket) > max {
			fmt.Fprintf(stdout, "  … %d more\n", len(bucket)-max)
		}
	}
	return ccFindingsExitOK
}

func readFindingsFiles(paths ...string) ([]cc.Finding, error) {
	var all []cc.Finding
	for _, p := range paths {
		fh, err := os.Open(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
		for sc.Scan() {
			line := sc.Bytes()
			if len(line) == 0 {
				continue
			}
			var f cc.Finding
			if err := json.Unmarshal(line, &f); err != nil {
				continue
			}
			all = append(all, f)
		}
		fh.Close()
	}
	return all, nil
}

func groupBySignalKind(in []cc.Finding) map[string][]cc.Finding {
	out := map[string][]cc.Finding{}
	for _, f := range in {
		key := string(f.SignalType) + "/" + string(f.FindingKind)
		out[key] = append(out[key], f)
	}
	for k := range out {
		bucket := out[k]
		sort.Slice(bucket, func(i, j int) bool {
			return bucket[i].TS.After(bucket[j].TS)
		})
		out[k] = bucket
	}
	return out
}

// renderFinding prints one finding from typed fields ONLY — never from
// raw JSONL bytes. The §0.13 PII guarantee depends on this discipline.
func renderFinding(w io.Writer, f cc.Finding) {
	fmt.Fprintf(w, "  %s  %s  severity=%s\n",
		f.TS.Format("2006-01-02T15:04:05Z"),
		f.WindowAnchor,
		f.Severity,
	)
	if f.Metric != "" {
		fmt.Fprintf(w, "    metric=%s", f.Metric)
		if f.MetricAttrs.Model != "" {
			fmt.Fprintf(w, " model=%s", f.MetricAttrs.Model)
		}
		if f.MetricAttrs.QuerySource != "" {
			fmt.Fprintf(w, " query_source=%s", f.MetricAttrs.QuerySource)
		}
		if f.MetricAttrs.Type != "" {
			fmt.Fprintf(w, " type=%s", f.MetricAttrs.Type)
		}
		fmt.Fprintln(w)
	}
	if f.Detail != nil && f.Detail.FieldName != "" {
		fmt.Fprintf(w, "    field=%s namespace=%s\n", f.Detail.FieldName, f.Detail.Namespace)
	}
	if f.Expected.IsInt && f.Observed.IsInt && (f.Expected.Int != 0 || f.Observed.Int != 0) {
		fmt.Fprintf(w, "    expected=%d observed=%d delta_pct=%.1f\n", f.Expected.Int, f.Observed.Int, f.DeltaPct)
	}
	if f.SessionID != "" {
		fmt.Fprintf(w, "    session_id=%s\n", f.SessionID)
	}
}

func ccFindingsUsageText() string {
	return `Usage: thermo cc-findings [--severity=info|warn|error] [--signal=metric|log|trace] [--top=N]

Reviews CC OTEL drift findings written by the embedded reconciliation
loop. Reads ~/.coolant/cc-otel-findings.jsonl (and .1 rotation sibling),
groups by (signal_type, finding_kind), prints top-N per category.

The findings JSONL is filing-grade for the Anthropic product team.
Filter privacy-sensitive identity attributes via jq before attaching
to a bug report.`
}
