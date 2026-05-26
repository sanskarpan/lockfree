package stack

import (
	"sync"
	"testing"
)

// mutexStack is a mutex-based stack for comparison
type mutexStack[T any] struct {
	mu    sync.Mutex
	items []T
}

func newMutexStack[T any]() *mutexStack[T] {
	return &mutexStack[T]{items: make([]T, 0)}
}

func (s *mutexStack[T]) Push(value T) {
	s.mu.Lock()
	s.items = append(s.items, value)
	s.mu.Unlock()
}

func (s *mutexStack[T]) Pop() (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.items) == 0 {
		var zero T
		return zero, false
	}

	lastIdx := len(s.items) - 1
	value := s.items[lastIdx]
	s.items = s.items[:lastIdx]
	return value, true
}

func BenchmarkStackPush(b *testing.B) {
	s := New[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Push(i)
	}
}

func BenchmarkMutexStackPush(b *testing.B) {
	s := newMutexStack[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Push(i)
	}
}

func BenchmarkStackPop(b *testing.B) {
	s := New[int]()
	for i := 0; i < b.N; i++ {
		s.Push(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Pop()
	}
}

func BenchmarkMutexStackPop(b *testing.B) {
	s := newMutexStack[int]()
	for i := 0; i < b.N; i++ {
		s.Push(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Pop()
	}
}

func BenchmarkStackPushPop(b *testing.B) {
	s := New[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Push(i)
		s.Pop()
	}
}

func BenchmarkMutexStackPushPop(b *testing.B) {
	s := newMutexStack[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Push(i)
		s.Pop()
	}
}

// Concurrent benchmarks
func BenchmarkStackConcurrentPush(b *testing.B) {
	s := New[int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			s.Push(i)
			i++
		}
	})
}

func BenchmarkMutexStackConcurrentPush(b *testing.B) {
	s := newMutexStack[int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			s.Push(i)
			i++
		}
	})
}

func BenchmarkStackConcurrentPushPop(b *testing.B) {
	s := New[int]()
	// Pre-populate
	for i := 0; i < 1000; i++ {
		s.Push(i)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				s.Push(i)
			} else {
				s.Pop()
			}
			i++
		}
	})
}

func BenchmarkMutexStackConcurrentPushPop(b *testing.B) {
	s := newMutexStack[int]()
	// Pre-populate
	for i := 0; i < 1000; i++ {
		s.Push(i)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				s.Push(i)
			} else {
				s.Pop()
			}
			i++
		}
	})
}

func BenchmarkStackConcurrentMixed(b *testing.B) {
	s := New[int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			switch i % 4 {
			case 0, 1:
				s.Push(i)
			case 2:
				s.Pop()
			case 3:
				s.Peek()
			}
			i++
		}
	})
}

func BenchmarkMutexStackConcurrentMixed(b *testing.B) {
	s := newMutexStack[int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			switch i % 4 {
			case 0, 1:
				s.Push(i)
			case 2:
				s.Pop()
			}
			i++
		}
	})
}

// High contention benchmarks
func BenchmarkStackHighContention(b *testing.B) {
	s := New[int]()
	numGoroutines := 8
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	b.ResetTimer()
	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < b.N/numGoroutines; i++ {
				s.Push(i)
				s.Pop()
			}
		}()
	}
	wg.Wait()
}

func BenchmarkMutexStackHighContention(b *testing.B) {
	s := newMutexStack[int]()
	numGoroutines := 8
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	b.ResetTimer()
	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < b.N/numGoroutines; i++ {
				s.Push(i)
				s.Pop()
			}
		}()
	}
	wg.Wait()
}
