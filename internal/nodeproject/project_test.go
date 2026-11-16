// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package nodeproject

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestNeedsInstallChecksEachPackageWithDependencies(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"scripts":{"setup":"npm ci"}}`)
	writeFile(t, dir, "web/package.json", `{"dependencies":{"@byteplus/rtc":"4.69.3"},"devDependencies":{"vite":"8.3.5"}}`)
	writeFile(t, dir, "server/package.json", `{"dependencies":{"koa":"2"}}`)
	for _, name := range []string{"web/node_modules/@byteplus/rtc/package.json", "web/node_modules/vite/package.json", "server/node_modules/koa/package.json"} {
		missing, err := NeedsInstall(dir, true)
		if err != nil || !missing {
			t.Fatalf("before installing %s: missing=%v err=%v", name, missing, err)
		}
		writeFile(t, dir, name, `{}`)
	}
	for range 2 {
		if missing, err := NeedsInstall(dir, true); missing || err != nil {
			t.Fatalf("installed child packages: missing=%v err=%v", missing, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("root node_modules: %v", err)
	}
	writeFile(t, dir, "package.json", `{"devDependencies":{"concurrently":"9"}}`)
	if missing, err := NeedsInstall(dir, true); !missing || err != nil {
		t.Fatalf("legacy root dependency: missing=%v err=%v", missing, err)
	}
}

func TestNeedsInstallReportsInvalidManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{`)
	if _, err := NeedsInstall(dir, true); err == nil {
		t.Fatal("invalid manifest accepted")
	}
}

func TestProjectEngineRanges(t *testing.T) {
	for _, tc := range []struct {
		name, node, npm string
		wantFailure     string
	}{
		{"supported", "v22.21.1", "10.9.4", ""},
		{"old minor", "v22.12.0", "10.9.4", "node"},
		{"new major", "v23.0.0", "10.9.4", "node"},
		{"old npm", "v22.21.1", "9.9.0", "npm"},
		{"new npm", "v22.21.1", "11.0.0", "npm"},
		{"prerelease", "v22.21.1-rc.1", "10.9.4", "node"},
		{"invalid", "unexpected-output", "10.9.4", "node"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkg := Package{Engines: map[string]string{"node": ">=22.13.0 <23", "npm": ">=10 <11"}}
			checks := checkEngines(context.Background(), pkg, func(_ context.Context, tool string) (string, error) {
				if tool == "node" {
					return tc.node, nil
				}
				return tc.npm, nil
			})
			if len(checks) != 2 {
				t.Fatalf("checks=%+v", checks)
			}
			for _, check := range checks {
				if (check.Err != nil) != (check.Tool == tc.wantFailure) {
					t.Fatalf("unexpected result: %+v", check)
				}
				if check.Err != nil && strings.Contains(check.Err.Error(), "unexpected-output") {
					t.Fatal("invalid version output was echoed")
				}
			}
		})
	}
}

func TestProjectWithoutEngineRequirements(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{}`)
	t.Setenv("PATH", t.TempDir())
	checks, err := CheckEngines(context.Background(), dir)
	if err != nil || len(checks) != 0 {
		t.Fatalf("checks=%+v err=%v", checks, err)
	}
}
