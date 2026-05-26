package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sanskar/lockfree/counter"
	"github.com/sanskar/lockfree/list"
	"github.com/sanskar/lockfree/queue"
	"github.com/sanskar/lockfree/ringbuffer"
	"github.com/sanskar/lockfree/stack"
	"golang.org/x/time/rate"
)

const (
	defaultRingBufCap       = 16
	clientQueueSize         = 32
	readLimit               = 1 << 20
	writeWait               = 10 * time.Second
	pongWait                = 60 * time.Second
	pingPeriod              = (pongWait * 9) / 10
	readHeaderTimeout       = 5 * time.Second
	httpReadTimeout         = 10 * time.Second
	httpWriteTimeout        = 15 * time.Second
	httpIdleTimeout         = 60 * time.Second
	contentSecurityPolicy   = "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'; object-src 'none'; connect-src 'self'; img-src 'self' data:; script-src 'self'; style-src 'self'"
	permissionsPolicyHeader = "accelerometer=(), camera=(), geolocation=(), gyroscope=(), microphone=(), payment=(), usb=()"
)

type Message struct {
	Type      string      `json:"type"`
	Operation string      `json:"operation"`
	Value     interface{} `json:"value,omitempty"`
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type TenantState struct {
	id                  string
	mu                  sync.Mutex
	stack               *stack.Stack[int]
	queue               *queue.Queue[int]
	ringbuffer          *ringbuffer.RingBuffer[int]
	counter             *counter.Counter
	sortedList          *list.List[int, string]
	clients             map[*client]struct{}
	clientsLock         sync.RWMutex
	ringbufferCapacity  int
	ringbufferOverwrite bool
}

type client struct {
	conn      *websocket.Conn
	send      chan Message
	done      chan struct{}
	closeOnce sync.Once
	tenant    *TenantState
	principal Principal
	limiter   *rate.Limiter
}

func (c *client) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

type App struct {
	cfg             Config
	log             *slog.Logger
	metrics         *Metrics
	users           *UserDirectory
	sessions        *SessionManager
	persistence     *PersistenceManager
	httpLimiter     *rateLimiterStore
	loginLimiter    *rateLimiterStore
	staticDir       string
	tenantsMu       sync.RWMutex
	tenants         map[string]*TenantState
	ready           atomic.Bool
	recoveredFrom   string
	adminAPIToken   string
	legacyAdminAuth string
}

func NewApp(cfg Config) (*App, error) {
	logger := newLogger(cfg)
	users, err := loadUserDirectory(cfg.UsersFile)
	if err != nil {
		return nil, err
	}

	var sessions *SessionManager
	if cfg.AuthEnabled() {
		sessions, err = NewSessionManager(cfg.SessionSecret, cfg.PreviousSessionSecrets, cfg.SessionTTL)
		if err != nil {
			return nil, err
		}
	}

	app := &App{
		cfg:             cfg,
		log:             logger,
		metrics:         &Metrics{},
		users:           users,
		sessions:        sessions,
		httpLimiter:     newRateLimiterStore(rate.Limit(cfg.HTTPRequestRate), cfg.HTTPBurst, 10*time.Minute),
		loginLimiter:    newRateLimiterStore(rate.Limit(cfg.LoginRequestRate), cfg.LoginBurst, 15*time.Minute),
		tenants:         make(map[string]*TenantState),
		adminAPIToken:   cfg.AdminAPIToken,
		legacyAdminAuth: legacyAdminToken(),
	}

	app.persistence = NewPersistenceManager(cfg, logger, app.snapshotState)
	app.persistence.observe = func(result string, at time.Time) {
		app.metrics.ObservePersistence(result)
		if result == "save_success" {
			app.metrics.SetLastSnapshot(at)
		}
	}
	state, recoveredFrom, err := app.persistence.Load()
	if err != nil {
		return nil, err
	}
	app.recoveredFrom = recoveredFrom
	if !state.GeneratedAt.IsZero() {
		app.metrics.SetLastSnapshot(state.GeneratedAt)
	}
	app.restoreState(state)
	app.ready.Store(true)
	return app, nil
}

func newTenantState(id string) *TenantState {
	return &TenantState{
		id:                  id,
		stack:               stack.New[int](),
		queue:               queue.New[int](),
		ringbuffer:          ringbuffer.New[int](defaultRingBufCap, false),
		counter:             counter.New(),
		sortedList:          list.New[int, string](list.IntCompare),
		clients:             make(map[*client]struct{}),
		ringbufferCapacity:  defaultRingBufCap,
		ringbufferOverwrite: false,
	}
}

func (a *App) restoreState(state persistedState) {
	if len(state.Tenants) == 0 {
		a.tenants["default"] = newTenantState("default")
		return
	}
	for tenantID, snapshot := range state.Tenants {
		tenant := newTenantState(tenantID)
		restoreTenantSnapshot(tenant, snapshot)
		a.tenants[tenantID] = tenant
	}
}

func restoreTenantSnapshot(tenant *TenantState, snapshot tenantSnapshot) {
	for i := len(snapshot.Stack) - 1; i >= 0; i-- {
		tenant.stack.Push(snapshot.Stack[i])
	}
	for _, value := range snapshot.Queue {
		tenant.queue.Enqueue(value)
	}
	if snapshot.RingBuffer.Capacity > 0 {
		tenant.ringbufferCapacity = snapshot.RingBuffer.Capacity
		tenant.ringbufferOverwrite = snapshot.RingBuffer.Overwrite
		tenant.ringbuffer = ringbuffer.New[int](snapshot.RingBuffer.Capacity, snapshot.RingBuffer.Overwrite)
		for _, value := range snapshot.RingBuffer.Items {
			_ = tenant.ringbuffer.Write(value)
		}
	}
	if snapshot.Counter != 0 {
		tenant.counter.Add(snapshot.Counter)
	}
	for _, key := range snapshot.List {
		tenant.sortedList.Insert(key, fmt.Sprintf("value_%d", key))
	}
}

func (a *App) snapshotState() persistedState {
	a.tenantsMu.RLock()
	defer a.tenantsMu.RUnlock()

	result := persistedState{
		Version: persistedSchemaVersion,
		Tenants: make(map[string]tenantSnapshot, len(a.tenants)),
	}
	for tenantID, tenant := range a.tenants {
		result.Tenants[tenantID] = tenant.snapshot()
	}
	return result
}

func (t *TenantState) snapshot() tenantSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	return tenantSnapshot{
		Stack: cloneInts(t.stack.ToSlice()),
		Queue: cloneInts(t.queue.ToSlice()),
		RingBuffer: ringBufferSnapshot{
			Capacity:  int(t.ringbuffer.Cap()),
			Overwrite: t.ringbufferOverwrite,
		},
		Counter: t.counter.Get(),
		List:    tenantListKeys(t.sortedList.ToSlice()),
	}
}

