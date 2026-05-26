package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHost                 = "127.0.0.1"
	defaultPort                 = "8081"
	defaultDataDir              = "data"
	defaultSnapshotDebounce     = 500 * time.Millisecond
	defaultBackupInterval       = 30 * time.Minute
	defaultSessionTTL           = 8 * time.Hour
	defaultBackupRetention      = 24
	defaultHTTPRequestRate      = 10
	defaultHTTPBurst            = 40
	defaultLoginRequestRate     = 1
	defaultLoginBurst           = 5
	defaultWebSocketMessageRate = 25
	defaultWebSocketBurst       = 50
)

type Config struct {
	Host                   string
	Port                   string
	DataDir                string
	StateFile              string
	BackupDir              string
	UsersFile              string
	SessionSecret          string
	PreviousSessionSecrets []string
	AdminAPIToken          string
	TrustProxyHeaders      bool
	SnapshotDebounce       time.Duration
	BackupInterval         time.Duration
	SessionTTL             time.Duration
	BackupRetention        int
	HTTPRequestRate        int
	HTTPBurst              int
	LoginRequestRate       int
	LoginBurst             int
	WebSocketMessageRate   int
	WebSocketBurst         int
	LogLevel               string
	LogFormat              string
}

func LoadConfig() (Config, error) {
	dataDir := envOrDefault("LOCKFREE_DATA_DIR", defaultDataDir)
	stateFile := envOrDefault("LOCKFREE_STATE_FILE", filepath.Join(dataDir, "state.json"))
	backupDir := envOrDefault("LOCKFREE_BACKUP_DIR", filepath.Join(dataDir, "backups"))

	snapshotDebounce, err := durationEnv("LOCKFREE_SNAPSHOT_DEBOUNCE", defaultSnapshotDebounce)
	if err != nil {
		return Config{}, err
	}
	backupInterval, err := durationEnv("LOCKFREE_BACKUP_INTERVAL", defaultBackupInterval)
	if err != nil {
		return Config{}, err
	}
	sessionTTL, err := durationEnv("LOCKFREE_SESSION_TTL", defaultSessionTTL)
	if err != nil {
		return Config{}, err
	}

	backupRetention, err := intEnv("LOCKFREE_BACKUP_RETENTION", defaultBackupRetention)
	if err != nil {
		return Config{}, err
	}
	httpRate, err := intEnv("LOCKFREE_HTTP_RATE", defaultHTTPRequestRate)
	if err != nil {
		return Config{}, err
	}
	httpBurst, err := intEnv("LOCKFREE_HTTP_BURST", defaultHTTPBurst)
	if err != nil {
		return Config{}, err
	}
	loginRate, err := intEnv("LOCKFREE_LOGIN_RATE", defaultLoginRequestRate)
	if err != nil {
		return Config{}, err
	}
	loginBurst, err := intEnv("LOCKFREE_LOGIN_BURST", defaultLoginBurst)
	if err != nil {
		return Config{}, err
	}
	wsRate, err := intEnv("LOCKFREE_WS_RATE", defaultWebSocketMessageRate)
	if err != nil {
		return Config{}, err
	}
	wsBurst, err := intEnv("LOCKFREE_WS_BURST", defaultWebSocketBurst)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Host:                   envOrDefault("HOST", defaultHost),
		Port:                   strings.TrimPrefix(envOrDefault("PORT", defaultPort), ":"),
		DataDir:                dataDir,
		StateFile:              stateFile,
		BackupDir:              backupDir,
		UsersFile:              strings.TrimSpace(os.Getenv("LOCKFREE_USERS_FILE")),
		SessionSecret:          strings.TrimSpace(os.Getenv("LOCKFREE_SESSION_SECRET")),
		PreviousSessionSecrets: splitCSVEnv("LOCKFREE_PREVIOUS_SESSION_SECRETS"),
		AdminAPIToken:          strings.TrimSpace(os.Getenv("LOCKFREE_ADMIN_API_TOKEN")),
		TrustProxyHeaders:      boolEnv("LOCKFREE_TRUST_PROXY_HEADERS"),
		SnapshotDebounce:       snapshotDebounce,
		BackupInterval:         backupInterval,
		SessionTTL:             sessionTTL,
		BackupRetention:        backupRetention,
		HTTPRequestRate:        httpRate,
		HTTPBurst:              httpBurst,
		LoginRequestRate:       loginRate,
		LoginBurst:             loginBurst,
		WebSocketMessageRate:   wsRate,
		WebSocketBurst:         wsBurst,
		LogLevel:               strings.ToLower(envOrDefault("LOCKFREE_LOG_LEVEL", "info")),
		LogFormat:              strings.ToLower(envOrDefault("LOCKFREE_LOG_FORMAT", "json")),
	}

	return cfg, cfg.Validate()
}

func (c Config) Addr() string {
	return net.JoinHostPort(c.Host, c.Port)
}

func (c Config) AuthEnabled() bool {
	return c.UsersFile != "" && c.SessionSecret != ""
}

func (c Config) Validate() error {
	if c.Port == "" {
		return fmt.Errorf("port must not be empty")
	}
	if c.UsersFile != "" && c.SessionSecret == "" {
		return fmt.Errorf("LOCKFREE_SESSION_SECRET must be set when LOCKFREE_USERS_FILE is configured")
	}
	if c.UsersFile == "" && c.SessionSecret != "" {
		return fmt.Errorf("LOCKFREE_USERS_FILE must be set when LOCKFREE_SESSION_SECRET is configured")
	}
	if c.SnapshotDebounce <= 0 {
		return fmt.Errorf("LOCKFREE_SNAPSHOT_DEBOUNCE must be positive")
	}
	if c.BackupInterval < 0 {
		return fmt.Errorf("LOCKFREE_BACKUP_INTERVAL must be zero or positive")
	}
	if c.SessionTTL <= 0 {
		return fmt.Errorf("LOCKFREE_SESSION_TTL must be positive")
	}
	if c.BackupRetention < 1 {
		return fmt.Errorf("LOCKFREE_BACKUP_RETENTION must be at least 1")
	}
	if c.HTTPRequestRate < 1 || c.HTTPBurst < 1 {
		return fmt.Errorf("HTTP rate limits must be at least 1")
	}
	if c.LoginRequestRate < 1 || c.LoginBurst < 1 {
		return fmt.Errorf("login rate limits must be at least 1")
	}
	if c.WebSocketMessageRate < 1 || c.WebSocketBurst < 1 {
		return fmt.Errorf("websocket rate limits must be at least 1")
	}

	host := strings.Trim(strings.ToLower(c.Host), "[]")
	if !isLoopbackHost(host) && !c.AuthEnabled() {
		return fmt.Errorf("non-loopback binding requires LOCKFREE_USERS_FILE and LOCKFREE_SESSION_SECRET")
	}
	return nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func splitCSVEnv(name string) []string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func boolEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func intEnv(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	return value, nil
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", name, err)
	}
	return value, nil
}
