package cc

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// allEightCCMetrics is the authoritative list per
// dev/otel/cc-otel-reference.md §3 — coolant must recognize all eight.
var allEightCCMetrics = []string{
	"claude_code.session.count",
	"claude_code.lines_of_code.count",
	"claude_code.pull_request.count",
	"claude_code.commit.count",
	"claude_code.cost.usage",
	"claude_code.token.usage",
	"claude_code.code_edit_tool.decision",
	"claude_code.active_time.total",
}

func TestSchemaMap_RecognizesAllEightDocumentedMetrics(t *testing.T) {
	for _, name := range allEightCCMetrics {
		if !IsKnownCCMetric(name) {
			t.Errorf("schema map must recognize %q (per dev/otel/cc-otel-reference.md §3)", name)
		}
	}
}

func TestSchemaMap_UnknownMetricRoutesToSentinel(t *testing.T) {
	if IsKnownCCMetric("claude_code.brand_new_metric_we_havent_seen") {
		t.Errorf("unknown metric must route to sentinel, not match existing entry")
	}
}

func TestSchemaFingerprint_RecordsMetricNameAndAttrKeys(t *testing.T) {
	a := NewAdapter(AdapterConfig{})
	a.ObserveMetric("claude_code.token.usage", []string{"model", "query_source", "type"})
	a.ObserveResourceAttrKey("service.version")

	fp := a.Fingerprint()
	if !fp.HasMetric("claude_code.token.usage") {
		t.Errorf("fingerprint should record metric name")
	}
	for _, key := range []string{"model", "query_source", "type"} {
		if !fp.HasAttrKey(key) {
			t.Errorf("fingerprint should record per-data-point attr key %q", key)
		}
	}
	// Receiver-side allowlist seeds the documented resource keys at
	// NewAdapter time so the fingerprint covers them even before the
	// first POST.
	for _, key := range []string{"service.name", "service.version", "session.id"} {
		if !fp.HasResourceAttrKey(key) {
			t.Errorf("fingerprint should record allowlisted resource key %q", key)
		}
	}
	// ObserveResourceAttrKey records observed-but-unallowlisted keys
	// would surface as schema_drift; the seed already covers
	// service.version so this is a redundant write that must not break.
	if !fp.HasResourceAttrKey("service.version") {
		t.Errorf("ObserveResourceAttrKey should leave the seeded key in place")
	}
}

func TestSchemaDrift_RateLimitedByFieldNameAndCCVersion(t *testing.T) {
	w, _ := newTestWriter(t)
	a := NewAdapter(AdapterConfig{Findings: w, CCVersion: "1.x.x"})
	for i := 0; i < 100; i++ {
		a.ObserveSchemaDrift("brand_new_field", "claude_code.namespace", "1.x.x")
	}
	w2, _ := newTestWriter(t)
	a2 := NewAdapter(AdapterConfig{Findings: w2, CCVersion: "2.x.x"})
	a2.ObserveSchemaDrift("brand_new_field", "claude_code.namespace", "2.x.x")

	if got := a.driftCount("brand_new_field", "1.x.x"); got != 1 {
		t.Errorf("expected single schema_drift per (field, version), got %d", got)
	}
	if got := a2.driftCount("brand_new_field", "2.x.x"); got != 1 {
		t.Errorf("different cc_version should fire its own schema_drift, got %d", got)
	}
}

