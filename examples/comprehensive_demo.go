package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/sanskar/lockfree/counter"
	"github.com/sanskar/lockfree/list"
	"github.com/sanskar/lockfree/queue"
	"github.com/sanskar/lockfree/ringbuffer"
	"github.com/sanskar/lockfree/stack"
)

func main() {
	fmt.Println("=== Lock-Free Data Structures Demo ===")
	fmt.Println()

	demonstrateStack()
	demonstrateQueue()
	demonstrateCounter()
	demonstrateRingBuffer()
	demonstrateList()
	demonstrateConcurrency()
}

func demonstrateStack() {
	fmt.Println("1. Lock-Free Stack (LIFO)")
	fmt.Println("---------------------------")

	s := stack.New[string]()

	// Push elements
	s.Push("First")
	s.Push("Second")
	s.Push("Third")

	fmt.Printf("Stack length: %d\n", s.Len())
	fmt.Printf("Stack contents (top to bottom): %v\n", s.ToSlice())

	// Pop elements
	for !s.IsEmpty() {
		val, _ := s.Pop()
		fmt.Printf("Popped: %s\n", val)
	}

	fmt.Println()
}

func demonstrateQueue() {
	fmt.Println("2. Lock-Free Queue (FIFO)")
	fmt.Println("---------------------------")

	q := queue.New[int]()

	// Enqueue elements
	for i := 1; i <= 5; i++ {
		q.Enqueue(i * 10)
	}

	fmt.Printf("Queue length: %d\n", q.Len())
	fmt.Printf("Queue contents (front to back): %v\n", q.ToSlice())

	// Dequeue elements
	for i := 0; i < 3; i++ {
		val, _ := q.Dequeue()
		fmt.Printf("Dequeued: %d\n", val)
	}

	fmt.Printf("Remaining: %v\n", q.ToSlice())
	fmt.Println()
}

func demonstrateCounter() {
	fmt.Println("3. Lock-Free Counter")
	fmt.Println("---------------------")

	// Simple counter
	c := counter.New()
	c.Inc()
	c.Inc()
	c.Add(5)
	fmt.Printf("Counter value: %d\n", c.Get())

	// Striped counter (better for high contention)
	sc := counter.NewStriped()
	for i := 0; i < 10; i++ {
		sc.Inc()
	}
	fmt.Printf("Striped counter value: %d\n", sc.Get())

	// MinMax tracker
	mm := counter.NewMinMax()
	values := []int64{5, 2, 8, 1, 9, 3}
	for _, v := range values {
		mm.Update(v)
	}
	fmt.Printf("Min: %d, Max: %d\n", mm.Min(), mm.Max())

	// Accumulator
	acc := counter.NewAccumulator()
	for _, v := range values {
		acc.Add(v)
	}
	fmt.Printf("Sum: %d, Count: %d, Average: %.2f\n", acc.Sum(), acc.Count(), acc.Average())

	fmt.Println()
}

func demonstrateRingBuffer() {
	fmt.Println("4. Lock-Free Ring Buffer")
	fmt.Println("--------------------------")

	rb := ringbuffer.New[string](5, false)

	// Write elements
	messages := []string{"msg1", "msg2", "msg3", "msg4"}
	for _, msg := range messages {
		if err := rb.Write(msg); err != nil {
			fmt.Printf("Write error: %v\n", err)
		}
	}

	fmt.Printf("Buffer capacity: %d\n", rb.Cap())
	fmt.Printf("Buffer length: %d\n", rb.Len())
	fmt.Printf("Available slots: %d\n", rb.Available())

	// Read elements
	for i := 0; i < 2; i++ {
		msg, _ := rb.Read()
		fmt.Printf("Read: %s\n", msg)
	}

	fmt.Printf("Remaining length: %d\n", rb.Len())
	fmt.Println()
}

func demonstrateList() {
	fmt.Println("5. Lock-Free Sorted List")
	fmt.Println("--------------------------")

	l := list.New[int, string](list.IntCompare)

	// Insert elements (in random order)
	l.Insert(5, "five")
	l.Insert(2, "two")
	l.Insert(8, "eight")
	l.Insert(1, "one")

	fmt.Printf("List length: %d\n", l.Len())

	// Elements are automatically sorted
	fmt.Println("Sorted contents:")
	l.Range(func(key int, value string) bool {
		fmt.Printf("  %d: %s\n", key, value)
		return true
	})

	// Search
	if val, found := l.Search(5); found {
		fmt.Printf("Found key 5: %s\n", val)
	}

	// Delete
	l.Delete(2)
	fmt.Printf("After deleting key 2, length: %d\n", l.Len())

	fmt.Println()
}

func demonstrateConcurrency() {
	fmt.Println("6. Concurrent Operations Demo")
	fmt.Println("-------------------------------")

	// Concurrent stack operations
	s := stack.New[int]()
	var wg sync.WaitGroup

	// Multiple goroutines pushing
	numGoroutines := 10
	itemsPerGoroutine := 100

	start := time.Now()

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				s.Push(id*itemsPerGoroutine + j)
			}
		}(i)
	}
	wg.Wait()

	elapsed := time.Since(start)

	fmt.Printf("Concurrent push: %d goroutines, %d items each\n", numGoroutines, itemsPerGoroutine)
	fmt.Printf("Total items: %d\n", s.Len())
	fmt.Printf("Time taken: %v\n", elapsed)

	// Concurrent counter test
	c := counter.NewStriped()

	start = time.Now()

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()

	elapsed = time.Since(start)

	fmt.Printf("\nConcurrent counter increment:\n")
	fmt.Printf("Final value: %d\n", c.Get())
	fmt.Printf("Time taken: %v\n", elapsed)

	fmt.Println("\n=== Demo Complete ===")
}
