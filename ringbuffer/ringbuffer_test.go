package ringbuffer

import (
	"math/rand"
	"sync"
	"testing"
)

func TestRingBufferBasicOperations(t *testing.T) {
	rb := New[int](5, false)

	if !rb.IsEmpty() {
		t.Error("New ring buffer should be empty")
	}

	// Capacity is rounded up to next power of 2 (5 -> 8)
	if rb.Cap() < 5 {
		t.Errorf("Expected capacity >= 5, got %d", rb.Cap())
	}

	if rb.Len() != 0 {
		t.Errorf("Expected length 0, got %d", rb.Len())
	}
}

func TestRingBufferWriteRead(t *testing.T) {
	rb := New[int](5, false)

	// Write single item
	if err := rb.Write(1); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if rb.IsEmpty() {
		t.Error("Buffer should not be empty after write")
	}

	if rb.Len() != 1 {
		t.Errorf("Expected length 1, got %d", rb.Len())
	}

	// Read the item
	val, err := rb.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if val != 1 {
		t.Errorf("Expected 1, got %d", val)
	}

	if !rb.IsEmpty() {
		t.Error("Buffer should be empty after reading all items")
	}
}

func TestRingBufferFIFO(t *testing.T) {
	rb := New[int](5, false)

	// Write multiple items
	for i := 1; i <= 4; i++ {
		if err := rb.Write(i); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	// Read in FIFO order
	for i := 1; i <= 4; i++ {
		val, err := rb.Read()
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		if val != i {
			t.Errorf("Expected %d, got %d", i, val)
		}
	}
}

func TestRingBufferFullNoOverwrite(t *testing.T) {
	rb := New[int](4, false) // capacity 4 (power of 2)

	// Fill the buffer completely
	for i := 0; i < 4; i++ {
		if err := rb.Write(i + 1); err != nil {
			t.Fatalf("Write %d failed: %v", i+1, err)
		}
	}

	// Buffer should be full now
	if !rb.IsFull() {
		t.Error("Buffer should be full")
	}

	// Try to write when full
	err := rb.Write(5)
	if err != ErrBufferFull {
		t.Errorf("Expected ErrBufferFull, got %v", err)
	}

	// Read one item
	val, err := rb.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if val != 1 {
		t.Errorf("Expected 1, got %d", val)
	}

	// Now we should be able to write
	if err := rb.Write(5); err != nil {
		t.Fatalf("Write after read failed: %v", err)
	}
}

func TestRingBufferOverwrite(t *testing.T) {
	rb := New[int](4, true)

	// Fill the buffer
	for i := 0; i < 4; i++ {
		if err := rb.Write(i + 1); err != nil {
			t.Fatalf("write %d failed: %v", i+1, err)
		}
	}

	// Overwrite should evict the oldest item.
	if err := rb.Write(5); err != nil {
		t.Fatalf("overwrite write failed: %v", err)
	}

	// Read all items in FIFO order. The oldest item (1) should be gone.
	expected := []int{2, 3, 4, 5}
	for i, want := range expected {
		val, err := rb.Read()
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		if val != want {
			t.Errorf("At index %d: expected %d, got %d", i, want, val)
		}
	}
}

func TestRingBufferOverwriteMultipleWraps(t *testing.T) {
	rb := New[int](4, true)

	for i := 1; i <= 10; i++ {
		if err := rb.Write(i); err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}
	}

	expected := []int{7, 8, 9, 10}
	for i, want := range expected {
		val, err := rb.Read()
		if err != nil {
			t.Fatalf("read %d failed: %v", i, err)
		}
		if val != want {
			t.Fatalf("at index %d: expected %d, got %d", i, want, val)
		}
	}
}

func TestRingBufferTryOperations(t *testing.T) {
	rb := New[int](4, false) // capacity 4 (power of 2)

	// TryRead on empty buffer
	if _, ok := rb.TryRead(); ok {
		t.Error("TryRead should return false on empty buffer")
	}

	// TryWrite
	if !rb.TryWrite(1) {
		t.Error("TryWrite should succeed on non-full buffer")
	}

	// TryRead
	val, ok := rb.TryRead()
	if !ok || val != 1 {
		t.Errorf("TryRead failed: got (%d, %v), want (1, true)", val, ok)
	}

	// Fill buffer completely
	for i := 0; i < 4; i++ {
		rb.TryWrite(i + 1)
	}

	// TryWrite on full buffer should fail
	if rb.TryWrite(99) {
		t.Error("TryWrite should fail on full buffer")
	}
}

