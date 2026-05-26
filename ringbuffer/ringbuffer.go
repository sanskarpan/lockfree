// Package ringbuffer provides a lock-free ring buffer (circular buffer) implementation.
// The ring buffer is a fixed-size, FIFO data structure using sequence numbers for synchronization.
package ringbuffer

import (
	"errors"
	"sync/atomic"
)

var (
	// ErrBufferFull is returned when trying to write to a full buffer
	// and overwrite is disabled.
	ErrBufferFull = errors.New("ring buffer is full")

	// ErrBufferEmpty is returned when trying to read from an empty buffer.
	ErrBufferEmpty = errors.New("ring buffer is empty")
)

// slot represents a single buffer slot with sequence number for synchronization
type slot[T any] struct {
	sequence atomic.Uint64
	value    T
}

// RingBuffer is a lock-free, fixed-size circular buffer using sequence numbers.
// It supports concurrent read and write operations without data races.
//
// The buffer uses sequence numbers to synchronize access:
// - Writers increment write sequence
// - Readers increment read sequence
// - Each slot has its own sequence for fine-grained synchronization
//
// Zero value is NOT ready to use; use New() to create a buffer.
type RingBuffer[T any] struct {
	buffer    []slot[T]
	mask      uint64 // capacity - 1 (for power-of-2 sizes)
	capacity  uint64
	overwrite bool
	readPos   atomic.Uint64
	writePos  atomic.Uint64
}

// New creates a new ring buffer with the specified capacity.
// Capacity will be rounded up to the next power of 2 for efficiency.
func New[T any](capacity int, allowOverwrite bool) *RingBuffer[T] {
	if capacity <= 0 {
		panic("ring buffer capacity must be positive")
	}

	// Round up to next power of 2
	actualCap := uint64(1)
	for actualCap < uint64(capacity) {
		actualCap <<= 1
	}

	rb := &RingBuffer[T]{
		buffer:    make([]slot[T], actualCap),
		mask:      actualCap - 1,
		capacity:  actualCap,
		overwrite: allowOverwrite,
	}

	// Initialize sequence numbers
	for i := uint64(0); i < actualCap; i++ {
		rb.buffer[i].sequence.Store(i)
	}

	return rb
}

// Write adds an item to the buffer.
// Returns ErrBufferFull if the buffer is full.
//
// This operation is lock-free and safe for concurrent use.
func (rb *RingBuffer[T]) Write(item T) error {
	for {
		writePos := rb.writePos.Load()
		slot := &rb.buffer[writePos&rb.mask]
		seq := slot.sequence.Load()

		// Check if this slot is available for writing
		diff := int64(seq) - int64(writePos)

		if diff == 0 {
			// Slot is available, try to claim it
			if rb.writePos.CompareAndSwap(writePos, writePos+1) {
				// Successfully claimed, write the value
				slot.value = item
				// Make value visible to readers
				slot.sequence.Store(writePos + 1)
				return nil
			}
		} else if diff < 0 {
			// Buffer is full
			if !rb.overwrite {
				return ErrBufferFull
			}
			rb.discardOldest(writePos)
		}
		// else: slot not ready yet, retry
	}
}

func (rb *RingBuffer[T]) discardOldest(writePos uint64) {
	for {
		readPos := rb.readPos.Load()

		// Another reader may already have created space.
		if writePos-readPos < rb.capacity {
			return
		}

		slot := &rb.buffer[readPos&rb.mask]
		seq := slot.sequence.Load()
		diff := int64(seq) - int64(readPos+1)

		if diff < 0 {
			return
		}
		if diff > 0 {
			continue
		}

		if rb.readPos.CompareAndSwap(readPos, readPos+1) {
			var zero T
			slot.value = zero
			slot.sequence.Store(readPos + rb.capacity)
			return
		}
	}
}

