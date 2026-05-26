package counter

import (
	"math"
	"sync"
	"testing"
)

func TestCounterBasicOperations(t *testing.T) {
	c := New()

	if c.Get() != 0 {
		t.Errorf("New counter should be 0, got %d", c.Get())
	}

	// Test Inc
	if val := c.Inc(); val != 1 {
		t.Errorf("After Inc, expected 1, got %d", val)
	}

	// Test Dec
	if val := c.Dec(); val != 0 {
		t.Errorf("After Dec, expected 0, got %d", val)
	}

	// Test Add
	if val := c.Add(10); val != 10 {
		t.Errorf("After Add(10), expected 10, got %d", val)
	}

	// Test Set
	c.Set(42)
	if val := c.Get(); val != 42 {
		t.Errorf("After Set(42), expected 42, got %d", val)
	}
}

func TestCounterNewWithValue(t *testing.T) {
	c := NewWithValue(100)
	if val := c.Get(); val != 100 {
		t.Errorf("Expected 100, got %d", val)
	}
}

func TestCounterCompareAndSwap(t *testing.T) {
	c := New()
	c.Set(5)

	// Successful CAS
	if !c.CompareAndSwap(5, 10) {
		t.Error("CAS should succeed when old value matches")
	}
	if val := c.Get(); val != 10 {
		t.Errorf("After successful CAS, expected 10, got %d", val)
	}

	// Failed CAS
	if c.CompareAndSwap(5, 15) {
		t.Error("CAS should fail when old value doesn't match")
	}
	if val := c.Get(); val != 10 {
		t.Errorf("After failed CAS, expected 10, got %d", val)
	}
}

func TestCounterSwap(t *testing.T) {
	c := New()
	c.Set(5)

	old := c.Swap(10)
	if old != 5 {
		t.Errorf("Swap should return old value 5, got %d", old)
	}
	if val := c.Get(); val != 10 {
		t.Errorf("After Swap, expected 10, got %d", val)
	}
}

func TestCounterReset(t *testing.T) {
	c := New()
	c.Set(42)

	old := c.Reset()
	if old != 42 {
		t.Errorf("Reset should return old value 42, got %d", old)
	}
	if val := c.Get(); val != 0 {
		t.Errorf("After Reset, expected 0, got %d", val)
	}
}

func TestCounterConcurrentInc(t *testing.T) {
	c := New()
	numGoroutines := 100
	incsPerGoroutine := 1000

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < incsPerGoroutine; j++ {
				c.Inc()
			}
		}()
	}

	wg.Wait()

	expected := int64(numGoroutines * incsPerGoroutine)
	if val := c.Get(); val != expected {
		t.Errorf("Expected %d, got %d", expected, val)
	}
}

func TestCounterConcurrentMixed(t *testing.T) {
	c := NewWithValue(10000)
	numGoroutines := 50
	opsPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Incrementers
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				c.Inc()
			}
		}()
	}

	// Decrementers
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				c.Dec()
			}
		}()
	}

	wg.Wait()

	// Should be back to initial value
	expected := int64(10000)
	if val := c.Get(); val != expected {
		t.Errorf("Expected %d, got %d", expected, val)
	}
}

func TestStripedCounterBasicOperations(t *testing.T) {
	sc := NewStriped()

	if val := sc.Get(); val != 0 {
		t.Errorf("New striped counter should be 0, got %d", val)
	}

	sc.Inc()
	if val := sc.Get(); val != 1 {
		t.Errorf("After Inc, expected 1, got %d", val)
	}

	sc.Dec()
	if val := sc.Get(); val != 0 {
		t.Errorf("After Dec, expected 0, got %d", val)
	}

	sc.Add(10)
	if val := sc.Get(); val != 10 {
		t.Errorf("After Add(10), expected 10, got %d", val)
	}
}

func TestStripedCounterReset(t *testing.T) {
	sc := NewStriped()
	sc.Add(42)

	old := sc.Reset()
	if old != 42 {
		t.Errorf("Reset should return 42, got %d", old)
	}
	if val := sc.Get(); val != 0 {
		t.Errorf("After Reset, expected 0, got %d", val)
	}
}

func TestStripedCounterConcurrent(t *testing.T) {
	sc := NewStriped()
	numGoroutines := 100
	incsPerGoroutine := 1000

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < incsPerGoroutine; j++ {
				sc.Inc()
			}
		}()
	}

	wg.Wait()

	expected := int64(numGoroutines * incsPerGoroutine)
	if val := sc.Get(); val != expected {
		t.Errorf("Expected %d, got %d", expected, val)
	}
}

