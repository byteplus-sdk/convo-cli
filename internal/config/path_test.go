// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"path/filepath"
	"testing"

	"convo-cli/internal/errs"
)

func TestProjectFilePathAcceptsInside(t *testing.T) {
	dir := "/tmp/proj"
	for _, name := range []string{"scene.json", "./scene.json", "server/scenes/default.json"} {
		got, err := ProjectFilePath(dir, name)
		if err != nil {
			t.Fatalf("ProjectFilePath(%q) unexpected error: %v", name, err)
		}
		want := filepath.Join(dir, name)
		if got != want {
			t.Fatalf("ProjectFilePath(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestProjectFilePathRejectsEscape(t *testing.T) {
	dir := "/tmp/proj"
	for _, name := range []string{
		"/etc/passwd",            // absolute
		`C:\Windows\win.ini`,     // Windows drive absolute
		"../outside.json",        // parent escape
		"../../etc/passwd",       // deeper escape
		"sub/../../outside.json", // escape after cleaning
		"",                       // empty
	} {
		_, err := ProjectFilePath(dir, name)
		typed, ok := errs.As(err)
		if !ok || typed.Code != "byteplus-rtc.config.invalid_field" {
			t.Fatalf("ProjectFilePath(%q) = %v, want byteplus-rtc.config.invalid_field error", name, err)
		}
	}
}
