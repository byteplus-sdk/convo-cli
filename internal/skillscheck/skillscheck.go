// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package skillscheck tracks whether globally installed official skills match
// the running convo-cli release.
package skillscheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"convo-cli/internal/meta"
	"convo-cli/internal/paths"
	"convo-cli/internal/product"
	updatecheck "convo-cli/internal/update"
)

type state struct {
	Version  string    `json:"version"`
	SyncedAt time.Time `json:"synced_at"`
}

// Notice describes skill content that does not match the running binary.
type Notice struct {
	Current string `json:"current"`
	Target  string `json:"target"`
}

var pending struct {
	sync.RWMutex
	value *Notice
}

func statePath() string { return filepath.Join(paths.StateDir(), "skills-state.json") }

// SyncedVersion returns the version recorded after the last successful sync.
func SyncedVersion() string {
	var s state
	data, err := os.ReadFile(statePath())
	if err != nil || json.Unmarshal(data, &s) != nil {
		return ""
	}
	return strings.TrimPrefix(s.Version, "v")
}

// Init reads persisted state and prepares a cache-only drift notice.
func Init(version string) {
	pending.Lock()
	defer pending.Unlock()
	pending.value = nil
	if disabled(version) {
		return
	}
	var s state
	if data, err := os.ReadFile(statePath()); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	current := strings.TrimPrefix(s.Version, "v")
	target := strings.TrimPrefix(version, "v")
	if current != target {
		pending.value = &Notice{Current: current, Target: target}
	}
}

// Status returns the external Skills lifecycle state without modifying it.
func Status(version string) string {
	if disabled(version) {
		return "skipped"
	}
	if SyncedVersion() == strings.TrimPrefix(version, "v") {
		return "synchronized"
	}
	return "out_of_sync"
}

func disabled(version string) bool {
	return os.Getenv(product.Current().SkillsNotifierEnv) != "" || updatecheck.AutomationDisabled() || !releaseVersion(version)
}

// Pending returns the current drift notice as an Agent-facing object.
func Pending() map[string]any {
	pending.RLock()
	defer pending.RUnlock()
	if pending.value == nil {
		return nil
	}
	current := pending.value.Current
	if current == "" {
		current = "not installed"
	}
	return map[string]any{
		"current": pending.value.Current,
		"target":  pending.value.Target,
		"message": meta.BinName + " skills " + current + " out of sync with binary " + pending.value.Target + ", run: " + meta.BinName + " skills sync",
		"command": meta.BinName + " skills sync",
	}
}

// MarkSynced records a successful global skill installation.
func MarkSynced(version string) error {
	data, err := json.Marshal(state{Version: strings.TrimPrefix(version, "v"), SyncedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	if err := paths.WriteFileAtomic(statePath(), data); err != nil {
		return err
	}
	pending.Lock()
	pending.value = nil
	pending.Unlock()
	return nil
}

func releaseVersion(v string) bool {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.SplitN(v, "+", 2)
	v = parts[0]
	if strings.Contains(v, "dev") || strings.Contains(v, "dirty") || strings.Contains(v, "-g") {
		return false
	}
	base := strings.SplitN(v, "-", 2)[0]
	n := strings.Split(base, ".")
	return len(n) == 3 && n[0] != "" && n[1] != "" && n[2] != ""
}
