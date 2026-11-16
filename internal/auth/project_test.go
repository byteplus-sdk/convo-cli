// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"os"
	"path/filepath"
	"testing"

	"convo-cli/internal/config"
	"convo-cli/internal/product"
)

func TestProjectAuthUsesNearestConfigAndEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default("fixture", "voice-agent", "web")
	cfg.Auth.Project = true
	if err := config.Save(cfg, config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "server")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	t.Setenv(product.Current().AuthHomeEnv, "")
	want := filepath.Join(dir, product.Current().AuthDirName, "auth.json")
	if got, err := authFilePath(); err != nil || got != want {
		t.Fatalf("path=%s err=%v want=%s", got, err, want)
	}
	override := t.TempDir()
	t.Setenv(product.Current().AuthHomeEnv, override)
	if got, err := authFilePath(); err != nil || got != filepath.Join(override, "auth.json") {
		t.Fatalf("override path=%s err=%v", got, err)
	}
}

func TestProjectAuthDoesNotUseUnavailableDefaultHome(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default("fixture", "voice-agent", "web")
	cfg.Auth.Project = true
	if err := config.Save(cfg, config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv(product.Current().AuthHomeEnv, "")
	previous := userHomeDir
	userHomeDir = func() (string, error) { t.Fatal("project auth accessed default home"); return "", nil }
	defer func() { userHomeDir = previous }()
	if err := PreflightTokenStore(); err != nil {
		t.Fatal(err)
	}
	if err := SaveToken(TokenSet{AccessToken: "fixture"}); err != nil {
		t.Fatal(err)
	}
	token, err := LoadToken()
	if err != nil || token.AccessToken != "fixture" {
		t.Fatalf("load: %v", err)
	}
	if err := DeleteToken(); err != nil {
		t.Fatal(err)
	}
}
