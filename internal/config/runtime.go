package config

import (
	"path/filepath"
	"sync"
)

// RuntimePaths holds the filesystem paths the user can change at runtime from
// the Settings page. It is shared by POINTER between the HTTP container and the
// UI handlers so updates are observed live.
type RuntimePaths struct {
	mu        sync.RWMutex
	engramDB  string
	claudeDir string
}

func NewRuntimePaths(engramDB, claudeDir string) *RuntimePaths {
	return &RuntimePaths{engramDB: engramDB, claudeDir: claudeDir}
}

func (p *RuntimePaths) EngramDB() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.engramDB
}

// DataDir is the directory containing the Engram database (used for audit logs,
// cloud.json, import backups).
func (p *RuntimePaths) DataDir() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return filepath.Dir(p.engramDB)
}

func (p *RuntimePaths) ClaudeDir() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.claudeDir
}

func (p *RuntimePaths) SetEngramDB(path string) {
	p.mu.Lock()
	p.engramDB = path
	p.mu.Unlock()
}

func (p *RuntimePaths) SetClaudeDir(dir string) {
	p.mu.Lock()
	p.claudeDir = dir
	p.mu.Unlock()
}
