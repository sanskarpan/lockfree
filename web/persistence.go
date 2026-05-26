package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const persistedSchemaVersion = 2

type persistedState struct {
	Version     int                       `json:"version"`
	GeneratedAt time.Time                 `json:"generated_at"`
	Tenants     map[string]tenantSnapshot `json:"tenants"`
}

type tenantSnapshot struct {
	Stack      []int              `json:"stack"`
	Queue      []int              `json:"queue"`
	RingBuffer ringBufferSnapshot `json:"ringbuffer"`
	Counter    int64              `json:"counter"`
	List       []int              `json:"list"`
}

type ringBufferSnapshot struct {
	Capacity  int   `json:"capacity"`
	Overwrite bool  `json:"overwrite"`
	Items     []int `json:"items"`
}

type legacyState struct {
	Stack      tenantListState    `json:"stack"`
	Queue      tenantListState    `json:"queue"`
	RingBuffer legacyRingBuffer   `json:"ringbuffer"`
	Counter    legacyCounterState `json:"counter"`
	List       tenantListState    `json:"list"`
}

type tenantListState struct {
	Items []int `json:"items"`
}

type legacyRingBuffer struct {
	Capacity int `json:"capacity"`
}

type legacyCounterState struct {
	Value int64 `json:"value"`
}

type PersistenceManager struct {
	cfg          Config
	log          *slog.Logger
	snapshotFn   func() persistedState
	observe      func(result string, at time.Time)
	mu           sync.Mutex
	saveTimer    *time.Timer
	lastSaveErr  error
	lastSaveTime time.Time
	lastBackup   time.Time
}

func NewPersistenceManager(cfg Config, logger *slog.Logger, snapshotFn func() persistedState) *PersistenceManager {
	return &PersistenceManager{
		cfg:        cfg,
		log:        logger,
		snapshotFn: snapshotFn,
	}
}

func (p *PersistenceManager) Init() error {
	if err := os.MkdirAll(p.cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(p.cfg.BackupDir, 0o750); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	return nil
}

func (p *PersistenceManager) Load() (persistedState, string, error) {
	if err := p.Init(); err != nil {
		return persistedState{}, "", err
	}

	if _, err := os.Stat(p.cfg.StateFile); errors.Is(err, os.ErrNotExist) {
		return persistedState{Version: persistedSchemaVersion, Tenants: map[string]tenantSnapshot{}}, "", nil
	}

	state, err := p.readStateFile(p.cfg.StateFile)
	if err == nil {
		return state, p.cfg.StateFile, nil
	}

	p.log.Warn("primary snapshot load failed, attempting backup recovery", "error", err)
	backupFiles, listErr := filepath.Glob(filepath.Join(p.cfg.BackupDir, "*.json"))
	if listErr != nil {
		return persistedState{}, "", err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(backupFiles)))
	for _, candidate := range backupFiles {
		recovered, recoverErr := p.readStateFile(candidate)
		if recoverErr == nil {
			p.log.Warn("recovered persisted state from backup", "backup", candidate)
			return recovered, candidate, nil
		}
	}
	return persistedState{}, "", err
}

func (p *PersistenceManager) readStateFile(path string) (persistedState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return persistedState{}, err
	}

	var current persistedState
	if err := json.Unmarshal(data, &current); err == nil && current.Version >= 1 {
		return migrateState(current), nil
	}

	var legacy legacyState
	if err := json.Unmarshal(data, &legacy); err == nil {
		return migrateLegacyState(legacy), nil
	}

	return persistedState{}, fmt.Errorf("parse state file %s: unsupported schema", path)
}

func migrateState(input persistedState) persistedState {
	if input.Tenants == nil {
		input.Tenants = map[string]tenantSnapshot{}
	}
	if input.Version < persistedSchemaVersion && len(input.Tenants) == 0 {
		input.Tenants["default"] = tenantSnapshot{}
	}
	input.Version = persistedSchemaVersion
	return input
}

func migrateLegacyState(input legacyState) persistedState {
	return persistedState{
		Version:     persistedSchemaVersion,
		GeneratedAt: time.Now().UTC(),
		Tenants: map[string]tenantSnapshot{
			"default": {
				Stack: input.Stack.Items,
				Queue: input.Queue.Items,
				RingBuffer: ringBufferSnapshot{
					Capacity:  input.RingBuffer.Capacity,
					Overwrite: false,
				},
				Counter: input.Counter.Value,
				List:    input.List.Items,
			},
		},
	}
}

func (p *PersistenceManager) ScheduleSave() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.saveTimer != nil {
		p.saveTimer.Stop()
	}
	p.saveTimer = time.AfterFunc(p.cfg.SnapshotDebounce, func() {
		if err := p.SaveNow(context.Background(), "scheduled"); err != nil {
			p.log.Error("scheduled persistence save failed", "error", err)
		}
	})
}

func (p *PersistenceManager) SaveNow(ctx context.Context, reason string) error {
	_ = ctx
	state := p.snapshotFn()
	state.Version = persistedSchemaVersion
	state.GeneratedAt = time.Now().UTC()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := p.cfg.StateFile + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o640); err != nil {
		p.recordSaveResult(err)
		return err
	}
	if err := os.Rename(tmpPath, p.cfg.StateFile); err != nil {
		p.recordSaveResult(err)
		return err
	}

	if p.cfg.BackupInterval == 0 || time.Since(p.lastBackup) >= p.cfg.BackupInterval {
		if _, err := p.backupBytes(data); err != nil {
			p.log.Error("backup write failed", "reason", reason, "error", err)
		}
	}

	p.recordSaveResult(nil)
	return nil
}

func (p *PersistenceManager) BackupNow() (string, error) {
	state := p.snapshotFn()
	state.Version = persistedSchemaVersion
	state.GeneratedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", err
	}
	return p.backupBytes(data)
}

func (p *PersistenceManager) backupBytes(data []byte) (string, error) {
	name := "state-" + time.Now().UTC().Format("20060102T150405Z") + ".json"
	path := filepath.Join(p.cfg.BackupDir, name)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		if p.observe != nil {
			p.observe("backup_error", time.Now())
		}
		return "", err
	}
	p.lastBackup = time.Now()
	if p.observe != nil {
		p.observe("backup_success", p.lastBackup)
	}
	if err := p.trimBackups(); err != nil {
		p.log.Warn("backup retention trim failed", "error", err)
	}
	return path, nil
}

func (p *PersistenceManager) trimBackups() error {
	entries, err := os.ReadDir(p.cfg.BackupDir)
	if err != nil {
		return err
	}

	type backupEntry struct {
		name string
		path string
		time time.Time
	}
	var backups []backupEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		backups = append(backups, backupEntry{
			name: entry.Name(),
			path: filepath.Join(p.cfg.BackupDir, entry.Name()),
			time: info.ModTime(),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].time.After(backups[j].time)
	})

	for idx, entry := range backups {
		if idx < p.cfg.BackupRetention {
			continue
		}
		if err := os.Remove(entry.path); err != nil {
			return err
		}
	}
	return nil
}

func (p *PersistenceManager) recordSaveResult(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSaveErr = err
	if err == nil {
		p.lastSaveTime = time.Now()
		if p.observe != nil {
			p.observe("save_success", p.lastSaveTime)
		}
		return
	}
	if p.observe != nil {
		p.observe("save_error", time.Now())
	}
}

func (p *PersistenceManager) Ready() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastSaveErr
}

func (p *PersistenceManager) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.saveTimer != nil {
		p.saveTimer.Stop()
		p.saveTimer = nil
	}
}
