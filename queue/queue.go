// Package queue provides a lock-free FIFO queue implementation using the Michael-Scott algorithm.
// The queue is safe for concurrent use by multiple goroutines without external synchronization.
package queue

import (
	"sync/atomic"
	"unsafe"
)

// node represents a single element in the queue
type node[T any] struct {
	value T
	next  unsafe.Pointer // *node[T]
}

// Queue is a lock-free FIFO queue implementation using the Michael-Scott algorithm.
// This algorithm uses two pointers (head and tail) and CAS operations to maintain
// a consistent queue state across concurrent operations.
//
// The queue uses a sentinel (dummy) node to simplify the implementation.
// The zero value is NOT ready to use; use New() to create a queue.
type Queue[T any] struct {
	head unsafe.Pointer // *node[T]
	tail unsafe.Pointer // *node[T]
	len  atomic.Int64   // track length atomically
}

// New creates and returns a new empty lock-free queue.
func New[T any]() *Queue[T] {
	// Create a sentinel (dummy) node
	sentinel := &node[T]{}
	q := &Queue[T]{
		head: unsafe.Pointer(sentinel),
		tail: unsafe.Pointer(sentinel),
	}
	return q
}

// Enqueue adds a value to the back of the queue.
// This operation is lock-free and safe for concurrent use.
//
// Time complexity: O(1) amortized
func (q *Queue[T]) Enqueue(value T) {
	newNode := &node[T]{value: value}
	newNodePtr := unsafe.Pointer(newNode)

	for {
		// Read tail and its next pointer
		tail := atomic.LoadPointer(&q.tail)
		tailNode := (*node[T])(tail)
		next := atomic.LoadPointer(&tailNode.next)

		// Check if tail is still the same (consistency check)
		if tail == atomic.LoadPointer(&q.tail) {
			// Check if tail is actually pointing to the last node
			if next == nil {
				// Try to link the new node at the end of the list
				if atomic.CompareAndSwapPointer(&tailNode.next, next, newNodePtr) {
					// Enqueue succeeded; try to swing tail to the new node
					atomic.CompareAndSwapPointer(&q.tail, tail, newNodePtr)
					q.len.Add(1)
					return
				}
			} else {
				// Tail is falling behind; try to advance it
				atomic.CompareAndSwapPointer(&q.tail, tail, next)
			}
		}
	}
}

// Dequeue removes and returns the value at the front of the queue.
// Returns the value and true if successful, or zero value and false if the queue is empty.
// This operation is lock-free and safe for concurrent use.
//
// Time complexity: O(1) amortized
func (q *Queue[T]) Dequeue() (T, bool) {
	for {
		// Read head, tail, and head's next pointer
		head := atomic.LoadPointer(&q.head)
		tail := atomic.LoadPointer(&q.tail)
		headNode := (*node[T])(head)
		next := atomic.LoadPointer(&headNode.next)

		// Check if head is still the same (consistency check)
		if head == atomic.LoadPointer(&q.head) {
			// Check if queue is empty or tail is falling behind
			if head == tail {
				// Check if queue is truly empty
				if next == nil {
					var zero T
					return zero, false
				}
				// Tail is falling behind; try to advance it
				atomic.CompareAndSwapPointer(&q.tail, tail, next)
			} else {
				// Read value before CAS to avoid race with another dequeue
				nextNode := (*node[T])(next)
				value := nextNode.value

				// Try to swing head to the next node
				if atomic.CompareAndSwapPointer(&q.head, head, next) {
					q.len.Add(-1)
					return value, true
				}
			}
		}
	}
}

// Peek returns the value at the front of the queue without removing it.
// Returns the value and true if successful, or zero value and false if the queue is empty.
// This operation is lock-free and safe for concurrent use.
//
// Note: In a concurrent environment, the value may be modified by another goroutine
// immediately after this function returns.
//
// Time complexity: O(1)
func (q *Queue[T]) Peek() (T, bool) {
	head := atomic.LoadPointer(&q.head)
	headNode := (*node[T])(head)
	next := atomic.LoadPointer(&headNode.next)

	if next == nil {
		var zero T
		return zero, false
	}

	nextNode := (*node[T])(next)
	return nextNode.value, true
}

// Len returns the current length of the queue.
// This operation is lock-free and safe for concurrent use.
//
// Note: In a highly concurrent environment, the length may change
// immediately after this function returns.
//
// Time complexity: O(1)
func (q *Queue[T]) Len() int64 {
	return q.len.Load()
}

// IsEmpty returns true if the queue is empty.
// This operation is lock-free and safe for concurrent use.
//
// Time complexity: O(1)
func (q *Queue[T]) IsEmpty() bool {
	head := atomic.LoadPointer(&q.head)
	headNode := (*node[T])(head)
	next := atomic.LoadPointer(&headNode.next)
	return next == nil
}

// ToSlice returns all elements in the queue as a slice (from front to back).
// This operation creates a snapshot of the queue at a moment in time.
// The queue may be modified by other goroutines during this operation.
//
// Time complexity: O(n)
func (q *Queue[T]) ToSlice() []T {
	var result []T

	// Start from head's next (skip sentinel)
	head := atomic.LoadPointer(&q.head)
	current := (*node[T])(head)
	next := atomic.LoadPointer(&current.next)

	for next != nil {
		nextNode := (*node[T])(next)
		result = append(result, nextNode.value)
		next = atomic.LoadPointer(&nextNode.next)
	}

	return result
}
