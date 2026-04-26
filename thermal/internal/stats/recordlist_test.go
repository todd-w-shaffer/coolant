package stats

import (
	"encoding/json"
	"testing"
	"time"
)

func mkRec(value int64, agentID, sessionID string, atSec int) RecordEntry {
	return RecordEntry{
		Value:     value,
		AgentID:   agentID,
		SessionID: sessionID,
		At:        time.Unix(int64(atSec), 0).UTC(),
	}
}

func mkBurst(count int64, sessionID string, atSec int) BurstRecord {
	return BurstRecord{
		Count:     count,
		WindowS:   2,
		SessionID: sessionID,
		At:        time.Unix(int64(atSec), 0).UTC(),
	}
}

// ── RecordList insertion / sort / cap / dedup / ties ───────

func TestRecordListInsertSorted(t *testing.T) {
	var rl RecordList
	for i, v := range []int64{3, 7, 1, 9, 5, 8, 2} {
		rl = rl.Insert(mkRec(v, "a"+string(rune('0'+i)), "s1", i))
	}
	if len(rl) != recordListCap {
		t.Fatalf("cap: want %d, got %d", recordListCap, len(rl))
	}
	want := []int64{9, 8, 7, 5, 3}
	for i, v := range want {
		if rl[i].Value != v {
			t.Errorf("rl[%d].Value: want %d, got %d", i, v, rl[i].Value)
		}
	}
}

func TestRecordListDedupByCompositeKey(t *testing.T) {
	var rl RecordList
	rl = rl.Insert(mkRec(5, "agent-x", "sess-1", 100))
	rl = rl.Insert(mkRec(8, "agent-x", "sess-1", 200))
	if len(rl) != 1 {
		t.Fatalf("dedup: want 1 entry, got %d", len(rl))
	}
	if rl[0].Value != 8 {
		t.Errorf("dedup: higher value wins, want 8, got %d", rl[0].Value)
	}
}

func TestRecordListDedupKeepsHigherEvenOnLaterArrival(t *testing.T) {
	var rl RecordList
	rl = rl.Insert(mkRec(8, "agent-x", "sess-1", 100))
	rl = rl.Insert(mkRec(5, "agent-x", "sess-1", 200))
	if len(rl) != 1 {
		t.Fatalf("dedup: want 1, got %d", len(rl))
	}
	if rl[0].Value != 8 {
		t.Errorf("dedup: original higher value wins, got %d", rl[0].Value)
	}
}

func TestRecordListTieKeepsAll(t *testing.T) {
	var rl RecordList
	for i := 0; i < 6; i++ {
		rl = rl.Insert(mkRec(10, "a"+string(rune('0'+i)), "s"+string(rune('0'+i)), i))
	}
	if len(rl) != 6 {
		t.Fatalf("tie-keeps-all: want 6 entries beyond cap=%d, got %d", recordListCap, len(rl))
	}
}

func TestRecordListZeroValueBoundaryTruncatesToCap(t *testing.T) {
	// Six entries with distinct composite keys, all value=0 (e.g.
	// every agent.stop NTP-clamped to a zero duration). Without the
	// `boundaryValue == 0` short-circuit in sortAndTrim, the
	// tie-keep rule would let the list grow without bound. Pin
	// truncation to recordListCap so a regression here can't slip
	// past CI.
	var rl RecordList
	for i := 0; i < 6; i++ {
		rl = rl.Insert(mkRec(0, "a"+string(rune('0'+i)), "s"+string(rune('0'+i)), i))
	}
	if len(rl) != recordListCap {
		t.Errorf("zero-value boundary should truncate to cap=%d, got %d (boundary tie-keep regression?)", recordListCap, len(rl))
	}
}

func TestBurstRecordListZeroCountBoundaryTruncatesToCap(t *testing.T) {
	var bl BurstRecordList
	for i := 0; i < 6; i++ {
		bl = bl.Insert(mkBurst(0, "s"+string(rune('0'+i)), i))
	}
	if len(bl) != recordListCap {
		t.Errorf("zero-count boundary should truncate to cap=%d, got %d", recordListCap, len(bl))
	}
}

// ── RecordList custom UnmarshalJSON (v1↔v2 migration) ──────

func TestRecordListUnmarshalAcceptsArray(t *testing.T) {
	at := time.Unix(1700000000, 0).UTC()
	src, _ := json.Marshal([]RecordEntry{
		{Value: 9, AgentID: "a1", At: at},
		{Value: 7, AgentID: "a2", At: at.Add(time.Hour)},
	})
	var rl RecordList
	if err := rl.UnmarshalJSON(src); err != nil {
		t.Fatalf("unmarshal array: %v", err)
	}
	if len(rl) != 2 {
		t.Errorf("array form: want 2, got %d", len(rl))
	}
}

func TestRecordListUnmarshalAcceptsSingleObject(t *testing.T) {
	at := time.Unix(1700000000, 0).UTC()
	src, _ := json.Marshal(RecordEntry{Value: 42, AgentID: "legacy", At: at})
	var rl RecordList
	if err := rl.UnmarshalJSON(src); err != nil {
		t.Fatalf("unmarshal single: %v", err)
	}
	if len(rl) != 1 {
		t.Fatalf("v1 fallback: want 1 entry, got %d", len(rl))
	}
	if rl[0].Value != 42 {
		t.Errorf("v1 fallback: want value 42, got %d", rl[0].Value)
	}
}

