package stats

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestSnapshotRoundtrip(t *testing.T) {
	at := time.Date(2026, 4, 25, 10, 15, 43, 0, time.UTC)
	want := Snapshot{
		SchemaVersion: CurrentSchemaVersion,
		FirstSeen:     at,
		LastUpdated:   at,
		ByType:        map[string]int64{"general-purpose": 198, "Explore": 31},
		ByProject:     map[string]int64{"coolant": 187, "thermal-enterprise": 43},
		Records: Records{
			PeakConcurrent:    RecordList{{Value: 7, SessionID: "s1", At: at}},
			LongestAgentS:     RecordList{{Value: 312, AgentID: "a1", AgentType: "Explore", Project: "coolant", At: at}},
			LongestSessionS:   RecordList{{Value: 8943, SessionID: "s2", At: at}},
			MostAgentsSession: RecordList{{Value: 32, SessionID: "s3", At: at}},
			BiggestBurst:      BurstRecordList{{Count: 6, WindowS: 2, SessionID: "s4", At: at}},
		},
		Daily: map[string]Counters{
			"2026-04-25": {AgentsStarted: 12, AgentsCompleted: 12, Sessions: 2, TranscriptBytesTotal: 12345, GateCapEvents: 3},
		},
	}

	buf, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Snapshot
	if err := json.Unmarshal(buf, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("roundtrip mismatch\nwant: %+v\ngot:  %+v", want, got)
	}
}

func TestSnapshotLifetimeSumsDaily(t *testing.T) {
	s := Snapshot{
		Daily: map[string]Counters{
			"2026-04-25": {AgentsStarted: 5, AgentsCompleted: 4, Sessions: 1, GateCapEvents: 2},
			"2026-04-24": {AgentsStarted: 3, AgentsCompleted: 3, Sessions: 1, TranscriptBytesTotal: 100},
			"2026-04-23": {AgentsOrphaned: 1, AgentsStarted: 2, AgentsCompleted: 1},
		},
	}
	got := s.Lifetime()
	want := Counters{
		AgentsStarted:        10,
		AgentsCompleted:      8,
		AgentsOrphaned:       1,
		Sessions:             2,
		TranscriptBytesTotal: 100,
		GateCapEvents:        2,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Lifetime sum mismatch:\nwant: %+v\ngot:  %+v", want, got)
	}
}
