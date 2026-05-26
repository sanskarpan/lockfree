package list

import (
	"math"
	"math/rand"
	"sync"
	"testing"
)

func TestListBasicOperations(t *testing.T) {
	l := New[int, string](IntCompare)

	if !l.IsEmpty() {
		t.Error("New list should be empty")
	}

	if l.Len() != 0 {
		t.Errorf("Expected length 0, got %d", l.Len())
	}
}

func TestListInsertSearch(t *testing.T) {
	l := New[int, string](IntCompare)

	// Insert single element
	if !l.Insert(1, "one") {
		t.Error("Insert should return true for new key")
	}

	if l.IsEmpty() {
		t.Error("List should not be empty after insert")
	}

	if l.Len() != 1 {
		t.Errorf("Expected length 1, got %d", l.Len())
	}

	// Search for the element
	val, found := l.Search(1)
	if !found || val != "one" {
		t.Errorf("Search failed: got (%s, %v), want (one, true)", val, found)
	}

	// Search for non-existent element
	_, found = l.Search(2)
	if found {
		t.Error("Search should return false for non-existent key")
	}
}

func TestListSortedOrder(t *testing.T) {
	l := New[int, int](IntCompare)

	// Insert in random order
	keys := []int{5, 2, 8, 1, 9, 3}
	for _, key := range keys {
		l.Insert(key, key*10)
	}

	// Check sorted order
	expected := []int{1, 2, 3, 5, 8, 9}
	slice := l.ToSlice()

	if len(slice) != len(expected) {
		t.Fatalf("Expected length %d, got %d", len(expected), len(slice))
	}

	for i, item := range slice {
		if item.Key != expected[i] {
			t.Errorf("At index %d: expected %d, got %d", i, expected[i], item.Key)
		}
	}
}

func TestListUpdateExisting(t *testing.T) {
	l := New[int, string](IntCompare)

	l.Insert(1, "first")

	// Try to insert with same key (should fail without updating)
	if l.Insert(1, "updated") {
		t.Error("Insert should return false when key already exists")
	}

	// Value should still be the original
	val, _ := l.Search(1)
	if val != "first" {
		t.Errorf("Expected 'first', got '%s'", val)
	}

	// Length should not change
	if l.Len() != 1 {
		t.Errorf("Expected length 1, got %d", l.Len())
	}
}

func TestListDelete(t *testing.T) {
	l := New[int, string](IntCompare)

	l.Insert(1, "one")
	l.Insert(2, "two")
	l.Insert(3, "three")

	// Delete existing key
	if !l.Delete(2) {
		t.Error("Delete should return true for existing key")
	}

	if l.Len() != 2 {
		t.Errorf("Expected length 2 after delete, got %d", l.Len())
	}

	// Verify deleted
	if l.Contains(2) {
		t.Error("List should not contain deleted key")
	}

	// Delete non-existent key
	if l.Delete(99) {
		t.Error("Delete should return false for non-existent key")
	}
}

func TestListContains(t *testing.T) {
	l := New[int, int](IntCompare)

	l.Insert(1, 10)
	l.Insert(2, 20)

	if !l.Contains(1) {
		t.Error("Contains should return true for existing key")
	}

	if l.Contains(3) {
		t.Error("Contains should return false for non-existent key")
	}
}

func TestListRange(t *testing.T) {
	l := New[int, int](IntCompare)

	// Insert elements
	for i := 1; i <= 5; i++ {
		l.Insert(i, i*10)
	}

	// Range over all elements
	count := 0
	l.Range(func(key, value int) bool {
		count++
		if value != key*10 {
			t.Errorf("Expected value %d, got %d", key*10, value)
		}
		return true
	})

	if count != 5 {
		t.Errorf("Expected to visit 5 elements, visited %d", count)
	}

	// Range with early termination
	count = 0
	l.Range(func(key, value int) bool {
		count++
		return key < 3 // Stop after key 3
	})

	if count != 3 {
		t.Errorf("Expected to visit 3 elements before stopping, visited %d", count)
	}
}

func TestListConcurrentInsert(t *testing.T) {
	l := New[int, int](IntCompare)
	numGoroutines := 50
	itemsPerGoroutine := 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(base int) {
			defer wg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				key := base*itemsPerGoroutine + j
				l.Insert(key, key)
			}
		}(i)
	}

	wg.Wait()

	expectedLen := int64(numGoroutines * itemsPerGoroutine)
	if l.Len() != expectedLen {
		t.Errorf("Expected length %d, got %d", expectedLen, l.Len())
	}

	// Verify all elements
	for i := 0; i < int(expectedLen); i++ {
		if !l.Contains(i) {
			t.Errorf("List should contain key %d", i)
		}
	}
}

func TestListConcurrentInsertDelete(t *testing.T) {
	l := New[int, int](IntCompare)
	numGoroutines := 25
	itemsPerGoroutine := 40

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Inserters
	for i := 0; i < numGoroutines; i++ {
		go func(base int) {
			defer wg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				key := base*itemsPerGoroutine + j
				l.Insert(key, key)
			}
		}(i)
	}

	// Deleters
	deletedCount := make([]int, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			// Delete half of the items
			for j := 0; j < itemsPerGoroutine/2; j++ {
				key := idx*itemsPerGoroutine + j
				if l.Delete(key) {
					deletedCount[idx]++
				}
			}
		}(i)
	}

	wg.Wait()

	// Count total deletions
	totalDeleted := 0
	for _, count := range deletedCount {
		totalDeleted += count
	}

	expectedRemaining := int64(numGoroutines*itemsPerGoroutine - totalDeleted)
	if l.Len() != expectedRemaining {
		t.Errorf("Expected length %d, got %d (total deleted: %d)",
			expectedRemaining, l.Len(), totalDeleted)
	}
}

