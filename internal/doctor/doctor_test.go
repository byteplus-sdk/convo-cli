// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"convo-cli/internal/auth"
	"convo-cli/internal/config"
	"convo-cli/internal/meta"
	"convo-cli/internal/product"
)

const validSceneJSON = `{"SceneConfig":{"Name":"Default"},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"BytePlus","ProviderParams":{}},"LLMConfig":{"Mode":"BytePlusArk","EndPointId":"ep-test","APIKey":"model-key"},"TTSConfig":{"Provider":"byteplus_Bidirectional_streaming","ProviderParams":{}}},"AgentConfig":{"UserID":"voice_agent"}}}`

func writeDefaultScene(t *testing.T, dir string, raw string) {
	t.Helper()
	path := filepath.Join(dir, "server", "scenes", "default.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

const validBytePlusSceneJSON = `{"SceneConfig":{"Name":"BytePlus voice agent","ContractVersion":"2025-05-01"},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"BytePlus","ProviderParams":{"Mode":"SeedASR","Language":"en-US","AppId":"${BYTEPLUS_SEED_SPEECH_APP_ID}","AccessToken":"${BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN}","ApiResourceId":"volc.seedasr.sauc.duration","StreamMode":2,"enable_nonstream":true}},"LLMConfig":{"Mode":"BytePlusArk","EndPointId":"${BYTEPLUS_MODEL_ENDPOINT_ID}","APIKey":"${BYTEPLUS_MODEL_API_KEY}"},"TTSConfig":{"Provider":"byteplus_Bidirectional_streaming","ProviderParams":{"app":{"appid":"${BYTEPLUS_SEED_SPEECH_APP_ID}","token":"${BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN}"},"audio":{"voice_type":"en_female_stokie_uranus_bigtts"},"ResourceId":"seed-tts-2.0"}}},"AgentConfig":{"UserID":"voice_agent"}}}`

func writeBytePlusRemoteProject(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"package.json":               `{"scripts":{"dev":"concurrently"}}`,
		"web/package.json":           `{"dependencies":{"@byteplus/rtc":"4.69.3"}}`,
		"web/src/App.tsx":            `export default function App() { return null; }`,
		"server/package.json":        `{"scripts":{"start":"node app.js"}}`,
		"server/app.js":              `console.log("server")`,
		product.BytePlusTaskfileName: "version: 2\nscene: voice-agent\nplatform: web\nsdk:\n  name: \"@byteplus/rtc\"\n  version: \"4.69.3\"\nruntime:\n  topology: web-server\n  agent_control: server\n",
		"server/scenes/default.json": validBytePlusSceneJSON,
	}
	for name, contents := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBytePlusDoctorUsesEnvironmentFallbackAndRemoteSDK(t *testing.T) {
	previous := product.SelectedID
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() { product.SelectedID = previous })
	restoreTokenStore := auth.SetTokenStoreForTest(auth.NewMemoryTokenStore())
	t.Cleanup(restoreTokenStore)
	t.Setenv("RTC_APP_ID", "byteplus-app")
	t.Setenv("RTC_APP_KEY", "")
	t.Setenv("BYTEPLUS_ACCESS_KEY_ID", "byteplus-ak")
	t.Setenv("BYTEPLUS_SECRET_ACCESS_KEY", "byteplus-sk")
	t.Setenv("BYTEPLUS_SEED_SPEECH_APP_ID", "speech-app")
	t.Setenv("BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN", "speech-token")
	t.Setenv("BYTEPLUS_MODEL_ENDPOINT_ID", "model-endpoint")
	t.Setenv("BYTEPLUS_MODEL_API_KEY", "model-key")

	dir := t.TempDir()
	writeBytePlusRemoteProject(t, dir)
	cfg := config.Default("byteplus-demo", "voice-agent", "web")
	checks := RunProject(cfg, config.Path(dir), dir)
	for _, id := range []string{"project.sdk", "project.credential", "project.agent.auth", "project.agent.config"} {
		check, found := findCheck(checks, id)
		if !found || check.Status != PASS {
			t.Errorf("%s = %+v, want PASS", id, check)
		}
	}
}

func findCheck(checks []Check, id string) (Check, bool) {
	for _, c := range checks {
		if c.ID == id {
			return c, true
		}
	}
	return Check{}, false
}

func TestProjectToolchainReportsUnsupportedEngine(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"engines":{"node":">999.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	check, ok := findCheck(projectToolchainChecks(dir), "project.toolchain.node")
	if !ok || check.Status != FAIL || check.Hint == "" {
		t.Fatalf("unsupported engine: %+v", check)
	}
}

func TestAgentChecksAuth(t *testing.T) {
	cfg := config.Default("demo", "voice-agent", "web")
	restore := auth.SetTokenStoreForTest(auth.NewMemoryTokenStore())
	defer restore()

	// No Signin token -> FAIL.
	c, ok := findCheck(agentChecks(cfg, t.TempDir()), "project.agent.auth")
	if !ok || c.Status != FAIL {
		t.Fatalf("expected agent.auth FAIL without token, got %+v", c)
	}

	stsJSON, _ := json.Marshal(auth.STSCredential{AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "SESSION"})
	if err := auth.SaveToken(auth.TokenSet{
		AccessToken:  string(stsJSON),
		RefreshToken: "refresh",
		ClientID:     auth.DefaultLocalClientID,
		ExpiresAt:    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	c, _ = findCheck(agentChecks(cfg, t.TempDir()), "project.agent.auth")
	if c.Status != PASS {
		t.Fatalf("expected agent.auth PASS with STS token, got %+v", c)
	}
}

func TestAgentChecksTargetConsistency(t *testing.T) {
	cfg := config.Default("demo", "voice-agent", "web")
	cfg.Agent.TargetUserID = "someone-else" // != rtc.user_id (user-01)
	c, ok := findCheck(agentChecks(cfg, t.TempDir()), "project.agent.target")
	if !ok || c.Status != WARN {
		t.Fatalf("expected agent.target WARN on mismatch, got %+v", c)
	}
}

func TestVoiceAgentNotInCLITier(t *testing.T) {
	// agent checks belong to the project tier only.
	for _, c := range RunCLI() {
		if c.Level != LevelCLI {
			t.Fatalf("RunCLI produced a non-CLI check: %+v", c)
		}
	}
}

func TestCLIUpdateUnknownIsNotPass(t *testing.T) {
	oldVersion := meta.Version
	meta.Version = "1.0.0"
	t.Cleanup(func() { meta.Version = oldVersion })
	t.Setenv(product.Current().StateDirEnv, t.TempDir())
	t.Setenv("CI", "")
	t.Setenv("CONTINUOUS_INTEGRATION", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITLAB_CI", "")
	t.Setenv("BUILDKITE", "")
	t.Setenv("JENKINS_URL", "")
	t.Setenv("TF_BUILD", "")
	t.Setenv("CIRCLECI", "")
	t.Setenv("TRAVIS", "")
	t.Setenv("TEAMCITY_VERSION", "")
	t.Setenv("CODEBUILD_BUILD_ID", "")
	t.Setenv(product.Current().UpdateNotifierEnv, "")
	check, ok := findCheck(RunCLI(), "cli.update")
	if !ok || check.Status != UNKNOWN {
		t.Fatalf("expected UNKNOWN update evidence, got %+v", check)
	}
	report := summarize([]Check{check})
	if report.Passed != 0 || report.Unknown != 1 || !report.Runnable {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestCLIUpdateSkippedInCI(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	check, ok := findCheck(RunCLI(), "cli.update")
	if !ok || check.Status != SKIP {
		t.Fatalf("expected SKIP update evidence, got %+v", check)
	}
}

func TestServerManagedAgentChecksServerConfigAndCredentials(t *testing.T) {
	t.Setenv("BYTEPLUS_ACCESS_KEY_ID", "AKID")
	t.Setenv("BYTEPLUS_SECRET_ACCESS_KEY", "SECRET")
	dir := t.TempDir()
	writeDefaultScene(t, dir, validSceneJSON)
	checks := serverManagedAgentChecks(config.Default("demo", "voice-agent", "web"), dir)
	if c, ok := findCheck(checks, "project.agent.auth"); !ok || c.Status != PASS {
		t.Fatalf("expected server auth PASS, got %+v", c)
	}
	if c, ok := findCheck(checks, "project.agent.config"); !ok || c.Status != PASS {
		t.Fatalf("expected server config PASS, got %+v", c)
	}
}

func TestServerManagedAgentConfigRejectsIncompleteShape(t *testing.T) {
	dir := t.TempDir()
	writeDefaultScene(t, dir, `{}`)
	check := serverAgentConfigCheck(config.Default("demo", "voice-agent", "web"), dir)
	if check.Status != FAIL || check.Hint == "" {
		t.Fatalf("incomplete server agent config should fail with guidance: %+v", check)
	}
}

func TestServerManagedProjectIssuesTokenAtRuntime(t *testing.T) {
	t.Setenv("RTC_APP_ID", "appid")
	t.Setenv("RTC_APP_KEY", "appkey")
	t.Setenv("BYTEPLUS_ACCESS_KEY_ID", "AKID")
	t.Setenv("BYTEPLUS_SECRET_ACCESS_KEY", "SECRET")
	dir := t.TempDir()
	writeDefaultScene(t, dir, validSceneJSON)
	taskfile := "version: 2\nscene: voice-agent\nplatform: web\nruntime:\n  topology: web-server\n  agent_control: server\n"
	if err := os.WriteFile(filepath.Join(dir, "byteplus-rtc.taskfile.yaml"), []byte(taskfile), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("demo", "voice-agent", "web")
	c, ok := findCheck(RunProject(cfg, "", dir), "project.token")
	if !ok || c.Status != PASS || c.Detail != "issued by the companion server for each session" {
		t.Fatalf("server-managed token check = %+v", c)
	}
}
