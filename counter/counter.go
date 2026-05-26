// Package counter provides lock-free counter implementations and atomic utilities.
// All operations are safe for concurrent use by multiple goroutines without external synchronization.
package counter

import (
	"math"
	"runtime"
	"sync/atomic"
	"unsafe"
)

// Counter is a simple lock-free counter using atomic operations.
// The zero value is ready to use.
type Counter struct {
	value atomic.Int64
}

// New creates and returns a new counter initialized to zero.
func New() *Counter {
	return &Counter{}
}

// NewWithValue creates and returns a new counter initialized to the specified value.
func NewWithValue(initial int64) *Counter {
	c := &Counter{}
	c.value.Store(initial)
	return c
}

// Inc increments the counter by 1 and returns the new value.
// This operation is atomic and lock-free.
func (c *Counter) Inc() int64 {
	return c.value.Add(1)
}

// Dec decrements the counter by 1 and returns the new value.
// This operation is atomic and lock-free.
func (c *Counter) Dec() int64 {
	return c.value.Add(-1)
}

// Add adds the specified delta to the counter and returns the new value.
// This operation is atomic and lock-free.
func (c *Counter) Add(delta int64) int64 {
	return c.value.Add(delta)
}

// Get returns the current value of the counter.
// This operation is atomic and lock-free.
func (c *Counter) Get() int64 {
	return c.value.Load()
}

// Set sets the counter to the specified value.
// This operation is atomic and lock-free.
func (c *Counter) Set(value int64) {
	c.value.Store(value)
}

// CompareAndSwap executes the compare-and-swap operation for the counter.
// If the current value equals old, it's replaced with new and returns true.
// Otherwise, it returns false.
// This operation is atomic and lock-free.
func (c *Counter) CompareAndSwap(old, new int64) bool {
	return c.value.CompareAndSwap(old, new)
}

// Swap atomically stores new into the counter and returns the previous value.
// This operation is atomic and lock-free.
func (c *Counter) Swap(new int64) int64 {
	return c.value.Swap(new)
}

// Reset resets the counter to zero and returns the previous value.
// This operation is atomic and lock-free.
func (c *Counter) Reset() int64 {
	return c.value.Swap(0)
}

// StripedCounter is a lock-free counter optimized for high contention scenarios.
// It uses multiple internal counters (stripes) to reduce contention.
// Each goroutine tends to use the same stripe, reducing cache line bouncing.
//
// StripedCounter is more expensive to read but much faster to increment
// under high contention compared to a simple Counter.
type StripedCounter struct {
	stripes []stripedInt64
	numCPU  int
}

const cacheLineSize = 64

type stripedInt64 struct {
	value atomic.Int64
	_     [cacheLineSize - unsafe.Sizeof(atomic.Int64{})]byte
}

// NewStriped creates and returns a new striped counter.
// It creates one stripe per CPU for optimal performance.
func NewStriped() *StripedCounter {
	numCPU := runtime.NumCPU()
	// Ensure at least 4 stripes, max 256
	if numCPU < 4 {
		numCPU = 4
	}
	if numCPU > 256 {
		numCPU = 256
	}

	return &StripedCounter{
		stripes: make([]stripedInt64, numCPU),
		numCPU:  numCPU,
	}
}

// getStripeIndex returns the stripe index for the current goroutine.
// A stack-address hash gives each goroutine a stable-enough stripe
// without adding another contended atomic to every operation.
func (sc *StripedCounter) getStripeIndex() int {
	var slot int
	hash := uintptr(unsafe.Pointer(&slot))
	hash ^= hash >> 7
	hash ^= hash >> 13
	return int(hash % uintptr(sc.numCPU))
}

// Inc increments the counter by 1.
// This operation is atomic and lock-free.
func (sc *StripedCounter) Inc() {
	idx := sc.getStripeIndex()
	sc.stripes[idx].value.Add(1)
}

