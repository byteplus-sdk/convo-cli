// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package paths owns user-level lifecycle state paths for convo-cli.
package paths

import (
	"os"
	"path/filepath"
	"runtime"

	"convo-cli/internal/product"
)

// StateDir returns the selected product's platform-standard private directory
// used for update and skills state. The profile override is useful for tests
// and managed environments that need a deterministic location.
func StateDir() string {
	profile := product.Current()
	if dir := os.Getenv(profile.StateDirEnv); dir != "" {
		return dir
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, profile.StateDirName)
}

// WriteFileAtomic writes a private state file without exposing partial JSON to
// concurrent CLI processes.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil && runtime.GOOS != "windows" {
		return err
	}
	f, err := os.CreateTemp(dir, ".cli-state-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	return os.Rename(tmp, path)
}
