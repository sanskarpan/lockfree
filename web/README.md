# Lock-Free Data Structures Web Visualizer

Interactive real-time web visualizer for exploring lock-free data structures in action.

## Overview

This web interface provides a visual, interactive way to explore and understand how lock-free data structures work. Each data structure can be manipulated in real-time with instant visual feedback showing the internal state.

## Features

- **Real-Time Updates**: WebSocket-based communication for instant visualization updates
- **Interactive Controls**: Buttons and inputs to perform operations on each data structure
- **Authenticated Sessions**: Login-backed browser sessions with role-aware access control
- **Tenant Isolation**: Each user sees and mutates only their own tenant state
- **Persistence**: Atomic snapshots and rotating backups for restart recovery
- **Observability**: `/livez`, `/healthz`, `/readyz`, `/metrics`, structured logs, and request IDs
- **Visual Representations**: Unique visualizations for each data structure
- **Performance Metrics**: Track the number of operations performed
- **Responsive Design**: Works on desktop and mobile devices
- **Modern UI**: Dark theme with smooth animations

## Data Structures Visualized

### 1. Lock-Free Stack (Treiber)
- **Operations**: Push, Pop, Clear
- **Visualization**: Vertical stack showing LIFO ordering
- **Algorithm**: Classic Treiber Stack using CAS on head pointer

### 2. Lock-Free Queue (Michael-Scott)
- **Operations**: Enqueue, Dequeue, Clear
- **Visualization**: Horizontal queue showing FIFO ordering
- **Algorithm**: Industry-standard Michael-Scott queue with sentinel node

### 3. Lock-Free Ring Buffer
- **Operations**: Write, Read, Clear
- **Visualization**: Circular buffer with capacity and fill indicators
- **Algorithm**: Sequence-based synchronization (LMAX Disruptor-style)
- **Info Display**: Length, Capacity, Available space

### 4. Lock-Free Counter
- **Operations**: Increment, Decrement, Add, Reset
- **Visualization**: Large number display with progress bar
- **Algorithm**: Atomic operations

### 5. Lock-Free Sorted List (Harris-Michael)
- **Operations**: Insert, Delete, Search, Clear
- **Visualization**: Horizontal list showing sorted order
- **Algorithm**: Harris-Michael list with atomic marking
- **Features**: Search feedback messages

## Quick Start

### Running the Server

```bash
# From the repository root
go run ./web

# Or from the web directory
cd web
go run .

# Authenticated mode
go run ./cmd/hashpassword -password change-me
LOCKFREE_USERS_FILE=./deploy/users.example.json \
LOCKFREE_SESSION_SECRET=change-me \
go run ./web
```

### Accessing the Interface

Open your web browser and navigate to:
```
http://localhost:8081
```

If auth is enabled, you will be redirected to `/login`.

## How to Use

### Stack Operations
1. Enter a number in the input field
2. Click **Push** to add to the stack
3. Click **Pop** to remove from the top
4. Click **Clear** to empty the stack

### Queue Operations
1. Enter a number in the input field
2. Click **Enqueue** to add to the rear
3. Click **Dequeue** to remove from the front
4. Click **Clear** to empty the queue

### Ring Buffer Operations
1. Enter a number in the input field
2. Click **Write** to add to the buffer
3. Click **Read** to remove from the buffer
4. Click **Clear** to empty the buffer
5. Watch the circular visualization update in real-time

### Counter Operations
1. Click **Increment** to add 1
2. Click **Decrement** to subtract 1
3. Enter a number and click **Add** to add custom value
4. Click **Reset** to set back to 0
5. Watch the progress bar and number update

### List Operations
1. Enter a key (number) in the input field
2. Click **Insert** to add the key to the sorted list
3. Click **Delete** to remove the key
4. Click **Search** to find if the key exists
5. Click **Clear** to empty the list
6. Watch for success/error messages

## Architecture

### Backend (Go)

- `server.go`: process bootstrap and graceful shutdown
- `app.go`: HTTP/WebSocket routes, tenant state, authz enforcement, and request handling
- `auth.go`: user directory loading, role model, and signed session cookies
- `config.go`: environment parsing and startup validation
- `persistence.go`: atomic snapshots, backup rotation, migration, and recovery
- `ops.go`: structured logging, rate limiting, request tracing, and Prometheus-style metrics

### Frontend (JavaScript)

**Files**:
- `index.html`: Structure and layout
- `style.css`: Modern dark theme styling
- `app.js`: WebSocket client and visualization logic

**Key Features**:
- Automatic reconnection on disconnect
- Real-time state synchronization
- Smooth animations for operations
- Responsive grid layout

## Technical Details

### Default Binding

For safety, the server listens on `127.0.0.1` by default. Any non-loopback bind requires:

```bash
HOST=0.0.0.0 \
PORT=8081 \
LOCKFREE_USERS_FILE=./deploy/users.example.json \
LOCKFREE_SESSION_SECRET=change-me \
go run ./web
```

### WebSocket Protocol

