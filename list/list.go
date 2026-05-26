// Package list provides a lock-free sorted linked list implementation.
// The list maintains elements in sorted order and supports concurrent operations.
package list

import (
	"sync/atomic"
	"unsafe"
)

// node represents a single element in the linked list
type node[K comparable, V any] struct {
	key    K
	value  V
	next   unsafe.Pointer // *node[K, V]
	marked atomic.Bool    // logical deletion flag
}

// compareFunc is a function that compares two keys.
// Returns:
//   - negative if a < b
//   - zero if a == b
//   - positive if a > b
type compareFunc[K comparable] func(a, b K) int

// List is a lock-free sorted linked list.
// It uses the Harris-Michael algorithm with atomic marking for deletion.
//
// The list maintains elements in sorted order based on the compare function.
// Zero value is NOT ready to use; use New() to create a list.
type List[K comparable, V any] struct {
	head    *node[K, V]
	compare compareFunc[K]
	len     atomic.Int64
}

// New creates a new lock-free sorted linked list.
// The compare function determines the sort order of elements.
func New[K comparable, V any](compare compareFunc[K]) *List[K, V] {
	if compare == nil {
		panic("list compare function must not be nil")
	}

	var zeroKey K
	var zeroValue V

	// Create sentinel head and tail nodes
	head := &node[K, V]{key: zeroKey, value: zeroValue}

	return &List[K, V]{
		head:    head,
		compare: compare,
	}
}

// search finds the position to insert/delete a key
// Returns pred, curr where key should be between pred and curr
func (l *List[K, V]) search(key K) (*node[K, V], *node[K, V]) {
retry:
	pred := l.head
	curr := (*node[K, V])(atomic.LoadPointer(&pred.next))

	for curr != nil {
		succ := (*node[K, V])(atomic.LoadPointer(&curr.next))

		// If current node is marked, try to remove it
		if curr.marked.Load() {
			// Try to physically remove the marked node
			if !atomic.CompareAndSwapPointer(&pred.next, unsafe.Pointer(curr), unsafe.Pointer(succ)) {
				// Someone else modified pred, retry
				goto retry
			}
			curr = succ
			continue
		}

		// Check if we've found the position
		cmp := l.compare(curr.key, key)
		if cmp >= 0 {
			return pred, curr
		}

		pred = curr
		curr = succ
	}

	return pred, nil
}

// Insert adds a key-value pair to the list.
// If the key already exists, it returns false without updating the value.
// Returns true if a new entry was added, false if the key already exists.
//
// This operation is lock-free and safe for concurrent use.
func (l *List[K, V]) Insert(key K, value V) bool {
	for {
		pred, curr := l.search(key)

		// Check if key already exists
		if curr != nil && l.compare(curr.key, key) == 0 {
			// Key exists, don't update (to avoid race conditions)
			return false
		}

		// Create new node
		newNode := &node[K, V]{
			key:   key,
			value: value,
			next:  unsafe.Pointer(curr),
		}

		// Try to insert between pred and curr
		if atomic.CompareAndSwapPointer(&pred.next, unsafe.Pointer(curr), unsafe.Pointer(newNode)) {
			l.len.Add(1)
			return true
		}
		// CAS failed, retry
	}
}

// Delete removes a key from the list.
// Returns true if the key was found and deleted, false otherwise.
//
// This operation is lock-free and safe for concurrent use.
func (l *List[K, V]) Delete(key K) bool {
	for {
		pred, curr := l.search(key)

		// Key not found
		if curr == nil || l.compare(curr.key, key) != 0 {
			return false
		}

		// Check if already marked
		if curr.marked.Load() {
			continue
		}

		// Try to mark the node as deleted
		if !curr.marked.CompareAndSwap(false, true) {
			// Failed to mark, retry
			continue
		}

		// Successfully marked, now try to physically remove
		succ := (*node[K, V])(atomic.LoadPointer(&curr.next))
		atomic.CompareAndSwapPointer(&pred.next, unsafe.Pointer(curr), unsafe.Pointer(succ))

		l.len.Add(-1)
		return true
	}
}

// Search looks up a key in the list.
// Returns the value and true if found, zero value and false otherwise.
//
// This operation is lock-free and safe for concurrent use.
func (l *List[K, V]) Search(key K) (V, bool) {
	_, curr := l.search(key)

	if curr != nil && l.compare(curr.key, key) == 0 {
		if !curr.marked.Load() {
			return curr.value, true
		}
	}

	var zero V
	return zero, false
}

// Contains checks if a key exists in the list.
// Returns true if the key is found, false otherwise.
//
// This operation is lock-free and safe for concurrent use.
func (l *List[K, V]) Contains(key K) bool {
	_, found := l.Search(key)
	return found
}

// Len returns the approximate length of the list.
// In a concurrent environment, this may not be exact.
func (l *List[K, V]) Len() int64 {
	return l.len.Load()
}

// IsEmpty returns true if the list is empty.
func (l *List[K, V]) IsEmpty() bool {
	curr := (*node[K, V])(atomic.LoadPointer(&l.head.next))
	return curr == nil
}

// Range calls the provided function for each key-value pair in the list.
// The function receives the key and value. If it returns false, iteration stops.
// The iteration is performed in sorted order.
//
// Note: This creates a snapshot of the list at a moment in time.
// Concurrent modifications may not be reflected.
func (l *List[K, V]) Range(fn func(key K, value V) bool) {
	curr := (*node[K, V])(atomic.LoadPointer(&l.head.next))

	for curr != nil {
		succ := (*node[K, V])(atomic.LoadPointer(&curr.next))

		if !curr.marked.Load() {
			if !fn(curr.key, curr.value) {
				return
			}
		}

		curr = succ
	}
}

// ToSlice returns all key-value pairs as a slice in sorted order.
// This creates a snapshot of the list at a moment in time.
func (l *List[K, V]) ToSlice() []struct {
	Key   K
	Value V
} {
	var result []struct {
		Key   K
		Value V
	}

	l.Range(func(key K, value V) bool {
		result = append(result, struct {
			Key   K
			Value V
		}{Key: key, Value: value})
		return true
	})

	return result
}

// IntCompare is a comparison function for integers.
func IntCompare(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// StringCompare is a comparison function for strings.
func StringCompare(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
