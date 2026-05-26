package list

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anishathalye/porcupine"
)

type listInput struct {
	Op  string
	Key int
}

type listOutput struct {
	OK    bool
	Value string
}

func TestLinearizableListHistory(t *testing.T) {
	t.Parallel()

	l := New[int, string](IntCompare)
	var clock atomic.Int64
	var historyMu sync.Mutex
	history := make([]porcupine.Operation, 0, 256)

	record := func(clientID int, input listInput, fn func() listOutput) {
		call := clock.Add(1)
		output := fn()
		ret := clock.Add(1)
		historyMu.Lock()
		history = append(history, porcupine.Operation{
			ClientId: clientID,
			Input:    input,
			Call:     call,
			Output:   output,
			Return:   ret,
		})
		historyMu.Unlock()
	}

	var wg sync.WaitGroup
	for clientID := 0; clientID < 4; clientID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			base := id * 10
			for i := 0; i < 10; i++ {
				key := base + i
				record(id, listInput{Op: "insert", Key: key}, func() listOutput {
					return listOutput{OK: l.Insert(key, fmt.Sprintf("value_%d", key))}
				})
				record(id, listInput{Op: "search", Key: key}, func() listOutput {
					value, ok := l.Search(key)
					return listOutput{OK: ok, Value: value}
				})
				record(id, listInput{Op: "delete", Key: key}, func() listOutput {
					return listOutput{OK: l.Delete(key)}
				})
			}
		}(clientID)
	}
	wg.Wait()

	model := porcupine.Model{
		Init: func() interface{} {
			return map[int]bool{}
		},
		Step: func(state interface{}, input interface{}, output interface{}) (bool, interface{}) {
			current := cloneSet(state.(map[int]bool))
			in := input.(listInput)
			out := output.(listOutput)

			switch in.Op {
			case "insert":
				if current[in.Key] {
					return !out.OK, current
				}
				if !out.OK {
					return false, state
				}
				current[in.Key] = true
				return true, current
			case "search":
				present := current[in.Key]
				expectedValue := fmt.Sprintf("value_%d", in.Key)
				if present {
					return out.OK && out.Value == expectedValue, current
				}
				return !out.OK, current
			case "delete":
				if current[in.Key] {
					if !out.OK {
						return false, state
					}
					delete(current, in.Key)
					return true, current
				}
				return !out.OK, current
			default:
				return false, state
			}
		},
	}

	if result := porcupine.CheckOperationsTimeout(model, history, 5*time.Second); result != porcupine.Ok {
		t.Fatalf("expected list history to be linearizable, got %v", result)
	}
}

func cloneSet(input map[int]bool) map[int]bool {
	output := make(map[int]bool, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
