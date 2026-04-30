package cc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
)

// Resource-attribute allowlist (§3.2). user.email is intentionally
// EXCLUDED — only its presence sets currentResourceAttrsHasEmail.
var resourceAttrAllowlist = map[string]bool{
	"service.name":      true,
	"service.version":   true,
	"user.account_uuid": true,
	"session.id":        true,
	"organization.id":   true,
}

// Per-data-point attribute allowlist (§3.2 + §0.13).
var dataPointAttrAllowlist = map[string]bool{
	"model":        true,
	"query_source": true,
	"type":         true,
	"speed":        true,
	"effort":       true,
	"start_type":   true,
	"tool_name":    true,
	"decision":     true,
	"source":       true,
	"language":     true,
}

const (
	defaultBodyCapBytes  int64 = 1 * 1024 * 1024
	defaultJSONLRateCap  int64 = 10 * 1024 * 1024
	defaultRateWindow          = time.Minute
	defaultShutdownGrace       = 2 * time.Second
)

// ReceiverConfig configures the embedded OTLP HTTP receiver.
type ReceiverConfig struct {
	Addr      string  // default 127.0.0.1
	Port      int     // default 4318
	JSONLPath string  // $TMPDIR/coolant-$USER.cc-otel.jsonl when empty
	Findings  *Writer // findings writer for one-shot kinds
	// CCVersion is recorded into one-shot findings emitted by the
	// receiver (e.g., receiver_bind_failed) so the writer can correlate
	// across cc_version churn — empty for the receiver since it fires
	// before the schema fingerprint sees its first metric.
	CCVersion      string
	CoolantVersion string
}

// Receiver is the embedded OTLP/HTTP+protobuf receiver.
type Receiver struct {
	cfg    ReceiverConfig
	server *http.Server

	mu      sync.Mutex
	jsonlMu sync.Mutex // serializes JSONL appends across goroutines

	listener net.Listener

	rateMu          sync.Mutex
	rateBytes       int64
	rateWindowStart time.Time
	rateLimited     bool

	emailFlagMu          sync.RWMutex
	hasEmailFlag         bool
	lastSuccessfulPostTS time.Time
}

// NewReceiver constructs a Receiver — does not start it. Start binds
// and serves; Shutdown drains.
func NewReceiver(cfg ReceiverConfig) (*Receiver, error) {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 4318
	}
	if cfg.JSONLPath == "" {
		return nil, errors.New("receiver: JSONLPath required")
	}
	if cfg.Findings == nil {
		return nil, errors.New("receiver: Findings writer required")
	}
	return &Receiver{cfg: cfg}, nil
}

// Addr returns the bind address (post-Start, includes the actual port
// when the test passed Port=0).
func (r *Receiver) Addr() string {
	if r.listener != nil {
		return r.listener.Addr().String()
	}
	return fmt.Sprintf("%s:%d", r.cfg.Addr, r.cfg.Port)
}

// HasResourceEmailFlag reflects whether the most recent metric carried
// resource_attrs.user.email — used by §0.9 transient_auth_gap detection.
func (r *Receiver) HasResourceEmailFlag() bool {
	r.emailFlagMu.RLock()
	defer r.emailFlagMu.RUnlock()
	return r.hasEmailFlag
}

// LastSuccessfulPostTS is the timestamp of the last accepted POST.
// Used by ReconcileToday to disambiguate cc_otel_offline (receiver-down)
// from tailer-stuck per §0.9.
func (r *Receiver) LastSuccessfulPostTS() time.Time {
	r.emailFlagMu.RLock()
	defer r.emailFlagMu.RUnlock()
	return r.lastSuccessfulPostTS
}

// Start binds the listener with SO_REUSEADDR/SO_REUSEPORT, starts the
// HTTP server in a background goroutine, and returns once the listener
// is bound (or an error if bind fails — in which case it also fires
// ONE receiver_bind_failed finding through the writer).
func (r *Receiver) Start() error {
	if err := os.MkdirAll(filepath.Dir(r.cfg.JSONLPath), 0o700); err != nil {
		return err
	}
	lc := net.ListenConfig{Control: setReuseAddr}
	addr := fmt.Sprintf("%s:%d", r.cfg.Addr, r.cfg.Port)
	l, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		_ = r.cfg.Findings.Write(Finding{
			Schema:         1,
			TS:             time.Now().UTC(),
			WindowAnchor:   time.Now().UTC().Format("2006-01-02"),
			SignalType:     SignalTypeMetric,
			FindingKind:    KindReceiverBindFailed,
			CCVersion:      r.cfg.CCVersion,
			CoolantVersion: r.cfg.CoolantVersion,
			Severity:       SeverityInfo,
			Detail: &SchemaDriftDetail{
				FieldName: "bind:" + addr,
			},
		})
		return err
	}
	r.listener = l

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/metrics", r.handleMetrics)
	mux.HandleFunc("/v1/logs", r.handle404)
	mux.HandleFunc("/v1/traces", r.handle404)

	r.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		_ = r.server.Serve(l)
	}()
	return nil
}