// Dec decrements the counter by 1.
// This operation is atomic and lock-free.
func (sc *StripedCounter) Dec() {
	idx := sc.getStripeIndex()
	sc.stripes[idx].value.Add(-1)
}

// Add adds the specified delta to the counter.
// This operation is atomic and lock-free.
func (sc *StripedCounter) Add(delta int64) {
	idx := sc.getStripeIndex()
	sc.stripes[idx].value.Add(delta)
}

// Get returns the current value of the counter by summing all stripes.
// This operation is more expensive than a simple counter's Get.
// The value may not be exact in highly concurrent scenarios.
func (sc *StripedCounter) Get() int64 {
	var sum int64
	for i := 0; i < sc.numCPU; i++ {
		sum += sc.stripes[i].value.Load()
	}
	return sum
}

// Reset resets all stripes to zero and returns the previous total value.
// This operation is NOT atomic across all stripes.
func (sc *StripedCounter) Reset() int64 {
	var sum int64
	for i := 0; i < sc.numCPU; i++ {
		sum += sc.stripes[i].value.Swap(0)
	}
	return sum
}

// MinMax tracks both minimum and maximum values atomically.
// Useful for statistics collection.
type MinMax struct {
	min atomic.Int64
	max atomic.Int64
}

// NewMinMax creates a new MinMax tracker.
// Initial min is set to max int64, initial max is set to min int64.
func NewMinMax() *MinMax {
	mm := &MinMax{}
	mm.min.Store(math.MaxInt64)
	mm.max.Store(math.MinInt64)
	return mm
}

// Update updates the min and max with the given value if necessary.
// Returns true if either min or max was updated.
func (mm *MinMax) Update(value int64) bool {
	updated := false

	// Update min
	for {
		oldMin := mm.min.Load()
		if value >= oldMin {
			break
		}
		if mm.min.CompareAndSwap(oldMin, value) {
			updated = true
			break
		}
	}

	// Update max
	for {
		oldMax := mm.max.Load()
		if value <= oldMax {
			break
		}
		if mm.max.CompareAndSwap(oldMax, value) {
			updated = true
			break
		}
	}

	return updated
}

// Min returns the current minimum value.
func (mm *MinMax) Min() int64 {
	return mm.min.Load()
}

// Max returns the current maximum value.
func (mm *MinMax) Max() int64 {
	return mm.max.Load()
}

// Reset resets min and max to their initial values.
func (mm *MinMax) Reset() {
	mm.min.Store(math.MaxInt64)
	mm.max.Store(math.MinInt64)
}

// Accumulator provides lock-free accumulation with count tracking.
// Useful for computing averages and statistics.
type Accumulator struct {
	sum   atomic.Int64
	count atomic.Int64
}

// NewAccumulator creates a new accumulator.
func NewAccumulator() *Accumulator {
	return &Accumulator{}
}

// Add adds a value to the accumulator.
func (a *Accumulator) Add(value int64) {
	a.sum.Add(value)
	a.count.Add(1)
}

// Sum returns the current sum.
func (a *Accumulator) Sum() int64 {
	return a.sum.Load()
}

// Count returns the current count.
func (a *Accumulator) Count() int64 {
	return a.count.Load()
}

// Average returns the current average.
// Returns 0 if count is 0.
func (a *Accumulator) Average() float64 {
	count := a.count.Load()
	if count == 0 {
		return 0
	}
	return float64(a.sum.Load()) / float64(count)
}

// Reset resets sum and count to zero.
func (a *Accumulator) Reset() {
	a.sum.Store(0)
	a.count.Store(0)
}

// Snapshot returns the current sum and count atomically.
// Note: This is not a true atomic snapshot across both values,
// but provides a consistent view for most use cases.
func (a *Accumulator) Snapshot() (sum int64, count int64) {
	count = a.count.Load()
	sum = a.sum.Load()
	return sum, count
}
