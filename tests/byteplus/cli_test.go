// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package byteplus_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"convo-cli/internal/config"
	"convo-cli/internal/nodeproject"
	"convo-cli/internal/product"
)

func TestStandaloneCLI(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "convo-cli")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	buildArgs := []string{"build", "-o", binary}
	if templateURL := os.Getenv("BYTEPLUS_RTC_DEMO_TEMPLATE_URL"); templateURL != "" {
		checksum := os.Getenv("BYTEPLUS_RTC_DEMO_TEMPLATE_SHA256")
		if checksum == "" {
			t.Fatal("BYTEPLUS_RTC_DEMO_TEMPLATE_SHA256 is required with BYTEPLUS_RTC_DEMO_TEMPLATE_URL")
		}
		buildArgs = append(buildArgs, "-ldflags", fmt.Sprintf("-X convo-cli/internal/template.bytePlusVoiceAgentRemoteURL=%s -X convo-cli/internal/template.bytePlusVoiceAgentRemoteSHA256=%s", templateURL, checksum))
	}
	buildArgs = append(buildArgs, ".")
	build := exec.Command("go", buildArgs...)
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}

	deps := exec.Command("go", "list", "-deps", ".")
	deps.Dir = root
	output, err := deps.CombinedOutput()
	if err != nil {
		t.Fatalf("dependencies: %v\n%s", err, output)
	}
	if strings.Contains(string(output), "internal/defaultskills") {
		t.Fatal("production build includes reference Skill content")
	}
	home := t.TempDir()
	run := func(t *testing.T, args ...string) []byte {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = home
		cmd.Env = append(os.Environ(), "CONVO_CLI_HOME="+home, "CONVO_CLI_STATE_DIR="+home, "CONVO_CLI_NO_UPDATE_NOTIFIER=1", "CONVO_CLI_NO_SKILLS_NOTIFIER=1", "CI=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
		return output
	}
	for _, args := range [][]string{{"--help"}, {"dev", "--help"}, {"auth", "login", "--help"}, {"token", "issue", "--help"}, {"agent", "--help"}, {"agent", "resources", "--help"}, {"docs", "--help"}, {"explain-error", "INVALID_TOKEN", "--format", "json"}, {"explain-error", "1004005", "--format", "json"}, {"explain-error", "SignatureDoesNotMatch", "--format", "json"}, {"skills", "list", "--format", "json"}, {"skills", "read", "byted-bp-convo-guide"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) { run(t, args...) })
	}
	t.Run("resource setup dry run", func(t *testing.T) {
		if err := config.Save(config.Default("resource-selection", "voice-agent", "web"), filepath.Join(home, "byteplus-rtc.config.yaml")); err != nil {
			t.Fatal(err)
		}
		output := run(t, "agent", "resources", "--app-id", "fixture-app", "--write", "--dry-run", "--format", "json")
		var preview struct {
			Data struct {
				Keys        []string `json:"keys"`
				ClearedKeys []string `json:"conditionally_cleared_keys"`
			} `json:"data"`
		}
		if err := json.Unmarshal(output, &preview); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(preview.Data.Keys, ","), "RTC_APP_ID,RTC_APP_KEY") {
			t.Fatal("missing RTC credential preview")
		}
		if strings.Join(preview.Data.ClearedKeys, ",") != "BYTEPLUS_MODEL_API_KEY,BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN" {
			t.Fatalf("credential reset preview: %s", output)
		}
		if _, err := os.Stat(filepath.Join(home, ".env.local")); !os.IsNotExist(err) {
			t.Fatal("dry run wrote configuration")
		}
	})
	version := run(t, "version", "--format", "json")
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(version, &envelope); err != nil || !envelope.OK || envelope.Data.Name != "convo-cli" {
		t.Fatalf("version: %s (%v)", version, err)
	}
	initOutput := run(t, "init", filepath.Join(home, "demo"), "--scene", "voice-agent", "--platform", "web", "--dry-run", "--format", "json")
	if !strings.Contains(string(initOutput), "byteplus-rtc.config.yaml") || !strings.Contains(string(initOutput), "agent resources --write") {
		t.Fatalf("init: %s", initOutput)
	}
	if _, err := os.Stat(filepath.Join(home, "demo")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created project: %v", err)
	}
	run(t, "init", filepath.Join(home, "demo"), "--scene", "voice-agent", "--platform", "web", "--format", "json")
	demo := filepath.Join(home, "demo")
	if !nodeproject.IsAIGCDemo(demo) {
		t.Fatal("generated project is missing the demo package or startup scripts")
	}
	web, err := nodeproject.Read(filepath.Join(demo, "web"))
	if err != nil || web.Dependencies["@byteplus/rtc"] != "~"+product.BytePlusSDKVersion {
		t.Fatalf("generated web SDK dependency: %q (%v)", web.Dependencies["@byteplus/rtc"], err)
	}
	ignore, err := os.ReadFile(filepath.Join(home, "demo", ".gitignore"))
	if err != nil || !strings.Contains(string(ignore), ".convo-cli/") {
		t.Fatalf("project auth ignore: %s (%v)", ignore, err)
	}
}
