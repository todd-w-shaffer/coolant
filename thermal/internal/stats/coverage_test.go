package stats

import (
	"reflect"
	"testing"
)

// TestCountersAddCoversAllFields reflect-walks every exported field
// on Counters and asserts Add produces a sum that covers it. Catches
// silent drift when telemetry adds new fields and forgets to wire
// them into Add.
func TestCountersAddCoversAllFields(t *testing.T) {
	a, b := Counters{}, Counters{}
	v1 := reflect.ValueOf(&a).Elem()
	v2 := reflect.ValueOf(&b).Elem()
	tt := v1.Type()
	// Set every field of `a` to 1 and `b` to 2 — sum must equal 3
	// per field for the result to "touch" each one.
	for i := 0; i < v1.NumField(); i++ {
		v1.Field(i).SetInt(1)
		v2.Field(i).SetInt(2)
	}
	sum := a.Add(b)
	vs := reflect.ValueOf(sum)
	for i := 0; i < tt.NumField(); i++ {
		name := tt.Field(i).Name
		if got := vs.Field(i).Int(); got != 3 {
			t.Errorf("Counters.Add did not cover field %q: want 3, got %d", name, got)
		}
	}
}

// TestComputeDeltaCoversAllAdditiveFields reflect-walks Counters and
// asserts every additive field appears in the per-day delta. Fields
// in computeDeltaExempt are skipped (they're explicitly non-additive).
func TestComputeDeltaCoversAllAdditiveFields(t *testing.T) {
	tt := reflect.TypeOf(Counters{})
	day := "2026-04-25"
	for i := 0; i < tt.NumField(); i++ {
		name := tt.Field(i).Name
		if _, exempt := nonAdditiveCounterFields[name]; exempt {
			continue
		}
		// Build baseline (zero) and current (field=2) Counters.
		baseline := Snapshot{Daily: map[string]Counters{day: {}}}
		current := map[string]Counters{day: {}}
		v := reflect.ValueOf(&current).Elem().MapIndex(reflect.ValueOf(day))
		copyV := reflect.New(v.Type()).Elem()
		copyV.Set(v)
		copyV.Field(i).SetInt(2)
		current[day] = copyV.Interface().(Counters)

		delta := computeDelta(nil, nil, current, baseline)
		got := reflect.ValueOf(delta.Daily[day]).Field(i).Int()
		if got != 2 {
			t.Errorf("computeDelta did not cover field %q: want 2, got %d (add to computeDeltaExempt if non-additive)", name, got)
		}
	}
}
