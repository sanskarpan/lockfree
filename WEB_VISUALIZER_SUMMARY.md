# Web Visualizer Summary

## Purpose

The `web/` package exposes a small HTTP server and WebSocket API for exploring the data structures interactively in a browser.

## Current Behavior

- Start from the repository root with `go run ./web`.
- Static assets are resolved without depending on the caller’s current working directory.
- `/` serves the visualizer UI.
- `/ws` upgrades to WebSocket and sends an initial state message.
- mutating operations are broadcast to all connected clients
- `getState` replies only to the requesting client

## Protocol Notes

Messages use the shape:

```json
{
  "type": "stack|queue|ringbuffer|counter|list|getState",
  "operation": "push|pop|enqueue|dequeue|write|read|...",
  "value": 42
}
```

Responses include:
- `success`
- `data`
- `value` when an operation returns a concrete item
- `error` for invalid requests or failed operations

## Hardening Added During Audit

- single-writer WebSocket connections via per-client send queues
- HTTP server timeouts
- safer origin checks for WebSocket upgrades
- non-destructive server-side state snapshots
- regression tests for root startup, invalid payloads, and multi-client consistency
- a Playwright-based headless smoke test that drives the real UI over HTTP/WebSocket

## Limitations

- The UI is still a demo surface, not an authenticated production application.
- The ring buffer visualization shows occupancy, not per-slot values or head/tail positions.
