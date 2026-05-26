package stack

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestStackBasicOperations(t *testing.T) {
	s := New[int]()

	// Test empty stack
	if !s.IsEmpty() {
		t.Error("New stack should be empty")
	}

	if s.Len() != 0 {
		t.Errorf("Expected length 0, got %d", s.Len())
	}

	// Test Pop on empty stack
	if _, ok := s.Pop(); ok {
		t.Error("Pop on empty stack should return false")
	}

	// Test Peek on empty stack
	if _, ok := s.Peek(); ok {
		t.Error("Peek on empty stack should return false")
	}
}

func TestStackPushPop(t *testing.T) {
	s := New[int]()

	// Push single element
	s.Push(1)
	if s.IsEmpty() {
		t.Error("Stack should not be empty after Push")
	}
	if s.Len() != 1 {
		t.Errorf("Expected length 1, got %d", s.Len())
	}

	// Peek should return the element without removing it
	if val, ok := s.Peek(); !ok || val != 1 {
		t.Errorf("Peek failed: got (%v, %v), want (1, true)", val, ok)
	}
	if s.Len() != 1 {
		t.Error("Peek should not modify stack length")
	}

	// Pop should return the element
	if val, ok := s.Pop(); !ok || val != 1 {
		t.Errorf("Pop failed: got (%v, %v), want (1, true)", val, ok)
	}
	if !s.IsEmpty() {
		t.Error("Stack should be empty after popping all elements")
	}
}

func TestStackLIFO(t *testing.T) {
	s := New[int]()

	// Push multiple elements
	for i := 1; i <= 5; i++ {
		s.Push(i)
	}

	// Verify LIFO order
	for i := 5; i >= 1; i-- {
		val, ok := s.Pop()
		if !ok {
			t.Fatalf("Pop failed at iteration %d", i)
		}
		if val != i {
			t.Errorf("Expected %d, got %d", i, val)
		}
	}

	if !s.IsEmpty() {
		t.Error("Stack should be empty after popping all elements")
	}
}

func TestStackClear(t *testing.T) {
	s := New[string]()

	s.Push("a")
	s.Push("b")
	s.Push("c")

	s.Clear()

	if !s.IsEmpty() {
		t.Error("Stack should be empty after Clear")
	}
	if s.Len() != 0 {
		t.Errorf("Expected length 0 after Clear, got %d", s.Len())
	}
}

func TestStackToSlice(t *testing.T) {
	s := New[int]()

	// Empty stack
	slice := s.ToSlice()
	if len(slice) != 0 {
		t.Error("ToSlice on empty stack should return empty slice")
	}

	// Push elements
	s.Push(1)
	s.Push(2)
	s.Push(3)

	slice = s.ToSlice()
	expected := []int{3, 2, 1} // Top to bottom

	if len(slice) != len(expected) {
		t.Fatalf("Expected slice length %d, got %d", len(expected), len(slice))
	}

	for i, val := range slice {
		if val != expected[i] {
			t.Errorf("At index %d: expected %d, got %d", i, expected[i], val)
		}
	}
}

func TestStackConcurrentPush(t *testing.T) {
	s := New[int]()
	numGoroutines := 100
	itemsPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent pushes
	for i := 0; i < numGoroutines; i++ {
		go func(base int) {
			defer wg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				s.Push(base*itemsPerGoroutine + j)
			}
		}(i)
	}

	wg.Wait()

	expectedLen := int64(numGoroutines * itemsPerGoroutine)
	if s.Len() != expectedLen {
		t.Errorf("Expected length %d, got %d", expectedLen, s.Len())
	}

	// Verify we can pop all elements
	count := 0
	for {
		if _, ok := s.Pop(); !ok {
			break
		}
		count++
	}

	if count != int(expectedLen) {
		t.Errorf("Expected to pop %d elements, got %d", expectedLen, count)
	}
}