func cloneInts(values []int) []int {
	if len(values) == 0 {
		return []int{}
	}
	cloned := make([]int, len(values))
	copy(cloned, values)
	return cloned
}

func tenantListKeys(items []struct {
	Key   int
	Value string
}) []int {
	keys := make([]int, len(items))
	for idx, item := range items {
		keys[idx] = item.Key
	}
	return keys
}

func (a *App) tenantForID(id string) *TenantState {
	a.tenantsMu.RLock()
	tenant, ok := a.tenants[id]
	a.tenantsMu.RUnlock()
	if ok {
		return tenant
	}

	a.tenantsMu.Lock()
	defer a.tenantsMu.Unlock()
	if tenant, ok = a.tenants[id]; ok {
		return tenant
	}
	tenant = newTenantState(id)
	a.tenants[id] = tenant
	return tenant
}

func (a *App) routes() (http.Handler, string, error) {
	staticDir, err := resolveStaticDir()
	if err != nil {
		return nil, "", err
	}
	a.staticDir = staticDir

	mux := http.NewServeMux()
	mux.HandleFunc("/livez", a.handleLivez)
	mux.HandleFunc("/healthz", a.handleHealthz)
	mux.HandleFunc("/readyz", a.handleReadyz)
	mux.HandleFunc("/metrics", a.handleMetrics)
	mux.HandleFunc("/api/v1/login", a.handleLogin)
	mux.HandleFunc("/api/v1/logout", a.handleLogout)
	mux.HandleFunc("/api/v1/session", a.handleSession)
	mux.HandleFunc("/api/v1/admin/backup", a.handleBackup)
	mux.HandleFunc("/ws", a.handleWebSocket)
	mux.HandleFunc("/login", a.handleLoginPage)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	mux.HandleFunc("/", a.handleIndex)

	handler := a.recoverMiddleware(a.securityHeaders(a.loggingMiddleware(a.rateLimitMiddleware(mux))))
	return handler, staticDir, nil
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if a.authEnabled() {
		if _, err := a.currentPrincipal(r); err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
	}
	http.ServeFile(w, r, filepath.Join(a.staticDir, "index.html"))
}

