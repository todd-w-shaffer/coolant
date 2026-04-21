package collector

import (
	"testing"
	"time"
)

func TestParseBattery(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantPresent   bool
		wantPercent   float64
		wantState     BatteryState
		wantRemaining time.Duration
	}{
		{
			name: "DischargingWithEstimate",
			input: `Now drawing from 'Battery Power'
 -InternalBattery-0 (id=21692515)	47%; discharging; 2:57 remaining present: true`,
			wantPresent:   true,
			wantPercent:   47,
			wantState:     BatteryDischarging,
			wantRemaining: 2*time.Hour + 57*time.Minute,
		},
		{
			name: "ChargingWithEstimate",
			input: `Now drawing from 'AC Power'
 -InternalBattery-0 (id=21692515)	85%; charging; 0:42 remaining present: true`,
			wantPresent:   true,
			wantPercent:   85,
			wantState:     BatteryCharging,
			wantRemaining: 42 * time.Minute,
		},
		{
			name: "Charged",
			input: `Now drawing from 'AC Power'
 -InternalBattery-0 (id=21692515)	100%; charged; 0:00 remaining present: true`,
			wantPresent:   true,
			wantPercent:   100,
			wantState:     BatteryCharged,
			wantRemaining: 0,
		},
		{
			name: "ACNotCharging",
			input: `Now drawing from 'AC Power'
 -InternalBattery-0 (id=21692515)	85%; AC attached; not charging; 0:00 remaining present: true`,
			wantPresent:   true,
			wantPercent:   85,
			wantState:     BatteryACNotCharging,
			wantRemaining: 0,
		},
		{
			name: "NoEstimate",
			input: `Now drawing from 'Battery Power'
 -InternalBattery-0 (id=21692515)	47%; discharging; (no estimate)`,
			wantPresent:   true,
			wantPercent:   47,
			wantState:     BatteryDischarging,
			wantRemaining: 0,
		},
		{
			name: "FinishingCharge",
			input: `Now drawing from 'AC Power'
 -InternalBattery-0 (id=21692515)	99%; finishing charge; 0:02 remaining present: true`,
			wantPresent:   true,
			wantPercent:   99,
			wantState:     BatteryFinishing,
			wantRemaining: 2 * time.Minute,
		},
		{
			name:        "NoBatteryLine",
			input:       `Now drawing from 'AC Power'`,
			wantPresent: false,
		},
		{
			name:        "MalformedEmpty",
			input:       "",
			wantPresent: false,
		},
		{
			name:        "MalformedGarbage",
			input:       "this is not pmset output at all\nrandom text",
			wantPresent: false,
		},
		{
			name:          "LeadingTab",
			input:         "Now drawing from 'Battery Power'\n\t -InternalBattery-0 (id=21692515)\t47%; discharging; 2:57 remaining present: true",
			wantPresent:   true,
			wantPercent:   47,
			wantState:     BatteryDischarging,
			wantRemaining: 2*time.Hour + 57*time.Minute,
		},
		{
			name:          "LeadingDoubleSpace",
			input:         "Now drawing from 'Battery Power'\n  -InternalBattery-0 (id=21692515)\t47%; discharging; 2:57 remaining present: true",
			wantPresent:   true,
			wantPercent:   47,
			wantState:     BatteryDischarging,
			wantRemaining: 2*time.Hour + 57*time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stats SystemStats
			parseBattery(tt.input, &stats)

			if stats.BatteryPresent != tt.wantPresent {
				t.Errorf("BatteryPresent = %v, want %v", stats.BatteryPresent, tt.wantPresent)
			}
			if !tt.wantPresent {
				return
			}
			if stats.BatteryPercent != tt.wantPercent {
				t.Errorf("BatteryPercent = %v, want %v", stats.BatteryPercent, tt.wantPercent)
			}
			if stats.BatteryState != tt.wantState {
				t.Errorf("BatteryState = %v, want %v", stats.BatteryState, tt.wantState)
			}
			if stats.BatteryTimeRemaining != tt.wantRemaining {
				t.Errorf("BatteryTimeRemaining = %v, want %v", stats.BatteryTimeRemaining, tt.wantRemaining)
			}
		})
	}
}
