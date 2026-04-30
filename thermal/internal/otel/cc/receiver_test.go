package cc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func newTestReceiver(t *testing.T) (*Receiver, string) {
	t.Helper()
	jsonl := filepath.Join(t.TempDir(), "cc-otel.jsonl")
	w, _ := newTestWriter(t)
	r, err := NewReceiver(ReceiverConfig{
		Addr:      "127.0.0.1",
		Port:      freePort(t),
		JSONLPath: jsonl,
		Findings:  w,
	})
	if err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	t.Cleanup(func() { _ = r.Shutdown(context.Background()) })
	return r, jsonl
}

func sampleRequest() []byte {
	req := &metricsv1.MetricsData{
		ResourceMetrics: []*metricsv1.ResourceMetrics{{
			Resource: &resourcev1.Resource{
				Attributes: []*commonv1.KeyValue{
					{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "claude-code"}}},
					{Key: "service.version", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "1.x.x"}}},
					{Key: "session.id", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "abcd1234"}}},
					{Key: "user.account_uuid", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "01BWBeN28..."}}},
				},
			},
			ScopeMetrics: []*metricsv1.ScopeMetrics{{
				Metrics: []*metricsv1.Metric{
					{
						Name: "claude_code.token.usage",
						Data: &metricsv1.Metric_Sum{Sum: &metricsv1.Sum{
							DataPoints: []*metricsv1.NumberDataPoint{{
								Attributes: []*commonv1.KeyValue{
									{Key: "model", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "claude-opus-4-7"}}},
									{Key: "query_source", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "subagent"}}},
									{Key: "type", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "input"}}},
								},
								Value:        &metricsv1.NumberDataPoint_AsInt{AsInt: 1234},
								TimeUnixNano: uint64(time.Now().UnixNano()),
							}},
						}},
					},
				},
			}},
		}},
	}
	body, err := proto.Marshal(req)
	if err != nil {
		panic(err)
	}
	return body
}

func postProto(t *testing.T, addr string, body []byte) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Post("http://"+addr+"/v1/metrics", "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, respBody
}

func readJSONL(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("line %q: %v", string(line), err)
		}
		out = append(out, m)
	}
	return out
}

func TestReceiver_AcceptsValidPostAndWritesJSONL(t *testing.T) {
	r, jsonl := newTestReceiver(t)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	resp, _ := postProto(t, r.Addr(), sampleRequest())
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var lines []map[string]any
	for i := 0; i < 50 && len(lines) == 0; i++ {
		lines = readJSONL(t, jsonl)
		if len(lines) == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 JSONL line, got %d", len(lines))
	}
	got := lines[0]
	if got["schema"].(float64) != 1 {
		t.Errorf("schema field missing/wrong: %v", got["schema"])
	}
	if got["metric"].(string) != "claude_code.token.usage" {
		t.Errorf("metric wrong: %v", got["metric"])
	}
	if got["value"].(float64) != 1234 {
		t.Errorf("value wrong: %v", got["value"])
	}
	resAttrs := got["resource_attrs"].(map[string]any)
	if resAttrs["session.id"].(string) != "abcd1234" {
		t.Errorf("session.id missing: %v", resAttrs)
	}
	attrs := got["attrs"].(map[string]any)
	if attrs["model"].(string) != "claude-opus-4-7" || attrs["query_source"].(string) != "subagent" {
		t.Errorf("attrs wrong: %v", attrs)
	}
}

func TestReceiver_RejectsInvalidProtobuf(t *testing.T) {
	r, jsonl := newTestReceiver(t)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	resp, _ := postProto(t, r.Addr(), []byte{0xFF, 0xFE, 0xFD})
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 on invalid protobuf, got %d", resp.StatusCode)
	}
	if lines := readJSONL(t, jsonl); len(lines) != 0 {
		t.Errorf("invalid protobuf must not write JSONL, got %d lines", len(lines))
	}
}

func TestReceiver_LogsAndTracesReturn404(t *testing.T) {
	r, _ := newTestReceiver(t)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	for _, p := range []string{"/v1/logs", "/v1/traces"} {
		resp, err := http.Post("http://"+r.Addr()+p, "application/x-protobuf", strings.NewReader(""))
		if err != nil {
			t.Fatalf("post %s: %v", p, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Errorf("expected 404 on %s, got %d", p, resp.StatusCode)
		}
	}
}

func TestReceiver_ConcurrentPushesIntactJSONL(t *testing.T) {
	r, jsonl := newTestReceiver(t)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	body := sampleRequest()

	const goroutines = 8
	const each = 4
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				resp, _ := postProto(t, r.Addr(), body)
				if resp.StatusCode != 200 {
					t.Errorf("status %d", resp.StatusCode)
				}
			}
		}()
	}
	wg.Wait()

	expected := goroutines * each
	var lines []map[string]any
	for i := 0; i < 100 && len(lines) < expected; i++ {
		lines = readJSONL(t, jsonl)
		if len(lines) < expected {
			time.Sleep(30 * time.Millisecond)
		}
	}
	if len(lines) != expected {
		t.Fatalf("expected %d intact lines, got %d", expected, len(lines))
	}
}

