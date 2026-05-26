package queue

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anishathalye/porcupine"
)

type queueInput struct {
	Op    string
	Value int
}

type queueOutput struct {
	OK    bool
	Value int
}

func TestLinearizableQueueHistory(t *testing.T) {
	t.Parallel()

	q := New[int]()
	var clock atomic.Int64
	var historyMu sync.Mutex
	history := make([]porcupine.Operation, 0, 256)

	record := func(clientID int, input queueInput, fn func() queueOutput) {
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
			for i := 0; i < 15; i++ {
				value := base + i
				record(id, queueInput{Op: "enqueue", Value: value}, func() queueOutput {
					q.Enqueue(value)
					return queueOutput{OK: true}
				})
				record(id, queueInput{Op: "dequeue"}, func() queueOutput {
					value, ok := q.Dequeue()
					return queueOutput{OK: ok, Value: value}
				})
			}
		}(clientID)
	}
	wg.Wait()

	model := porcupine.Model{
		Init: func() interface{} {
			return []int{}
		},
		Step: func(state interface{}, input interface{}, output interface{}) (bool, interface{}) {
			current := append([]int(nil), state.([]int)...)
			in := input.(queueInput)
			out := output.(queueOutput)

			switch in.Op {
			case "enqueue":
				if !out.OK {
					return false, state
				}
				next := append(current, in.Value)
				return true, next
			case "dequeue":
				if len(current) == 0 {
					return !out.OK, current
				}
				if !out.OK || out.Value != current[0] {
					return false, state
				}
				return true, append([]int(nil), current[1:]...)
			default:
				return false, state
			}
		},
	}

	if result := porcupine.CheckOperationsTimeout(model, history, 5*time.Second); result != porcupine.Ok {
		t.Fatalf("expected queue history to be linearizable, got %v", result)
	}
}
