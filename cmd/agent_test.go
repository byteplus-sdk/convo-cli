// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"convo-cli/internal/config"
	"convo-cli/internal/errs"
	"convo-cli/internal/product"
)

// A malicious/untrusted agent.config_file must not let `agent start` read files
// outside the project directory (path-traversal hardening).
func TestBuildStartRequestRejectsConfigPathTraversal(t *testing.T) {
	for _, cfgFile := range []string{"../evil.json", "/etc/passwd"} {
		cfg := &config.Config{
			Project: config.Project{Scene: "voice-agent"},
			RTC:     config.RTC{AppID: "app123456789012345678901", RoomID: "room-01", UserID: "user-01"},
			Agent:   config.Agent{ConfigFile: cfgFile},
		}
		_, err := buildStartRequest(cfg, t.TempDir())
		typed, ok := errs.As(err)
		if !ok || typed.Code != "byteplus-rtc.config.invalid_field" {
			t.Fatalf("config_file %q: err = %v, want byteplus-rtc.config.invalid_field", cfgFile, err)
		}
	}
}

// An empty agent.user_id must not clobber the scene's validated AgentConfig.UserID,
// otherwise `agent start` would launch a task the AI cannot join under.
func TestBuildStartRequestKeepsSceneUserIDWhenConfigEmpty(t *testing.T) {
	dir := t.TempDir()
	scene := `{
      "SceneConfig": {"Name": "demo"},
      "VoiceChat": {
        "Config": {
          "ASRConfig": {"Provider": "BytePlus", "ProviderParams": {}},
          "LLMConfig": {"Mode": "BytePlusArk", "EndPointId": "ep-test", "APIKey": "test-api-key"},
          "TTSConfig": {"Provider": "byteplus_Bidirectional_streaming", "ProviderParams": {}}
        },
        "AgentConfig": {"UserID": "scene-agent"}
      }
    }`
	sceneDir := filepath.Join(dir, "server", "scenes")
	if err := os.MkdirAll(sceneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sceneDir, "default.json"), []byte(scene), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Project: config.Project{Scene: "voice-agent"},
		RTC:     config.RTC{AppID: "app123456789012345678901", RoomID: "room-01", UserID: "user-01"},
		Agent:   config.Agent{ConfigFile: "server/scenes/default.json", UserID: ""}, // empty on purpose
	}
	req, err := buildStartRequest(cfg, dir)
	if err != nil {
		t.Fatalf("buildStartRequest: %v", err)
	}
	if got, _ := req.AgentConfig["UserID"].(string); got != "scene-agent" {
		t.Fatalf("AgentConfig.UserID = %q, want scene-agent (not clobbered to empty)", got)
	}
}

func TestBytePlusAgentStartDryRunRedactsProviderSecrets(t *testing.T) {
	previousProduct, previousDryRun, previousFormat := product.SelectedID, flagDryRun, flagFormat
	product.SelectedID, flagDryRun, flagFormat = product.BytePlusID, true, "json"
	t.Cleanup(func() { product.SelectedID, flagDryRun, flagFormat = previousProduct, previousDryRun, previousFormat })
	dir := t.TempDir()
	t.Chdir(dir)
	cfg := config.Default("demo", "voice-agent", "web")
	cfg.RTC.AppID = "app"
	cfg.Agent.TaskID = "task"
	if err := config.Save(cfg, config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "server", "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	scene := `{"SceneConfig":{},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"BytePlus","ProviderParams":{"AccessToken":"speech-secret"}},"LLMConfig":{"Mode":"BytePlusArk","EndPointId":"endpoint","APIKey":"model-secret"},"TTSConfig":{"Provider":"byteplus_Bidirectional_streaming","ProviderParams":{}}},"AgentConfig":{"UserID":"agent"}}}`
	if err := os.WriteFile(filepath.Join(dir, "server", "scenes", "default.json"), []byte(scene), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout := captureCommandStdout(t, func() error { return newAgentStartCmd().Execute() })
	if strings.Contains(stdout, "speech-secret") || strings.Contains(stdout, "model-secret") || !strings.Contains(stdout, "[REDACTED]") {
		t.Fatalf("unexpected preview: %s", stdout)
	}
}

func TestAgentCommandsMatchSupportedLifecycle(t *testing.T) {
	commands := newAgentCmd()
	if command, _, err := commands.Find([]string{"update"}); err == nil && command != commands {
		t.Fatalf("unsupported command remains registered: %s", command.CommandPath())
	}
	for _, name := range []string{"start", "stop", "status", "resources"} {
		command, _, err := commands.Find([]string{name})
		if err != nil || command == commands {
			t.Fatalf("missing agent %s: %v", name, err)
		}
	}
}
