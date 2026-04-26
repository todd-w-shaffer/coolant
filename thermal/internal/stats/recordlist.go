package stats

import (
	"encoding/json"
	"sort"
)

// RecordListCap is the leaderboard depth — top-N stored per category.
// Locked at 5 to fit a scoreboard column without scroll while keeping
// the per-Snapshot deep-copy hot path negligible (≈ 5 × 7 categories
// = 35 entries). Exported so CLI consumers (`thermo stats --top`)
// share the same upper bound rather than hardcoding it.
const RecordListCap = 5

// RecordList is the leaderboard slice that replaces the top-1
// `RecordEntry` field on `Records`. Entries are kept sorted desc by
// Value, deduped by composite key (AgentID|SessionID), and truncated
// to RecordListCap. Equal-value ties at the boundary are kept (the
// slice may exceed cap when every slot is at the same Value).
type RecordList []RecordEntry

// Insert adds an entry, dedupes by composite key with higher-Value
// winning, sorts desc by Value, and truncates to RecordListCap unless
// the boundary lands inside a tie.
func (rl RecordList) Insert(e RecordEntry) RecordList {
	key := recordKey(e)
	for i, existing := range rl {
		if recordKey(existing) == key {
			if e.Value > existing.Value {
				rl[i] = e
			}
			return rl.sortAndTrim()
		}
	}
	rl = append(rl, e)
	return rl.sortAndTrim()
}

// Merge unions two leaderboards via successive Insert — yields
// dedup-by-key + sorted + truncated output. Used by Checkpoint when
// folding the in-memory candidate into the freshly re-read disk
// records.
func (rl RecordList) Merge(other RecordList) RecordList {
	out := make(RecordList, len(rl))
	copy(out, rl)
	for _, e := range other {
		out = out.Insert(e)
	}
	return out
}

// Top returns the highest-value entry, or a zero-value RecordEntry
// when the list is empty. Convenience for callers that want top-1
// semantics without indexing into a possibly-empty slice.
func (rl RecordList) Top() RecordEntry {
	if len(rl) == 0 {
		return RecordEntry{}
	}
	return rl[0]
}

// UnmarshalJSON accepts both the v2 array shape and the v1
// single-object shape, so a v1 cache loads cleanly into the new
// slice type without losing the historical record.
func (rl *RecordList) UnmarshalJSON(b []byte) error {
	var arr []RecordEntry
	if err := json.Unmarshal(b, &arr); err == nil {
		*rl = arr
		return nil
	}
	var single RecordEntry
	if err := json.Unmarshal(b, &single); err != nil {
		return err
	}
	*rl = RecordList{single}
	return nil
}

func recordKey(e RecordEntry) string {
	return e.AgentID + "|" + e.SessionID
}

func (rl RecordList) sortAndTrim() RecordList {
	sort.SliceStable(rl, func(i, j int) bool {
		if rl[i].Value != rl[j].Value {
			return rl[i].Value > rl[j].Value
		}
		// Stable secondary: more recent At first so ties surface
		// freshest entry at the boundary.
		return rl[i].At.After(rl[j].At)
	})
	if len(rl) <= RecordListCap {
		return rl
	}
	// Boundary tie-keep applies only when the boundary value is
	// non-zero — otherwise a stream of zero-duration entries with
	// distinct composite keys (e.g. NTP-clamped agent stops) would
	// grow the list without bound. Zero is never a real high score.
	boundaryValue := rl[RecordListCap-1].Value
	if boundaryValue == 0 {
		return rl[:RecordListCap]
	}
	end := RecordListCap
	for end < len(rl) && rl[end].Value == boundaryValue {
		end++
	}
	return rl[:end]
}

// BurstRecordList is the parallel slice for `BurstRecord` — same
// shape rules (sort, cap, dedup) but composite key is SessionID|At
// since BurstRecord has no AgentID. Lives as its own type rather
// than reusing a generic because BurstRecord exposes Count, not
// Value.
type BurstRecordList []BurstRecord

func (bl BurstRecordList) Insert(e BurstRecord) BurstRecordList {
	key := burstKey(e)
	for i, existing := range bl {
		if burstKey(existing) == key {
			if e.Count > existing.Count {
				bl[i] = e
			}
			return bl.sortAndTrim()
		}
	}
	bl = append(bl, e)
	return bl.sortAndTrim()
}

func (bl BurstRecordList) Merge(other BurstRecordList) BurstRecordList {
	out := make(BurstRecordList, len(bl))
	copy(out, bl)
	for _, e := range other {
		out = out.Insert(e)
	}
	return out
}

func (bl BurstRecordList) Top() BurstRecord {
	if len(bl) == 0 {
		return BurstRecord{}
	}
	return bl[0]
}

func (bl *BurstRecordList) UnmarshalJSON(b []byte) error {
	var arr []BurstRecord
	if err := json.Unmarshal(b, &arr); err == nil {
		*bl = arr
		return nil
	}
	var single BurstRecord
	if err := json.Unmarshal(b, &single); err != nil {
		return err
	}
	*bl = BurstRecordList{single}
	return nil
}

func burstKey(e BurstRecord) string {
	return e.SessionID + "|" + e.At.Format("2006-01-02T15:04:05Z07:00")
}

func (bl BurstRecordList) sortAndTrim() BurstRecordList {
	sort.SliceStable(bl, func(i, j int) bool {
		if bl[i].Count != bl[j].Count {
			return bl[i].Count > bl[j].Count
		}
		return bl[i].At.After(bl[j].At)
	})
	if len(bl) <= RecordListCap {
		return bl
	}
	boundaryCount := bl[RecordListCap-1].Count
	if boundaryCount == 0 {
		return bl[:RecordListCap]
	}
	end := RecordListCap
	for end < len(bl) && bl[end].Count == boundaryCount {
		end++
	}
	return bl[:end]
}
