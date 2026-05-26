package queue

import (
	"runtime"
	"sync"
	"testing"
)

func TestQueueBasicOperations(t *testing.T) {
	q := New[int]()

	// Test empty queue
	if !q.IsEmpty() {
		t.Error("New queue should be empty")
	}

	if q.Len() != 0 {
		t.Errorf("Expected length 0, got %d", q.Len())
	}

	// Test Dequeue on empty queue
	if _, ok := q.Dequeue(); ok {
		t.Error("Dequeue on empty queue should return false")
	}

	// Test Peek on empty queue
	if _, ok := q.Peek(); ok {
		t.Error("Peek on empty queue should return false")
	}
}

func TestQueueEnqueueDequeue(t *testing.T) {
	q := New[int]()

	// Enqueue single element
	q.Enqueue(1)
	if q.IsEmpty() {
		t.Error("Queue should not be empty after Enqueue")
	}
	if q.Len() != 1 {
		t.Errorf("Expected length 1, got %d", q.Len())
	}

	// Peek should return the element without removing it
	if val, ok := q.Peek(); !ok || val != 1 {
		t.Errorf("Peek failed: got (%v, %v), want (1, true)", val, ok)
	}
	if q.Len() != 1 {
		t.Error("Peek should not modify queue length")
	}

	// Dequeue should return the element
	if val, ok := q.Dequeue(); !ok || val != 1 {
		t.Errorf("Dequeue failed: got (%v, %v), want (1, true)", val, ok)
	}
	if !q.IsEmpty() {
		t.Error("Queue should be empty after dequeuing all elements")
	}
}

func TestQueueFIFO(t *testing.T) {
	q := New[int]()

	// Enqueue multiple elements
	for i := 1; i <= 5; i++ {
		q.Enqueue(i)
	}

	// Verify FIFO order
	for i := 1; i <= 5; i++ {
		val, ok := q.Dequeue()
		if !ok {
			t.Fatalf("Dequeue failed at iteration %d", i)
		}
		if val != i {
			t.Errorf("Expected %d, got %d", i, val)
		}
	}

	if !q.IsEmpty() {
		t.Error("Queue should be empty after dequeuing all elements")
	}
}

func TestQueueToSlice(t *testing.T) {
	q := New[int]()

	// Empty queue
	slice := q.ToSlice()
	if len(slice) != 0 {
		t.Error("ToSlice on empty queue should return empty slice")
	}

	// Enqueue elements
	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)

	slice = q.ToSlice()
	expected := []int{1, 2, 3} // Front to back

	if len(slice) != len(expected) {
		t.Fatalf("Expected slice length %d, got %d", len(expected), len(slice))
	}

	for i, val := range slice {
		if val != expected[i] {
			t.Errorf("At index %d: expected %d, got %d", i, expected[i], val)
		}
	}
}

func TestQueueConcurrentEnqueue(t *testing.T) {
	q := New[int]()
	numGoroutines := 100
	itemsPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent enqueues
	for i := 0; i < numGoroutines; i++ {
		go func(base int) {
			defer wg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				q.Enqueue(base*itemsPerGoroutine + j)
			}
		}(i)
	}

	wg.Wait()

	expectedLen := int64(numGoroutines * itemsPerGoroutine)
	if q.Len() != expectedLen {
		t.Errorf("Expected length %d, got %d", expectedLen, q.Len())
	}

	// Verify we can dequeue all elements
	count := 0
	for {
		if _, ok := q.Dequeue(); !ok {
			break
		}
		count++
	}

	if count != int(expectedLen) {
		t.Errorf("Expected to dequeue %d elements, got %d", expectedLen, count)
	}
}

func TestQueueConcurrentEnqueueDequeue(t *testing.T) {
	q := New[int]()
	numGoroutines := 50
	itemsPerGoroutine := 100

	var enqWg, deqWg sync.WaitGroup
	enqWg.Add(numGoroutines)
	deqWg.Add(numGoroutines)

	// Concurrent enqueues
	for i := 0; i < numGoroutines; i++ {
		go func(base int) {
			defer enqWg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				q.Enqueue(base*itemsPerGoroutine + j)
			}
		}(i)
	}

	// Concurrent dequeues
	dequeuedCount := make([]int, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer deqWg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				if _, ok := q.Dequeue(); ok {
					dequeuedCount[idx]++
				}
				runtime.Gosched() // Give other goroutines a chance
			}
		}(i)
	}

	enqWg.Wait()
	deqWg.Wait()

	// Count total dequeued
	totalDequeued := 0
	for _, count := range dequeuedCount {
		totalDequeued += count
	}

	// Remaining items
	remaining := q.Len()

	expectedTotal := int64(numGoroutines * itemsPerGoroutine)
	actualTotal := int64(totalDequeued) + remaining

	if actualTotal != expectedTotal {
		t.Errorf("Expected total %d, got %d (dequeued: %d, remaining: %d)",
			expectedTotal, actualTotal, totalDequeued, remaining)
	}
}