func (a *App) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if !a.authEnabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if _, err := a.currentPrincipal(r); err == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	http.ServeFile(w, r, filepath.Join(a.staticDir, "login.html"))
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !a.authEnabled() {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "authentication is disabled"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if !sameOriginHTTP(r, a.cfg.TrustProxyHeaders) {
		a.metrics.ObserveAuth("origin_rejected")
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "invalid origin"})
		return
	}
	ip := remoteIP(r, a.cfg.TrustProxyHeaders)
	if !a.loginLimiter.Allow(ip) {
		a.metrics.ObserveRateLimit("login")
		writeJSON(w, http.StatusTooManyRequests, errorResponse{Error: "rate limit exceeded"})
		return
	}

	var payload loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		a.metrics.ObserveAuth("bad_request")
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid login payload"})
		return
	}
	principal, err := a.users.Authenticate(payload.Username, payload.Password)
	if err != nil {
		a.metrics.ObserveAuth("invalid_credentials")
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid credentials"})
		return
	}

	cookie, err := a.sessions.IssueCookie(r, principal, requestScheme(r, a.cfg.TrustProxyHeaders) == "https")
	if err != nil {
		a.metrics.ObserveAuth("issue_failed")
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create session"})
		return
	}
	http.SetCookie(w, cookie)
	a.metrics.ObserveAuth("success")
	writeJSON(w, http.StatusOK, map[string]any{"principal": principal})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if a.authEnabled() && !sameOriginHTTP(r, a.cfg.TrustProxyHeaders) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "invalid origin"})
		return
	}
	if a.sessions != nil {
		http.SetCookie(w, a.sessions.ClearCookie(requestScheme(r, a.cfg.TrustProxyHeaders) == "https"))
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) handleSession(w http.ResponseWriter, r *http.Request) {
	principal, err := a.currentPrincipal(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "authentication required"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"principal": principal,
		"auth": map[string]any{
			"enabled": a.authEnabled(),
		},
	})
}

func (a *App) handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if !a.isAdminRequest(r) {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "admin authentication required"})
		return
	}
	path, err := a.persistence.BackupNow()
	if err != nil {
		a.metrics.ObservePersistence("backup_error")
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "backup failed"})
		return
	}
	a.metrics.ObservePersistence("backup_success")
	writeJSON(w, http.StatusOK, map[string]any{"path": path})
}

func (a *App) handleLivez(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	status := http.StatusOK
	body := map[string]any{
		"status": "ok",
	}
	if err := a.persistence.Ready(); err != nil {
		status = http.StatusServiceUnavailable
		body["status"] = "degraded"
		body["error"] = err.Error()
	}
	writeJSON(w, status, body)
}

