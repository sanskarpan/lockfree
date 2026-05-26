package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

type requestIDContextKey struct{}
type traceIDContextKey struct{}
type principalContextKey struct{}

func newLogger(cfg Config) *slog.Logger {
	level := new(slog.LevelVar)
	switch cfg.LogLevel {
	case "debug":
		level.Set(slog.LevelDebug)
	case "warn":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}

	handlerOptions := &slog.HandlerOptions{Level: level}
	if cfg.LogFormat == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, handlerOptions))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, handlerOptions))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(data)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func requestScheme(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if value := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); value != "" {
			return strings.ToLower(value)
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func requestHost(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if value := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); value != "" {
			return value
		}
	}
	return r.Host
}

func remoteIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if value := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); value != "" {
			return value
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func sameOriginHTTP(r *http.Request, trustProxy bool) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	expected := canonicalHostPort(requestHost(r, trustProxy), requestScheme(r, trustProxy))
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.ToLower(parsed.Scheme) != requestScheme(r, trustProxy) {
		return false
	}
	return canonicalHostPort(parsed.Host, parsed.Scheme) == expected
}

func extractTraceID(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Traceparent"))
	if header == "" {
		return randomID(16)
	}
	parts := strings.Split(header, "-")
	if len(parts) != 4 || len(parts[1]) != 32 {
		return randomID(16)
	}
	return parts[1]
}

type rateLimiterStore struct {
	mu      sync.Mutex
	limit   rate.Limit
	burst   int
	ttl     time.Duration
	entries map[string]*limiterEntry
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newRateLimiterStore(limit rate.Limit, burst int, ttl time.Duration) *rateLimiterStore {
	return &rateLimiterStore{
		limit:   limit,
		burst:   burst,
		ttl:     ttl,
		entries: make(map[string]*limiterEntry),
	}
}

func (s *rateLimiterStore) Allow(key string) bool {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	for name, entry := range s.entries {
		if now.Sub(entry.lastSeen) > s.ttl {
			delete(s.entries, name)
		}
	}

	entry, ok := s.entries[key]
	if !ok {
		entry = &limiterEntry{limiter: rate.NewLimiter(s.limit, s.burst)}
		s.entries[key] = entry
	}
	entry.lastSeen = now
	return entry.limiter.Allow()
}

type Metrics struct {
	httpRequests        sync.Map
	httpDurations       sync.Map
	authAttempts        sync.Map
	wsMessages          sync.Map
	persistenceResults  sync.Map
	rateLimitRejections sync.Map
	activeWS            atomic.Int64
	lastSnapshotUnix    atomic.Int64
}

func counterFromMap(m *sync.Map, key string) *atomic.Int64 {
	if value, ok := m.Load(key); ok {
		return value.(*atomic.Int64)
	}
	counter := &atomic.Int64{}
	actual, _ := m.LoadOrStore(key, counter)
	return actual.(*atomic.Int64)
}

func (m *Metrics) ObserveHTTP(method, path string, status int, duration time.Duration) {
	counterFromMap(&m.httpRequests, fmt.Sprintf("%s|%s|%d", method, path, status)).Add(1)
	durations := counterFromMap(&m.httpDurations, fmt.Sprintf("%s|%s|sum_ms", method, path))
	durations.Add(duration.Milliseconds())
	counterFromMap(&m.httpDurations, fmt.Sprintf("%s|%s|count", method, path)).Add(1)
}

func (m *Metrics) ObserveAuth(result string) {
	counterFromMap(&m.authAttempts, result).Add(1)
}

func (m *Metrics) ObserveWSMessage(messageType, operation, result string) {
	counterFromMap(&m.wsMessages, fmt.Sprintf("%s|%s|%s", messageType, operation, result)).Add(1)
}

func (m *Metrics) ObservePersistence(result string) {
	counterFromMap(&m.persistenceResults, result).Add(1)
}

func (m *Metrics) ObserveRateLimit(scope string) {
	counterFromMap(&m.rateLimitRejections, scope).Add(1)
}

func (m *Metrics) SetLastSnapshot(ts time.Time) {
	m.lastSnapshotUnix.Store(ts.Unix())
}

func (m *Metrics) renderPrometheus(ready bool, tenantCount int) []byte {
	var buf bytes.Buffer
	buf.WriteString("# HELP lockfree_service_ready Whether the service is ready to serve traffic.\n")
	buf.WriteString("# TYPE lockfree_service_ready gauge\n")
	if ready {
		buf.WriteString("lockfree_service_ready 1\n")
	} else {
		buf.WriteString("lockfree_service_ready 0\n")
	}
	buf.WriteString("# HELP lockfree_active_websocket_connections Active WebSocket connections.\n")
	buf.WriteString("# TYPE lockfree_active_websocket_connections gauge\n")
	buf.WriteString("lockfree_active_websocket_connections " + strconv.FormatInt(m.activeWS.Load(), 10) + "\n")
	buf.WriteString("# HELP lockfree_tenants Total configured tenant states.\n")
	buf.WriteString("# TYPE lockfree_tenants gauge\n")
	buf.WriteString("lockfree_tenants " + strconv.Itoa(tenantCount) + "\n")
	if snapshotUnix := m.lastSnapshotUnix.Load(); snapshotUnix > 0 {
		buf.WriteString("# HELP lockfree_last_snapshot_timestamp_seconds Unix timestamp of the last successful snapshot.\n")
		buf.WriteString("# TYPE lockfree_last_snapshot_timestamp_seconds gauge\n")
		buf.WriteString("lockfree_last_snapshot_timestamp_seconds " + strconv.FormatInt(snapshotUnix, 10) + "\n")
	}

	renderCounterMap(&buf, "lockfree_http_requests_total", "Total HTTP requests.", []string{"method", "path", "status"}, &m.httpRequests)
	renderCounterMap(&buf, "lockfree_http_request_duration_milliseconds", "Aggregate HTTP request duration in milliseconds.", []string{"method", "path", "stat"}, &m.httpDurations)
	renderCounterMap(&buf, "lockfree_auth_attempts_total", "Authentication attempts.", []string{"result"}, &m.authAttempts)
	renderCounterMap(&buf, "lockfree_websocket_messages_total", "WebSocket messages processed.", []string{"type", "operation", "result"}, &m.wsMessages)
	renderCounterMap(&buf, "lockfree_persistence_operations_total", "Persistence operations.", []string{"result"}, &m.persistenceResults)
	renderCounterMap(&buf, "lockfree_rate_limit_rejections_total", "Rate-limit rejections.", []string{"scope"}, &m.rateLimitRejections)
	return buf.Bytes()
}

func renderCounterMap(buf *bytes.Buffer, metricName, help string, labels []string, store *sync.Map) {
	buf.WriteString("# HELP " + metricName + " " + help + "\n")
	buf.WriteString("# TYPE " + metricName + " counter\n")
	type entry struct {
		key   string
		value int64
	}
	var entries []entry
	store.Range(func(key, value any) bool {
		entries = append(entries, entry{key: key.(string), value: value.(*atomic.Int64).Load()})
		return true
	})
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].key < entries[j].key
	})
	for _, item := range entries {
		parts := strings.Split(item.key, "|")
		buf.WriteString(metricName)
		if len(parts) == len(labels) {
			buf.WriteString("{")
			for i, label := range labels {
				if i > 0 {
					buf.WriteString(",")
				}
				buf.WriteString(label + "=\"" + parts[i] + "\"")
			}
			buf.WriteString("}")
		}
		buf.WriteString(" " + strconv.FormatInt(item.value, 10) + "\n")
	}
}
