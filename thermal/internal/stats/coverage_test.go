package stats

import (
	"reflect"
	"testing"
)

// TestCountersAddCoversAllFields reflect-walks every int64 field on
// Counters and asserts Add produces a sum that covers it. Catches
// silent drift when telemetry adds new fields and forgets to wire
// them into Add. Map-typed fields are covered by
// TestCountersAddCoversAllMapFields below — SetInt panics on
// reflect.Map, so the int loop must skip them. Any kind not
// explicitly handled is t.Fatalf'd so a future field-shape addition
// is forced to declare its coverage path.
func TestCountersAddCoversAllFields(t *testing.T) {
	a, b := Counters{}, Counters{}
	v1 := reflect.ValueOf(&a).Elem()
	v2 := reflect.ValueOf(&b).Elem()
	tt := v1.Type()
	for i := 0; i < v1.NumField(); i++ {
		switch tt.Field(i).Type.Kind() {
		case reflect.Int64:
			v1.Field(i).SetInt(1)
			v2.Field(i).SetInt(2)
		case reflect.Map:
			// Covered by TestCountersAddCoversAllMapFields.
			continue
		default:
			t.Fatalf("unhandled field kind %s on %s — add a coverage path",
				tt.Field(i).Type.Kind(), tt.Field(i).Name)
		}
	}
	sum := a.Add(b)
	vs := reflect.ValueOf(sum)
	for i := 0; i < tt.NumField(); i++ {
		if tt.Field(i).Type.Kind() != reflect.Int64 {
			continue
		}
		name := tt.Field(i).Name
		if got := vs.Field(i).Int(); got != 3 {
			t.Errorf("Counters.Add did not cover field %q: want 3, got %d", name, got)
		}
	}
}

// TestComputeDeltaCoversAllAdditiveFields reflect-walks Counters and
// asserts every int64 additive field appears in the per-day delta.
// Fields in nonAdditiveCounterFields are skipped (explicitly
// non-additive). Map fields are covered separately by
// TestComputeDeltaCoversAllMapFields; the map-walk does NOT consult
// nonAdditiveCounterFields — that exempt set governs the
// int64-additive-vs-non-additive distinction, which doesn't apply to
// pure-additive map fields.
func TestComputeDeltaCoversAllAdditiveFields(t *testing.T) {
	tt := reflect.TypeOf(Counters{})
	day := "2026-04-25"
	for i := 0; i < tt.NumField(); i++ {
		name := tt.Field(i).Name
		if _, exempt := nonAdditiveCounterFields[name]; exempt {
			continue
		}
		switch tt.Field(i).Type.Kind() {
		case reflect.Int64:
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
				t.Errorf("computeDelta did not cover field %q: want 2, got %d (add to nonAdditiveCounterFields if non-additive)", name, got)
			}
		case reflect.Map:
			continue
		default:
			t.Fatalf("unhandled field kind %s on %s — add a coverage path",
				tt.Field(i).Type.Kind(), name)
		}
	}
}

// TestCountersAddCoversAllMapFields reflect-walks Counters map fields
// and asserts Add key-sums them, returns a non-aliased fresh map, and
// preserves nil + nil → nil per the §3.1 nil-preservation rule.
func TestCountersAddCoversAllMapFields(t *testing.T) {
	tt := reflect.TypeOf(Counters{})
	for i := 0; i < tt.NumField(); i++ {
		if tt.Field(i).Type.Kind() != reflect.Map {
			continue
		}
		name := tt.Field(i).Name

		// Key-sum check: a={"k1":1}, b={"k1":2,"k2":5} → {"k1":3,"k2":5}.
		var a, b Counters
		va := reflect.ValueOf(&a).Elem().Field(i)
		vb := reflect.ValueOf(&b).Elem().Field(i)
		va.Set(reflect.ValueOf(map[string]int64{"k1": 1}))
		vb.Set(reflect.ValueOf(map[string]int64{"k1": 2, "k2": 5}))
		sum := a.Add(b)
		got := reflect.ValueOf(sum).Field(i).Interface().(map[string]int64)
		if got["k1"] != 3 || got["k2"] != 5 || len(got) != 2 {
			t.Errorf("Add did not key-sum map field %q: got %v", name, got)
		}

		// Non-aliasing: mutating result must not perturb either input.
		got["k1"] = 999
		if reflect.ValueOf(a).Field(i).Interface().(map[string]int64)["k1"] != 1 {
			t.Errorf("Add aliased result to a's map field %q", name)
		}
		if reflect.ValueOf(b).Field(i).Interface().(map[string]int64)["k1"] != 2 {
			t.Errorf("Add aliased result to b's map field %q", name)
		}

		// nil + nil → nil (omitempty preservation).
		var z1, z2 Counters
		zsum := z1.Add(z2)
		zgot := reflect.ValueOf(zsum).Field(i).Interface().(map[string]int64)
		if zgot != nil {
			t.Errorf("Add(nil,nil) for field %q: want nil, got %v", name, zgot)
		}
	}
}

// TestComputeDeltaCoversAllMapFields reflect-walks Counters map
// fields and asserts computeDelta carries per-key subtraction,
// including negative values for keys removed from current relative
// to baseline (per §3.4 cap-collapse semantics). Map fields are
// unconditionally pure-additive — they are NOT consulted against
// nonAdditiveCounterFields.
func TestComputeDeltaCoversAllMapFields(t *testing.T) {
	tt := reflect.TypeOf(Counters{})
	day := "2026-04-25"
	for i := 0; i < tt.NumField(); i++ {
		if tt.Field(i).Type.Kind() != reflect.Map {
			continue
		}
		name := tt.Field(i).Name

		baseline := Snapshot{Daily: map[string]Counters{day: {}}}
		baseB := reflect.ValueOf(&baseline.Daily).Elem().MapIndex(reflect.ValueOf(day))
		baseV := reflect.New(baseB.Type()).Elem()
		baseV.Set(baseB)
		baseV.Field(i).Set(reflect.ValueOf(map[string]int64{"foo": 5, "bar": 3}))
		baseline.Daily[day] = baseV.Interface().(Counters)

		current := map[string]Counters{day: {}}
		curB := reflect.ValueOf(&current).Elem().MapIndex(reflect.ValueOf(day))
		curV := reflect.New(curB.Type()).Elem()
		curV.Set(curB)
		curV.Field(i).Set(reflect.ValueOf(map[string]int64{"foo": 8, "bar": 1, "baz": 2}))
		current[day] = curV.Interface().(Counters)

		delta := computeDelta(nil, nil, current, baseline)
		got := reflect.ValueOf(delta.Daily[day]).Field(i).Interface().(map[string]int64)
		if got["foo"] != 3 {
			t.Errorf("computeDelta map field %q [foo]: want 3, got %d", name, got["foo"])
		}
		if got["bar"] != -2 {
			t.Errorf("computeDelta map field %q [bar] negative case: want -2, got %d", name, got["bar"])
		}
		if got["baz"] != 2 {
			t.Errorf("computeDelta map field %q [baz] new key: want 2, got %d", name, got["baz"])
		}
	}
}
