package cc

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/stats"
)

// knownCCMetrics is the schema map (§3.1 + §0.2): every documented CC
// OTEL metric name from dev/otel/cc-otel-reference.md §3, plus an
// optional sentinel for unknowns that surface as schema_drift findings.
var knownCCMetrics = map[string]bool{
	"claude_code.session.count":           true,
	"claude_code.lines_of_code.count":     true,
	"claude_code.pull_request.count":      true,
	"claude_code.commit.count":            true,
	"claude_code.cost.usage":              true,
	"claude_code.token.usage":             true,
	"claude_code.code_edit_tool.decision": true,
	"claude_code.active_time.total":       true,
}

// knownCCAttrPrefixes is the schema vocabulary for attribute namespaces
// the adapter will recognize as legitimate CC OTEL fields. The
// gen_ai.* prefix (§0.2 OTel semantic-conventions migration tracking)
// is included so an upcoming migration surfaces as an observable
// event, not silent breakage.
var knownCCAttrPrefixes = []string{
	"claude_code.",
	"gen_ai.",
}

// IsKnownCCMetric reports whether the metric name appears in the §3
// documented set. Unknown names surface as schema_drift.
func IsKnownCCMetric(name string) bool {
	return knownCCMetrics[name]
}

// IsKnownCCAttr reports whether the attribute name belongs to a
// recognized CC namespace (claude_code.* or gen_ai.*).
func IsKnownCCAttr(name string) bool {
	for _, p := range knownCCAttrPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// AdapterConfig wires the adapter to its writer + version surface.
type AdapterConfig struct {
	Findings       *Writer
	CCVersion      string
	CoolantVersion string
}

// Adapter holds the schema map, the lifetime fingerprint, the
// rate-limited drift table, and the FiredThisLifetime gate referenced
// by the receiver, tailer, and reconcile loop. One Adapter per process
// per the spec.
type Adapter struct {
	cfg AdapterConfig

	mu                  sync.RWMutex
	observedMetrics     map[string]bool
	observedAttrKeys    map[string]bool
	observedResourceKey map[string]bool
	driftFired          map[string]bool // key: field_name|cc_version

	// Live-throughput fan-in surface. Wired post-construction via
	// SetTailer / SetLastReceiverPostTS — nil means "OTEL token view
	// not configured", and OTELTokens / IsOTELLive degrade to zero.
	tailer             *MetricsTailer
	lastReceiverPostTS func() time.Time
}

// NewAdapter constructs an Adapter with seeded schema state.
func NewAdapter(cfg AdapterConfig) *Adapter {
	a := &Adapter{
		cfg:                 cfg,
		observedMetrics:     map[string]bool{},
		observedAttrKeys:    map[string]bool{},
		observedResourceKey: map[string]bool{},
		driftFired:          map[string]bool{},
	}
	for k := range resourceAttrAllowlist {
		a.observedResourceKey[k] = true
	}
	return a
}

// ObserveMetric records that a metric was seen plus the per-data-point
// attribute keys. Used by the schema fingerprint.
func (a *Adapter) ObserveMetric(name string, attrKeys []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.observedMetrics[name] = true
	for _, k := range attrKeys {
		a.observedAttrKeys[k] = true
	}
}

// ObserveResourceAttrKey records that the receiver saw a resource-attr
// key (e.g., service.version). Values are not recorded — only keys.
func (a *Adapter) ObserveResourceAttrKey(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.observedResourceKey[key] = true
}

// Fingerprint returns a snapshot of the lifetime fingerprint.
func (a *Adapter) Fingerprint() Fingerprint {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := Fingerprint{
		Metrics:    make(map[string]bool, len(a.observedMetrics)),
		AttrKeys:   make(map[string]bool, len(a.observedAttrKeys)),
		ResourceKs: make(map[string]bool, len(a.observedResourceKey)),
	}
	for k := range a.observedMetrics {
		out.Metrics[k] = true
	}
	for k := range a.observedAttrKeys {
		out.AttrKeys[k] = true
	}
	for k := range a.observedResourceKey {
		out.ResourceKs[k] = true
	}
	return out
}

// Fingerprint is a snapshot of the schema fingerprint (§0.2). Metric
// names + attribute KEYS — never values.
type Fingerprint struct {
	Metrics    map[string]bool
	AttrKeys   map[string]bool
	ResourceKs map[string]bool
}

func (fp Fingerprint) HasMetric(name string) bool       { return fp.Metrics[name] }
func (fp Fingerprint) HasResourceAttrKey(k string) bool { return fp.ResourceKs[k] }
func (fp Fingerprint) HasAttrKey(k string) bool         { return fp.AttrKeys[k] }

// ObserveSchemaDrift fires a single schema_drift finding for a given
// (field_name, cc_version) tuple per process lifetime. Per §0.2,
// repeated drift on the same tuple suppresses; a different
// field_name OR a different cc_version each gets its own finding.
func (a *Adapter) ObserveSchemaDrift(fieldName, namespace, ccVersion string) {
	a.mu.Lock()
	key := fieldName + "|" + ccVersion
	if a.driftFired[key] {
		a.mu.Unlock()
		return
	}
	a.driftFired[key] = true
	a.mu.Unlock()

	if a.cfg.Findings == nil {
		return
	}
	_ = a.cfg.Findings.Write(Finding{
		Schema:         1,
		TS:             time.Now().UTC(),
		WindowAnchor:   time.Now().UTC().Format("2006-01-02"),
		SignalType:     SignalTypeMetric,
		FindingKind:    KindSchemaDrift,
		CCVersion:      ccVersion,
		CoolantVersion: a.cfg.CoolantVersion,
		Severity:       SeverityError, // GA metrics → error per §3.4
		Detail: &SchemaDriftDetail{
			FieldName: fieldName,
			Namespace: namespace,
		},
	})
}

func (a *Adapter) driftCount(field, ccVersion string) int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.driftFired[field+"|"+ccVersion] {
		return 1
	}
	return 0
}

