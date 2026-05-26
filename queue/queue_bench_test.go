package queue

import (
	"sync"
	"testing"
)

// mutexQueue is a mutex-based queue for comparison
type mutexQueue[T any] struct {
	mu    sync.Mutex
	items []T
}

func newMutexQueue[T any]() *mutexQueue[T] {
	return &mutexQueue[T]{items: make([]T, 0)}
}

func (q *mutexQueue[T]) Enqueue(value T) {
	q.mu.Lock()
	q.items = append(q.items, value)
	q.mu.Unlock()
}

func (q *mutexQueue[T]) Dequeue() (T, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		var zero T
		return zero, false
	}

	value := q.items[0]
	q.items = q.items[1:]
	return value, true
}

func BenchmarkQueueEnqueue(b *testing.B) {
	q := New[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
	}
}

func BenchmarkMutexQueueEnqueue(b *testing.B) {
	q := newMutexQueue[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
	}
}

func BenchmarkQueueDequeue(b *testing.B) {
	q := New[int]()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Dequeue()
	}
}

func BenchmarkMutexQueueDequeue(b *testing.B) {
	q := newMutexQueue[int]()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Dequeue()
	}
}

func BenchmarkQueueEnqueueDequeue(b *testing.B) {
	q := New[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
		q.Dequeue()
	}
}

func BenchmarkMutexQueueEnqueueDequeue(b *testing.B) {
	q := newMutexQueue[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
		q.Dequeue()
	}
}

// Concurrent benchmarks
func BenchmarkQueueConcurrentEnqueue(b *testing.B) {
	q := New[int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			q.Enqueue(i)
			i++
		}
	})
}

func BenchmarkMutexQueueConcurrentEnqueue(b *testing.B) {
	q := newMutexQueue[int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			q.Enqueue(i)
			i++
		}
	})
}

func BenchmarkQueueConcurrentEnqueueDequeue(b *testing.B) {
	q := New[int]()
	// Pre-populate
	for i := 0; i < 1000; i++ {
		q.Enqueue(i)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				q.Enqueue(i)
			} else {
				q.Dequeue()
			}
			i++
		}
	})
}

func BenchmarkMutexQueueConcurrentEnqueueDequeue(b *testing.B) {
	q := newMutexQueue[int]()
	// Pre-populate
	for i := 0; i < 1000; i++ {
		q.Enqueue(i)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				q.Enqueue(i)
			} else {
				q.Dequeue()
			}
			i++
		}
	})
}

func BenchmarkQueueConcurrentMixed(b *testing.B) {
	q := New[int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			switch i % 4 {
			case 0, 1:
				q.Enqueue(i)
			case 2:
				q.Dequeue()
			case 3:
				q.Peek()
			}
			i++
		}
	})
}

func BenchmarkMutexQueueConcurrentMixed(b *testing.B) {
	q := newMutexQueue[int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			switch i % 4 {
			case 0, 1:
				q.Enqueue(i)
			case 2:
				q.Dequeue()
			}
			i++
		}
	})
}

// Producer-Consumer benchmarks
func BenchmarkQueueProducerConsumer(b *testing.B) {
	q := New[int]()
	var wg sync.WaitGroup
	wg.Add(2)

	b.ResetTimer()

	// Producer
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			q.Enqueue(i)
		}
	}()

	// Consumer
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			for {
				if _, ok := q.Dequeue(); ok {
					break
				}
			}
		}
	}()

	wg.Wait()
}

func BenchmarkMutexQueueProducerConsumer(b *testing.B) {
	q := newMutexQueue[int]()
	var wg sync.WaitGroup
	wg.Add(2)

	b.ResetTimer()

	// Producer
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			q.Enqueue(i)
		}
	}()

	// Consumer
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			for {
				if _, ok := q.Dequeue(); ok {
					break
				}
			}
		}
	}()

	wg.Wait()
}

// High contention benchmarks
func BenchmarkQueueHighContention(b *testing.B) {
	q := New[int]()
	numGoroutines := 8
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	b.ResetTimer()
	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < b.N/numGoroutines; i++ {
				q.Enqueue(i)
				q.Dequeue()
			}
		}()
	}
	wg.Wait()
}

func BenchmarkMutexQueueHighContention(b *testing.B) {
	q := newMutexQueue[int]()
	numGoroutines := 8
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	b.ResetTimer()
	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < b.N/numGoroutines; i++ {
				q.Enqueue(i)
				q.Dequeue()
			}
		}()
	}
	wg.Wait()
}
