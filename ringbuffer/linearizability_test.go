package ringbuffer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anishathalye/porcupine"
)

type rbInput struct {
	Op    string
	Value int
}

type rbOutput struct {
	OK    bool
	Value int
}

type rbState struct {
	Items     []int
	Capacity  int
	Overwrite bool
}

func TestLinearizableRingBufferHistory(t *testing.T) {
	t.Parallel()
	runRingBufferLinearizabilityTest(t, false)
}

func TestLinearizableRingBufferOverwriteHistory(t *testing.T) {
	t.Parallel()
	runRingBufferLinearizabilityTest(t, true)
}

func runRingBufferLinearizabilityTest(t *testing.T, overwrite bool) {
	t.Helper()

	rb := New[int](4, overwrite)
	var clock atomic.Int64
	var historyMu sync.Mutex
	history := make([]porcupine.Operation, 0, 256)

	record := func(clientID int, input rbInput, fn func() rbOutput) {
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
			base := id * 100
			for i := 0; i < 12; i++ {
				value := base + i
				record(id, rbInput{Op: "write", Value: value}, func() rbOutput {
					err := rb.Write(value)
					return rbOutput{OK: err == nil}
				})
				record(id, rbInput{Op: "read"}, func() rbOutput {
					value, err := rb.Read()
					return rbOutput{OK: err == nil, Value: value}
				})
			}
		}(clientID)
	}
	wg.Wait()

	model := porcupine.Model{
		Init: func() interface{} {
			return rbState{Items: []int{}, Capacity: 4, Overwrite: overwrite}
		},
		Step: func(state interface{}, input interface{}, output interface{}) (bool, interface{}) {
			current := state.(rbState)
			nextState := rbState{
				Items:     append([]int(nil), current.Items...),
				Capacity:  current.Capacity,
				Overwrite: current.Overwrite,
			}
			in := input.(rbInput)
			out := output.(rbOutput)

			switch in.Op {
			case "write":
				if len(nextState.Items) >= nextState.Capacity {
					if !nextState.Overwrite {
						return !out.OK, nextState
					}
					nextState.Items = append(nextState.Items[1:], in.Value)
					return out.OK, nextState
				}
				if !out.OK {
					return false, state
				}
				nextState.Items = append(nextState.Items, in.Value)
				return true, nextState
			case "read":
				if len(nextState.Items) == 0 {
					return !out.OK, nextState
				}
				if !out.OK || out.Value != nextState.Items[0] {
					return false, state
				}
				nextState.Items = append([]int(nil), nextState.Items[1:]...)
				return true, nextState
			default:
				return false, state
			}
		},
	}

	if result := porcupine.CheckOperationsTimeout(model, history, 5*time.Second); result != porcupine.Ok {
		t.Fatalf("expected ring buffer history to be linearizable, got %v (overwrite=%v)", result, overwrite)
	}
}
