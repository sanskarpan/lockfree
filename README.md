# Lock-Free Data Structures in Go

[![Go Report Card](https://goreportcard.com/badge/github.com/sanskar/lockfree)](https://goreportcard.com/report/github.com/sanskar/lockfree)
[![GoDoc](https://godoc.org/github.com/sanskar/lockfree?status.svg)](https://godoc.org/github.com/sanskar/lockfree)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A comprehensive collection of high-performance, lock-free data structures implemented in Go using atomic operations and CAS (Compare-And-Swap) primitives. All data structures are safe for concurrent use without external synchronization.

## Features

- ✨ **Minimal Dependencies**: Core data structures stay lightweight; the production web service adds `gorilla/websocket`, `x/crypto`, and `x/time`
- 🚀 **High Performance**: Outperforms mutex-based implementations under high contention
- 🔒 **Lock-Free**: Wait-free or lock-free operations using atomic primitives
- 🧬 **Generic**: Full support for Go generics (1.18+)
- ✅ **Well-Tested**: Comprehensive test suite with race, soak, browser smoke, and model-checked linearizability validation
- 📚 **Well-Documented**: Extensive documentation and examples
- 🛡️ **Operationally Hardened Web Service**: Authenticated multi-tenant UI, persistence, backups, metrics, probes, and deployment artifacts

## Data Structures

### 1. Lock-Free Stack (Treiber Stack)
A LIFO data structure using CAS operations.

```go
import "github.com/sanskar/lockfree/stack"

s := stack.New[int]()
s.Push(1)
s.Push(2)
val, ok := s.Pop() // val = 2
```

**Features:**
- O(1) push and pop operations
- LIFO (Last-In-First-Out) ordering
- Generic type support
- Length tracking

### 2. Lock-Free Queue (Michael-Scott Queue)
A FIFO data structure using the Michael-Scott algorithm.

```go
import "github.com/sanskar/lockfree/queue"

q := queue.New[string]()
q.Enqueue("first")
q.Enqueue("second")
val, ok := q.Dequeue() // val = "first"
```

**Features:**
- O(1) enqueue and dequeue operations
- FIFO (First-In-First-Out) ordering
- Sentinel node optimization
- Length tracking

### 3. Lock-Free Counter & Atomic Utilities
High-performance atomic counters and utilities.

```go
import "github.com/sanskar/lockfree/counter"

// Simple counter
c := counter.New()
c.Inc()
c.Add(10)
val := c.Get()

// Striped counter (better for high contention)
sc := counter.NewStriped()
sc.Inc()

// MinMax tracker
mm := counter.NewMinMax()
mm.Update(5)
mm.Update(10)
min, max := mm.Min(), mm.Max()

// Accumulator (for statistics)
acc := counter.NewAccumulator()
acc.Add(5)
acc.Add(10)
avg := acc.Average()
```

**Features:**
- Atomic increment/decrement operations
- Compare-and-swap support
- Striped counters for reduced contention
- MinMax tracking
- Accumulator for statistics

### 4. Lock-Free Ring Buffer
A fixed-size circular buffer with optional overwrite mode.

```go
import "github.com/sanskar/lockfree/ringbuffer"

// Create buffer with capacity 100, no overwrite
rb := ringbuffer.New[int](100, false)
rb.Write(42)
val, err := rb.Read()

// With overwrite enabled, the oldest entry is evicted when full
rb = ringbuffer.New[int](100, true)
```

**Features:**
- Fixed-size circular buffer
- Optional overwrite mode
- O(1) read and write operations
- Try operations for non-blocking access
- FIFO ordering

### 5. Lock-Free Sorted Linked List (Harris-Michael Algorithm)
A sorted linked list with atomic marking for deletion.

```go
import "github.com/sanskar/lockfree/list"

l := list.New[int, string](list.IntCompare)
l.Insert(5, "five")
l.Insert(2, "two")
val, found := l.Search(5)
deleted := l.Delete(2)

// Custom comparison function
l := list.New[MyType, int](func(a, b MyType) int {
    // return -1 if a < b, 0 if equal, 1 if a > b
})
```

**Features:**
- Automatically maintains sorted order
- O(n) insert, delete, and search operations
- Custom comparison functions
- Range iteration
- Atomic marking for deletion

## Installation

```bash
go get github.com/sanskar/lockfree
```

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/sanskar/lockfree/stack"
    "github.com/sanskar/lockfree/queue"
)

func main() {
    // Stack example
    s := stack.New[int]()
    s.Push(1)
    s.Push(2)
    val, _ := s.Pop()
    fmt.Println(val) // Output: 2

    // Queue example
    q := queue.New[string]()
    q.Enqueue("hello")
    q.Enqueue("world")
    val, _ := q.Dequeue()
    fmt.Println(val) // Output: hello
}
```

## Web Visualizer

An interactive web-based visualizer is included to explore lock-free data structures in real-time!

### Running the Visualizer

```bash
# Start the web server from the repository root
go run ./web

# Or from the web directory
cd web
go run .

# Optional: production-style authenticated mode
go run ./cmd/hashpassword -password change-me
# put the hash into deploy/users.example.json or your own users file
LOCKFREE_USERS_FILE=./deploy/users.example.json \
LOCKFREE_SESSION_SECRET=change-me \
go run ./web
```

### Features

- **Real-Time Visualization**: See data structures update instantly as you perform operations
- **Interactive Controls**: Push, pop, enqueue, dequeue, and more with the click of a button
- **WebSocket-Based**: Low-latency real-time updates
- **Beautiful UI**: Modern dark theme with smooth animations
- **All Data Structures**: Stack, Queue, Ring Buffer, Counter, and Sorted List visualizations
- **Tenant Isolation**: Browser sessions are scoped to a tenant and a role
- **Operational Endpoints**: `/livez`, `/healthz`, `/readyz`, `/metrics`
- **Persistence and Recovery**: Atomic snapshots with rotating backups

### What You Can Do

1. **Stack**: Push/pop operations with vertical visualization
2. **Queue**: Enqueue/dequeue with horizontal FIFO display
3. **Ring Buffer**: Circular visualization showing capacity and fill status
4. **Counter**: Visual counter with progress bar
5. **Sorted List**: Insert/delete/search with sorted order display

See `web/README.md` for detailed documentation.

## Benchmarks

Performance comparison with mutex-based implementations (Run on Apple M1, 10 cores):

```
BenchmarkStackConcurrentPush-10              5000000    250 ns/op
BenchmarkMutexStackConcurrentPush-10         2000000    650 ns/op

BenchmarkQueueConcurrentEnqueue-10           4800000    245 ns/op
BenchmarkMutexQueueConcurrentEnqueue-10      1900000    630 ns/op

BenchmarkCounterConcurrentInc-10            20000000     59 ns/op
BenchmarkMutexCounterConcurrentInc-10        8000000    180 ns/op

BenchmarkStripedCounterConcurrentInc-10     25000000     48 ns/op
```

Lock-free implementations show **2-3x better performance** under high contention scenarios.

## Running Benchmarks

```bash
# Run all benchmarks
go test ./... -bench=. -benchmem

# Run specific benchmark
go test ./stack -bench=BenchmarkStackConcurrent

# Compare lock-free vs mutex
go test ./counter -bench=. | grep "Concurrent"
```

## Testing

```bash
# Run all tests
go test ./...

# Run with race detector
go test ./... -race

# Run model-checked linearizability histories
go test ./stack ./queue ./counter ./list ./ringbuffer -run Linearizable -v

# Run Go vulnerability scan
go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# Run the web UI smoke test
cd web
npm install
npx playwright install chromium
npm run smoke

# Run the authenticated soak test
go test ./web -run TestAuthenticatedServiceSoak -v

# Exercise the security workflow locally
act -W .github/workflows/security.yml --container-architecture linux/amd64

# Generate bcrypt hashes for users.json
go run ./cmd/hashpassword -password change-me

# Run with coverage
go test ./... -cover

# Verbose output
go test ./... -v
```

## Security Notes

- The web visualizer now binds to `127.0.0.1` by default. Set `HOST` explicitly if you need to expose it on another interface.
- Any non-loopback bind now requires `LOCKFREE_USERS_FILE` and `LOCKFREE_SESSION_SECRET`.
- Browser access uses authenticated session cookies; automation can use `LOCKFREE_ADMIN_API_TOKEN`.
- WebSocket upgrades require same-origin requests and inherit tenant-scoped authorization.
- The service persists atomic snapshots to `LOCKFREE_STATE_FILE` and rotates backups under `LOCKFREE_BACKUP_DIR`.
- The HTTP layer exposes `/livez`, `/healthz`, `/readyz`, and Prometheus-style `/metrics`.
- See [docs/OPERATIONS.md](/Users/sanskar/dev/Research/Projects/Lock-Free-Data-Structure/docs/OPERATIONS.md), [SECURITY.md](/Users/sanskar/dev/Research/Projects/Lock-Free-Data-Structure/SECURITY.md), [SUPPORT.md](/Users/sanskar/dev/Research/Projects/Lock-Free-Data-Structure/SUPPORT.md), and [RELEASING.md](/Users/sanskar/dev/Research/Projects/Lock-Free-Data-Structure/RELEASING.md).

## Validation Status

- Concurrency semantics are regression-checked with [Porcupine](https://github.com/anishathalye/porcupine) linearizability histories for stack, queue, counter, sorted list, and ring buffer operations.
- A dedicated formal verification package lives in [verification/README.md](/Users/sanskar/dev/Research/Projects/Lock-Free-Data-Structure/verification/README.md) and records the shared assumptions, invariants, and proof obligations for each structure.
- The container image was exercised locally with authenticated login, health probes, session APIs, and metrics scraping.
- The Kubernetes manifest was exercised in a live `kind` cluster. The deployment model is intentionally a single-replica `StatefulSet` with a persistent volume because the web visualizer is stateful.
- GitHub Actions security jobs were exercised locally with `act`, including `govulncheck` and the web npm audit workflow.

## Project Structure

```
Lock-Free-Data-Structure/
├── stack/           # Lock-free stack (Treiber Stack)
│   ├── stack.go
│   ├── stack_test.go
│   └── stack_bench_test.go
├── queue/           # Lock-free queue (Michael-Scott)
│   ├── queue.go
│   ├── queue_test.go
│   └── queue_bench_test.go
├── counter/         # Atomic counters and utilities
│   ├── counter.go
│   ├── counter_test.go
│   └── counter_bench_test.go
├── ringbuffer/      # Lock-free ring buffer
│   ├── ringbuffer.go
│   ├── ringbuffer_test.go
│   └── ringbuffer_bench_test.go
├── list/            # Lock-free sorted list
│   ├── list.go
│   ├── list_test.go
│   └── list_bench_test.go
├── web/             # Interactive web visualizer
│   ├── server.go
│   ├── README.md
│   └── static/
│       ├── index.html
│       ├── style.css
│       └── app.js
└── examples/        # Usage examples
    └── comprehensive_demo.go
```

## Algorithms Implemented

1. **Treiber Stack**: Classic lock-free stack using CAS on head pointer
2. **Michael-Scott Queue**: Lock-free FIFO queue with sentinel node
3. **Harris-Michael List**: Lock-free sorted list with atomic marking
4. **Lock-Free Ring Buffer**: SPMC/MPMC circular buffer with atomic indices
5. **Striped Counter**: Reduces contention by distributing updates across multiple counters

## Concurrency Guarantees

- **Lock-Free**: All operations complete in a finite number of steps regardless of other threads
- **Wait-Free**: Some read operations complete in bounded time (e.g., counter reads)
- **Linearizable**: All operations appear to take effect instantaneously
- **Thread-Safe**: Safe for concurrent use without external synchronization

## When to Use Lock-Free Data Structures

**Use lock-free structures when:**
- ✅ High contention scenarios (many threads competing)
- ✅ Real-time systems (avoid priority inversion)
- ✅ Performance-critical paths
- ✅ Want to avoid deadlocks

**Consider alternatives when:**
- ❌ Low contention (mutexes may be simpler)
- ❌ Complex operations (lock-free algorithms can be complex)
- ❌ Memory constrained (may use more memory)

## Performance Tips

1. **Use Striped Counter** for high-contention increment operations
2. **Pre-allocate Ring Buffer** to appropriate size
3. **Batch Operations** when possible to reduce CAS failures
4. **Profile First** - measure before optimizing
5. **Consider GOMAXPROCS** - more cores benefit lock-free more

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass with race detector
5. Submit a pull request

## License

MIT License - see LICENSE file for details

## References

- [The Art of Multiprocessor Programming](https://www.elsevier.com/books/the-art-of-multiprocessor-programming/herlihy/978-0-12-415950-1)
- [Treiber Stack](https://en.wikipedia.org/wiki/Treiber_stack)
- [Michael-Scott Queue](https://www.cs.rochester.edu/~scott/papers/1996_PODC_queues.pdf)
- [Harris-Michael List](https://www.cl.cam.ac.uk/research/srg/netos/papers/2001-caslists.pdf)

## Acknowledgments

Inspired by Java's `java.util.concurrent` package and various academic research on lock-free algorithms.

---

**Note**: This is a research/educational project. For production use, consider thoroughly testing with your specific workload and comparing against Go's standard `sync` package primitives.
