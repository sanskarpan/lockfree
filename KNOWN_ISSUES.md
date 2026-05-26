# Known Issues and Limitations

## Current state

No known correctness or race-detector failures remain after the 2026-05-25 audit.

## Limitations that still matter

### 1. Snapshot helpers are observational, not linearizable

Methods like `ToSlice()`, `Len()`, `Peek()`, and `Range()` are safe to call concurrently, but they do not promise a globally consistent snapshot under arbitrary concurrent mutation. This is typical for low-overhead concurrent containers.

### 2. Memory reclamation is delegated to Go GC

The lock-free algorithms do not implement hazard pointers, epochs, or other explicit reclamation schemes. Safety depends on Go’s managed runtime rather than custom pointer lifetime management.

### 3. Performance claims are workload-specific

The benchmark suite is useful, but results vary by CPU, scheduler behavior, cache topology, and contention pattern. Treat benchmark output as evidence, not a blanket guarantee.

### 4. The web visualizer is a demo

The `web/` package is intended for local exploration:
- no authentication
- no persistent storage
- no rate limiting
- no deployment packaging in this repository

## Validation reference

The current baseline is:
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `govulncheck ./...`

Detailed audit notes and resolved issues are tracked in [todo.md](./todo.md).
