package model

// RingBuffer is a fixed-capacity circular buffer that overwrites the oldest
// element on Push when full. All mutations are O(1) with zero allocation.
type RingBuffer[T any] struct {
	buf  []T
	head int // index of oldest element
	len  int // current number of elements
	cap  int // fixed capacity
}

// NewRingBuffer allocates a ring buffer with the given capacity.
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	return &RingBuffer[T]{
		buf: make([]T, capacity),
		cap: capacity,
	}
}

// Push appends an item, overwriting the oldest if at capacity. O(1), no alloc.
func (r *RingBuffer[T]) Push(item T) {
	idx := (r.head + r.len) % r.cap
	if r.len == r.cap {
		// Overwrite oldest, advance head
		r.buf[r.head] = item
		r.head = (r.head + 1) % r.cap
	} else {
		r.buf[idx] = item
		r.len++
	}
}

// Len returns the current number of elements.
func (r *RingBuffer[T]) Len() int {
	return r.len
}

// Cap returns the fixed capacity.
func (r *RingBuffer[T]) Cap() int {
	return r.cap
}

// At returns the element at logical index i (0 = oldest). Panics if out of range.
func (r *RingBuffer[T]) At(i int) T {
	if i < 0 || i >= r.len {
		panic("RingBuffer: index out of range")
	}
	return r.buf[(r.head+i)%r.cap]
}

// Last returns the most recent n items (newest last), allocating a new slice.
// If n > Len(), returns all elements. Intended for View() paths, not Update().
func (r *RingBuffer[T]) Last(n int) []T {
	if n > r.len {
		n = r.len
	}
	if n == 0 {
		return nil
	}
	out := make([]T, n)
	start := r.len - n
	for i := 0; i < n; i++ {
		out[i] = r.buf[(r.head+start+i)%r.cap]
	}
	return out
}

// Slice returns all elements oldest-first as a new slice. Allocates.
// Intended for View() paths, not Update().
func (r *RingBuffer[T]) Slice() []T {
	return r.Last(r.len)
}

// Peek returns the most recent element. Panics if empty.
func (r *RingBuffer[T]) Peek() T {
	if r.len == 0 {
		panic("RingBuffer: peek on empty buffer")
	}
	return r.buf[(r.head+r.len-1)%r.cap]
}

// Clear resets the buffer to empty without deallocating.
func (r *RingBuffer[T]) Clear() {
	r.head = 0
	r.len = 0
}