func TestRecordListUnmarshalRejectsGarbage(t *testing.T) {
	var rl RecordList
	if err := rl.UnmarshalJSON([]byte(`"not an entry"`)); err == nil {
		t.Errorf("garbage payload should error, got nil")
	}
}

// ── BurstRecordList parallels ───────────────────────────────

func TestBurstRecordListInsertSorted(t *testing.T) {
	var bl BurstRecordList
	for i, c := range []int64{3, 7, 1, 9, 5, 8, 2} {
		bl = bl.Insert(mkBurst(c, "s"+string(rune('0'+i)), i))
	}
	if len(bl) != recordListCap {
		t.Fatalf("burst cap: want %d, got %d", recordListCap, len(bl))
	}
	want := []int64{9, 8, 7, 5, 3}
	for i, c := range want {
		if bl[i].Count != c {
			t.Errorf("bl[%d].Count: want %d, got %d", i, c, bl[i].Count)
		}
	}
}

func TestBurstRecordListDedupByCompositeKey(t *testing.T) {
	var bl BurstRecordList
	at := time.Unix(1700000000, 0).UTC()
	bl = bl.Insert(BurstRecord{Count: 5, WindowS: 2, SessionID: "s1", At: at})
	bl = bl.Insert(BurstRecord{Count: 9, WindowS: 2, SessionID: "s1", At: at})
	if len(bl) != 1 {
		t.Fatalf("burst dedup: want 1, got %d", len(bl))
	}
	if bl[0].Count != 9 {
		t.Errorf("burst dedup: want count 9, got %d", bl[0].Count)
	}
}

func TestBurstRecordListUnmarshalAcceptsArray(t *testing.T) {
	at := time.Unix(1700000000, 0).UTC()
	src, _ := json.Marshal([]BurstRecord{
		{Count: 9, WindowS: 2, SessionID: "s1", At: at},
		{Count: 4, WindowS: 2, SessionID: "s2", At: at.Add(time.Hour)},
	})
	var bl BurstRecordList
	if err := bl.UnmarshalJSON(src); err != nil {
		t.Fatalf("unmarshal burst array: %v", err)
	}
	if len(bl) != 2 {
		t.Errorf("burst array: want 2, got %d", len(bl))
	}
}

func TestBurstRecordListUnmarshalAcceptsSingleObject(t *testing.T) {
	at := time.Unix(1700000000, 0).UTC()
	src, _ := json.Marshal(BurstRecord{Count: 7, WindowS: 2, SessionID: "legacy", At: at})
	var bl BurstRecordList
	if err := bl.UnmarshalJSON(src); err != nil {
		t.Fatalf("unmarshal burst single: %v", err)
	}
	if len(bl) != 1 {
		t.Fatalf("burst v1 fallback: want 1, got %d", len(bl))
	}
	if bl[0].Count != 7 {
		t.Errorf("burst v1 fallback: want count 7, got %d", bl[0].Count)
	}
}

// ── leaderboard-merge (max-merge replacement) ──────────────

func TestRecordListMergeUnionsDedupsTruncates(t *testing.T) {
	var disk RecordList
	disk = disk.Insert(mkRec(10, "a1", "s1", 100))
	disk = disk.Insert(mkRec(8, "a2", "s2", 200))
	disk = disk.Insert(mkRec(6, "a3", "s3", 300))

	var cand RecordList
	// Overlapping AgentID+SessionID with disk a2: higher value wins.
	cand = cand.Insert(mkRec(12, "a2", "s2", 400))
	cand = cand.Insert(mkRec(7, "a4", "s4", 500))

	merged := disk.Merge(cand)
	if len(merged) > recordListCap {
		t.Fatalf("merge exceeded cap: %d", len(merged))
	}
	// Order: 12, 10, 8 should NOT appear (a2 deduped to 12), 7, 6.
	want := []int64{12, 10, 7, 6}
	if len(merged) != len(want) {
		t.Fatalf("merge length: want %d, got %d (%+v)", len(want), len(merged), merged)
	}
	for i, v := range want {
		if merged[i].Value != v {
			t.Errorf("merged[%d].Value: want %d, got %d", i, v, merged[i].Value)
		}
	}
}

func TestBurstRecordListMergeUnionsDedupsTruncates(t *testing.T) {
	atA := time.Unix(1700000000, 0).UTC()
	atB := time.Unix(1700001000, 0).UTC()

	var disk BurstRecordList
	disk = disk.Insert(BurstRecord{Count: 10, WindowS: 2, SessionID: "s1", At: atA})
	disk = disk.Insert(BurstRecord{Count: 8, WindowS: 2, SessionID: "s2", At: atA})

	var cand BurstRecordList
	// Overlap: same SessionID+At as disk s1, higher count wins.
	cand = cand.Insert(BurstRecord{Count: 12, WindowS: 2, SessionID: "s1", At: atA})
	cand = cand.Insert(BurstRecord{Count: 7, WindowS: 2, SessionID: "s3", At: atB})

	merged := disk.Merge(cand)
	want := []int64{12, 8, 7}
	if len(merged) != len(want) {
		t.Fatalf("burst merge length: want %d, got %d (%+v)", len(want), len(merged), merged)
	}
	for i, c := range want {
		if merged[i].Count != c {
			t.Errorf("merged burst[%d].Count: want %d, got %d", i, c, merged[i].Count)
		}
	}
}
