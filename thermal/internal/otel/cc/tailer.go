package cc

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// AggregateKey identifies one aggregation bucket — the metric name plus
// a canonical string of attribute pairs and the UTC dayKey. Per-day
// keying lets the rollover sweep prune yesterday's data without losing
// today's running totals (and lets ReconcileWindow sum across days).
type AggregateKey struct {
	Metric string
	Attrs  string // canonical "k1=v1\x00k2=v2" form, sorted by key
	DayKey string // UTC YYYY-MM-DD
}

// AggregateValue carries the running sum and count plus the most-recent
// data-point timestamp.
type AggregateValue struct {
	Sum    float64
	Count  int64
	LastTS time.Time
}

// MetricsTailer mirrors collector/events.go's polling-tail pattern with
// two intentional deltas (§3.2): explicit bufio.Scanner buffer cap
// (events.go's default-buffer silently drops >64KB lines) and
// truncation-resets-aggregate (events.go works because JSONL is
// replay-authoritative; this aggregate is a sliding window).
type MetricsTailer struct {
	JSONLPath    string
	PollInterval time.Duration

	// Findings is optional — when set, oversize lines fire ONE
	// oversize_jsonl_line one-shot per process lifetime.
	Findings *Writer

	mu        sync.RWMutex
	aggregate map[AggregateKey]*AggregateValue
	offset    int64
	inode     uint64

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewMetricsTailer constructs a tailer rooted at path with default
// 500ms poll cadence (matches the events tailer; tests override).
func NewMetricsTailer(path string) *MetricsTailer {
	return &MetricsTailer{
		JSONLPath:    path,
		PollInterval: 500 * time.Millisecond,
		aggregate:    map[AggregateKey]*AggregateValue{},
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

// Start kicks off the polling goroutine.
func (mt *MetricsTailer) Start() {
	go mt.run()
}

// Stop shuts down the tailer. Idempotent.
func (mt *MetricsTailer) Stop() {
	mt.stopOnce.Do(func() {
		close(mt.stopCh)
	})
	<-mt.doneCh
}

func (mt *MetricsTailer) run() {
	defer close(mt.doneCh)
	ticker := time.NewTicker(mt.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-mt.stopCh:
			return
		case <-ticker.C:
			mt.poll()
		}
	}
}

func (mt *MetricsTailer) poll() {
	fh, err := os.Open(mt.JSONLPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		return
	}
	defer fh.Close()

	info, err := fh.Stat()
	if err != nil {
		return
	}

	curInode := inodeOf(info)
	mt.mu.Lock()
	if mt.inode != 0 && curInode != mt.inode {
		// File was recreated; reset aggregate so we don't double-count
		// across re-read of surviving lines.
		mt.aggregate = map[AggregateKey]*AggregateValue{}
		mt.offset = 0
	} else if info.Size() < mt.offset {
		// Truncation.
		mt.aggregate = map[AggregateKey]*AggregateValue{}
		mt.offset = 0
	}
	mt.inode = curInode
	startOffset := mt.offset
	mt.mu.Unlock()

	if _, err := fh.Seek(startOffset, io.SeekStart); err != nil {
		return
	}

	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		mt.absorbLine(line)
	}
	if err := sc.Err(); err != nil {
		// bufio.ErrTooLong fires when a line exceeds the 1 MB cap.
		if errors.Is(err, bufio.ErrTooLong) && mt.Findings != nil {
			_ = mt.Findings.Write(Finding{
				Schema:       1,
				TS:           time.Now().UTC(),
				WindowAnchor: time.Now().UTC().Format("2006-01-02"),
				SignalType:   SignalTypeMetric,
				FindingKind:  KindOversizeJSONLLine,
				Severity:     SeverityInfo,
			})
		}
	}

	newOffset, err := fh.Seek(0, io.SeekCurrent)
	if err != nil {
		return
	}
	mt.mu.Lock()
	mt.offset = newOffset
	mt.mu.Unlock()
}

func (mt *MetricsTailer) absorbLine(line []byte) {
	var jl jsonlLine
	if err := json.Unmarshal(line, &jl); err != nil {
		return
	}
	if jl.Metric == "" {
		return
	}

	ts, _ := time.Parse(time.RFC3339Nano, jl.TS)
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	key := AggregateKey{
		Metric: jl.Metric,
		Attrs:  canonicalAttrs(jl.Attrs),
		DayKey: ts.UTC().Format("2006-01-02"),
	}

	mt.mu.Lock()
	defer mt.mu.Unlock()
	v, ok := mt.aggregate[key]
	if !ok {
		v = &AggregateValue{}
		mt.aggregate[key] = v
	}
	v.Sum += jl.Value
	v.Count++
	if ts.After(v.LastTS) {
		v.LastTS = ts
	}
}

// canonicalAttrs returns a stable string form of attrs so two
// data points with the same logical attribute set hash to the same
// AggregateKey regardless of map iteration order.
func canonicalAttrs(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(0)
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(attrs[k])
	}
	return sb.String()
}

// Sum returns the running sum for a (metric, attrs) bucket across all
// days currently held in the aggregate.
func (mt *MetricsTailer) Sum(metric string, attrs map[string]string) float64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	canonical := canonicalAttrs(attrs)
	var total float64
	for k, v := range mt.aggregate {
		if k.Metric == metric && k.Attrs == canonical {
			total += v.Sum
		}
	}
	return total
}

// SumDay returns the running sum for a (metric, attrs, day) bucket.
// dayKey is "YYYY-MM-DD" UTC.
func (mt *MetricsTailer) SumDay(metric string, attrs map[string]string, dayKey string) float64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	if v, ok := mt.aggregate[AggregateKey{Metric: metric, Attrs: canonicalAttrs(attrs), DayKey: dayKey}]; ok {
		return v.Sum
	}
	return 0
}

// Count returns the number of data points absorbed for a (metric, attrs) bucket
// across all days currently held in the aggregate.
func (mt *MetricsTailer) Count(metric string, attrs map[string]string) int64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	canonical := canonicalAttrs(attrs)
	var total int64
	for k, v := range mt.aggregate {
		if k.Metric == metric && k.Attrs == canonical {
			total += v.Count
		}
	}
	return total
}

// SnapshotAggregate returns a deep copy of the current aggregate state
// for the reconcile loop's causally-consistent comparison (§0.10).
func (mt *MetricsTailer) SnapshotAggregate() map[AggregateKey]AggregateValue {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	out := make(map[AggregateKey]AggregateValue, len(mt.aggregate))
	for k, v := range mt.aggregate {
		out[k] = *v
	}
	return out
}

// PruneOlderThan drops aggregate entries whose DayKey is older than
// `windowDays`. Called by the rollover goroutine to bound the
// aggregate's growth across multi-week thermo uptime (§3.2). Per-day
// keying lets us drop yesterday's data without losing today's running
// totals.
func (mt *MetricsTailer) PruneOlderThan(windowDays int) {
	if windowDays <= 0 {
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -windowDays).Format("2006-01-02")
	mt.mu.Lock()
	defer mt.mu.Unlock()
	for k := range mt.aggregate {
		if k.DayKey < cutoff {
			delete(mt.aggregate, k)
		}
	}
}