// Shutdown drains the HTTP server with a bounded grace window.
func (r *Receiver) Shutdown(ctx context.Context) error {
	if r.server == nil {
		return nil
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultShutdownGrace)
		defer cancel()
	}
	return r.server.Shutdown(ctx)
}

func (r *Receiver) handle404(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented in v1", http.StatusNotFound)
}

func (r *Receiver) handleMetrics(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, defaultBodyCapBytes))
	if err != nil {
		// MaxBytesReader returns a *MaxBytesError on overflow.
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	if shed := r.checkRateCap(int64(len(body))); shed {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}

	// MetricsData has the same wire format as ExportMetricsServiceRequest
	// (both `repeated ResourceMetrics resource_metrics = 1`). Decoding
	// into MetricsData avoids importing the collector/metrics/v1 package
	// (which pulls in gRPC stubs). Per spec §0.1b — gRPC is out.
	var data metricsv1.MetricsData
	if err := proto.Unmarshal(body, &data); err != nil {
		http.Error(w, "invalid protobuf", http.StatusBadRequest)
		return
	}

	if err := r.writeJSONL(&data); err != nil {
		http.Error(w, "write error", http.StatusInternalServerError)
		return
	}

	r.emailFlagMu.Lock()
	r.lastSuccessfulPostTS = time.Now().UTC()
	r.emailFlagMu.Unlock()

	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
}

// checkRateCap implements the §3.2 sliding-window 10MB/min rate cap.
// Returns true if the request should be shed (and fires
// receiver_rate_limited at most once per offending minute).
func (r *Receiver) checkRateCap(n int64) bool {
	r.rateMu.Lock()
	defer r.rateMu.Unlock()
	now := time.Now()
	if now.Sub(r.rateWindowStart) >= defaultRateWindow {
		r.rateWindowStart = now
		r.rateBytes = 0
		r.rateLimited = false
		// fresh minute — let the writer fire receiver_rate_limited again
		// next time the cap is hit.
		r.cfg.Findings.ResetOneShot(KindReceiverRateLimited)
	}
	if r.rateBytes+n > defaultJSONLRateCap {
		if !r.rateLimited {
			r.rateLimited = true
			_ = r.cfg.Findings.Write(Finding{
				Schema:         1,
				TS:             now.UTC(),
				WindowAnchor:   now.UTC().Format("2006-01-02"),
				SignalType:     SignalTypeMetric,
				FindingKind:    KindReceiverRateLimited,
				CCVersion:      r.cfg.CCVersion,
				CoolantVersion: r.cfg.CoolantVersion,
				Severity:       SeverityInfo,
			})
		}
		return true
	}
	r.rateBytes += n
	return false
}

// jsonlLine is the on-disk shape of one cc-otel JSONL row (§3.2).
type jsonlLine struct {
	Schema        int               `json:"schema"`
	TS            string            `json:"ts"`
	Metric        string            `json:"metric"`
	Value         float64           `json:"value"`
	ResourceAttrs map[string]string `json:"resource_attrs,omitempty"`
	Attrs         map[string]string `json:"attrs,omitempty"`
}

func (r *Receiver) writeJSONL(data *metricsv1.MetricsData) error {
	r.jsonlMu.Lock()
	defer r.jsonlMu.Unlock()

	fh, err := os.OpenFile(r.cfg.JSONLPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer fh.Close()
	bw := bufio.NewWriter(fh)

	now := time.Now().UTC()
	hasEmailAny := false
	for _, rm := range data.GetResourceMetrics() {
		attrs := rm.GetResource().GetAttributes()
		resAttrs := filterAttrs(attrs, resourceAttrAllowlist)
		if observedEmail(attrs) {
			hasEmailAny = true
		}
		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				if err := r.writeMetric(bw, m, resAttrs, now); err != nil {
					return err
				}
			}
		}
	}

	r.emailFlagMu.Lock()
	r.hasEmailFlag = hasEmailAny
	r.emailFlagMu.Unlock()

	return bw.Flush()
}