func TestQueueTypes(t *testing.T) {
	// Test with different types
	t.Run("String", func(t *testing.T) {
		q := New[string]()
		q.Enqueue("hello")
		q.Enqueue("world")

		val, ok := q.Dequeue()
		if !ok || val != "hello" {
			t.Errorf("Expected 'hello', got '%s'", val)
		}
	})

	t.Run("Struct", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}

		q := New[Person]()
		q.Enqueue(Person{"Alice", 30})
		q.Enqueue(Person{"Bob", 25})

		val, ok := q.Dequeue()
		if !ok || val.Name != "Alice" || val.Age != 30 {
			t.Errorf("Expected Alice(30), got %v", val)
		}
	})

	t.Run("Pointer", func(t *testing.T) {
		q := New[*int]()
		a, b := 1, 2
		q.Enqueue(&a)
		q.Enqueue(&b)

		val, ok := q.Dequeue()
		if !ok || *val != 1 {
			t.Errorf("Expected pointer to 1, got %v", val)
		}
	})
}

func TestQueueInterleavedOperations(t *testing.T) {
	q := New[int]()

	// Interleave enqueues and dequeues
	q.Enqueue(1)
	q.Enqueue(2)

	val, ok := q.Dequeue()
	if !ok || val != 1 {
		t.Errorf("Expected 1, got %d", val)
	}

	q.Enqueue(3)
	q.Enqueue(4)

	val, ok = q.Dequeue()
	if !ok || val != 2 {
		t.Errorf("Expected 2, got %d", val)
	}

	val, ok = q.Dequeue()
	if !ok || val != 3 {
		t.Errorf("Expected 3, got %d", val)
	}

	q.Enqueue(5)

	val, ok = q.Dequeue()
	if !ok || val != 4 {
		t.Errorf("Expected 4, got %d", val)
	}

	val, ok = q.Dequeue()
	if !ok || val != 5 {
		t.Errorf("Expected 5, got %d", val)
	}

	if !q.IsEmpty() {
		t.Error("Queue should be empty")
	}
}

func TestQueueStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	q := New[int]()
	numGoroutines := 100
	operations := 1000

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2) // enqueuers and dequeuers

	// Enqueuers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				q.Enqueue(id*operations + j)
			}
		}(i)
	}

	// Dequeuers
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				q.Dequeue()
				runtime.Gosched()
			}
		}()
	}

	wg.Wait()

	// The queue should be consistent (no crashes or deadlocks)
	t.Logf("Final queue length: %d", q.Len())
}

func TestQueueProducerConsumer(t *testing.T) {
	q := New[int]()
	numProducers := 10
	numConsumers := 10
	itemsPerProducer := 100

	var prodWg, consWg sync.WaitGroup
	prodWg.Add(numProducers)
	consWg.Add(numConsumers)

	consumed := make(map[int]int) // Track consumed items
	var mu sync.Mutex

	// Producers
	for i := 0; i < numProducers; i++ {
		go func(id int) {
			defer prodWg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				q.Enqueue(id*itemsPerProducer + j)
			}
		}(i)
	}

	// Consumers
	for i := 0; i < numConsumers; i++ {
		go func() {
			defer consWg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				if val, ok := q.Dequeue(); ok {
					mu.Lock()
					consumed[val]++
					mu.Unlock()
				}
				runtime.Gosched()
			}
		}()
	}

	prodWg.Wait()
	consWg.Wait()

	// Drain remaining items
	for {
		if val, ok := q.Dequeue(); ok {
			mu.Lock()
			consumed[val]++
			mu.Unlock()
		} else {
			break
		}
	}

	// Verify all items consumed exactly once
	expectedItems := numProducers * itemsPerProducer
	if len(consumed) != expectedItems {
		t.Errorf("Expected %d unique items, got %d", expectedItems, len(consumed))
	}

	for val, count := range consumed {
		if count != 1 {
			t.Errorf("Item %d consumed %d times, expected 1", val, count)
		}
	}
}

func TestQueueConcurrentProducersPreservePerProducerOrder(t *testing.T) {
	q := New[int]()

	const producers = 8
	const itemsPerProducer = 200
	const totalItems = producers * itemsPerProducer

	var producerWG sync.WaitGroup
	producerWG.Add(producers)
	for producer := 0; producer < producers; producer++ {
		go func(id int) {
			defer producerWG.Done()
			base := id * itemsPerProducer
			for seq := 0; seq < itemsPerProducer; seq++ {
				q.Enqueue(base + seq)
			}
		}(producer)
	}

	received := make([]int, 0, totalItems)
	for len(received) < totalItems {
		if val, ok := q.Dequeue(); ok {
			received = append(received, val)
			continue
		}

		if len(received) == totalItems {
			break
		}
		runtime.Gosched()
	}

	producerWG.Wait()

	if len(received) != totalItems {
		t.Fatalf("expected %d dequeued items, got %d", totalItems, len(received))
	}

	lastSeqByProducer := make([]int, producers)
	for i := range lastSeqByProducer {
		lastSeqByProducer[i] = -1
	}
	seen := make([]int, totalItems)

	for _, value := range received {
		if value < 0 || value >= totalItems {
			t.Fatalf("dequeued value out of range: %d", value)
		}
		seen[value]++

		producerID := value / itemsPerProducer
		seq := value % itemsPerProducer
		if seq <= lastSeqByProducer[producerID] {
			t.Fatalf("producer %d sequence regressed from %d to %d", producerID, lastSeqByProducer[producerID], seq)
		}
		lastSeqByProducer[producerID] = seq
	}

	for value, count := range seen {
		if count != 1 {
			t.Fatalf("value %d observed %d times, expected exactly once", value, count)
		}
	}
}
