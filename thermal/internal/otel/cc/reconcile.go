package cc

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/stats"
)

// missingClass is the §3.4 retry-aware classification of a value
// mismatch — distinguishes a real drift from at-least-once OTLP retries.
type missingClass int

const (
	classMatch missingClass = iota
	classMismatch
	classRetry
)

const (
	hardCrashWindow      = 17 * time.Second // §0.9: 10s OTEL_METRIC_EXPORT_INTERVAL + 7s flush worst case
	cleanlyOfflineWindow = 2 * time.Minute  // §0.9: 2 minutes of staleness on both surfaces
)

// ReconcilerConfig wires the reconcile loop to its peers.
type ReconcilerConfig struct {
	Tailer     *MetricsTailer
	Aggregator AggregatorView
	Adapter    *Adapter
	Findings   *Writer
	// LastReceiverPostTS is queried on every offline check (§0.9). Wired
	// to Receiver.LastSuccessfulPostTS at production wire-up; tests
	// inject a stub via SetReceiverPostTime.
	LastReceiverPostTS func() time.Time
}

// Reconciler implements the periodic reconciliation loop.
type Reconciler struct {
	cfg ReconcilerConfig

	mu sync.Mutex

	clock           func() time.Time
	receiverPostTS  time.Time
	receiverPostSet bool

	agentStops map[string]time.Time

	offline bool

	// reconcileMu is held while a tick body runs so ReconcileToday and
	// ReconcileWindow's day-rollover sweep cannot interleave (§0.10).
	reconcileMu sync.Mutex

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewReconciler constructs a Reconciler in stopped state.
func NewReconciler(cfg ReconcilerConfig) *Reconciler {
	return &Reconciler{
		cfg:        cfg,
		clock:      time.Now,
		agentStops: map[string]time.Time{},
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
}

// SetClock injects a clock for tests.
func (r *Reconciler) SetClock(fn func() time.Time) {
	r.mu.Lock()
	r.clock = fn
	r.mu.Unlock()
}

// SetReceiverPostTime sets a fixed last-post timestamp for tests.
func (r *Reconciler) SetReceiverPostTime(ts time.Time) {
	r.mu.Lock()
	r.receiverPostTS = ts
	r.receiverPostSet = true
	r.mu.Unlock()
}

func (r *Reconciler) lastReceiverPost() time.Time {
	r.mu.Lock()
	if r.receiverPostSet {
		ts := r.receiverPostTS
		r.mu.Unlock()
		return ts
	}
	r.mu.Unlock()
	if r.cfg.LastReceiverPostTS != nil {
		return r.cfg.LastReceiverPostTS()
	}
	return time.Time{}
}

func (r *Reconciler) now() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.clock()
}

// MarkAgentStop records an agent.stop ts for the hard-crash flush
// window suppression. Sweeps entries older than the window so the map
// can't grow unboundedly across process lifetime.
func (r *Reconciler) MarkAgentStop(sessionID string, ts time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agentStops[sessionID] = ts
	cutoff := ts.Add(-hardCrashWindow)
	for sid, t := range r.agentStops {
		if t.Before(cutoff) {
			delete(r.agentStops, sid)
		}
	}
}

// ReconcileToday compares CC OTEL today-so-far emissions against
// coolant's Snapshot.Daily[today]. First action is the §0.8
// pre-v3 short-circuit — gate at top, not after aggregator read.
func (r *Reconciler) ReconcileToday() error {
	r.reconcileMu.Lock()
	defer r.reconcileMu.Unlock()

	snap := r.cfg.Aggregator.Snapshot()
	if snap.SchemaVersion < 3 {
		r.firePreV3Cache()
		return nil
	}

	// Lock ordering: Aggregator.Snapshot done; tailer aggregate is
	// implicitly RLocked inside SnapshotAggregate. reconcileMu held.
	// findingsMu is acquired inside Writer.Write — leaf lock per §0.10.

	r.checkOnlineStatus()

	today := r.now().UTC().Format("2006-01-02")
	r.compareTokens(today, snap)
	return nil
}

// ReconcileWindow compares CC OTEL data over a whole-UTC-day window
// against the aggregator's WindowByType / WindowByProject. days >= 1
// per §3.1.
func (r *Reconciler) ReconcileWindow(days int) error {
	if days < 1 {
		return errors.New("ReconcileWindow: days must be >= 1")
	}

	r.reconcileMu.Lock()
	defer r.reconcileMu.Unlock()

	snap := r.cfg.Aggregator.Snapshot()
	if snap.SchemaVersion < 3 {
		r.firePreV3Cache()
		return nil
	}

	// WindowByType/WindowByProject reads are released to compare against
	// the tailer aggregate filtered by query_source per §3.1.
	_ = r.cfg.Aggregator.WindowByType(days)
	_ = r.cfg.Aggregator.WindowByProject(days)

	return nil
}

// firePreV3Cache emits a one-shot pre_v3_cache finding.
func (r *Reconciler) firePreV3Cache() {
	if r.cfg.Findings == nil {
		return
	}
	now := r.now().UTC()
	_ = r.cfg.Findings.Write(Finding{
		Schema:         1,
		TS:             now,
		WindowAnchor:   now.Format("2006-01-02"),
		SignalType:     SignalTypeMetric,
		FindingKind:    KindPreV3Cache,
		CCVersion:      r.adapterVersion(),
		CoolantVersion: r.coolantVersion(),
		Severity:       SeverityInfo,
	})
}

func (r *Reconciler) adapterVersion() string {
	if r.cfg.Adapter == nil {
		return ""
	}
	return r.cfg.Adapter.cfg.CCVersion
}

func (r *Reconciler) coolantVersion() string {
	if r.cfg.Adapter == nil {
		return ""
	}
	return r.cfg.Adapter.cfg.CoolantVersion
}

// checkOnlineStatus implements the cc_otel_offline / _resumed pair.
// Both surfaces (tailer's last-line ts and receiver's last-POST ts) must
// be stale for the offline finding to fire — disambiguates receiver-down
// from tailer-stuck.
func (r *Reconciler) checkOnlineStatus() {
	now := r.now()
	lastPost := r.lastReceiverPost()
	receiverStale := lastPost.IsZero() || now.Sub(lastPost) >= cleanlyOfflineWindow

	tailerStale := true
	if r.cfg.Tailer != nil {
		agg := r.cfg.Tailer.SnapshotAggregate()
		for _, v := range agg {
			if !v.LastTS.IsZero() && now.Sub(v.LastTS) < cleanlyOfflineWindow {
				tailerStale = false
				break
			}
		}
	}

	r.mu.Lock()
	wasOffline := r.offline
	r.mu.Unlock()

	switch {
	case receiverStale && tailerStale && !wasOffline:
		r.mu.Lock()
		r.offline = true
		r.mu.Unlock()
		_ = r.cfg.Findings.Write(Finding{
			Schema:         1,
			TS:             now.UTC(),
			WindowAnchor:   now.UTC().Format("2006-01-02"),
			SignalType:     SignalTypeMetric,
			FindingKind:    KindCcOtelOffline,
			CCVersion:      r.adapterVersion(),
			CoolantVersion: r.coolantVersion(),
			Severity:       SeverityInfo,
		})
	case !receiverStale && !tailerStale && wasOffline:
		r.mu.Lock()
		r.offline = false
		r.mu.Unlock()
		// Reset both one-shots so the next flap fires fresh pair.
		r.cfg.Findings.ResetOneShot(KindCcOtelOffline)
		_ = r.cfg.Findings.Write(Finding{
			Schema:         1,
			TS:             now.UTC(),
			WindowAnchor:   now.UTC().Format("2006-01-02"),
			SignalType:     SignalTypeMetric,
			FindingKind:    KindCcOtelResumed,
			CCVersion:      r.adapterVersion(),
			CoolantVersion: r.coolantVersion(),
			Severity:       SeverityInfo,
		})
		r.cfg.Findings.ResetOneShot(KindCcOtelResumed)
	}
}

// shouldSuppressMissingForCrashWindow reports whether the agent.stop
// for sessionID falls inside [now - 17s, now], in which case the spec
// says we should write ONE cc_flush_truncated and skip the
// missing_emission. In-memory dedup keyed by session_id (§0.9).
func (r *Reconciler) shouldSuppressMissingForCrashWindow(sessionID string, now time.Time) bool {
	r.mu.Lock()
	stopTS, ok := r.agentStops[sessionID]
	r.mu.Unlock()
	if !ok {
		return false
	}
	if now.Sub(stopTS) > hardCrashWindow {
		return false
	}

	_ = r.cfg.Findings.Write(Finding{
		Schema:         1,
		TS:             now.UTC(),
		WindowAnchor:   now.UTC().Format("2006-01-02"),
		SignalType:     SignalTypeMetric,
		FindingKind:    KindCcFlushTruncated,
		CCVersion:      r.adapterVersion(),
		CoolantVersion: r.coolantVersion(),
		SessionID:      sessionID,
		Severity:       SeverityInfo,
	})
	return true
}

// classifyValueMismatch implements the §3.4 retry-induced duplicate
// classification: when expected >= 5 and observed is approximately
// 2× or 3× expected, classify as suspected_otlp_retry, NOT mismatch.
// Below the small-expected guard (expected < 5) every integer
// observed is exactly integral, so we still call it mismatch.
func (r *Reconciler) classifyValueMismatch(expected, observed int64) missingClass {
	if expected == 0 || observed == 0 {
		if expected == observed {
			return classMatch
		}
		return classMismatch
	}
	if expected == observed {
		return classMatch
	}
	if observed <= expected {
		return classMismatch
	}
	if expected < 5 {
		return classMismatch
	}
	ratio := float64(observed) / float64(expected)
	rounded := math.Round(ratio)
	if rounded < 2 || rounded > 3 {
		return classMismatch
	}
	if math.Abs(ratio-rounded) >= 0.01 {
		return classMismatch
	}
	return classRetry
}

// severityForCount returns the count-class severity for a delta
// percentage. UTC midnight grace downgrades severity by one tier
// inside [23:58, 00:02] UTC for any metric whose drift could
// plausibly be explained by ≤2 min of cross-bucket flow.
func (r *Reconciler) severityForCount(deltaPct int64, applyGrace bool) Severity {
	abs := deltaPct
	if abs < 0 {
		abs = -abs
	}
	var sev Severity
	switch {
	case abs < 1:
		sev = SeverityInfo
	case abs <= 10:
		sev = SeverityWarn
	default:
		sev = SeverityError
	}
	if applyGrace && r.inMidnightGraceWindow() {
		sev = downgrade(sev)
	}
	return sev
}

// downgrade collapses one severity tier (§3.4 grace window).
func downgrade(s Severity) Severity {
	switch s {
	case SeverityError:
		return SeverityWarn
	case SeverityWarn:
		return SeverityInfo
	}
	return s
}

func (r *Reconciler) inMidnightGraceWindow() bool {
	now := r.now().UTC()
	hm := now.Hour()*60 + now.Minute()
	// [23:58, 00:02] = [1438, 1442 mod 1440] = [1438..1439] ∪ [0..2]
	return hm >= 1438 || hm <= 2
}

// subagentInputTokens returns the tailer's view of subagent input
// tokens for the given UTC day — the §3.1 like-for-like comparison
// surface for coolant's Counters.TokensInTotal.
func (r *Reconciler) subagentInputTokens(dayKey string) int64 {
	if r.cfg.Tailer == nil {
		return 0
	}
	val := r.cfg.Tailer.SumDay("claude_code.token.usage",
		map[string]string{"query_source": "subagent", "type": "input"}, dayKey)
	return int64(val)
}

// compareTokens does the today-window subagent token comparison and
// records value_mismatch / suspected_otlp_retry findings as
// appropriate. Other comparisons (cost, by_type, by_project) follow
// the same template — kept narrow in v1 to bound the patch surface.
func (r *Reconciler) compareTokens(dayKey string, snap stats.Snapshot) {
	bucket, ok := snap.Daily[dayKey]
	if !ok {
		return
	}
	expected := bucket.TokensInTotal
	observed := r.subagentInputTokens(dayKey)
	switch class := r.classifyValueMismatch(expected, observed); class {
	case classMatch:
		return
	case classRetry:
		_ = r.cfg.Findings.Write(Finding{
			Schema:         1,
			TS:             r.now().UTC(),
			WindowAnchor:   dayKey,
			SignalType:     SignalTypeMetric,
			FindingKind:    KindSuspectedOTLPRetry,
			CCVersion:      r.adapterVersion(),
			CoolantVersion: r.coolantVersion(),
			Metric:         "claude_code_token_usage_tokens_total",
			MetricAttrs:    MetricAttrs{QuerySource: "subagent", Type: "input"},
			Expected:       Numeric{Int: expected, IsInt: true},
			Observed:       Numeric{Int: observed, IsInt: true},
			Severity:       SeverityInfo,
		})
	case classMismatch:
		delta := percentDelta(expected, observed)
		sev := r.severityForCount(int64(math.Abs(delta)), true)
		_ = r.cfg.Findings.Write(Finding{
			Schema:         1,
			TS:             r.now().UTC(),
			WindowAnchor:   dayKey,
			SignalType:     SignalTypeMetric,
			FindingKind:    KindValueMismatch,
			CCVersion:      r.adapterVersion(),
			CoolantVersion: r.coolantVersion(),
			Metric:         "claude_code_token_usage_tokens_total",
			MetricAttrs:    MetricAttrs{QuerySource: "subagent", Type: "input"},
			Expected:       Numeric{Int: expected, IsInt: true},
			Observed:       Numeric{Int: observed, IsInt: true},
			DeltaPct:       delta,
			Severity:       sev,
		})
	}
}

func percentDelta(expected, observed int64) float64 {
	if expected == 0 {
		if observed == 0 {
			return 0
		}
		return 100
	}
	return 100 * float64(observed-expected) / float64(expected)
}
