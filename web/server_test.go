package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
)

func TestConfigRejectsRemoteBindingWithoutSessionAuth(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Host = "0.0.0.0"
	cfg.UsersFile = ""
	cfg.SessionSecret = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected non-loopback bind without session auth to fail validation")
	}
}

func TestHTTPServesIndexFromRepoRootCWDAnonymous(t *testing.T) {
	t.Chdir("..")

	app := newTestApp(t, validTestConfig(t))
	handler, _, err := app.routes()
	if err != nil {
		t.Fatalf("routes failed: %v", err)
	}

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	if !strings.Contains(string(body), "Lock-Free Data Structures Visualizer") {
		t.Fatalf("unexpected HTML body: %q", string(body))
	}
	if got := resp.Header.Get("Content-Security-Policy"); !strings.Contains(got, "script-src 'self'") {
		t.Fatalf("expected restrictive CSP, got %q", got)
	}
}

func TestLoginSessionAndAuthenticatedIndex(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.UsersFile = writeUsersFile(t, []testUser{{Username: "operator", Password: "secret-pass", Tenant: "tenant-a", Role: RoleOperator}})
	cfg.SessionSecret = "session-secret"

	app := newTestApp(t, cfg)
	handler, _, err := app.routes()
	if err != nil {
		t.Fatalf("routes failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newCookieClient(t)

	resp, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected redirect to login, got %d", resp.StatusCode)
	}

	loginBody := bytes.NewBufferString(`{"username":"operator","password":"secret-pass"}`)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/login", loginBody)
	if err != nil {
		t.Fatalf("build login request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", server.URL)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected login success, got %d", resp.StatusCode)
	}

	resp, err = client.Get(server.URL + "/api/v1/session")
	if err != nil {
		t.Fatalf("GET /api/v1/session failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200 for session, got %d", resp.StatusCode)
	}

	var session struct {
		Principal Principal `json:"principal"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode session response failed: %v", err)
	}
	if session.Principal.Tenant != "tenant-a" || session.Principal.Role != RoleOperator {
		t.Fatalf("unexpected session principal: %+v", session.Principal)
	}
}

func TestViewerCannotMutateWebSocketState(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.UsersFile = writeUsersFile(t, []testUser{{Username: "viewer", Password: "secret-pass", Tenant: "tenant-a", Role: RoleViewer}})
	cfg.SessionSecret = "session-secret"

	app := newTestApp(t, cfg)
	handler, _, err := app.routes()
	if err != nil {
		t.Fatalf("routes failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newCookieClient(t)
	loginTestUser(t, client, server.URL, "viewer", "secret-pass")

	conn := mustDialAuthenticatedWS(t, client, server.URL)
	defer func() { _ = conn.Close() }()

	initial := readWSMessage(t, conn)
	if initial.Type != "state" || !initial.Success {
		t.Fatalf("expected initial state, got %+v", initial)
	}

	if err := conn.WriteJSON(Message{Type: "stack", Operation: "push", Value: 42}); err != nil {
		t.Fatalf("failed to send websocket mutation: %v", err)
	}
	msg := readWSMessage(t, conn)
	if msg.Success || !strings.Contains(msg.Error, "forbidden") {
		t.Fatalf("expected forbidden mutation response, got %+v", msg)
	}
}

func TestTenantIsolationAcrossSessions(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.UsersFile = writeUsersFile(t, []testUser{
		{Username: "alice", Password: "secret-pass", Tenant: "tenant-a", Role: RoleOperator},
		{Username: "bob", Password: "secret-pass", Tenant: "tenant-b", Role: RoleOperator},
	})
	cfg.SessionSecret = "session-secret"

	app := newTestApp(t, cfg)
	handler, _, err := app.routes()
	if err != nil {
		t.Fatalf("routes failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	clientA := newCookieClient(t)
	clientB := newCookieClient(t)
	loginTestUser(t, clientA, server.URL, "alice", "secret-pass")
	loginTestUser(t, clientB, server.URL, "bob", "secret-pass")

	connA := mustDialAuthenticatedWS(t, clientA, server.URL)
	defer func() { _ = connA.Close() }()
	connB := mustDialAuthenticatedWS(t, clientB, server.URL)
	defer func() { _ = connB.Close() }()

	_ = readWSMessage(t, connA)
	_ = readWSMessage(t, connB)

	if err := connA.WriteJSON(Message{Type: "stack", Operation: "push", Value: 7}); err != nil {
		t.Fatalf("tenant A push failed: %v", err)
	}
	pushReply := readWSMessage(t, connA)
	if !pushReply.Success {
		t.Fatalf("expected tenant A push success, got %+v", pushReply)
	}

	if err := connB.WriteJSON(Message{Type: "getState", Operation: "getState"}); err != nil {
		t.Fatalf("tenant B getState failed: %v", err)
	}
	stateB := readWSMessage(t, connB)
	items := intSliceFromNestedData(t, stateB.Data, "stack", "items")
	if len(items) != 0 {
		t.Fatalf("expected isolated tenant B stack, got %v", items)
	}
}

func TestAdminBackupEndpointAndRecovery(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.UsersFile = writeUsersFile(t, []testUser{{Username: "admin", Password: "secret-pass", Tenant: "tenant-a", Role: RoleAdmin}})
	cfg.SessionSecret = "session-secret"
	cfg.AdminAPIToken = "ops-token"

	app := newTestApp(t, cfg)
	tenant := app.tenantForID("tenant-a")
	tenant.mu.Lock()
	tenant.stack.Push(9)
	tenant.mu.Unlock()

	if err := app.persistence.SaveNow(t.Context(), "test"); err != nil {
		t.Fatalf("save snapshot failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backup", nil)
	req.Header.Set("Authorization", "Bearer ops-token")
	recorder := httptest.NewRecorder()
	app.handleBackup(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected backup success, got %d", recorder.Code)
	}

	reloaded := newTestApp(t, cfg)
	snapshot := reloaded.snapshotState()
	items := snapshot.Tenants["tenant-a"].Stack
	if len(items) != 1 || items[0] != 9 {
		t.Fatalf("expected recovered stack [9], got %v", items)
	}
}

func TestLegacyStateMigration(t *testing.T) {
	cfg := validTestConfig(t)
	legacy := `{"stack":{"items":[5,4]},"queue":{"items":[1,2]},"ringbuffer":{"capacity":16},"counter":{"value":3},"list":{"items":[2,8]}}`
	if err := os.WriteFile(cfg.StateFile, []byte(legacy), 0o640); err != nil {
		t.Fatalf("write legacy state failed: %v", err)
	}

	app := newTestApp(t, cfg)
	snapshot := app.snapshotState()
	tenant := snapshot.Tenants["default"]
	if len(tenant.Stack) != 2 || tenant.Stack[0] != 5 || tenant.Stack[1] != 4 {
		t.Fatalf("unexpected migrated stack: %v", tenant.Stack)
	}
	if tenant.Counter != 3 {
		t.Fatalf("expected migrated counter 3, got %d", tenant.Counter)
	}
}

type testUser struct {
	Username string
	Password string
	Tenant   string
	Role     UserRole
}

func validTestConfig(t *testing.T) Config {
	t.Helper()
	dataDir := t.TempDir()
	return Config{
		Host:                 "127.0.0.1",
		Port:                 "8081",
		DataDir:              dataDir,
		StateFile:            filepath.Join(dataDir, "state.json"),
		BackupDir:            filepath.Join(dataDir, "backups"),
		SnapshotDebounce:     10 * time.Millisecond,
		BackupInterval:       0,
		SessionTTL:           time.Hour,
		BackupRetention:      3,
		HTTPRequestRate:      100,
		HTTPBurst:            100,
		LoginRequestRate:     100,
		LoginBurst:           100,
		WebSocketMessageRate: 100,
		WebSocketBurst:       100,
		LogLevel:             "error",
		LogFormat:            "json",
	}
}

func newTestApp(t *testing.T, cfg Config) *App {
	t.Helper()
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	t.Cleanup(app.Close)
	return app
}

func writeUsersFile(t *testing.T, users []testUser) string {
	t.Helper()
	type userEntry struct {
		Username     string   `json:"username"`
		PasswordHash string   `json:"password_hash"`
		Tenant       string   `json:"tenant"`
		Role         UserRole `json:"role"`
	}

	payload := struct {
		Version int         `json:"version"`
		Users   []userEntry `json:"users"`
	}{Version: 1}

	for _, user := range users {
		hash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			t.Fatalf("bcrypt hash failed: %v", err)
		}
		payload.Users = append(payload.Users, userEntry{
			Username:     user.Username,
			PasswordHash: string(hash),
			Tenant:       user.Tenant,
			Role:         user.Role,
		})
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal users file failed: %v", err)
	}
	path := filepath.Join(t.TempDir(), "users.json")
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatalf("write users file failed: %v", err)
	}
	return path
}

func newCookieClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar failed: %v", err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func loginTestUser(t *testing.T, client *http.Client, baseURL, username, password string) {
	t.Helper()
	body := bytes.NewBufferString(fmt.Sprintf(`{"username":%q,"password":%q}`, username, password))
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/login", body)
	if err != nil {
		t.Fatalf("build login request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", baseURL)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected login success, got %d with body %s", resp.StatusCode, bodyBytes)
	}
}

func mustDialAuthenticatedWS(t *testing.T, client *http.Client, httpURL string) *websocket.Conn {
	t.Helper()
	parsed, err := url.Parse(httpURL)
	if err != nil {
		t.Fatalf("parse URL failed: %v", err)
	}
	wsURL := "ws" + strings.TrimPrefix(httpURL, "http") + "/ws"
	header := http.Header{}
	var cookieHeader []string
	for _, cookie := range client.Jar.Cookies(parsed) {
		cookieHeader = append(cookieHeader, cookie.String())
	}
	if len(cookieHeader) > 0 {
		header.Set("Cookie", strings.Join(cookieHeader, "; "))
	}
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			t.Fatalf("dial websocket failed: %v (status=%d body=%s)", err, resp.StatusCode, string(body))
		}
		t.Fatalf("dial websocket failed: %v", err)
	}
	return conn
}

func readWSMessage(t *testing.T, conn *websocket.Conn) Message {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg Message
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read websocket message failed: %v", err)
	}
	return msg
}

func intSliceFromNestedData(t *testing.T, data interface{}, outer, inner string) []int {
	t.Helper()
	root := data.(map[string]interface{})
	nested := root[outer].(map[string]interface{})
	values := nested[inner].([]interface{})
	result := make([]int, len(values))
	for idx, value := range values {
		result[idx] = int(value.(float64))
	}
	return result
}