func TestReceiver_BindFailureFiresOneFinding(t *testing.T) {
	port := freePort(t)
	taken, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("steal port: %v", err)
	}
	defer taken.Close()

	jsonl := filepath.Join(t.TempDir(), "cc-otel.jsonl")
	w, findingsPath := newTestWriter(t)
	r, err := NewReceiver(ReceiverConfig{
		Addr:      "127.0.0.1",
		Port:      port,
		JSONLPath: jsonl,
		Findings:  w,
	})
	if err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	if err := r.Start(); err == nil {
		t.Fatalf("expected bind failure on stolen port")
	}

	// One receiver_bind_failed finding should have landed in the writer.
	data, err := os.ReadFile(findingsPath)
	if err != nil {
		t.Fatalf("read findings: %v", err)
	}
	count := strings.Count(string(data), `"finding_kind":"receiver_bind_failed"`)
	if count != 1 {
		t.Errorf("expected 1 receiver_bind_failed finding, got %d", count)
	}
}

func TestReceiver_BodyCapReturns413(t *testing.T) {
	r, jsonl := newTestReceiver(t)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Build a body well over 1 MB by stuffing many large data points.
	bigReq := &metricsv1.MetricsData{
		ResourceMetrics: []*metricsv1.ResourceMetrics{{
			ScopeMetrics: []*metricsv1.ScopeMetrics{{
				Metrics: []*metricsv1.Metric{{
					Name: "claude_code.token.usage",
					Data: &metricsv1.Metric_Sum{Sum: &metricsv1.Sum{}},
				}},
			}},
		}},
	}
	dps := bigReq.ResourceMetrics[0].ScopeMetrics[0].Metrics[0].Data.(*metricsv1.Metric_Sum).Sum
	// large unique-string attr per point: 2KB × 800 ≈ 1.6 MB
	for i := 0; i < 800; i++ {
		dps.DataPoints = append(dps.DataPoints, &metricsv1.NumberDataPoint{
			Attributes: []*commonv1.KeyValue{{
				Key:   "model",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: strings.Repeat("X", 2048)}},
			}},
			Value: &metricsv1.NumberDataPoint_AsInt{AsInt: int64(i)},
		})
	}
	big, err := proto.Marshal(bigReq)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(big) < 1024*1024 {
		t.Fatalf("test fixture too small: %d", len(big))
	}

	resp, _ := postProto(t, r.Addr(), big)
	if resp.StatusCode != 413 {
		t.Errorf("expected 413 for >1MB body, got %d", resp.StatusCode)
	}
	if lines := readJSONL(t, jsonl); len(lines) != 0 {
		t.Errorf("oversize body must not write JSONL, got %d lines", len(lines))
	}
}

func TestReceiver_StripsUserEmail(t *testing.T) {
	r, jsonl := newTestReceiver(t)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	req := &metricsv1.MetricsData{
		ResourceMetrics: []*metricsv1.ResourceMetrics{{
			Resource: &resourcev1.Resource{
				Attributes: []*commonv1.KeyValue{
					{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "claude-code"}}},
					{Key: "user.email", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "alice@example.com"}}},
					{Key: "team.password", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "hunter2"}}},
				},
			},
			ScopeMetrics: []*metricsv1.ScopeMetrics{{
				Metrics: []*metricsv1.Metric{{
					Name: "claude_code.token.usage",
					Data: &metricsv1.Metric_Sum{Sum: &metricsv1.Sum{
						DataPoints: []*metricsv1.NumberDataPoint{{
							Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 1},
						}},
					}},
				}},
			}},
		}},
	}
	body, _ := proto.Marshal(req)
	if resp, _ := postProto(t, r.Addr(), body); resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}

	var lines []map[string]any
	for i := 0; i < 50 && len(lines) == 0; i++ {
		lines = readJSONL(t, jsonl)
		if len(lines) == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if len(lines) == 0 {
		t.Fatal("no lines written")
	}
	resAttrs := lines[0]["resource_attrs"].(map[string]any)
	if _, ok := resAttrs["user.email"]; ok {
		t.Errorf("user.email must be stripped from JSONL: %v", resAttrs)
	}
	if _, ok := resAttrs["team.password"]; ok {
		t.Errorf("unknown resource attr team.password must be dropped: %v", resAttrs)
	}
	if !r.HasResourceEmailFlag() {
		t.Errorf("currentResourceAttrsHasEmail flag should be true after observing user.email")
	}
}

func TestReceiver_NaNInfDropped(t *testing.T) {
	r, jsonl := newTestReceiver(t)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	req := &metricsv1.MetricsData{
		ResourceMetrics: []*metricsv1.ResourceMetrics{{
			ScopeMetrics: []*metricsv1.ScopeMetrics{{
				Metrics: []*metricsv1.Metric{{
					Name: "claude_code.token.usage",
					Data: &metricsv1.Metric_Sum{Sum: &metricsv1.Sum{
						DataPoints: []*metricsv1.NumberDataPoint{
							{Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: math.NaN()}},
							{Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 42}},
							{Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: math.Inf(1)}},
						},
					}},
				}},
			}},
		}},
	}
	body, _ := proto.Marshal(req)
	postProto(t, r.Addr(), body)

	var lines []map[string]any
	for i := 0; i < 50 && len(lines) == 0; i++ {
		lines = readJSONL(t, jsonl)
		if len(lines) == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 finite point, got %d", len(lines))
	}
	if v, _ := lines[0]["value"].(float64); v != 42 {
		t.Errorf("expected value 42 (the only finite point), got %v", v)
	}
}