func TestMinMaxBasicOperations(t *testing.T) {
	mm := NewMinMax()

	mm.Update(5)
	if min := mm.Min(); min != 5 {
		t.Errorf("Expected min 5, got %d", min)
	}
	if max := mm.Max(); max != 5 {
		t.Errorf("Expected max 5, got %d", max)
	}

	mm.Update(3)
	if min := mm.Min(); min != 3 {
		t.Errorf("Expected min 3, got %d", min)
	}
	if max := mm.Max(); max != 5 {
		t.Errorf("Expected max 5, got %d", max)
	}

	mm.Update(10)
	if min := mm.Min(); min != 3 {
		t.Errorf("Expected min 3, got %d", min)
	}
	if max := mm.Max(); max != 10 {
		t.Errorf("Expected max 10, got %d", max)
	}
}

func TestMinMaxConcurrent(t *testing.T) {
	mm := NewMinMax()
	numGoroutines := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Each goroutine updates with its own range
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				mm.Update(int64(id*100 + j))
			}
		}(i)
	}

	wg.Wait()

	if min := mm.Min(); min != 0 {
		t.Errorf("Expected min 0, got %d", min)
	}

	expectedMax := int64((numGoroutines-1)*100 + 99)
	if max := mm.Max(); max != expectedMax {
		t.Errorf("Expected max %d, got %d", expectedMax, max)
	}
}

func TestMinMaxReset(t *testing.T) {
	mm := NewMinMax()
	mm.Update(5)
	mm.Update(10)

	mm.Reset()

	// After reset, should be back to initial state
	mm.Update(7)
	if min := mm.Min(); min != 7 {
		t.Errorf("Expected min 7 after reset, got %d", min)
	}
	if max := mm.Max(); max != 7 {
		t.Errorf("Expected max 7 after reset, got %d", max)
	}
}

func TestMinMaxExtremeValues(t *testing.T) {
	mm := NewMinMax()
	mm.Update(math.MinInt64)

	if min := mm.Min(); min != math.MinInt64 {
		t.Fatalf("expected min %d, got %d", int64(math.MinInt64), min)
	}
	if max := mm.Max(); max != math.MinInt64 {
		t.Fatalf("expected max %d, got %d", int64(math.MinInt64), max)
	}

	mm.Update(math.MaxInt64)

	if min := mm.Min(); min != math.MinInt64 {
		t.Fatalf("expected min to remain %d, got %d", int64(math.MinInt64), min)
	}
	if max := mm.Max(); max != math.MaxInt64 {
		t.Fatalf("expected max %d, got %d", int64(math.MaxInt64), max)
	}
}

func TestAccumulatorBasicOperations(t *testing.T) {
	acc := NewAccumulator()

	if sum := acc.Sum(); sum != 0 {
		t.Errorf("New accumulator should have sum 0, got %d", sum)
	}
	if count := acc.Count(); count != 0 {
		t.Errorf("New accumulator should have count 0, got %d", count)
	}
	if avg := acc.Average(); avg != 0 {
		t.Errorf("New accumulator should have average 0, got %f", avg)
	}

	acc.Add(10)
	acc.Add(20)
	acc.Add(30)

	if sum := acc.Sum(); sum != 60 {
		t.Errorf("Expected sum 60, got %d", sum)
	}
	if count := acc.Count(); count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}
	if avg := acc.Average(); avg != 20.0 {
		t.Errorf("Expected average 20.0, got %f", avg)
	}
}

func TestAccumulatorSnapshot(t *testing.T) {
	acc := NewAccumulator()
	acc.Add(10)
	acc.Add(20)

	sum, count := acc.Snapshot()
	if sum != 30 {
		t.Errorf("Expected sum 30, got %d", sum)
	}
	if count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}
}

func TestAccumulatorReset(t *testing.T) {
	acc := NewAccumulator()
	acc.Add(10)
	acc.Add(20)

	acc.Reset()

	if sum := acc.Sum(); sum != 0 {
		t.Errorf("Expected sum 0 after reset, got %d", sum)
	}
	if count := acc.Count(); count != 0 {
		t.Errorf("Expected count 0 after reset, got %d", count)
	}
}

func TestAccumulatorConcurrent(t *testing.T) {
	acc := NewAccumulator()
	numGoroutines := 100
	addsPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < addsPerGoroutine; j++ {
				acc.Add(int64(id))
			}
		}(i)
	}

	wg.Wait()

	expectedCount := int64(numGoroutines * addsPerGoroutine)
	if count := acc.Count(); count != expectedCount {
		t.Errorf("Expected count %d, got %d", expectedCount, count)
	}

	// Sum should be: sum of 0..99, each appearing 100 times
	// = 100 * (0 + 1 + 2 + ... + 99) = 100 * 4950 = 495000
	expectedSum := int64(100 * 4950)
	if sum := acc.Sum(); sum != expectedSum {
		t.Errorf("Expected sum %d, got %d", expectedSum, sum)
	}
}
