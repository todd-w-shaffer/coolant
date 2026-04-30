// Package cc ingests Claude Code OTEL emissions and reconciles them
// against coolant's JSONL-derived view, surfacing drift as filing-grade
// findings. Metrics-axis only in v1 per the spec at
// docs/_drafts/cc-otel-beta-adapter.spec.md §0.12.
package cc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Severity is the filing-grade severity of a finding (§3.4 table).
type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

// SignalType reserves room for log/trace findings in future specs (§0.12).
type SignalType string

const (
	SignalTypeMetric SignalType = "metric"
	SignalTypeLog    SignalType = "log"
	SignalTypeTrace  SignalType = "trace"
)

// FindingKind enumerates the documented finding categories from §0.3
// plus the per-cause one-shot kinds from §0.9.
type FindingKind string

const (
	KindSchemaDrift                 FindingKind = "schema_drift"
	KindValueMismatch               FindingKind = "value_mismatch"
	KindMissingEmission             FindingKind = "missing_emission"
	KindExtraEmission               FindingKind = "extra_emission"
	KindCardinalityCapped           FindingKind = "cardinality_capped"
	KindPreV3Cache                  FindingKind = "pre_v3_cache"
	KindCcOtelOffline               FindingKind = "cc_otel_offline"
	KindCcOtelResumed               FindingKind = "cc_otel_resumed"
	KindReceiverBindFailed          FindingKind = "receiver_bind_failed"
	KindReceiverRateLimited         FindingKind = "receiver_rate_limited"
	KindCcFlushTruncated            FindingKind = "cc_flush_truncated"
	KindTransientAuthGap            FindingKind = "transient_auth_gap"
	KindAuthGapResumed              FindingKind = "auth_gap_resumed"
	KindAuxiliaryTokensUnreconciled FindingKind = "auxiliary_tokens_unreconciled"
	KindSessionIDDisabled           FindingKind = "session_id_disabled"
	KindNonFiniteMetric             FindingKind = "non_finite_metric"
	KindOversizeJSONLLine           FindingKind = "oversize_jsonl_line"
	KindSuspectedOTLPRetry          FindingKind = "suspected_otlp_retry"
)

// oneShotKinds are the lifetime-gated kinds from §0.9 — fired once per
// process unless explicitly reset (e.g., on resumption).
var oneShotKinds = map[FindingKind]bool{
	KindCcOtelOffline:               true,
	KindCcOtelResumed:               true,
	KindReceiverBindFailed:          true,
	KindReceiverRateLimited:         true,
	KindNonFiniteMetric:             true,
	KindOversizeJSONLLine:           true,
	KindAuxiliaryTokensUnreconciled: true,
	KindSessionIDDisabled:           true,
	KindPreV3Cache:                  true,
	KindTransientAuthGap:            true,
	KindAuthGapResumed:              true,
}

// MetricAttrs is the typed-allowlist struct for per-data-point
// attributes (§0.13 PII guarantee). Adding a new field requires a code
// edit; unknown attribute keys arriving from CC OTEL are dropped at the
// receiver before this struct is populated. omitempty so attrs absent
// from a given metric don't clutter the JSONL.
type MetricAttrs struct {
	Model       string `json:"model,omitempty"`
	QuerySource string `json:"query_source,omitempty"`
	Type        string `json:"type,omitempty"`
	Speed       string `json:"speed,omitempty"`
	Effort      string `json:"effort,omitempty"`
	StartType   string `json:"start_type,omitempty"`
	ToolName    string `json:"tool_name,omitempty"`
	Decision    string `json:"decision,omitempty"`
	Source      string `json:"source,omitempty"`
	Language    string `json:"language,omitempty"`
}

// canonicalKey returns a stable key for identity-tuple dedup.
func (a MetricAttrs) canonicalKey() string {
	return a.Model + "|" + a.QuerySource + "|" + a.Type + "|" + a.Speed + "|" +
		a.Effort + "|" + a.StartType + "|" + a.ToolName + "|" + a.Decision + "|" +
		a.Source + "|" + a.Language
}