func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if !a.ready.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "starting"})
		return
	}
	if err := a.persistence.Ready(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "degraded", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func (a *App) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if a.authEnabled() && !a.isAdminRequest(r) && !isLoopbackHost(strings.Trim(remoteIP(r, a.cfg.TrustProxyHeaders), "[]")) {
		http.Error(w, "admin authentication required", http.StatusUnauthorized)
		return
	}
	a.tenantsMu.RLock()
	tenantCount := len(a.tenants)
	a.tenantsMu.RUnlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write(a.metrics.renderPrometheus(a.ready.Load(), tenantCount))
}

func (a *App) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	principal, err := a.currentPrincipal(r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(req *http.Request) bool {
			origin := req.Header.Get("Origin")
			if origin == "" {
				return true
			}
			originURL, parseErr := url.Parse(origin)
			if parseErr != nil {
				return false
			}
			scheme := requestScheme(req, a.cfg.TrustProxyHeaders)
			if strings.ToLower(originURL.Scheme) != scheme {
				return false
			}
			return canonicalHostPort(originURL.Host, originURL.Scheme) == canonicalHostPort(requestHost(req, a.cfg.TrustProxyHeaders), scheme)
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		a.log.Warn("websocket upgrade failed", "error", err)
		return
	}

	tenant := a.tenantForID(principal.Tenant)
	client := &client{
		conn:      conn,
		send:      make(chan Message, clientQueueSize),
		done:      make(chan struct{}),
		tenant:    tenant,
		principal: principal,
		limiter:   rate.NewLimiter(rate.Limit(a.cfg.WebSocketMessageRate), a.cfg.WebSocketBurst),
	}

	conn.SetReadLimit(readLimit)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	tenant.registerClient(client)
	a.metrics.activeWS.Add(1)
	go a.writePump(client)
	tenant.enqueue(client, tenant.stateMessage(principal))

	defer func() {
		tenant.unregisterClient(client)
		a.metrics.activeWS.Add(-1)
	}()

	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) {
				a.log.Warn("websocket read error", "tenant", principal.Tenant, "user", principal.Username, "error", err)
			}
			return
		}
		if !client.limiter.Allow() {
			a.metrics.ObserveRateLimit("ws_messages")
			tenant.enqueue(client, Message{Type: "error", Operation: msg.Operation, Success: false, Error: "rate limit exceeded"})
			continue
		}

		response, shouldBroadcast, mutated := tenant.processMessage(principal, msg)
		if msg.Type == "getState" {
			a.metrics.ObserveWSMessage(msg.Type, msg.Operation, "success")
		} else if response.Success {
			a.metrics.ObserveWSMessage(msg.Type, msg.Operation, "success")
		} else {
			a.metrics.ObserveWSMessage(msg.Type, msg.Operation, "error")
		}
		if mutated {
			a.persistence.ScheduleSave()
		}
		if shouldBroadcast {
			tenant.broadcast(response)
			continue
		}
		tenant.enqueue(client, response)
	}
}

