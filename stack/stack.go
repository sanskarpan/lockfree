// Package stack provides a lock-free stack implementation using the Treiber algorithm.
// The stack is safe for concurrent use by multiple goroutines without external synchronization.
package stack

import (
	"sync/atomic"
	"unsafe"
)

// node represents a single element in the stack
type node[T any] struct {
	value T
	next  *node[T]
}

// Stack is a lock-free stack implementation using Compare-And-Swap (CAS) operations.
// It implements the Treiber Stack algorithm, which is a classic lock-free data structure.
//
// The zero value is ready to use. A Stack must not be copied after first use.
type Stack[T any] struct {
	head unsafe.Pointer // *node[T]
	len  atomic.Int64   // track length atomically
}

// New creates and returns a new empty lock-free stack.
func New[T any]() *Stack[T] {
	return &Stack[T]{}
}

// Push adds a value to the top of the stack.
// This operation is lock-free and safe for concurrent use.
//
// Time complexity: O(1) amortized
func (s *Stack[T]) Push(value T) {
	newNode := &node[T]{value: value}

	for {
		// Load the current head
		oldHead := atomic.LoadPointer(&s.head)
		newNode.next = (*node[T])(oldHead)

		// Try to swap the head with the new node
		if atomic.CompareAndSwapPointer(&s.head, oldHead, unsafe.Pointer(newNode)) {
			s.len.Add(1)
			return
		}
		// If CAS failed, retry (another goroutine modified the head)
	}
}

// Pop removes and returns the value at the top of the stack.
// Returns the value and true if successful, or zero value and false if the stack is empty.
// This operation is lock-free and safe for concurrent use.
//
// Time complexity: O(1) amortized
func (s *Stack[T]) Pop() (T, bool) {
	for {
		// Load the current head
		oldHead := atomic.LoadPointer(&s.head)
		if oldHead == nil {
			var zero T
			return zero, false
		}

		oldNode := (*node[T])(oldHead)
		newHead := unsafe.Pointer(oldNode.next)

		// Try to swap the head with the next node
		if atomic.CompareAndSwapPointer(&s.head, oldHead, newHead) {
			s.len.Add(-1)
			return oldNode.value, true
		}
		// If CAS failed, retry
	}
}

// Peek returns the value at the top of the stack without removing it.
// Returns the value and true if successful, or zero value and false if the stack is empty.
// This operation is lock-free and safe for concurrent use.
//
// Note: In a concurrent environment, the value may be modified by another goroutine
// immediately after this function returns.
//
// Time complexity: O(1)
func (s *Stack[T]) Peek() (T, bool) {
	headPtr := atomic.LoadPointer(&s.head)
	if headPtr == nil {
		var zero T
		return zero, false
	}

	headNode := (*node[T])(headPtr)
	return headNode.value, true
}

// Len returns the current length of the stack.
// This operation is lock-free and safe for concurrent use.
//
// Note: In a highly concurrent environment, the length may change
// immediately after this function returns.
//
// Time complexity: O(1)
func (s *Stack[T]) Len() int64 {
	return s.len.Load()
}

// IsEmpty returns true if the stack is empty.
// This operation is lock-free and safe for concurrent use.
//
// Time complexity: O(1)
func (s *Stack[T]) IsEmpty() bool {
	return atomic.LoadPointer(&s.head) == nil
}

// Clear removes all elements from the stack.
// This operation is NOT lock-free and should not be called concurrently with other operations.
//
// Time complexity: O(1)
func (s *Stack[T]) Clear() {
	atomic.StorePointer(&s.head, nil)
	s.len.Store(0)
}

// ToSlice returns all elements in the stack as a slice (from top to bottom).
// This operation creates a snapshot of the stack at a moment in time.
// The stack may be modified by other goroutines during this operation.
//
// Time complexity: O(n)
func (s *Stack[T]) ToSlice() []T {
	var result []T

	// Start from the head
	current := (*node[T])(atomic.LoadPointer(&s.head))

	for current != nil {
		result = append(result, current.value)
		current = current.next
	}

	return result
}