// Numeric carries either an integer count or a finite float, never both
// — the type system prevents a string-typed value (potentially derived
// from a content-gated CC field) from landing in a finding's numeric
// surface (§0.13). Marshal flattens to whichever variant is set.
type Numeric struct {
	Int   int64
	Float float64
	IsInt bool
}

func (n Numeric) IsNonFinite() bool {
	if n.IsInt {
		return false
	}
	return math.IsNaN(n.Float) || math.IsInf(n.Float, 0)
}

func (n Numeric) MarshalJSON() ([]byte, error) {
	// IsInt is the explicit signal; the Int!=0/Float==0 fallback
	// covers the common shorthand `Numeric{Int: N}` without IsInt:true.
	if n.IsInt || (n.Int != 0 && n.Float == 0) {
		return json.Marshal(n.Int)
	}
	return json.Marshal(n.Float)
}

func (n *Numeric) UnmarshalJSON(data []byte) error {
	var f float64
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	if f == math.Trunc(f) && !math.IsInf(f, 0) {
		n.Int = int64(f)
		n.IsInt = true
		return nil
	}
	n.Float = f
	return nil
}

// SchemaDriftDetail is the structured form of a schema_drift finding's
// `detail` field (§0.13). Free-form strings are not allowed — the
// recorded values come from CC's schema vocabulary, never from arbitrary
// parsed values.
type SchemaDriftDetail struct {
	FieldName          string `json:"field_name,omitempty"`
	Namespace          string `json:"namespace,omitempty"`
	ObservedMetricName string `json:"observed_metric_name,omitempty"`
}

// Finding is the durable record written to ~/.coolant/cc-otel-findings.jsonl.
// Every numeric field is typed, every string field comes from a known
// allowlist or schema-level identifier (§0.13).
type Finding struct {
	Schema         int                `json:"schema"`
	TS             time.Time          `json:"ts"`
	WindowAnchor   string             `json:"window_anchor"`
	SignalType     SignalType         `json:"signal_type"`
	FindingKind    FindingKind        `json:"finding_kind"`
	CCVersion      string             `json:"cc_version,omitempty"`
	CoolantVersion string             `json:"coolant_version,omitempty"`
	Metric         string             `json:"metric,omitempty"`
	MetricAttrs    MetricAttrs        `json:"metric_attrs,omitempty"`
	SessionID      string             `json:"session_id,omitempty"`
	OrganizationID string             `json:"organization_id,omitempty"`
	Expected       Numeric            `json:"expected,omitempty"`
	Observed       Numeric            `json:"observed,omitempty"`
	DeltaPct       float64            `json:"delta_pct,omitempty"`
	Severity       Severity           `json:"severity"`
	Detail         *SchemaDriftDetail `json:"detail,omitempty"`
}

// identityTuple returns the §3.1 dedup key. Two findings with the same
// tuple are deduped within the tail-K window; differences in
// metric_attrs.query_source produce different tuples.
func (f Finding) identityTuple() string {
	return string(f.SignalType) + "\x00" +
		string(f.FindingKind) + "\x00" +
		f.WindowAnchor + "\x00" +
		f.Metric + "\x00" +
		f.MetricAttrs.canonicalKey() + "\x00" +
		f.SessionID + "\x00" +
		f.OrganizationID
}

// Default writer caps. Override on the Writer for tests.
const (
	defaultRotationSizeBytes int64 = 10 * 1024 * 1024
	defaultTailKDedup        int   = 512
)

// Writer is the structured findings writer (§3.4). All public methods
// are goroutine-safe; the in-process sync.Mutex is sufficient because
// the cc-otel writer is Go-only and never touched from bash (§0.10).
type Writer struct {
	mu                sync.Mutex
	path              string
	stderr            io.Writer
	firedThisLifetime map[FindingKind]bool
	RotationSizeBytes int64
	TailKDedup        int
}