func (a *App) writePump(client *client) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-client.done:
			return
		case msg := <-client.send:
			_ = client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.conn.WriteJSON(msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (t *TenantState) registerClient(client *client) {
	t.clientsLock.Lock()
	t.clients[client] = struct{}{}
	t.clientsLock.Unlock()
}

func (t *TenantState) unregisterClient(client *client) {
	t.clientsLock.Lock()
	delete(t.clients, client)
	t.clientsLock.Unlock()
	client.close()
}

func (t *TenantState) enqueue(client *client, msg Message) {
	select {
	case <-client.done:
		return
	default:
	}
	select {
	case client.send <- msg:
	default:
		t.unregisterClient(client)
	}
}

func (t *TenantState) broadcast(msg Message) {
	t.clientsLock.RLock()
	clients := make([]*client, 0, len(t.clients))
	for client := range t.clients {
		clients = append(clients, client)
	}
	t.clientsLock.RUnlock()

	for _, client := range clients {
		t.enqueue(client, msg)
	}
}

func (t *TenantState) processMessage(principal Principal, msg Message) (Message, bool, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch msg.Type {
	case "stack":
		return t.handleStackOperation(principal, msg)
	case "queue":
		return t.handleQueueOperation(principal, msg)
	case "ringbuffer":
		return t.handleRingBufferOperation(principal, msg)
	case "counter":
		return t.handleCounterOperation(principal, msg)
	case "list":
		return t.handleListOperation(principal, msg)
	case "getState":
		return t.stateMessageLocked(principal), false, false
	default:
		return Message{Type: "error", Operation: msg.Operation, Success: false, Error: fmt.Sprintf("unknown message type %q", msg.Type)}, false, false
	}
}

func forbid(msgType, op string, data any) (Message, bool, bool) {
	return Message{Type: msgType, Operation: op, Success: false, Error: "forbidden", Data: data}, false, false
}

func (t *TenantState) handleStackOperation(principal Principal, msg Message) (Message, bool, bool) {
	response := Message{Type: "stack", Operation: msg.Operation}
	switch msg.Operation {
	case "push":
		if !principal.CanWrite() {
			return forbid("stack", msg.Operation, t.getStackStateLocked())
		}
		val, err := parseIntValue(msg.Value)
		if err != nil {
			response.Success = false
			response.Error = err.Error()
			response.Data = t.getStackStateLocked()
			return response, false, false
		}
		t.stack.Push(val)
		response.Success = true
		response.Data = t.getStackStateLocked()
		return response, true, true
	case "pop":
		if !principal.CanWrite() {
			return forbid("stack", msg.Operation, t.getStackStateLocked())
		}
		val, ok := t.stack.Pop()
		response.Success = ok
		if ok {
			response.Value = val
		} else {
			response.Error = "stack is empty"
		}
		response.Data = t.getStackStateLocked()
		return response, true, true
	case "clear":
		if !principal.CanWrite() {
			return forbid("stack", msg.Operation, t.getStackStateLocked())
		}
		t.stack = stack.New[int]()
		response.Success = true
		response.Data = t.getStackStateLocked()
		return response, true, true
	default:
		response.Success = false
		response.Error = fmt.Sprintf("unknown stack operation %q", msg.Operation)
		response.Data = t.getStackStateLocked()
		return response, false, false
	}
}

func (t *TenantState) handleQueueOperation(principal Principal, msg Message) (Message, bool, bool) {
	response := Message{Type: "queue", Operation: msg.Operation}
	switch msg.Operation {
	case "enqueue":
		if !principal.CanWrite() {
			return forbid("queue", msg.Operation, t.getQueueStateLocked())
		}
		val, err := parseIntValue(msg.Value)
		if err != nil {
			response.Success = false
			response.Error = err.Error()
			response.Data = t.getQueueStateLocked()
			return response, false, false
		}
		t.queue.Enqueue(val)
		response.Success = true
		response.Data = t.getQueueStateLocked()
		return response, true, true
	case "dequeue":
		if !principal.CanWrite() {
			return forbid("queue", msg.Operation, t.getQueueStateLocked())
		}
		val, ok := t.queue.Dequeue()
		response.Success = ok
		if ok {
			response.Value = val
		} else {
			response.Error = "queue is empty"
		}
		response.Data = t.getQueueStateLocked()
		return response, true, true
	case "clear":
		if !principal.CanWrite() {
			return forbid("queue", msg.Operation, t.getQueueStateLocked())
		}
		t.queue = queue.New[int]()
		response.Success = true
		response.Data = t.getQueueStateLocked()
		return response, true, true
	default:
		response.Success = false
		response.Error = fmt.Sprintf("unknown queue operation %q", msg.Operation)
		response.Data = t.getQueueStateLocked()
		return response, false, false
	}
}

func (t *TenantState) handleRingBufferOperation(principal Principal, msg Message) (Message, bool, bool) {
	response := Message{Type: "ringbuffer", Operation: msg.Operation}
	switch msg.Operation {
	case "write":
		if !principal.CanWrite() {
			return forbid("ringbuffer", msg.Operation, t.getRingBufferStateLocked())
		}
		val, err := parseIntValue(msg.Value)
		if err != nil {
			response.Success = false
			response.Error = err.Error()
			response.Data = t.getRingBufferStateLocked()
			return response, false, false
		}
		err = t.ringbuffer.Write(val)
		response.Success = err == nil
		if err != nil {
			response.Error = err.Error()
		}
		response.Data = t.getRingBufferStateLocked()
		return response, true, err == nil
	case "read":
		if !principal.CanWrite() {
			return forbid("ringbuffer", msg.Operation, t.getRingBufferStateLocked())
		}
		val, err := t.ringbuffer.Read()
		response.Success = err == nil
		if err == nil {
			response.Value = val
		} else {
			response.Error = err.Error()
		}
		response.Data = t.getRingBufferStateLocked()
		return response, true, err == nil
	case "clear":
		if !principal.CanWrite() {
			return forbid("ringbuffer", msg.Operation, t.getRingBufferStateLocked())
		}
		t.ringbuffer = ringbuffer.New[int](t.ringbufferCapacity, t.ringbufferOverwrite)
		response.Success = true
		response.Data = t.getRingBufferStateLocked()
		return response, true, true
	default:
		response.Success = false
		response.Error = fmt.Sprintf("unknown ringbuffer operation %q", msg.Operation)
		response.Data = t.getRingBufferStateLocked()
		return response, false, false
	}
}

func (t *TenantState) handleCounterOperation(principal Principal, msg Message) (Message, bool, bool) {
	response := Message{Type: "counter", Operation: msg.Operation}
	if !principal.CanWrite() {
		return forbid("counter", msg.Operation, t.getCounterStateLocked())
	}
	switch msg.Operation {
	case "inc":
		t.counter.Inc()
	case "dec":
		t.counter.Dec()
	case "add":
		val, err := parseIntValue(msg.Value)
		if err != nil {
			response.Success = false
			response.Error = err.Error()
			response.Data = t.getCounterStateLocked()
			return response, false, false
		}
		t.counter.Add(int64(val))
	case "reset":
		t.counter = counter.New()
	default:
		response.Success = false
		response.Error = fmt.Sprintf("unknown counter operation %q", msg.Operation)
		response.Data = t.getCounterStateLocked()
		return response, false, false
	}
	response.Success = true
	response.Data = t.getCounterStateLocked()
	return response, true, true
}

func (t *TenantState) handleListOperation(principal Principal, msg Message) (Message, bool, bool) {
	response := Message{Type: "list", Operation: msg.Operation}
	switch msg.Operation {
	case "search":
		key, err := parseIntValue(msg.Value)
		if err != nil {
			response.Success = false
			response.Error = err.Error()
			response.Data = t.getListStateLocked()
			return response, false, false
		}
		value, found := t.sortedList.Search(key)
		response.Success = found
		if found {
			response.Value = value
		} else {
			response.Error = "key not found"
		}
		response.Data = t.getListStateLocked()
		return response, false, false
	case "insert":
		if !principal.CanWrite() {
			return forbid("list", msg.Operation, t.getListStateLocked())
		}
		key, err := parseIntValue(msg.Value)
		if err != nil {
			response.Success = false
			response.Error = err.Error()
			response.Data = t.getListStateLocked()
			return response, false, false
		}
		inserted := t.sortedList.Insert(key, fmt.Sprintf("value_%d", key))
		response.Success = inserted
		if !inserted {
			response.Error = "key already exists"
		}
		response.Data = t.getListStateLocked()
		return response, true, inserted
	case "delete":
		if !principal.CanWrite() {
			return forbid("list", msg.Operation, t.getListStateLocked())
		}
		key, err := parseIntValue(msg.Value)
		if err != nil {
			response.Success = false
			response.Error = err.Error()
			response.Data = t.getListStateLocked()
			return response, false, false
		}
		deleted := t.sortedList.Delete(key)
		response.Success = deleted
		if !deleted {
			response.Error = "key not found"
		}
		response.Data = t.getListStateLocked()
		return response, true, deleted
	case "clear":
		if !principal.CanWrite() {
			return forbid("list", msg.Operation, t.getListStateLocked())
		}
		t.sortedList = list.New[int, string](list.IntCompare)
		response.Success = true
		response.Data = t.getListStateLocked()
		return response, true, true
	default:
		response.Success = false
		response.Error = fmt.Sprintf("unknown list operation %q", msg.Operation)
		response.Data = t.getListStateLocked()
		return response, false, false
	}
}

func (t *TenantState) stateMessage(principal Principal) Message {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stateMessageLocked(principal)
}

func (t *TenantState) stateMessageLocked(principal Principal) Message {
	return Message{
		Type:      "state",
		Operation: "getState",
		Success:   true,
		Data: map[string]any{
			"session": map[string]any{
				"username": principal.Username,
				"tenant":   principal.Tenant,
				"role":     principal.Role,
			},
			"stack":      t.getStackStateLocked(),
			"queue":      t.getQueueStateLocked(),
			"ringbuffer": t.getRingBufferStateLocked(),
			"counter":    t.getCounterStateLocked(),
			"list":       t.getListStateLocked(),
		},
	}
}

func (t *TenantState) getStackStateLocked() map[string]any {
	items := t.stack.ToSlice()
	if items == nil {
		items = []int{}
	}
	return map[string]any{"items": items, "length": len(items), "empty": len(items) == 0}
}

func (t *TenantState) getQueueStateLocked() map[string]any {
	items := t.queue.ToSlice()
	if items == nil {
		items = []int{}
	}
	return map[string]any{"items": items, "length": len(items), "empty": len(items) == 0}
}

func (t *TenantState) getRingBufferStateLocked() map[string]any {
	return map[string]any{
		"length":    t.ringbuffer.Len(),
		"capacity":  t.ringbuffer.Cap(),
		"available": t.ringbuffer.Available(),
		"empty":     t.ringbuffer.IsEmpty(),
		"full":      t.ringbuffer.IsFull(),
	}
}

func (t *TenantState) getCounterStateLocked() map[string]any {
	return map[string]any{"value": t.counter.Get()}
}

func (t *TenantState) getListStateLocked() map[string]any {
	items := t.sortedList.ToSlice()
	keys := tenantListKeys(items)
	return map[string]any{"items": keys, "length": len(keys), "empty": len(keys) == 0}
}

func (a *App) authEnabled() bool {
	return a.users != nil && a.sessions != nil
}

func (a *App) Close() {
	if a.persistence != nil {
		a.persistence.Close()
	}
}

func (a *App) currentPrincipal(r *http.Request) (Principal, error) {
	if !a.authEnabled() {
		return Principal{Username: "anonymous", Tenant: "default", Role: RoleAdmin, AuthMethod: "anonymous"}, nil
	}
	return a.sessions.ParseRequest(r)
}

func (a *App) isAdminRequest(r *http.Request) bool {
	if principal, err := a.currentPrincipal(r); err == nil && principal.IsAdmin() {
		return true
	}
	bearer := bearerToken(r)
	if tokenMatches(bearer, a.adminAPIToken) || tokenMatches(bearer, a.legacyAdminAuth) {
		return true
	}
	return false
}

func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return ""
}