**Message Format**:
```json
{
  "type": "stack|queue|ringbuffer|counter|list",
  "operation": "push|pop|enqueue|dequeue|...",
  "value": 42,
  "success": true,
  "data": { /* structure state */ },
  "error": "error message if any"
}
```

### State Updates

Each operation triggers a state update that includes:
- Current items/value
- Length/size
- Additional metadata (capacity for ring buffer, etc.)

### Performance

- **WebSocket Latency**: < 1ms for local connections
- **Update Rate**: Real-time (instant)
- **Concurrent Support**: Multiple browser tabs can connect simultaneously
- **Memory Usage**: Minimal - visualizes existing data structures

## Customization

### Changing the Port

Edit `server.go`:
```go
port := ":8081"  // Change to desired port
```

### Styling

Edit `style.css` to customize:
- Colors (CSS variables at the top)
- Animations
- Layout
- Fonts

### Adding New Operations

1. Add operation handler in `server.go`
2. Add button in `index.html`
3. Add JavaScript function in `app.js`
4. Update visualization logic

## Dependencies

- **Go Packages**:
  - `github.com/gorilla/websocket` - WebSocket support
  - `golang.org/x/crypto/bcrypt` - password verification
  - `golang.org/x/time/rate` - rate limiting
  - All lock-free data structure packages from the main project

- **Frontend**:
  - No external dependencies (vanilla JavaScript)
  - SVG for ring buffer visualization

- **UI Smoke Testing**:
  - `playwright` for headless browser verification

## Browser Compatibility

- Chrome/Edge: ✅ Fully supported
- Firefox: ✅ Fully supported
- Safari: ✅ Fully supported
- Mobile browsers: ✅ Responsive design

## Development

### File Structure

```text
web/
├── app.go             # HTTP/WebSocket app and tenant orchestration
├── auth.go            # Authn/authz and session management
├── config.go          # Environment config and validation
├── ops.go             # Metrics, logging, probes, and middleware
├── persistence.go     # Snapshot, backup, restore, migration
├── server.go          # Process bootstrap and graceful shutdown
├── smoke.mjs          # Playwright smoke validation
├── README.md          # This file
└── static/
    ├── index.html     # Main application shell
    ├── login.html     # Login page
    ├── login.js       # Login workflow
    ├── style.css      # Styling
    └── app.js         # Browser client
```

### Running in Development

```bash
# Terminal 1: Run server with auto-reload
go run ./web

# Terminal 2: Make changes to static files
# Browser will auto-reconnect on server restart
```

### Running the UI Smoke Test

```bash
cd web
npm install
npx playwright install chromium
npm run smoke
```

### Running Model-Checked Concurrency Validation

```bash
go test ../stack ../queue ../counter ../list ../ringbuffer -run Linearizable -v
```

## Security Hardening

- Same-origin WebSocket enforcement
- Loopback-only default binding
- Mandatory `LOCKFREE_USERS_FILE` and `LOCKFREE_SESSION_SECRET` for any non-loopback bind
- Tenant-scoped roles: `viewer`, `operator`, `admin`
- Signed session cookies with secret rotation support
- HTTP security headers on all responses
- Request size limits, rate limiting, and timeouts on the Go server
- Atomic persistence snapshots with backup recovery
- Admin backup endpoint and Prometheus metrics
- Container, Kubernetes, and GitHub Actions security workflows have been exercised against the checked-in deployment assets

If you intentionally expose the visualizer outside loopback, set `HOST`, `LOCKFREE_USERS_FILE`, and `LOCKFREE_SESSION_SECRET`:

```bash
HOST=0.0.0.0 \
PORT=8081 \
LOCKFREE_USERS_FILE=./deploy/users.example.json \
LOCKFREE_SESSION_SECRET=change-me \
go run ./web
```

For Kubernetes, use [deploy/kubernetes/lockfree-visualizer.yaml](/Users/sanskar/dev/Research/Projects/Lock-Free-Data-Structure/deploy/kubernetes/lockfree-visualizer.yaml). It is intentionally a single-replica `StatefulSet` with a persistent volume because the visualizer persists tenant state locally.

## Troubleshooting

### Server won't start
- **Error**: "address already in use"
- **Solution**: Change the port in `server.go` or kill the process using port 8081

### WebSocket won't connect
- **Check**: Server is running
- **Check**: Correct URL in browser
- **Check**: No firewall blocking the connection

### Visualizations not updating
- **Check**: Connection status indicator (should be green)
- **Check**: Browser console for errors
- **Solution**: Refresh the page

## Future Enhancements

Potential improvements:
- [ ] Concurrent operation simulator (multiple goroutines)
- [ ] Performance benchmarking in the UI
- [ ] Animation speed controls
- [ ] Export/import data structure state
- [ ] Dark/light theme toggle
- [ ] Operation history/undo
- [ ] Stress test mode
- [ ] Comparison mode (lock-free vs mutex-based)

## License

MIT License - Same as the main project

## Author

Built as part of the Lock-Free Data Structures project

---

**Version**: 1.0.0
**Last Updated**: 2026-05-26
