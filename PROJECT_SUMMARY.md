# Lock-Free Data Structures - Project Summary

## Overview

This repository contains a Go library of concurrent data structures plus a WebSocket-based visualizer:
- `stack`: Treiber stack
- `queue`: Michael-Scott queue
- `counter`: atomic counter utilities, including striped counters, min/max tracking, and accumulation
- `ringbuffer`: fixed-size MPMC ring buffer with optional overwrite mode
- `list`: sorted linked list with logical deletion
- `web`: interactive visualizer for manual exploration

## Current Status

As of the 2026-05-25 audit:
- `go test ./...` passes
- `go test -race ./...` passes
- `go vet ./...` passes
- `govulncheck ./...` passes
- `npm run smoke` in `web/` passes with a headless Chromium flow
- the web visualizer works when started from the repository root with `go run ./web`
- automated HTTP/WebSocket regression coverage exists for the web layer

## Notable Design Decisions

- The core data structures rely on Go atomics and CAS loops.
- Snapshot-style helpers such as `ToSlice()` are best-effort reads, not linearizable snapshots under arbitrary concurrent mutation.
- The web visualizer serializes its own operations with a server mutex so UI state remains consistent even though the underlying structures are independently concurrent.
- The ring buffer rounds requested capacity up to the next power of two.

## Audit Fixes Applied

- Fixed the web server’s cwd-dependent static asset path resolution.
- Replaced destructive WebSocket state snapshots with non-destructive reads.
- Introduced per-client WebSocket send queues and a single writer per connection.
- Implemented actual overwrite semantics in the ring buffer.
- Fixed `MinMax` sentinel handling for `math.MinInt64`.
- Replaced overflow-prone integer comparison in `list.IntCompare`.
- Hardened the web protocol with structured errors and safer numeric parsing.
- Moved the project to a patched Go toolchain and tightened the default network exposure/security headers for the web server.
- Added focused regression tests for root HTTP startup, multi-client WebSocket consistency, invalid payloads, overwrite behavior, and numeric edge cases.
- Added a reproducible Playwright-based UI smoke test for the web visualizer.

## Remaining Risks

- The lock-free structures rely on Go’s managed memory model rather than custom hazard-pointer or epoch-based reclamation.
- Performance characteristics are workload-dependent; benchmark claims should be treated as environment-specific, not universal.
