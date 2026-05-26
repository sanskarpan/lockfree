# Formal Verification Package

This package records the proof obligations and abstraction boundaries for the lock-free algorithms in this repository.

It is intentionally complementary to the executable checks already in the tree:
- race tests
- model-checked linearizability histories
- soak tests
- benchmark regressions

The goal is to make the reasoning behind the algorithms explicit enough that a future maintainer can audit or extend the implementation without rediscovering the same invariants.

## Scope

The package covers:
- Treiber stack
- Michael-Scott queue
- striped atomic counter
- Harris-Michael sorted list
- sequence-based ring buffer

The package does not attempt to prove the Go runtime, the garbage collector, or the underlying atomic primitives. It assumes those platform guarantees hold.

## Shared Assumptions

All proofs in this package rely on the following assumptions:
- Atomic operations are linearizable with respect to the Go memory model.
- Pointers are only reclaimed by the garbage collector after they are no longer reachable.
- The caller respects the exported API contracts, including element comparators and ring-buffer capacity.
- Single-node logical states are stable across retries; retry loops may repeat but do not invent new abstract state.

## Common Proof Pattern

Each data structure is documented in the same way:
- abstract state
- representation invariants
- linearization points
- progress argument
- observable safety properties

That structure is deliberate. It makes the package easy to expand if a new lock-free type is added later.

## Stack

Abstract state:
- a LIFO sequence of values

Representation invariants:
- the head pointer either references the current top node or `nil`
- every reachable node is part of one acyclic next-chain
- removing a node never exposes it as part of a different logical stack

Linearization points:
- `Push` linearizes at the successful CAS that installs the new head
- `Pop` linearizes at the successful CAS that removes the observed head

Progress argument:
- operations retry only on interference
- at least one contending thread makes progress when CAS succeeds

Safety obligations:
- no duplicate pop of the same logical element
- no lost push after a successful head install

## Queue

Abstract state:
- a FIFO sequence of values with a permanent sentinel node

Representation invariants:
- head and tail always refer to reachable nodes in the same chain
- the sentinel node is never removed from the abstract structure
- `tail` may lag but never points outside the chain

Linearization points:
- `Enqueue` linearizes at the successful CAS that links the new node
- `Dequeue` linearizes at the successful CAS that advances the head

Progress argument:
- if one thread is stalled, another thread can still advance head or tail
- helping behavior bounds dependence on any one writer

Safety obligations:
- dequeue order matches enqueue order
- no phantom values appear when the queue is empty

## Counter

Abstract state:
- a signed integer value plus derived min/max/average summaries

Representation invariants:
- the total count matches the sequence of successful increments and decrements
- striped counters sum to the observable total
- min/max summaries are monotonic with respect to observed inputs

Linearization points:
- `Inc`, `Dec`, and `Add` linearize at the successful atomic update of the chosen stripe
- read-only queries linearize at the load that observes the snapshot

Progress argument:
- updates are bounded by finite CAS retries
- reads are wait-free under the current implementation

Safety obligations:
- no stripe can invent or drop an increment
- min and max cover the full `int64` domain

## Sorted List

Abstract state:
- a strictly sorted set of keys

Representation invariants:
- the list is sorted by the comparator
- logically deleted nodes are eventually unlinked
- marked nodes remain unreachable from future abstract traversals

Linearization points:
- `Insert` linearizes at the successful link CAS
- `Delete` linearizes at the successful mark CAS
- `Search` linearizes at the point it observes the search result

Progress argument:
- traversal only advances forward
- contention may cause retries, but no operation depends on a global lock

Safety obligations:
- ordering is preserved for the full comparator domain
- deletion does not disturb unrelated keys

## Ring Buffer

Abstract state:
- a bounded FIFO buffer with optional overwrite semantics

Representation invariants:
- each slot is associated with a monotonic sequence state
- readers only consume a slot when the sequence matches the expected generation
- overwrite mode evicts the oldest live element, not a random element

Linearization points:
- `Write` linearizes when the slot sequence becomes visible to readers
- `Read` linearizes when a reader successfully claims the expected sequence

Progress argument:
- producers and consumers advance independently
- overwrite mode avoids unbounded producer stall under full-buffer pressure

Safety obligations:
- non-overwrite mode returns a full-buffer error without mutating the abstract contents
- overwrite mode retains the most recent `capacity` writes

## How to Extend the Package

When adding a new concurrent structure, document it using the same template:
- abstract state
- invariants
- linearization points
- progress argument
- safety obligations

The key rule is to describe the structure at the level of observable behavior first, then map the implementation back to that model.