// Read removes and returns an item from the buffer.
// Returns ErrBufferEmpty if the buffer is empty.
//
// This operation is lock-free and safe for concurrent use.
func (rb *RingBuffer[T]) Read() (T, error) {
	for {
		readPos := rb.readPos.Load()
		slot := &rb.buffer[readPos&rb.mask]
		seq := slot.sequence.Load()

		// Check if this slot has data to read
		diff := int64(seq) - int64(readPos+1)

		if diff == 0 {
			// Data is available, try to claim it
			if rb.readPos.CompareAndSwap(readPos, readPos+1) {
				// Successfully claimed, read the value
				value := slot.value
				// Clear the value for GC
				var zero T
				slot.value = zero
				// Make slot available for next write
				slot.sequence.Store(readPos + rb.capacity)
				return value, nil
			}
		} else if diff < 0 {
			// Buffer is empty
			var zero T
			return zero, ErrBufferEmpty
		}
		// else: data not ready yet, retry
	}
}

// TryWrite attempts to write an item to the buffer without blocking.
// Returns true if successful, false if buffer is full.
//
// This operation is lock-free and safe for concurrent use.
func (rb *RingBuffer[T]) TryWrite(item T) bool {
	err := rb.Write(item)
	return err == nil
}

// TryRead attempts to read an item from the buffer without blocking.
// Returns the item and true if successful, zero value and false if buffer is empty.
//
// This operation is lock-free and safe for concurrent use.
func (rb *RingBuffer[T]) TryRead() (T, bool) {
	item, err := rb.Read()
	if err != nil {
		return item, false
	}
	return item, true
}

// Len returns the approximate current number of items in the buffer.
// Note: In a highly concurrent environment, this value may be stale.
//
// This operation is lock-free and safe for concurrent use.
func (rb *RingBuffer[T]) Len() int64 {
	writePos := rb.writePos.Load()
	readPos := rb.readPos.Load()
	return int64(writePos - readPos)
}

// Cap returns the capacity of the buffer.
func (rb *RingBuffer[T]) Cap() int64 {
	return int64(rb.capacity)
}

// IsEmpty returns true if the buffer appears empty.
// Note: In a concurrent environment, the buffer may not be empty
// immediately after this returns true.
//
// This operation is lock-free and safe for concurrent use.
func (rb *RingBuffer[T]) IsEmpty() bool {
	return rb.Len() == 0
}

// IsFull returns true if the buffer appears full.
// Note: In a concurrent environment, the buffer may not be full
// immediately after this returns true.
//
// This operation is lock-free and safe for concurrent use.
func (rb *RingBuffer[T]) IsFull() bool {
	return rb.Len() >= int64(rb.capacity)
}

// Clear resets the buffer to empty state.
// This operation is NOT safe for concurrent use with Read/Write.
func (rb *RingBuffer[T]) Clear() {
	readPos := rb.readPos.Load()
	writePos := rb.writePos.Load()

	// Drain all items
	for readPos < writePos {
		slot := &rb.buffer[readPos&rb.mask]
		var zero T
		slot.value = zero
		slot.sequence.Store(readPos + rb.capacity)
		readPos++
	}

	rb.readPos.Store(0)
	rb.writePos.Store(0)

	// Reinitialize sequence numbers
	for i := uint64(0); i < rb.capacity; i++ {
		rb.buffer[i].sequence.Store(i)
	}
}

// Available returns the approximate number of free slots in the buffer.
// Note: In a concurrent environment, this value may be stale.
func (rb *RingBuffer[T]) Available() int64 {
	return int64(rb.capacity) - rb.Len()
}

// Peek returns the oldest item in the buffer without removing it.
// Returns ErrBufferEmpty if the buffer is empty.
// Note: The item may be read by another goroutine immediately after this returns.
//
// This is not truly lock-free as it doesn't use CAS, but is safe for reading.
func (rb *RingBuffer[T]) Peek() (T, error) {
	readPos := rb.readPos.Load()
	slot := &rb.buffer[readPos&rb.mask]
	seq := slot.sequence.Load()

	if int64(seq)-int64(readPos+1) < 0 {
		var zero T
		return zero, ErrBufferEmpty
	}

	return slot.value, nil
}
