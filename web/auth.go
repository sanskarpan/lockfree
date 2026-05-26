package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName  = "lockfree_session"
	legacyAuthTokenEnv = "LOCKFREE_AUTH_TOKEN"
)

type UserRole string

const (
	RoleViewer   UserRole = "viewer"
	RoleOperator UserRole = "operator"
	RoleAdmin    UserRole = "admin"
)

type Principal struct {
	Username   string   `json:"username"`
	Tenant     string   `json:"tenant"`
	Role       UserRole `json:"role"`
	AuthMethod string   `json:"auth_method"`
}

func (p Principal) CanWrite() bool {
	return p.Role == RoleOperator || p.Role == RoleAdmin
}

func (p Principal) IsAdmin() bool {
	return p.Role == RoleAdmin
}

type userRecord struct {
	Username     string   `json:"username"`
	PasswordHash string   `json:"password_hash"`
	Tenant       string   `json:"tenant"`
	Role         UserRole `json:"role"`
	Disabled     bool     `json:"disabled,omitempty"`
}

type userFile struct {
	Version int          `json:"version"`
	Users   []userRecord `json:"users"`
}

type UserDirectory struct {
	users map[string]userRecord
}

func loadUserDirectory(path string) (*UserDirectory, error) {
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read users file: %w", err)
	}

	var parsed userFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse users file: %w", err)
	}
	if len(parsed.Users) == 0 {
		return nil, fmt.Errorf("users file does not define any users")
	}

	dir := &UserDirectory{users: make(map[string]userRecord, len(parsed.Users))}
	for _, user := range parsed.Users {
		user.Username = strings.TrimSpace(user.Username)
		user.Tenant = strings.TrimSpace(user.Tenant)
		if user.Username == "" {
			return nil, fmt.Errorf("user entry missing username")
		}
		if user.PasswordHash == "" {
			return nil, fmt.Errorf("user %q missing password_hash", user.Username)
		}
		if user.Tenant == "" {
			user.Tenant = "default"
		}
		if !isValidRole(user.Role) {
			return nil, fmt.Errorf("user %q has invalid role %q", user.Username, user.Role)
		}
		if _, exists := dir.users[user.Username]; exists {
			return nil, fmt.Errorf("duplicate user %q", user.Username)
		}
		dir.users[user.Username] = user
	}

	return dir, nil
}

func (d *UserDirectory) Authenticate(username, password string) (Principal, error) {
	if d == nil {
		return Principal{}, errors.New("authentication disabled")
	}

	user, ok := d.users[strings.TrimSpace(username)]
	if !ok || user.Disabled {
		return Principal{}, errors.New("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return Principal{}, errors.New("invalid credentials")
	}

	return Principal{
		Username:   user.Username,
		Tenant:     user.Tenant,
		Role:       user.Role,
		AuthMethod: "session",
	}, nil
}

func isValidRole(role UserRole) bool {
	return role == RoleViewer || role == RoleOperator || role == RoleAdmin
}

type sessionClaims struct {
	Username string   `json:"username"`
	Tenant   string   `json:"tenant"`
	Role     UserRole `json:"role"`
	Expiry   int64    `json:"expiry"`
}

type SessionManager struct {
	currentSecret  []byte
	previousSecret [][]byte
	ttl            time.Duration
}

func NewSessionManager(secret string, previous []string, ttl time.Duration) (*SessionManager, error) {
	if secret == "" {
		return nil, errors.New("session secret must not be empty")
	}

	manager := &SessionManager{
		currentSecret: []byte(secret),
		ttl:           ttl,
	}
	for _, value := range previous {
		manager.previousSecret = append(manager.previousSecret, []byte(value))
	}
	return manager, nil
}

func (m *SessionManager) IssueCookie(r *http.Request, principal Principal, secure bool) (*http.Cookie, error) {
	claims := sessionClaims{
		Username: principal.Username,
		Tenant:   principal.Tenant,
		Role:     principal.Role,
		Expiry:   time.Now().Add(m.ttl).Unix(),
	}
	value, err := m.encode(claims)
	if err != nil {
		return nil, err
	}
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
		MaxAge:   int(m.ttl.Seconds()),
	}, nil
}

func (m *SessionManager) ClearCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
}

func (m *SessionManager) ParseRequest(r *http.Request) (Principal, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return Principal{}, err
	}
	claims, err := m.decode(cookie.Value)
	if err != nil {
		return Principal{}, err
	}
	if time.Now().Unix() >= claims.Expiry {
		return Principal{}, errors.New("session expired")
	}
	return Principal{
		Username:   claims.Username,
		Tenant:     claims.Tenant,
		Role:       claims.Role,
		AuthMethod: "session",
	}, nil
}

func (m *SessionManager) encode(claims sessionClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := signSession(encodedPayload, m.currentSecret)
	return encodedPayload + "." + signature, nil
}

func (m *SessionManager) decode(value string) (sessionClaims, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return sessionClaims{}, errors.New("invalid session token")
	}

	payload, signature := parts[0], parts[1]
	for _, secret := range append([][]byte{m.currentSecret}, m.previousSecret...) {
		expected := signSession(payload, secret)
		if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1 {
			raw, err := base64.RawURLEncoding.DecodeString(payload)
			if err != nil {
				return sessionClaims{}, err
			}
			var claims sessionClaims
			if err := json.Unmarshal(raw, &claims); err != nil {
				return sessionClaims{}, err
			}
			if claims.Username == "" || claims.Tenant == "" || !isValidRole(claims.Role) {
				return sessionClaims{}, errors.New("invalid session claims")
			}
			return claims, nil
		}
	}
	return sessionClaims{}, errors.New("invalid session signature")
}

func signSession(payload string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	return value
}

func traceIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(traceIDContextKey{}).(string)
	return value
}

func randomID(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func legacyAdminToken() string {
	return strings.TrimSpace(os.Getenv(legacyAuthTokenEnv))
}