func TestSchemaDrift_DetailIsStructured(t *testing.T) {
	w, findingsPath := newTestWriter(t)
	a := NewAdapter(AdapterConfig{Findings: w, CCVersion: "1.x.x"})
	a.ObserveSchemaDrift("query_source", "claude_code", "1.x.x")
	contents, err := readBytesFile(findingsPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(contents)
	if !strings.Contains(got, `"finding_kind":"schema_drift"`) {
		t.Errorf("expected schema_drift finding: %s", got)
	}
	if !strings.Contains(got, `"detail"`) || !strings.Contains(got, `"field_name":"query_source"`) {
		t.Errorf("detail must be structured with field_name: %s", got)
	}
}

func TestCardinalityCap_DistinguishesFromExtraEmission(t *testing.T) {
	a := NewAdapter(AdapterConfig{})
	// Coolant's keyset contains __other => the cap fired.
	coolantKeys := map[string]int64{"a": 1, "b": 2, "__other": 5}
	if !a.IsCardinalityCapped("c", coolantKeys) {
		t.Errorf("when __other is in coolant's keyset, missing key 'c' is cardinality_capped, not extra_emission")
	}
	// Without __other => extra_emission territory.
	noOther := map[string]int64{"a": 1, "b": 2}
	if a.IsCardinalityCapped("c", noOther) {
		t.Errorf("without __other in coolant's keyset, missing key is NOT cardinality_capped")
	}
}

func TestQuerySource_LikeForLikeFilter(t *testing.T) {
	a := NewAdapter(AdapterConfig{})
	if a.IsSubagentTokenAttr(map[string]string{"query_source": "subagent", "type": "input"}) != true {
		t.Errorf("subagent input should match the like-for-like filter")
	}
	if a.IsSubagentTokenAttr(map[string]string{"query_source": "main", "type": "input"}) != false {
		t.Errorf("main input must NOT match (CC's main thread isn't a coolant-tracked agent)")
	}
	if a.IsSubagentTokenAttr(map[string]string{"query_source": "auxiliary", "type": "input"}) != false {
		t.Errorf("auxiliary input must NOT match (background tasks unreconciled in v1)")
	}
}

func TestSchemaMap_RecognizesGenAINamespace(t *testing.T) {
	if !IsKnownCCAttr("gen_ai.system") {
		t.Errorf("gen_ai.* namespace must be recognized so adapter can detect the migration as an event, not silent breakage (§0.2)")
	}
	if !IsKnownCCAttr("gen_ai.usage.input_tokens") {
		t.Errorf("gen_ai.usage.* fields per the OTel semantic-conventions migration path")
	}
}

// OTELTokenView contract: *Adapter implements the surface TokenCollector
// uses to fan in live OTEL throughput.

func TestOTELTokens_SumsAcrossQuerySources(t *testing.T) {
	a := NewAdapter(AdapterConfig{})
	tailer := NewMetricsTailer(filepath.Join(t.TempDir(), "cc-otel.jsonl"))
	a.SetTailer(tailer)

	now := time.Now().UTC()
	dayKey := now.Format("2006-01-02")

	// Three query_source values, all contributing to input. cacheRead /
	// cacheCreation use CC's CamelCase.
	seed := func(qs, typ string, val float64) {
		tailer.absorbLine(mustMarshalLine(t, jsonlLine{
			Schema: 1, TS: now.Format(time.RFC3339Nano),
			Metric: "claude_code.token.usage", Value: val,
			Attrs: map[string]string{"query_source": qs, "type": typ},
		}))
	}
	seed("main", "input", 100)
	seed("subagent", "input", 50)
	seed("auxiliary", "input", 25)
	seed("main", "output", 200)
	seed("main", "cacheRead", 30)
	seed("auxiliary", "cacheCreation", 7)
	_ = dayKey

	in, out, cC, cR, ok := a.OTELTokens(now)
	if !ok {
		t.Fatalf("OTELTokens want ok=true after seeding")
	}
	if in != 175 {
		t.Errorf("input want 175 (100+50+25 across all query_source), got %d", in)
	}
	if out != 200 {
		t.Errorf("output want 200, got %d", out)
	}
	if cC != 7 {
		t.Errorf("cacheCreate want 7 (auxiliary only), got %d", cC)
	}
	if cR != 30 {
		t.Errorf("cacheRead want 30 (main only), got %d", cR)
	}
}

func TestOTELTokens_NilTailerReturnsNotOK(t *testing.T) {
	a := NewAdapter(AdapterConfig{})
	in, out, cC, cR, ok := a.OTELTokens(time.Now())
	if ok || in != 0 || out != 0 || cC != 0 || cR != 0 {
		t.Errorf("nil tailer want zero-quadruple ok=false, got in=%d out=%d cC=%d cR=%d ok=%v", in, out, cC, cR, ok)
	}
}

func TestIsOTELLive_RespectsCleanlyOfflineWindow(t *testing.T) {
	a := NewAdapter(AdapterConfig{})

	// No probe wired → not live.
	if a.IsOTELLive(time.Now()) {
		t.Errorf("unset LastReceiverPostTS want IsOTELLive=false")
	}

	// Zero-time probe → not live (never seen a POST).
	a.SetLastReceiverPostTS(func() time.Time { return time.Time{} })
	if a.IsOTELLive(time.Now()) {
		t.Errorf("zero-time probe want IsOTELLive=false")
	}

	// Recent POST → live.
	now := time.Now().UTC()
	a.SetLastReceiverPostTS(func() time.Time { return now.Add(-30 * time.Second) })
	if !a.IsOTELLive(now) {
		t.Errorf("30s-stale POST want IsOTELLive=true (window is 2min)")
	}

	// >2min stale → not live.
	a.SetLastReceiverPostTS(func() time.Time { return now.Add(-3 * time.Minute) })
	if a.IsOTELLive(now) {
		t.Errorf("3min-stale POST want IsOTELLive=false")
	}
}

func TestObserveTokenSchemaDrift_PassesNamespaceConstant(t *testing.T) {
	w, findingsPath := newTestWriter(t)
	a := NewAdapter(AdapterConfig{Findings: w, CCVersion: "1.x.x"})

	a.ObserveTokenSchemaDrift("cacheRead", "1.x.x")

	data, err := readBytesFile(findingsPath)
	if err != nil {
		t.Fatalf("read findings: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"finding_kind":"schema_drift"`) {
		t.Errorf("expected schema_drift finding via passthrough: %s", got)
	}
	if !strings.Contains(got, `"field_name":"cacheRead"`) {
		t.Errorf("expected field_name=cacheRead in detail: %s", got)
	}
	if !strings.Contains(got, `"namespace":"claude_code"`) {
		t.Errorf("expected namespace=claude_code (passthrough constant): %s", got)
	}

	// Same (field, ccVersion) is suppressed; only one finding written.
	a.ObserveTokenSchemaDrift("cacheRead", "1.x.x")
	if got := a.driftCount("cacheRead", "1.x.x"); got != 1 {
		t.Errorf("passthrough must dedupe via existing ObserveSchemaDrift, got %d", got)
	}
}

// mustMarshalLine returns the JSONL bytes for a jsonlLine, since
// absorbLine takes raw bytes.
func mustMarshalLine(t *testing.T, jl jsonlLine) []byte {
	t.Helper()
	b, err := json.Marshal(jl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// readBytesFile is a small helper to read a file's contents — the test
// for the structured-detail finding asserts on the raw JSONL bytes.
func readBytesFile(path string) ([]byte, error) {
	return readFileBytes(path)
}
