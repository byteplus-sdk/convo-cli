// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"convo-cli/internal/config"
)

func TestProjectAuthenticationPersistsAcrossCommands(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default("fixture", "voice-agent", "web")
	if err := config.Save(cfg, config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".convo-cli", "auth.json")
	writeAuthFixture(t, path, time.Now().Add(time.Hour))
	env := []string{"CONVO_CLI_HOME=", "CONVO_CLI_NO_UPDATE_NOTIFIER=1", "CONVO_CLI_NO_SKILLS_NOTIFIER=1"}
	r := run(t, dir, env, "auth", "login", "--project-auth")
	if r.code != 0 {
		t.Fatalf("login: %s %s", r.stdout, r.stderr)
	}
	cfg, err := config.Load(config.Path(dir))
	if err != nil || !cfg.Auth.Project {
		t.Fatalf("project selection not saved: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil || !strings.Contains(string(raw), "/.convo-cli/") {
		t.Fatalf("ignore: %s %v", raw, err)
	}
	nested := filepath.Join(dir, "server")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"auth", "status"}, {"doctor", "cli"}, {"auth", "logout"}} {
		r = run(t, nested, env, args...)
		if r.code != 0 {
			t.Fatalf("%v: %s %s", args, r.stdout, r.stderr)
		}
		if args[0] == "auth" && args[1] == "status" && !strings.Contains(r.stdout, "auth.json") {
			t.Fatalf("status: %s", r.stdout)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("logout retained credentials: %v", err)
	}
}

func TestProjectAuthenticationDryRunAndConflictingOverride(t *testing.T) {
	dir := t.TempDir()
	if err := config.Save(config.Default("fixture", "voice-agent", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(config.Path(dir))
	for _, tc := range []struct {
		home    string
		args    []string
		success bool
	}{
		{"", []string{"--dry-run"}, true},
		{t.TempDir(), nil, false},
		{"", []string{"--store=keyring"}, false},
	} {
		args := append([]string{"auth", "login", "--project-auth"}, tc.args...)
		r := run(t, dir, []string{"CONVO_CLI_HOME=" + tc.home}, args...)
		if (r.code == 0) != tc.success {
			t.Fatalf("%v: %s %s", args, r.stdout, r.stderr)
		}
		after, _ := os.ReadFile(config.Path(dir))
		if string(before) != string(after) {
			t.Fatal("configuration changed")
		}
		for _, name := range []string{".convo-cli", ".gitignore"} {
			if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
				t.Fatalf("unexpected %s: %v", name, err)
			}
		}
	}
}

func TestAuthDryRunValidatesStore(t *testing.T) {
	r := run(t, t.TempDir(), nil, "auth", "login", "--dry-run", "--store=unsupported")
	if r.code == 0 || !strings.Contains(r.stdout, "invalid") {
		t.Fatalf("invalid store accepted: %s", r.stdout)
	}
}