// NewWriter returns a writer rooted at path. stderr defaults to
// os.Stderr when nil; tests inject a buffer to assert the
// no-content-leak message format from §0.13.
func NewWriter(path string, stderr io.Writer) *Writer {
	if stderr == nil {
		stderr = os.Stderr
	}
	return &Writer{
		path:              path,
		stderr:            stderr,
		firedThisLifetime: map[FindingKind]bool{},
		RotationSizeBytes: defaultRotationSizeBytes,
		TailKDedup:        defaultTailKDedup,
	}
}

// ResetOneShot clears the lifetime gate for one kind so a paired
// resume / re-fire (§0.9) doesn't get suppressed.
func (w *Writer) ResetOneShot(kind FindingKind) {
	w.mu.Lock()
	delete(w.firedThisLifetime, kind)
	w.mu.Unlock()
}

// Write appends a finding line if the dedup checks pass. One-shot
// kinds are gated by the in-memory firedThisLifetime map; all other
// kinds are deduped against the trailing K identity tuples in the
// primary file (§3.1).
func (w *Writer) Write(f Finding) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if oneShotKinds[f.FindingKind] {
		if w.firedThisLifetime[f.FindingKind] {
			return nil
		}
	} else {
		// Dedup-read failures are non-fatal — proceed with the write so
		// transient read errors don't drop findings. The "no silent
		// loss" rule (§0.13) applies to writes; reads are best-effort.
		if seen, err := w.tailIdentityTuples(); err == nil && seen[f.identityTuple()] {
			return nil
		}
	}

	if err := w.appendLine(f); err != nil {
		w.logFailure(err)
		return err
	}

	if oneShotKinds[f.FindingKind] {
		w.firedThisLifetime[f.FindingKind] = true
	}
	return nil
}

// appendLine handles the rotation check, mkdir-on-ENOENT retry, and
// fsync-on-close write. Atomic per the spec — open / write / fsync / close
// cycles produce one complete line per call.
func (w *Writer) appendLine(f Finding) error {
	if w.RotationSizeBytes > 0 {
		if info, err := os.Stat(w.path); err == nil && info.Size() >= w.RotationSizeBytes {
			_ = os.Rename(w.path, w.path+".1")
		}
	}

	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	for attempt := 0; attempt < 2; attempt++ {
		fh, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			if _, err = fh.Write(data); err == nil {
				_ = fh.Sync()
			}
			fh.Close()
			if err == nil {
				return nil
			}
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if mkErr := os.MkdirAll(filepath.Dir(w.path), 0o700); mkErr != nil {
			return mkErr
		}
	}
	return errors.New("findings: persistent write failure")
}

// logFailure writes the §0.13 stderr leak guard format. The failed
// payload itself is dropped — only the errno reaches stderr.
func (w *Writer) logFailure(err error) {
	fmt.Fprintf(w.stderr, "cc-findings: write failed: %v\n", err)
}

// tailIdentityTuples reads the trailing K identity tuples from the
// primary file and returns a set for membership checks. Across rotation
// the dedup state is lost (§3.4) — acceptable trade-off vs. re-reading
// the rotated file on every check.
func (w *Writer) tailIdentityTuples() (map[string]bool, error) {
	out := make(map[string]bool, w.TailKDedup)
	fh, err := os.Open(w.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return nil, err
	}
	defer fh.Close()

	const maxBuf = 1024 * 1024
	info, err := fh.Stat()
	if err != nil {
		return nil, err
	}

	readSize := info.Size()
	if readSize > maxBuf {
		_, err := fh.Seek(-maxBuf, io.SeekEnd)
		if err != nil {
			return nil, err
		}
		readSize = maxBuf
	}
	buf := make([]byte, readSize)
	if _, err := io.ReadFull(fh, buf); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	lines := bytes.Split(bytes.TrimRight(buf, "\n"), []byte("\n"))
	start := 0
	if len(lines) > w.TailKDedup {
		start = len(lines) - w.TailKDedup
	}
	for _, line := range lines[start:] {
		if len(line) == 0 {
			continue
		}
		var f Finding
		if err := json.Unmarshal(line, &f); err != nil {
			continue
		}
		out[f.identityTuple()] = true
	}
	return out, nil
}