func TestListConcurrentMixed(t *testing.T) {
	l := New[int, int](IntCompare)
	numGoroutines := 20
	operations := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(id)))

			for j := 0; j < operations; j++ {
				key := r.Intn(100)
				op := r.Intn(3)

				switch op {
				case 0: // Insert
					l.Insert(key, key*10)
				case 1: // Delete
					l.Delete(key)
				case 2: // Search
					l.Search(key)
				}
			}
		}(i)
	}

	wg.Wait()

	// List should be in consistent state (no crashes)
	t.Logf("Final list length: %d", l.Len())
}

func TestListStringKeys(t *testing.T) {
	l := New[string, int](StringCompare)

	l.Insert("apple", 1)
	l.Insert("banana", 2)
	l.Insert("cherry", 3)

	// Should be in alphabetical order
	slice := l.ToSlice()
	expected := []string{"apple", "banana", "cherry"}

	for i, item := range slice {
		if item.Key != expected[i] {
			t.Errorf("At index %d: expected %s, got %s", i, expected[i], item.Key)
		}
	}
}

func TestListCustomCompare(t *testing.T) {
	// Reverse order comparison
	reverseCompare := func(a, b int) int {
		return b - a // Reverse of normal comparison
	}

	l := New[int, string](reverseCompare)

	l.Insert(1, "one")
	l.Insert(2, "two")
	l.Insert(3, "three")

	// Should be in reverse order
	slice := l.ToSlice()
	expected := []int{3, 2, 1}

	for i, item := range slice {
		if item.Key != expected[i] {
			t.Errorf("At index %d: expected %d, got %d", i, expected[i], item.Key)
		}
	}
}

func TestListEdgeCases(t *testing.T) {
	l := New[int, int](IntCompare)

	// Delete from empty list
	if l.Delete(1) {
		t.Error("Delete from empty list should return false")
	}

	// Search in empty list
	if _, found := l.Search(1); found {
		t.Error("Search in empty list should return false")
	}

	// Insert and delete same element repeatedly
	for i := 0; i < 100; i++ {
		l.Insert(1, i)
		if !l.Contains(1) {
			t.Error("Element should exist after insert")
		}
		l.Delete(1)
		if l.Contains(1) {
			t.Error("Element should not exist after delete")
		}
	}
}

func TestListIntCompareExtremes(t *testing.T) {
	l := New[int, string](IntCompare)

	l.Insert(math.MaxInt, "max")
	l.Insert(0, "zero")
	l.Insert(math.MinInt, "min")

	items := l.ToSlice()
	expected := []int{math.MinInt, 0, math.MaxInt}
	if len(items) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(items))
	}

	for i, expectedKey := range expected {
		if items[i].Key != expectedKey {
			t.Fatalf("at index %d: expected key %d, got %d", i, expectedKey, items[i].Key)
		}
	}
}

func TestListNewPanicsOnNilCompare(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected New to panic when compare is nil")
		}
	}()

	New[int, string](nil)
}

func TestListRandomizedModel(t *testing.T) {
	l := New[int, int](IntCompare)
	model := make(map[int]int)
	rng := rand.New(rand.NewSource(42))

	for step := 0; step < 5000; step++ {
		key := rng.Intn(100)
		value := rng.Intn(1000)

		switch rng.Intn(4) {
		case 0:
			got := l.Insert(key, value)
			_, existed := model[key]
			if existed && got {
				t.Fatalf("step %d: Insert(%d) succeeded unexpectedly", step, key)
			}
			if !existed && !got {
				t.Fatalf("step %d: Insert(%d) failed unexpectedly", step, key)
			}
			if !existed {
				model[key] = value
			}
		case 1:
			got := l.Delete(key)
			_, existed := model[key]
			if got != existed {
				t.Fatalf("step %d: Delete(%d)=%v, want %v", step, key, got, existed)
			}
			delete(model, key)
		case 2:
			gotValue, gotFound := l.Search(key)
			wantValue, wantFound := model[key]
			if gotFound != wantFound {
				t.Fatalf("step %d: Search(%d) found=%v, want %v", step, key, gotFound, wantFound)
			}
			if gotFound && gotValue != wantValue {
				t.Fatalf("step %d: Search(%d) value=%d, want %d", step, key, gotValue, wantValue)
			}
		case 3:
			got := l.Contains(key)
			_, want := model[key]
			if got != want {
				t.Fatalf("step %d: Contains(%d)=%v, want %v", step, key, got, want)
			}
		}

		items := l.ToSlice()
		if len(items) != len(model) {
			t.Fatalf("step %d: ToSlice length=%d, want %d", step, len(items), len(model))
		}
		for i := 1; i < len(items); i++ {
			if items[i-1].Key >= items[i].Key {
				t.Fatalf("step %d: list order is not strictly increasing: %d then %d", step, items[i-1].Key, items[i].Key)
			}
		}
		for _, item := range items {
			wantValue, ok := model[item.Key]
			if !ok {
				t.Fatalf("step %d: unexpected key %d in list snapshot", step, item.Key)
			}
			if wantValue != item.Value {
				t.Fatalf("step %d: key %d value=%d, want %d", step, item.Key, item.Value, wantValue)
			}
		}
		if got, want := l.Len(), int64(len(model)); got != want {
			t.Fatalf("step %d: Len()=%d, want %d", step, got, want)
		}
	}
}