func TestStackConcurrentPushPop(t *testing.T) {
	s := New[int]()
	numGoroutines := 50
	itemsPerGoroutine := 100

	var pushWg, popWg sync.WaitGroup
	pushWg.Add(numGoroutines)
	popWg.Add(numGoroutines)

	// Concurrent pushes
	for i := 0; i < numGoroutines; i++ {
		go func(base int) {
			defer pushWg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				s.Push(base*itemsPerGoroutine + j)
			}
		}(i)
	}

	// Concurrent pops
	poppedCount := make([]int, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer popWg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				if _, ok := s.Pop(); ok {
					poppedCount[idx]++
				}
				runtime.Gosched() // Give other goroutines a chance
			}
		}(i)
	}

	pushWg.Wait()
	popWg.Wait()

	// Count total popped
	totalPopped := 0
	for _, count := range poppedCount {
		totalPopped += count
	}

	// Remaining items
	remaining := s.Len()

	expectedTotal := int64(numGoroutines * itemsPerGoroutine)
	actualTotal := int64(totalPopped) + remaining

	if actualTotal != expectedTotal {
		t.Errorf("Expected total %d, got %d (popped: %d, remaining: %d)",
			expectedTotal, actualTotal, totalPopped, remaining)
	}
}

func TestStackTypes(t *testing.T) {
	// Test with different types
	t.Run("String", func(t *testing.T) {
		s := New[string]()
		s.Push("hello")
		s.Push("world")

		val, ok := s.Pop()
		if !ok || val != "world" {
			t.Errorf("Expected 'world', got '%s'", val)
		}
	})

	t.Run("Struct", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}

		s := New[Person]()
		s.Push(Person{"Alice", 30})
		s.Push(Person{"Bob", 25})

		val, ok := s.Pop()
		if !ok || val.Name != "Bob" || val.Age != 25 {
			t.Errorf("Expected Bob(25), got %v", val)
		}
	})

	t.Run("Pointer", func(t *testing.T) {
		s := New[*int]()
		a, b := 1, 2
		s.Push(&a)
		s.Push(&b)

		val, ok := s.Pop()
		if !ok || *val != 2 {
			t.Errorf("Expected pointer to 2, got %v", val)
		}
	})
}

func TestStackStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	s := New[int]()
	numGoroutines := 100
	operations := 1000

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2) // pushers and poppers

	// Pushers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				s.Push(id*operations + j)
			}
		}(i)
	}

	// Poppers
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				s.Pop()
				runtime.Gosched()
			}
		}()
	}

	wg.Wait()

	// The stack should be consistent (no crashes or deadlocks)
	t.Logf("Final stack length: %d", s.Len())
}

func TestStackConcurrentValueIntegrity(t *testing.T) {
	s := New[int]()

	const producers = 8
	const consumers = 4
	const itemsPerProducer = 250
	const totalItems = producers * itemsPerProducer

	var producerWG sync.WaitGroup
	producerWG.Add(producers)
	for producer := 0; producer < producers; producer++ {
		go func(id int) {
			defer producerWG.Done()
			base := id * itemsPerProducer
			for i := 0; i < itemsPerProducer; i++ {
				s.Push(base + i)
			}
		}(producer)
	}

	doneProducing := make(chan struct{})
	go func() {
		producerWG.Wait()
		close(doneProducing)
	}()

	var consumedCount atomic.Int64
	seen := make([]int, totalItems)
	var seenMu sync.Mutex
	var consumerWG sync.WaitGroup
	consumerWG.Add(consumers)

	for i := 0; i < consumers; i++ {
		go func() {
			defer consumerWG.Done()
			for {
				if val, ok := s.Pop(); ok {
					if val < 0 || val >= totalItems {
						t.Errorf("popped value out of range: %d", val)
						return
					}
					seenMu.Lock()
					seen[val]++
					seenMu.Unlock()
					if consumedCount.Add(1) == totalItems {
						return
					}
					continue
				}

				select {
				case <-doneProducing:
					if s.IsEmpty() {
						return
					}
				default:
					runtime.Gosched()
				}
			}
		}()
	}

	consumerWG.Wait()

	if got := consumedCount.Load(); got != totalItems {
		t.Fatalf("expected to consume %d items, got %d", totalItems, got)
	}
	if !s.IsEmpty() {
		t.Fatalf("stack should be empty after consuming all items, len=%d", s.Len())
	}

	for value, count := range seen {
		if count != 1 {
			t.Fatalf("value %d observed %d times, expected exactly once", value, count)
		}
	}
}