func TestRingBufferPeek(t *testing.T) {
	rb := New[int](5, false)

	// Peek on empty buffer
	if _, err := rb.Peek(); err != ErrBufferEmpty {
		t.Errorf("Peek on empty buffer should return ErrBufferEmpty, got %v", err)
	}

	// Write and peek
	rb.Write(42)
	val, err := rb.Peek()
	if err != nil {
		t.Fatalf("Peek failed: %v", err)
	}
	if val != 42 {
		t.Errorf("Expected 42, got %d", val)
	}

	// Peek should not remove the item
	if rb.Len() != 1 {
		t.Error("Peek should not modify buffer length")
	}

	// Read should still return the same item
	val, _ = rb.Read()
	if val != 42 {
		t.Errorf("Expected 42, got %d", val)
	}
}

func TestRingBufferClear(t *testing.T) {
	rb := New[int](5, false)

	rb.Write(1)
	rb.Write(2)
	rb.Write(3)

	rb.Clear()

	if !rb.IsEmpty() {
		t.Error("Buffer should be empty after Clear")
	}
	if rb.Len() != 0 {
		t.Errorf("Expected length 0 after Clear, got %d", rb.Len())
	}
}

func TestRingBufferAvailable(t *testing.T) {
	rb := New[int](8, false) // capacity 8 (power of 2)

	// Initially, available should be capacity
	if avail := rb.Available(); avail != 8 {
		t.Errorf("Expected available 8, got %d", avail)
	}

	rb.Write(1)
	rb.Write(2)

	if avail := rb.Available(); avail != 6 {
		t.Errorf("Expected available 6, got %d", avail)
	}
}

func TestRingBufferConcurrentWriteRead(t *testing.T) {
	rb := New[int](100, false)
	numWriters := 10
	numReaders := 10
	itemsPerWriter := 100

	var wg sync.WaitGroup
	wg.Add(numWriters + numReaders)

	// Writers
	for i := 0; i < numWriters; i++ {
		go func(base int) {
			defer wg.Done()
			for j := 0; j < itemsPerWriter; j++ {
				for {
					if err := rb.Write(base*itemsPerWriter + j); err == nil {
						break
					}
					// Retry on full
				}
			}
		}(i)
	}

	// Readers
	readCount := make([]int, numReaders)
	for i := 0; i < numReaders; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < itemsPerWriter; j++ {
				for {
					if _, err := rb.Read(); err == nil {
						readCount[idx]++
						break
					}
					// Retry on empty
				}
			}
		}(i)
	}

	wg.Wait()

	// Count total reads
	totalReads := 0
	for _, count := range readCount {
		totalReads += count
	}

	// Total reads + remaining should equal total writes
	remaining := rb.Len()
	expectedTotal := int64(numWriters * itemsPerWriter)
	actualTotal := int64(totalReads) + remaining

	if actualTotal != expectedTotal {
		t.Errorf("Expected total %d, got %d (reads: %d, remaining: %d)",
			expectedTotal, actualTotal, totalReads, remaining)
	}
}

func TestRingBufferConcurrentTryOperations(t *testing.T) {
	rb := New[int](50, false)
	numGoroutines := 20
	operationsPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	writeSuccesses := make([]int, numGoroutines)
	readSuccesses := make([]int, numGoroutines)

	// Writers using TryWrite
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				if rb.TryWrite(j) {
					writeSuccesses[idx]++
				}
			}
		}(i)
	}

	// Readers using TryRead
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				if _, ok := rb.TryRead(); ok {
					readSuccesses[idx]++
				}
			}
		}(i)
	}

	wg.Wait()

	// Count totals
	totalWrites := 0
	for _, count := range writeSuccesses {
		totalWrites += count
	}
	totalReads := 0
	for _, count := range readSuccesses {
		totalReads += count
	}

	// Total reads + remaining should equal total writes
	remaining := rb.Len()
	if int64(totalReads)+remaining != int64(totalWrites) {
		t.Errorf("Inconsistent state: writes=%d, reads=%d, remaining=%d",
			totalWrites, totalReads, remaining)
	}
}

func TestRingBufferWrapAround(t *testing.T) {
	rb := New[int](5, false)

	// Write and read to cause wrap-around
	for i := 0; i < 10; i++ {
		rb.Write(i)
		val, _ := rb.Read()
		if val != i {
			t.Errorf("Expected %d, got %d", i, val)
		}
	}

	// Buffer should still be empty
	if !rb.IsEmpty() {
		t.Error("Buffer should be empty after balanced writes/reads")
	}
}

func TestRingBufferTypes(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		rb := New[string](5, false)
		rb.Write("hello")
		rb.Write("world")

		val, _ := rb.Read()
		if val != "hello" {
			t.Errorf("Expected 'hello', got '%s'", val)
		}
	})

	t.Run("Struct", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}

		rb := New[Person](5, false)
		rb.Write(Person{"Alice", 30})

		val, _ := rb.Read()
		if val.Name != "Alice" || val.Age != 30 {
			t.Errorf("Expected Alice(30), got %v", val)
		}
	})
}

