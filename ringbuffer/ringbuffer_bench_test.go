package ringbuffer

import (
	"sync"
	"testing"
)

// mutexRingBuffer is a mutex-based ring buffer for comparison
type mutexRingBuffer[T any] struct {
	mu     sync.Mutex
	buffer []T
	head   int
	tail   int
	size   int
}

func newMutexRingBuffer[T any](capacity int) *mutexRingBuffer[T] {
	return &mutexRingBuffer[T]{
		buffer: make([]T, capacity),
	}
}

func (rb *mutexRingBuffer[T]) Write(item T) error {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	next := (rb.tail + 1) % len(rb.buffer)
	if next == rb.head && rb.size > 0 {
		return ErrBufferFull
	}

	rb.buffer[rb.tail] = item
	rb.tail = next
	rb.size++
	return nil
}

func (rb *mutexRingBuffer[T]) Read() (T, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.size == 0 {
		var zero T
		return zero, ErrBufferEmpty
	}

	item := rb.buffer[rb.head]
	rb.head = (rb.head + 1) % len(rb.buffer)
	rb.size--
	return item, nil
}

func BenchmarkRingBufferWrite(b *testing.B) {
	rb := New[int](10000, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.TryWrite(i)
	}
}

func BenchmarkMutexRingBufferWrite(b *testing.B) {
	rb := newMutexRingBuffer[int](10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Write(i)
	}
}

func BenchmarkRingBufferRead(b *testing.B) {
	rb := New[int](b.N+1, false)
	for i := 0; i < b.N; i++ {
		rb.Write(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Read()
	}
}

func BenchmarkMutexRingBufferRead(b *testing.B) {
	rb := newMutexRingBuffer[int](b.N + 1)
	for i := 0; i < b.N; i++ {
		rb.Write(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Read()
	}
}

func BenchmarkRingBufferWriteRead(b *testing.B) {
	rb := New[int](1000, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Write(i)
		rb.Read()
	}
}

func BenchmarkMutexRingBufferWriteRead(b *testing.B) {
	rb := newMutexRingBuffer[int](1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Write(i)
		rb.Read()
	}
}

func BenchmarkRingBufferConcurrentWrite(b *testing.B) {
	rb := New[int](100000, false)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			rb.TryWrite(i)
			i++
		}
	})
}

func BenchmarkMutexRingBufferConcurrentWrite(b *testing.B) {
	rb := newMutexRingBuffer[int](100000)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			rb.Write(i)
			i++
		}
	})
}

func BenchmarkRingBufferProducerConsumer(b *testing.B) {
	rb := New[int](1000, false)
	var wg sync.WaitGroup
	wg.Add(2)

	b.ResetTimer()

	// Producer
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			for {
				if err := rb.Write(i); err == nil {
					break
				}
			}
		}
	}()

	// Consumer
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			for {
				if _, err := rb.Read(); err == nil {
					break
				}
			}
		}
	}()

	wg.Wait()
}

func BenchmarkMutexRingBufferProducerConsumer(b *testing.B) {
	rb := newMutexRingBuffer[int](1000)
	var wg sync.WaitGroup
	wg.Add(2)

	b.ResetTimer()

	// Producer
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			for {
				if err := rb.Write(i); err == nil {
					break
				}
			}
		}
	}()

	// Consumer
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			for {
				if _, err := rb.Read(); err == nil {
					break
				}
			}
		}
	}()

	wg.Wait()
}

func BenchmarkRingBufferMultiProducerConsumer(b *testing.B) {
	rb := New[int](1000, false)
	numProducers := 4
	numConsumers := 4
	var wg sync.WaitGroup
	wg.Add(numProducers + numConsumers)

	opsPerGoroutine := b.N / (numProducers + numConsumers)
	b.ResetTimer()

	// Producers
	for i := 0; i < numProducers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				for {
					if err := rb.Write(j); err == nil {
						break
					}
				}
			}
		}()
	}

	// Consumers
	for i := 0; i < numConsumers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				for {
					if _, err := rb.Read(); err == nil {
						break
					}
				}
			}
		}()
	}

	wg.Wait()
}

func BenchmarkRingBufferTryOperations(b *testing.B) {
	rb := New[int](1000, false)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				rb.TryWrite(i)
			} else {
				rb.TryRead()
			}
			i++
		}
	})
}

func BenchmarkRingBufferOverwrite(b *testing.B) {
	rb := New[int](100, true) // Allow overwrite
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Write(i)
	}
}