// IsCardinalityCapped tests whether a missing-from-coolant attribute is
// the cardinality cap firing (coolant's keyset contains "__other") vs.
// a real extra_emission (no __other present). Per §3.4 transition rule:
// the one-tick delay in extra_emission classification is the
// reconcile-loop's responsibility; this helper reports the
// instantaneous test only.
func (a *Adapter) IsCardinalityCapped(missingKey string, coolantKeyset map[string]int64) bool {
	if _, has := coolantKeyset[missingKey]; has {
		return false
	}
	_, hasOther := coolantKeyset["__other"]
	return hasOther
}

// IsSubagentTokenAttr is the §3.1 query_source like-for-like filter.
// Coolant's per-agent rolls correspond ONLY to subagent emissions —
// main-thread tokens are not in coolant's view, and auxiliary tokens
// are unreconciled in v1.
func (a *Adapter) IsSubagentTokenAttr(attrs map[string]string) bool {
	return attrs["query_source"] == "subagent"
}

// readFileBytes is a small file helper used by adapter tests; placed
// here so internal_test.go isn't required.
func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// AggregatorView narrows the stats.Aggregator surface the cc adapter
// actually consumes — keeps the package easy to fake from tests
// without a full Aggregator on disk and lets a future
// `cc-otel-daemon` subcommand swap in a remote-aggregator client.
type AggregatorView interface {
	Snapshot() stats.Snapshot
	WindowByType(days int) map[string]int64
	WindowByProject(days int) map[string]int64
}

var _ AggregatorView = (*stats.Aggregator)(nil)

// OTELTokenView is the TokenCollector-facing seam for the live-throughput
// fan-in: latest cumulative token reads, receiver liveness, and the
// drift-observe passthrough for schema gating. *Adapter implements it;
// tests provide their own fake to avoid booting the receiver.
type OTELTokenView interface {
	OTELTokens(now time.Time) (input, output, cacheCreate, cacheRead int64, ok bool)
	IsOTELLive(now time.Time) bool
	ObserveTokenSchemaDrift(field, ccVersion string)
}

var _ OTELTokenView = (*Adapter)(nil)

// SetTailer wires the metrics tailer so OTELTokens can read latest
// cumulative values. nil-safe: when unset, OTELTokens returns ok=false.
func (a *Adapter) SetTailer(t *MetricsTailer) {
	a.mu.Lock()
	a.tailer = t
	a.mu.Unlock()
}

// SetLastReceiverPostTS wires the receiver liveness probe so
// IsOTELLive can debounce against cleanlyOfflineWindow. nil-safe.
func (a *Adapter) SetLastReceiverPostTS(fn func() time.Time) {
	a.mu.Lock()
	a.lastReceiverPostTS = fn
	a.mu.Unlock()
}

// otelTokenQuerySources is the full set of CC OTEL query_source values
// the fan-in sums across. Excluding any of these would drop a slice of
// real traffic — main misses subagent + auxiliary, subagent misses
// main + auxiliary (the motivating gap), etc. Distinct from
// IsSubagentTokenAttr's like-for-like filter, which serves a different
// reconcile job and is intentionally narrower.
var otelTokenQuerySources = []string{"main", "subagent", "auxiliary"}

// OTELTokens sums the latest-cumulative values per `type` across all
// query_source values for the current UTC day. CC OTEL emits type as
// CamelCase ("cacheRead" / "cacheCreation"); the returned int64s are
// the canonical quadruple the collector consumes regardless of source.
// ok=true when at least one (type, query_source) bucket has data.
func (a *Adapter) OTELTokens(now time.Time) (input, output, cacheCreate, cacheRead int64, ok bool) {
	a.mu.RLock()
	tailer := a.tailer
	a.mu.RUnlock()
	if tailer == nil {
		return 0, 0, 0, 0, false
	}
	dayKey := now.UTC().Format("2006-01-02")
	sum := func(typeVal string) int64 {
		var total float64
		for _, qs := range otelTokenQuerySources {
			v, _, found := tailer.LatestCumulative(
				"claude_code.token.usage",
				map[string]string{"type": typeVal, "query_source": qs},
				dayKey,
			)
			if found {
				total += v
				ok = true
			}
		}
		return int64(total)
	}
	input = sum("input")
	output = sum("output")
	cacheCreate = sum("cacheCreation")
	cacheRead = sum("cacheRead")
	return
}

// IsOTELLive reports whether the receiver has accepted a POST within
// cleanlyOfflineWindow. False when no liveness probe is wired or the
// receiver has been silent for too long — the fan-in falls back to
// transcript-only in that case.
func (a *Adapter) IsOTELLive(now time.Time) bool {
	a.mu.RLock()
	fn := a.lastReceiverPostTS
	a.mu.RUnlock()
	if fn == nil {
		return false
	}
	lastPost := fn()
	return !lastPost.IsZero() && now.Sub(lastPost) < cleanlyOfflineWindow
}

// ObserveTokenSchemaDrift is the TokenCollector-facing wrapper over
// ObserveSchemaDrift that supplies the "claude_code" namespace so the
// fan-in doesn't have to depend on the namespace literal.
func (a *Adapter) ObserveTokenSchemaDrift(fieldName, ccVersion string) {
	a.ObserveSchemaDrift(fieldName, "claude_code", ccVersion)
}