// writeMetric writes one metric's data points. Only Sum and Gauge are
// documented for CC OTEL today; other types fall through harmlessly.
func (r *Receiver) writeMetric(w *bufio.Writer, m *metricsv1.Metric, resAttrs map[string]string, now time.Time) error {
	var dps []*metricsv1.NumberDataPoint
	switch d := m.GetData().(type) {
	case *metricsv1.Metric_Sum:
		dps = d.Sum.GetDataPoints()
	case *metricsv1.Metric_Gauge:
		dps = d.Gauge.GetDataPoints()
	default:
		return nil
	}
	for _, dp := range dps {
		if err := r.appendDataPoint(w, m.GetName(), dp, resAttrs, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *Receiver) appendDataPoint(w *bufio.Writer, name string, dp *metricsv1.NumberDataPoint, resAttrs map[string]string, now time.Time) error {
	val, ok := numericValue(dp)
	if !ok {
		_ = r.cfg.Findings.Write(Finding{
			Schema:         1,
			TS:             now,
			WindowAnchor:   now.Format("2006-01-02"),
			SignalType:     SignalTypeMetric,
			FindingKind:    KindNonFiniteMetric,
			CCVersion:      r.cfg.CCVersion,
			CoolantVersion: r.cfg.CoolantVersion,
			Metric:         name,
			Severity:       SeverityInfo,
		})
		return nil
	}
	ts := now
	if t := dp.GetTimeUnixNano(); t != 0 {
		ts = time.Unix(0, int64(t)).UTC()
	}
	line := jsonlLine{
		Schema:        1,
		TS:            ts.Format(time.RFC3339Nano),
		Metric:        name,
		Value:         val,
		ResourceAttrs: resAttrs,
		Attrs:         filterAttrs(dp.GetAttributes(), dataPointAttrAllowlist),
	}
	buf, err := json.Marshal(line)
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	_, err = w.Write(buf)
	return err
}

// numericValue extracts the data point's value, returning (val, false)
// if NaN/Inf or unset (non-finite per §3.2).
func numericValue(dp *metricsv1.NumberDataPoint) (float64, bool) {
	switch v := dp.GetValue().(type) {
	case *metricsv1.NumberDataPoint_AsInt:
		return float64(v.AsInt), true
	case *metricsv1.NumberDataPoint_AsDouble:
		f := v.AsDouble
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// filterAttrs keeps only the keys named in allowlist. Unknown keys
// (including content-gated and identity attrs) are dropped silently —
// the §0.13 PII guarantee depends on this allowlist discipline.
func filterAttrs(kvs []*commonv1.KeyValue, allowlist map[string]bool) map[string]string {
	out := map[string]string{}
	for _, kv := range kvs {
		if !allowlist[kv.GetKey()] {
			continue
		}
		out[kv.GetKey()] = anyValueToString(kv.GetValue())
	}
	return out
}

func observedEmail(kvs []*commonv1.KeyValue) bool {
	for _, kv := range kvs {
		if kv.GetKey() == "user.email" {
			s := anyValueToString(kv.GetValue())
			return s != ""
		}
	}
	return false
}

func anyValueToString(av *commonv1.AnyValue) string {
	if av == nil {
		return ""
	}
	switch v := av.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		return v.StringValue
	case *commonv1.AnyValue_IntValue:
		return strconv.FormatInt(v.IntValue, 10)
	case *commonv1.AnyValue_DoubleValue:
		return strconv.FormatFloat(v.DoubleValue, 'f', -1, 64)
	case *commonv1.AnyValue_BoolValue:
		return strconv.FormatBool(v.BoolValue)
	}
	return ""
}

// setReuseAddr is the net.ListenConfig.Control function that sets
// SO_REUSEADDR (and SO_REUSEPORT on Darwin). Without this, restarting
// thermo within ~30s on the same port generates spurious bind-failure
// findings.
func setReuseAddr(network, address string, c syscall.RawConn) error {
	var ctlErr error
	err := c.Control(func(fd uintptr) {
		if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
			ctlErr = err
			return
		}
		// SO_REUSEPORT is supported on Darwin and Linux; harmless if it
		// returns an error on other unixes.
		_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	})
	if err != nil {
		return err
	}
	return ctlErr
}
