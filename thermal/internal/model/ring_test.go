package model

import "testing"

func TestPushFillsToCapacity(t *testing.T) {
	r := NewRingBuffer[int](4)
	for i := 0; i < 4; i++ {
		r.Push(i)
	}
	if r.Len() != 4 {
		t.Fatalf("Len = %d, want 4", r.Len())
	}
	for i := 0; i < 4; i++ {
		if got := r.At(i); got != i {
			t.Errorf("At(%d) = %d, want %d", i, got, i)
		}
	}
}

func TestPushOverwritesOldest(t *testing.T) {
	r := NewRingBuffer[int](3)
	for i := 0; i < 5; i++ {
		r.Push(i)
	}
	// Pushed 0,1,2,3,4 into cap-3 buffer. Oldest should be 2.
	if r.Len() != 3 {
		t.Fatalf("Len = %d, want 3", r.Len())
	}
	want := []int{2, 3, 4}
	for i, w := range want {
		if got := r.At(i); got != w {
			t.Errorf("At(%d) = %d, want %d", i, got, w)
		}
	}
}

func TestLenDuringFillAndAfterWraparound(t *testing.T) {
	r := NewRingBuffer[int](5)
	for i := 0; i < 8; i++ {
		r.Push(i)
		wantLen := i + 1
		if wantLen > 5 {
			wantLen = 5
		}
		if r.Len() != wantLen {
			t.Errorf("after %d pushes: Len = %d, want %d", i+1, r.Len(), wantLen)
		}
	}
}

func TestAtOldestAndNewest(t *testing.T) {
	r := NewRingBuffer[string](3)
	r.Push("a")
	r.Push("b")
	r.Push("c")
	r.Push("d") // overwrites "a"

	if got := r.At(0); got != "b" {
		t.Errorf("At(0) = %q, want oldest %q", got, "b")
	}
	if got := r.At(r.Len() - 1); got != "d" {
		t.Errorf("At(Len-1) = %q, want newest %q", got, "d")
	}
}

func TestPeekReturnsNewest(t *testing.T) {
	r := NewRingBuffer[int](3)
	r.Push(10)
	if got := r.Peek(); got != 10 {
		t.Errorf("Peek = %d, want 10", got)
	}
	r.Push(20)
	r.Push(30)
	r.Push(40)
	if got := r.Peek(); got != 40 {
		t.Errorf("Peek after wraparound = %d, want 40", got)
	}
}

func TestLastLessThanLen(t *testing.T) {
	r := NewRingBuffer[int](5)
	for i := 0; i < 5; i++ {
		r.Push(i)
	}
	got := r.Last(3)
	want := []int{2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("Last(3) len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Last(3)[%d] = %d, want %d", i, got[i], w)
		}
	}
}

func TestLastEqualToLen(t *testing.T) {
	r := NewRingBuffer[int](4)
	for i := 0; i < 4; i++ {
		r.Push(i * 10)
	}
	got := r.Last(4)
	want := []int{0, 10, 20, 30}
	if len(got) != len(want) {
		t.Fatalf("Last(Len) len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Last(Len)[%d] = %d, want %d", i, got[i], w)
		}
	}
}

func TestLastGreaterThanLen(t *testing.T) {
	r := NewRingBuffer[int](4)
	r.Push(1)
	r.Push(2)
	got := r.Last(10)
	want := []int{1, 2}
	if len(got) != len(want) {
		t.Fatalf("Last(10) len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Last(10)[%d] = %d, want %d", i, got[i], w)
		}
	}
}

func TestLastOnEmpty(t *testing.T) {
	r := NewRingBuffer[int](4)
	got := r.Last(5)
	if got != nil {
		t.Errorf("Last on empty = %v, want nil", got)
	}
}

func TestPushAfterWraparoundPreservesOrdering(t *testing.T) {
	r := NewRingBuffer[int](3)
	// Push 7 items into cap-3 buffer
	for i := 0; i < 7; i++ {
		r.Push(i)
	}
	// Should contain 4,5,6 oldest-first
	got := r.Slice()
	want := []int{4, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("Slice len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Slice[%d] = %d, want %d", i, got[i], w)
		}
	}
}

func TestPanicOnAtOutOfRange(t *testing.T) {
	r := NewRingBuffer[int](3)
	r.Push(1)

	cases := []struct {
		name string
		idx  int
	}{
		{"negative", -1},
		{"equal to len", 1},
		{"way past", 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("At(%d) did not panic", tc.idx)
				}
			}()
			r.At(tc.idx)
		})
	}
}

func TestPanicOnPeekEmpty(t *testing.T) {
	r := NewRingBuffer[int](3)
	defer func() {
		if recover() == nil {
			t.Error("Peek on empty buffer did not panic")
		}
	}()
	r.Peek()
}

func TestClearResetsBuffer(t *testing.T) {
	r := NewRingBuffer[int](3)
	r.Push(1)
	r.Push(2)
	r.Clear()
	if r.Len() != 0 {
		t.Errorf("Len after Clear = %d, want 0", r.Len())
	}
	// Push after clear should work normally
	r.Push(99)
	if got := r.Peek(); got != 99 {
		t.Errorf("Peek after Clear+Push = %d, want 99", got)
	}
}

func TestCapReturnsCapacity(t *testing.T) {
	r := NewRingBuffer[int](42)
	if r.Cap() != 42 {
		t.Errorf("Cap = %d, want 42", r.Cap())
	}
}
