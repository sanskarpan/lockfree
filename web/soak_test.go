package main

import (
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestAuthenticatedServiceSoak(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping soak test in short mode")
	}

	workers := envInt(t, "LOCKFREE_SOAK_WORKERS", 6)
	cfg := validTestConfig(t)
	users := make([]testUser, 0, workers+1)
	for worker := 0; worker < workers; worker++ {
		users = append(users, testUser{
			Username: "operator-" + strconv.Itoa(worker),
			Password: "secret-pass",
			Tenant:   "tenant-" + strconv.Itoa(worker),
			Role:     RoleOperator,
		})
	}
	users = append(users, testUser{Username: "observer", Password: "secret-pass", Tenant: "tenant-0", Role: RoleViewer})
	cfg.UsersFile = writeUsersFile(t, users)
	cfg.SessionSecret = "session-secret"

	app := newTestApp(t, cfg)
	handler, _, err := app.routes()
	if err != nil {
		t.Fatalf("routes failed: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	iterations := envInt(t, "LOCKFREE_SOAK_ITERATIONS", 40)

	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			client := newCookieClient(t)
			username := "operator-" + strconv.Itoa(id)
			loginTestUser(t, client, server.URL, username, "secret-pass")
			conn := mustDialAuthenticatedWS(t, client, server.URL)
			defer conn.Close()
			_ = readWSMessage(t, conn)

			for iter := 0; iter < iterations; iter++ {
				if err := conn.WriteJSON(Message{Type: "counter", Operation: "inc"}); err != nil {
					t.Errorf("worker %d failed to increment: %v", id, err)
					return
				}
				reply := readWSMessage(t, conn)
				if !reply.Success {
					t.Errorf("worker %d received unsuccessful reply: %+v", id, reply)
					return
				}
			}
		}(worker)
	}
	wg.Wait()

	if err := app.persistence.SaveNow(t.Context(), "soak"); err != nil {
		t.Fatalf("save after soak failed: %v", err)
	}

	reloaded := newTestApp(t, cfg)
	snapshot := reloaded.snapshotState()
	var total int64
	for worker := 0; worker < workers; worker++ {
		total += snapshot.Tenants["tenant-"+strconv.Itoa(worker)].Counter
	}
	if total != int64(workers*iterations) {
		t.Fatalf("expected total counter %d, got %d", workers*iterations, total)
	}

	observer := newCookieClient(t)
	loginTestUser(t, observer, server.URL, "observer", "secret-pass")
	conn := mustDialAuthenticatedWS(t, observer, server.URL)
	defer conn.Close()
	_ = readWSMessage(t, conn)
	if err := conn.WriteJSON(Message{Type: "counter", Operation: "inc"}); err != nil {
		t.Fatalf("viewer write failed: %v", err)
	}
	reply := readWSMessage(t, conn)
	if reply.Success {
		t.Fatalf("expected viewer mutation to be rejected, got %+v", reply)
	}
	time.Sleep(25 * time.Millisecond)
}

func envInt(t *testing.T, name string, fallback int) int {
	t.Helper()

	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		t.Fatalf("invalid %s value %q", name, raw)
	}
	return value
}
