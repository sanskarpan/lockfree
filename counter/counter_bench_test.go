package counter

import (
	"sync"
	"testing"
)

// mutexCounter is a mutex-based counter for comparison
type mutexCounter struct {
	mu    sync.Mutex
	value int64
}

func (c *mutexCounter) Inc() int64 {
	c.mu.Lock()
	c.value++
	val := c.value
	c.mu.Unlock()
	return val
}

func (c *mutexCounter) Get() int64 {
	c.mu.Lock()
	val := c.value
	c.mu.Unlock()
	return val
}

func BenchmarkCounterInc(b *testing.B) {
	c := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Inc()
	}
}

func BenchmarkMutexCounterInc(b *testing.B) {
	c := &mutexCounter{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Inc()
	}
}

func BenchmarkCounterConcurrentInc(b *testing.B) {
	c := New()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Inc()
		}
	})
}

func BenchmarkMutexCounterConcurrentInc(b *testing.B) {
	c := &mutexCounter{}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Inc()
		}
	})
}

func BenchmarkStripedCounterInc(b *testing.B) {
	sc := NewStriped()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sc.Inc()
	}
}

func BenchmarkStripedCounterConcurrentInc(b *testing.B) {
	sc := NewStriped()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sc.Inc()
		}
	})
}

func BenchmarkCounterGet(b *testing.B) {
	c := New()
	c.Set(42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get()
	}
}

func BenchmarkStripedCounterGet(b *testing.B) {
	sc := NewStriped()
	sc.Add(42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sc.Get()
	}
}

func BenchmarkCounterIncAndGet(b *testing.B) {
	c := New()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Inc()
			c.Get()
		}
	})
}

func BenchmarkStripedCounterIncAndGet(b *testing.B) {
	sc := NewStriped()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sc.Inc()
			sc.Get()
		}
	})
}

func BenchmarkCounterCompareAndSwap(b *testing.B) {
	c := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.CompareAndSwap(int64(i), int64(i+1))
	}
}

func BenchmarkMinMaxUpdate(b *testing.B) {
	mm := NewMinMax()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mm.Update(int64(i))
	}
}

func BenchmarkMinMaxConcurrentUpdate(b *testing.B) {
	mm := NewMinMax()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			mm.Update(int64(i))
			i++
		}
	})
}

func BenchmarkAccumulatorAdd(b *testing.B) {
	acc := NewAccumulator()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		acc.Add(int64(i))
	}
}

func BenchmarkAccumulatorConcurrentAdd(b *testing.B) {
	acc := NewAccumulator()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			acc.Add(int64(i))
			i++
		}
	})
}

func BenchmarkAccumulatorAverage(b *testing.B) {
	acc := NewAccumulator()
	for i := 0; i < 100; i++ {
		acc.Add(int64(i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		acc.Average()
	}
}

// High contention benchmarks
func BenchmarkCounterHighContention(b *testing.B) {
	c := New()
	numGoroutines := 8
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	b.ResetTimer()
	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < b.N/numGoroutines; i++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()
}

func BenchmarkMutexCounterHighContention(b *testing.B) {
	c := &mutexCounter{}
	numGoroutines := 8
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	b.ResetTimer()
	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < b.N/numGoroutines; i++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()
}

func BenchmarkStripedCounterHighContention(b *testing.B) {
	sc := NewStriped()
	numGoroutines := 8
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	b.ResetTimer()
	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < b.N/numGoroutines; i++ {
				sc.Inc()
			}
		}()
	}
	wg.Wait()
}