func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("Content-Security-Policy", contentSecurityPolicy)
		headers.Set("Permissions-Policy", permissionsPolicyHeader)
		headers.Set("Referrer-Policy", "no-referrer")
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("X-Frame-Options", "DENY")
		headers.Set("Cross-Origin-Opener-Policy", "same-origin")
		headers.Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (a *App) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := remoteIP(r, a.cfg.TrustProxyHeaders)
		if !a.httpLimiter.Allow(key) {
			a.metrics.ObserveRateLimit("http")
			writeJSON(w, http.StatusTooManyRequests, errorResponse{Error: "rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := randomID(8)
		traceID := extractTraceID(r)
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		ctx = context.WithValue(ctx, traceIDContextKey{}, traceID)
		r = r.WithContext(ctx)

		recorder := &statusRecorder{ResponseWriter: w}
		recorder.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(recorder, r)
		duration := time.Since(start)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		a.metrics.ObserveHTTP(r.Method, normalizePath(r.URL.Path), status, duration)

		principal, _ := a.currentPrincipal(r)
		a.log.Info("request completed",
			"request_id", requestID,
			"trace_id", traceID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration_ms", duration.Milliseconds(),
			"remote_ip", remoteIP(r, a.cfg.TrustProxyHeaders),
			"user", principal.Username,
			"tenant", principal.Tenant,
		)
	})
}

