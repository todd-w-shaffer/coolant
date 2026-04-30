package cc

import (
	"strings"
	"testing"
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

// readBytesFile is a small helper to read a file's contents — the test
// for the structured-detail finding asserts on the raw JSONL bytes.
func readBytesFile(path string) ([]byte, error) {
	return readFileBytes(path)
}