func TestRingBufferReadEmpty(t *testing.T) {
	rb := New[int](5, false)

	_, err := rb.Read()
	if err != ErrBufferEmpty {
		t.Errorf("Expected ErrBufferEmpty, got %v", err)
	}
}

func TestRingBufferRandomizedModel(t *testing.T) {
	t.Run("NoOverwrite", func(t *testing.T) {
		runRingBufferModelTest(t, false)
	})
	t.Run("Overwrite", func(t *testing.T) {
		runRingBufferModelTest(t, true)
	})
}

func runRingBufferModelTest(t *testing.T, overwrite bool) {
	t.Helper()

	rng := rand.New(rand.NewSource(42))
	rb := New[int](8, overwrite)
	model := ringBufferModel{capacity: 8, overwrite: overwrite}

	for step := 0; step < 5000; step++ {
		switch rng.Intn(7) {
		case 0:
			value := rng.Intn(1000)
			gotErr := rb.Write(value)
			wantErr := model.Write(value)
			assertRingBufferErr(t, step, gotErr, wantErr)
		case 1:
			gotValue, gotErr := rb.Read()
			wantValue, wantErr := model.Read()
			assertRingBufferErr(t, step, gotErr, wantErr)
			if gotErr == nil && gotValue != wantValue {
				t.Fatalf("step %d: Read value=%d, want %d", step, gotValue, wantValue)
			}
		case 2:
			value := rng.Intn(1000)
			gotOK := rb.TryWrite(value)
			wantOK := model.TryWrite(value)
			if gotOK != wantOK {
				t.Fatalf("step %d: TryWrite=%v, want %v", step, gotOK, wantOK)
			}
		case 3:
			gotValue, gotOK := rb.TryRead()
			wantValue, wantOK := model.TryRead()
			if gotOK != wantOK {
				t.Fatalf("step %d: TryRead ok=%v, want %v", step, gotOK, wantOK)
			}
			if gotOK && gotValue != wantValue {
				t.Fatalf("step %d: TryRead value=%d, want %d", step, gotValue, wantValue)
			}
		case 4:
			gotValue, gotErr := rb.Peek()
			wantValue, wantErr := model.Peek()
			assertRingBufferErr(t, step, gotErr, wantErr)
			if gotErr == nil && gotValue != wantValue {
				t.Fatalf("step %d: Peek value=%d, want %d", step, gotValue, wantValue)
			}
		case 5:
			rb.Clear()
			model.Clear()
		case 6:
			if rb.Cap() != int64(model.capacity) {
				t.Fatalf("step %d: Cap=%d, want %d", step, rb.Cap(), model.capacity)
			}
		}

		if got, want := rb.Len(), int64(model.Len()); got != want {
			t.Fatalf("step %d: Len=%d, want %d", step, got, want)
		}
		if got, want := rb.Available(), int64(model.Available()); got != want {
			t.Fatalf("step %d: Available=%d, want %d", step, got, want)
		}
		if got, want := rb.IsEmpty(), model.IsEmpty(); got != want {
			t.Fatalf("step %d: IsEmpty=%v, want %v", step, got, want)
		}
		if got, want := rb.IsFull(), model.IsFull(); got != want {
			t.Fatalf("step %d: IsFull=%v, want %v", step, got, want)
		}
	}
}

func assertRingBufferErr(t *testing.T, step int, got, want error) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if got != want {
		t.Fatalf("step %d: error=%v, want %v", step, got, want)
	}
}

type ringBufferModel struct {
	items     []int
	capacity  int
	overwrite bool
}

func (m *ringBufferModel) Write(value int) error {
	if len(m.items) >= m.capacity {
		if !m.overwrite {
			return ErrBufferFull
		}
		copy(m.items, m.items[1:])
		m.items[len(m.items)-1] = value
		return nil
	}
	m.items = append(m.items, value)
	return nil
}

func (m *ringBufferModel) TryWrite(value int) bool {
	return m.Write(value) == nil
}

func (m *ringBufferModel) Read() (int, error) {
	if len(m.items) == 0 {
		return 0, ErrBufferEmpty
	}
	value := m.items[0]
	m.items = append([]int(nil), m.items[1:]...)
	return value, nil
}

func (m *ringBufferModel) TryRead() (int, bool) {
	value, err := m.Read()
	return value, err == nil
}

func (m *ringBufferModel) Peek() (int, error) {
	if len(m.items) == 0 {
		return 0, ErrBufferEmpty
	}
	return m.items[0], nil
}

func (m *ringBufferModel) Clear() {
	m.items = nil
}

func (m *ringBufferModel) Len() int {
	return len(m.items)
}

func (m *ringBufferModel) Available() int {
	return m.capacity - len(m.items)
}

func (m *ringBufferModel) IsEmpty() bool {
	return len(m.items) == 0
}

func (m *ringBufferModel) IsFull() bool {
	return len(m.items) == m.capacity
}