func (a *App) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				a.log.Error("panic recovered", "panic", recovered, "request_id", requestIDFromContext(r.Context()), "trace_id", traceIDFromContext(r.Context()))
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func normalizePath(path string) string {
	switch path {
	case "/", "/login", "/ws", "/metrics", "/healthz", "/readyz", "/livez", "/api/v1/login", "/api/v1/logout", "/api/v1/session", "/api/v1/admin/backup":
		return path
	default:
		if strings.HasPrefix(path, "/static/") {
			return "/static/*"
		}
		return "/other"
	}
}

func resolveStaticDir() (string, error) {
	candidates := []string{
		filepath.Join("web", "static"),
		"static",
	}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append([]string{
			filepath.Join(exeDir, "web", "static"),
			filepath.Join(exeDir, "static"),
		}, candidates...)
	}

	if cwd, err := os.Getwd(); err == nil {
		candidates = append([]string{
			filepath.Join(cwd, "web", "static"),
			filepath.Join(cwd, "static"),
		}, candidates...)
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		absCandidate, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, exists := seen[absCandidate]; exists {
			continue
		}
		seen[absCandidate] = struct{}{}
		info, err := os.Stat(filepath.Join(absCandidate, "index.html"))
		if err == nil && !info.IsDir() {
			return absCandidate, nil
		}
	}
	return "", fmt.Errorf("static assets not found; checked %d locations", len(seen))
}

func canonicalHostPort(hostport, scheme string) string {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		host = strings.Trim(strings.ToLower(hostport), "[]")
		port = defaultPortForScheme(scheme)
	} else {
		host = strings.ToLower(strings.Trim(host, "[]"))
		if port == "" {
			port = defaultPortForScheme(scheme)
		}
	}
	return net.JoinHostPort(host, port)
}

func defaultPortForScheme(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func tokenMatches(actual, expected string) bool {
	if actual == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func parseIntValue(value interface{}) (int, error) {
	switch v := value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, fmt.Errorf("value must be finite")
		}
		if math.Trunc(v) != v {
			return 0, fmt.Errorf("value must be an integer")
		}
		if v < float64(math.MinInt) || v > float64(math.MaxInt) {
			return 0, fmt.Errorf("value is out of range")
		}
		return int(v), nil
	case int:
		return v, nil
	case int64:
		if v < int64(math.MinInt) || v > int64(math.MaxInt) {
			return 0, fmt.Errorf("value is out of range")
		}
		return int(v), nil
	default:
		return 0, fmt.Errorf("invalid value")
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
