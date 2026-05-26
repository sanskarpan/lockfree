package counter

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anishathalye/porcupine"
)

type counterInput struct {
	Op    string
	Value int64
}

type counterOutput struct {
	Value int64
}

func TestLinearizableCounterHistory(t *testing.T) {
	t.Parallel()

	c := New()
	var clock atomic.Int64
	var historyMu sync.Mutex
	history := make([]porcupine.Operation, 0, 256)

	record := func(clientID int, input counterInput, fn func() counterOutput) {
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
			for i := 0; i < 20; i++ {
				record(id, counterInput{Op: "inc"}, func() counterOutput {
					return counterOutput{Value: c.Inc()}
				})
				record(id, counterInput{Op: "get"}, func() counterOutput {
					return counterOutput{Value: c.Get()}
				})
				record(id, counterInput{Op: "add", Value: int64(id + 1)}, func() counterOutput {
					return counterOutput{Value: c.Add(int64(id + 1))}
				})
				record(id, counterInput{Op: "dec"}, func() counterOutput {
					return counterOutput{Value: c.Dec()}
				})
			}
		}(clientID)
	}
	wg.Wait()

	model := porcupine.Model{
		Init: func() interface{} {
			return int64(0)
		},
		Step: func(state interface{}, input interface{}, output interface{}) (bool, interface{}) {
			current := state.(int64)
			in := input.(counterInput)
			out := output.(counterOutput)

			switch in.Op {
			case "inc":
				next := current + 1
				return out.Value == next, next
			case "dec":
				next := current - 1
				return out.Value == next, next
			case "add":
				next := current + in.Value
				return out.Value == next, next
			case "get":
				return out.Value == current, current
			default:
				return false, state
			}
		},
	}

	if result := porcupine.CheckOperationsTimeout(model, history, 5*time.Second); result != porcupine.Ok {
		t.Fatalf("expected counter history to be linearizable, got %v", result)
	}
}
