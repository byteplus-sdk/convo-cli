// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"convo-cli/internal/auth"
	"convo-cli/internal/config"
	"convo-cli/internal/errs"
)

func TestProjectAuthFirstLoginWritesPrivateCredentials(t *testing.T) {
	t.Cleanup(overrideAuthGlobals(t))
	t.Cleanup(auth.SetTokenStoreForTest(nil))
	t.Setenv("CONVO_CLI_HOME", "")
	dir := t.TempDir()
	t.Chdir(dir)
	if err := config.Save(config.Default("fixture", "voice-agent", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(auth.TokenSet{AccessToken: "fixture-access", RefreshToken: "fixture-refresh", ExpiresIn: 3600})
	}))
	defer tokenServer.Close()
	oauthTokenEndpoint = tokenServer.URL
	authOpenBrowser = func(raw string) error {
		cfg, err := config.Load(config.Path(dir))
		if err != nil || cfg.Auth.Project {
			t.Errorf("project selection persisted before authorization: %v", err)
		}
		return sendProjectAuthCallback(raw, false)
	}
	root := NewRootCmd()
	root.SetArgs([]string{"auth", "login", "--project-auth", "--browser=open"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	stored, err := auth.LoadToken()
	if err != nil || stored.AccessToken != "fixture-access" {
		t.Fatalf("project token not saved: %v", err)
	}
	for _, entry := range []struct {
		name string
		mode os.FileMode
	}{{".convo-cli", 0o700}, {filepath.Join(".convo-cli", "auth.json"), 0o600}} {
		info, err := os.Stat(filepath.Join(dir, entry.name))
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != entry.mode {
			t.Fatalf("%s mode=%o", entry.name, info.Mode().Perm())
		}
	}
}

func TestProjectAuthFailurePreservesCredentialSelection(t *testing.T) {
	for _, scenario := range []string{"noninteractive", "cancel", "denied", "timeout", "exchange", "save"} {
		t.Run(scenario, func(t *testing.T) {
			t.Cleanup(overrideAuthGlobals(t))
			t.Setenv("CONVO_CLI_HOME", "")
			dir := t.TempDir()
			t.Chdir(dir)
			if err := config.Save(config.Default("fixture", "voice-agent", "web"), config.Path(dir)); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(config.Path(dir))
			if err != nil {
				t.Fatal(err)
			}
			global := auth.FileTokenStore{Path: filepath.Join(t.TempDir(), "auth.json")}
			if err := global.Save(auth.TokenSet{AccessToken: "global-fixture"}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(auth.SetTokenStoreForTest(global))
			authIsTerminal = func(io.Reader) bool { return scenario == "cancel" }
			tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "exchange" {
					http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
					return
				}
				_ = json.NewEncoder(w).Encode(auth.TokenSet{AccessToken: "project-fixture", ExpiresIn: 3600})
			}))
			defer tokenServer.Close()
			oauthTokenEndpoint = tokenServer.URL
			authOpenBrowser = func(raw string) error {
				if scenario == "timeout" {
					return nil
				}
				if scenario == "save" {
					if err := os.Mkdir(filepath.Join(dir, ".convo-cli", "auth.json"), 0o700); err != nil {
						return err
					}
				}
				return sendProjectAuthCallback(raw, scenario == "denied")
			}
			args := []string{"auth", "login", "--project-auth", "--timeout=100ms"}
			if scenario != "noninteractive" && scenario != "cancel" {
				args = append(args, "--browser=open")
			}
			root := NewRootCmd()
			root.SetIn(strings.NewReader("c\n"))
			root.SetArgs(args)
			err = root.Execute()
			wantCode := "byteplus-rtc.auth.authorization_failed"
			switch scenario {
			case "noninteractive":
				wantCode = "byteplus-rtc.auth.interaction_required"
			case "exchange":
				wantCode = "byteplus-rtc.auth.token_exchange_failed"
			case "save":
				wantCode = "byteplus-rtc.auth.storage_unavailable"
			}
			typed, ok := errs.As(err)
			if !ok || typed.Code != wantCode {
				t.Fatalf("login error=%v, want %s", err, wantCode)
			}
			after, err := os.ReadFile(config.Path(dir))
			if err != nil || string(after) != string(before) {
				t.Fatalf("failed login changed project configuration: %v", err)
			}
			stored, err := auth.LoadToken()
			if err != nil || stored.AccessToken != "global-fixture" || auth.TokenLocation() != global.Location() {
				t.Fatalf("previous credential selection was not restored: %v", err)
			}
		})
	}
}

func sendProjectAuthCallback(raw string, denied bool) error {
	login, err := url.Parse(raw)
	if err != nil {
		return err
	}
	callback, err := url.Parse(login.Query().Get("redirect_uri"))
	if err != nil {
		return err
	}
	query := callback.Query()
	query.Set("state", login.Query().Get("state"))
	if denied {
		query.Set("error", "access_denied")
	} else {
		query.Set("code", "fixture-code")
	}
	callback.RawQuery = query.Encode()
	response, err := http.Get(callback.String())
	if err != nil {
		return err
	}
	return response.Body.Close()
}
