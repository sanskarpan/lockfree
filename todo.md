# Audit Todo

This file tracks the bugs and hardening work found during the end-to-end audit on 2026-05-25. The list is ordered by production risk, not by implementation effort.

Resolution status:
- Items 1-10 were fixed and validated during this audit.
- Validation covered `go test ./...`, `go test -race ./...`, `go vet ./...`, focused WebSocket/HTTP checks, and targeted benchmark reruns for the striped counter.
- A follow-up security pass also cleared `govulncheck ./...` and `npm audit`.

## Critical

### 1. Web visualizer is broken when started from the repository root
- Status: Fixed and validated
- Surface: `web/server.go`
- Reproduction:
  - Run `go run web/server.go` from the repository root.
  - Request `http://localhost:8081/`.
  - Result: `404 page not found`.
- Root cause:
  - Static assets are served from `http.Dir("./static")`, which only works when the process working directory is `web/`.
  - The path is coupled to the caller’s cwd instead of the source/binary location.
- Impact:
  - The advertised entrypoint is not usable in a clean environment unless the caller manually changes directories first.
  - Breaks local setup, CI smoke tests, and any production-style launch that does not use `web/` as cwd.
- Fix plan:
  - Resolve the static directory from the server source/binary location.
  - Add HTTP tests that verify the UI loads when started from the repository root context.

### 2. WebSocket state snapshots mutate live data and return inconsistent cross-client views
- Status: Fixed and validated
- Surface: `web/server.go`
- Reproduction:
  - Open two WebSocket clients concurrently.
  - Observe different initial state payloads for the same shared server state.
  - Example seen during audit: one client received a stack with items while another simultaneously received an empty stack.
- Root cause:
  - `getStackState()` pops all items and pushes them back.
  - `getQueueState()` dequeues all items and enqueues them back.
  - `clear` handlers also mutate structures by repeated pop/dequeue/delete loops.
  - None of those operations are serialized at the server layer, so snapshots are not linearizable.
- Impact:
  - Clients can see corrupted or contradictory state.
  - Concurrent state reads can reorder or lose data relative to in-flight operations.
  - Multi-client behavior is not trustworthy.
- Fix plan:
  - Stop using destructive reads for snapshots.
  - Serialize server-level operations and snapshots.
  - Rework clear/reset paths to swap/reset structures safely.
  - Add WebSocket regression tests for multi-client consistency.

### 3. WebSocket connection handling allows unsafe concurrent writes and weak failure isolation
- Status: Fixed and validated
- Surface: `web/server.go`
- Root cause:
  - A connection is added to the broadcast set before the initial state write is finished.
  - `sendState()` writes directly on the socket while `broadcastMessages()` can also write to the same socket.
  - Gorilla WebSocket requires a single writer per connection.
  - Slow or broken clients are handled inline in the broadcaster, so one bad client can stall fanout.
- Impact:
  - Possible `concurrent write to websocket connection` failures or undefined behavior.
  - Slow clients can degrade service for healthy clients.
  - Error handling is brittle and connection cleanup is awkward.
- Fix plan:
  - Introduce per-client send queues and a dedicated write pump.
  - Broadcast by enqueueing messages, not by writing inline.
  - Apply write deadlines and deterministic cleanup.

## High

### 4. Ring buffer public API advertises overwrite mode but silently ignores it
- Status: Fixed and validated
- Surface: `ringbuffer/ringbuffer.go`, README/docs
- Root cause:
  - `New(capacity, allowOverwrite)` accepts an overwrite flag but discards it.
  - Code comment explicitly says it is ignored.
- Impact:
  - Public API contract is violated.
  - Callers opting into overwrite mode still get `ErrBufferFull`.
  - Documentation and runtime behavior diverge.
- Fix plan:
  - Implement overwrite mode correctly.
  - Add tests for wraparound and newest-item retention semantics.
  - Update docs to match the actual guarantees.

### 5. `MinMax` cannot represent `math.MinInt64` correctly as the maximum
- Status: Fixed and validated
- Surface: `counter/counter.go`
- Root cause:
  - Initial max sentinel is set to `-int64(^uint64(0) >> 1)`, which is `-9223372036854775807`, not `math.MinInt64`.
  - If the first or largest observed value is `math.MinInt64`, max tracking is wrong.
- Impact:
  - Incorrect statistics for a valid part of the `int64` domain.
  - Silent correctness bug in monitoring/telemetry style usage.
- Fix plan:
  - Use explicit `math.MaxInt64` / `math.MinInt64` sentinels.
  - Add extreme-value tests.

### 6. `list.IntCompare` can overflow and break ordering
- Status: Fixed and validated
- Surface: `list/list.go`
- Root cause:
  - Comparison uses `a - b`.
  - Extreme integer values can overflow and return the wrong sign.
- Impact:
  - Sorted list order can be corrupted for large positive/negative keys.
  - Search/insert/delete correctness depends on the comparator, so the bug can invalidate the data structure.
- Fix plan:
  - Replace subtraction with branch-based comparison.
  - Add regression tests using `math.MinInt` / `math.MaxInt`.

### 7. Web server security and reliability defaults are too weak
- Status: Fixed and validated
- Surface: `web/server.go`
- Root cause:
  - `CheckOrigin` allows every origin.
  - HTTP server uses `http.ListenAndServe` with no read/write/idle timeouts.
  - Invalid message types/operations are silently accepted as zero-value responses.
- Impact:
  - Cross-origin WebSocket use is unrestricted.
  - Server is easier to abuse with slow clients.
  - Debugging bad client behavior is harder than it should be.
- Fix plan:
  - Restrict origins to same-host by default with explicit local-dev allowances.
  - Build an `http.Server` with timeouts.
  - Return structured error messages for invalid requests.

## Medium

### 8. `StripedCounter` distribution strategy does not reliably reduce contention
- Status: Fixed and validated
- Surface: `counter/counter.go`
- Root cause:
  - Stripe selection is derived from global runtime counters instead of a stable per-goroutine or per-call spread.
  - Stripes are not padded, so adjacent counters can share cache lines.
- Impact:
  - The striped implementation can underperform the simple atomic counter under load.
  - The implementation does not consistently deliver the performance contract described in docs.
- Fix plan:
  - Improve stripe selection and pad stripes to reduce false sharing.
  - Re-benchmark after the change.

### 9. Nil comparator is not rejected at construction time
- Status: Fixed and validated
- Surface: `list/list.go`
- Root cause:
  - `list.New()` accepts a nil comparison function and stores it without validation.
- Impact:
  - Callers get a delayed panic on first operation rather than a clear construction-time failure.
- Fix plan:
  - Panic immediately on nil comparator.
  - Add a focused constructor test.

### 10. Web layer has no automated test coverage
- Status: Fixed and validated
- Surface: `web/`
- Root cause:
  - There are no HTTP/WebSocket tests.
- Impact:
  - The currently broken root-start flow and inconsistent multi-client snapshots both shipped unnoticed.
- Fix plan:
  - Add HTTP and WebSocket tests that cover startup, initial state, mutation flows, invalid requests, and multi-client behavior.

## Validation checklist after fixes
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- End-to-end HTTP check for `http://localhost:8081/`
- End-to-end WebSocket checks for:
  - initial state
  - state mutation flows
  - invalid requests
  - multi-client consistency
  - ring buffer overwrite behavior
