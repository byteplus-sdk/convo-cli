// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package tests holds end-to-end tests that build the convo-cli binary and drive it
// as a subprocess against temp projects.
// They assert generated file trees, config validation, doctor items, error-code
// explanation and smoke — and that `init --dry-run` writes nothing.
package tests

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"convo-cli/internal/config"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "convo-cli-e2e-*")
	if err != nil {
		panic(err)
	}
	binName := "convo-cli"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath = filepath.Join(dir, binName)
	archive, err := remoteVoiceAgentArchive()
	if err != nil {
		panic("build remote template fixture: " + err.Error())
	}
	archiveSHA256 := sha256.Sum256(archive)
	templateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	oauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("grant_type") != "authorization_code" || r.PostForm.Get("code_verifier") == "" {
			http.Error(w, "invalid token request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "e2e-access",
			"refresh_token": "e2e-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	topicDocsServer := httptest.NewServer(http.HandlerFunc(serveTopicDocsFixture))
	if err := os.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache")); err != nil {
		panic("set template cache: " + err.Error())
	}
	// Build from the module root (parent of this tests/ dir).
	linkerFlags := fmt.Sprintf(
		"-X convo-cli/internal/template.bytePlusVoiceAgentRemoteURL=%s -X convo-cli/internal/template.bytePlusVoiceAgentRemoteSHA256=%x -X convo-cli/cmd.oauthAuthorizeEndpoint=%s/authorize -X convo-cli/cmd.oauthTokenEndpoint=%s/token -X convo-cli/internal/topicdocs.e2eEndpoint=%s",
		templateServer.URL,
		archiveSHA256,
		oauthServer.URL,
		oauthServer.URL,
		topicDocsServer.URL,
	)
	cmd := exec.Command("go", "build", "-tags", "topicdocs_e2e", "-ldflags", linkerFlags, "-o", binPath, ".")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("build convo-cli: " + err.Error() + "\n" + string(out))
	}
	// Darwin's os.UserCacheDir uses HOME/Library/Caches rather than
	// XDG_CACHE_HOME. Isolate both locations before driving the binary.
	if err := os.Setenv("HOME", dir); err != nil {
		panic("set test home: " + err.Error())
	}
	code := m.Run()
	templateServer.Close()
	oauthServer.Close()
	topicDocsServer.Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func serveTopicDocsFixture(w http.ResponseWriter, r *http.Request) {
	allowedUserAgent := regexp.MustCompile(`^convo-cli/0\.0\.1-dev invocation/(?:direct caller/e2e|skill caller/e2e skill/byted-bp-convo-guide#0\.0\.4) invocation-id/[0-9a-f-]{36}$`)
	if !allowedUserAgent.MatchString(r.UserAgent()) {
		http.Error(w, "unexpected user agent", http.StatusForbidden)
		return
	}
	if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-App-Key") != "" {
		http.Error(w, "forwarded credential", http.StatusBadRequest)
		return
	}
	var request struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
		Params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "malformed request", http.StatusBadRequest)
		return
	}
	if request.Method == "notifications/initialized" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var result any
	switch request.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": "2025-03-26",
			"serverInfo":      map[string]string{"name": "e2e-topic-docs", "version": "1.0.0"},
		}
	case "tools/list":
		result = map[string]any{"tools": []any{
			map[string]any{"name": "search_docs", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]string{"type": "string"}}, "additionalProperties": false}},
			map[string]any{"name": "fetch_doc", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]string{"type": "string"}}, "additionalProperties": false}},
			map[string]any{"name": "list_docs", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}},
		}}
	case "tools/call":
		text := ""
		isError := false
		switch request.Params.Name {
		case "search_docs":
			if request.Params.Arguments["query"] == "force-error" {
				text, isError = "fixture failure", true
			} else {
				text = `[{"id":"rtc/audio","score":0.95,"highlight":{"title":["<hl>Audio</hl> Guide"]}},{"id":"rtc/video","score":0.75,"highlight":{"content":["Video <hl>publish</hl>"]}}]`
			}
		case "fetch_doc":
			text = "# Fixture RTC Document\n\nExact markdown.  \n"
		case "list_docs":
			text = "- [Audio Guide](rtc/audio): Publish audio\n- [Video Guide](rtc/video): Publish video\n"
		default:
			text, isError = "unknown fixture tool", true
		}
		result = map[string]any{"content": []any{map[string]string{"type": "text", "text": text}}, "isError": isError}
	default:
		http.Error(w, "unexpected method", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
}

func remoteVoiceAgentArchive() ([]byte, error) {
	files := map[string]string{
		".env.example":                 "RTC_APP_ID=\nRTC_APP_KEY=\n",
		"package.json":                 `{"scripts":{"dev":"yarn --cwd server dev & yarn --cwd web start"}}`,
		"web/package.json":             `{"scripts":{"start":"echo web"}}`,
		"web/src/App.tsx":              "export default function App() { return null; }\n",
		"web/src/store/slices/room.ts": "BusinessId?: string;\n",
		"web/src/lib/useCommon.ts":     "await RtcClient.createEngine();\nif (rtc.BusinessId) { RtcClient.setBusinessId(rtc.BusinessId); }\nawait RtcClient.joinRoom();\n",
		"server/package.json":          `{"scripts":{"dev":"node app.js"}}`,
		"server/app.js":                "BYTEPLUS_RTC_CLI_PATH BYTEPLUS_RTC_PROJECT_DIR --runtime-request-stdin GetToken 2025-06-01 2025-05-01 BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN BYTEPLUS_MODEL_API_KEY\n businessId: env.BYTEPLUS_RTC_BUSINESS_ID?.trim() || undefined;\nopenApiUserAgent: env.BYTEPLUS_RTC_OPENAPI_USER_AGENT?.trim() || undefined;\nBusinessId: config.businessId,\nBusinessId: config.businessId,\nrequestData.headers[\"User-Agent\"] = config.openApiUserAgent;\nconst signer = new Signer(requestData, config.service);\n",
		"server/scenes/default.json":   `{"SceneConfig":{"Name":"Default"},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"BytePlus","ProviderParams":{}},"LLMConfig":{"Mode":"BytePlusArk","EndPointId":"ep-test","APIKey":"test-api-key"},"TTSConfig":{"Provider":"byteplus_Bidirectional_streaming","ProviderParams":{}}},"AgentConfig":{"UserID":"voice_agent","EnableConversationStateCallback":true}}}`,
		"byteplus-rtc.taskfile.yaml":   "version: 2\nscene: voice-agent\nplatform: web\nsdk:\n  name: '@byteplus/rtc'\n  version: '4.69.3'\nruntime:\n  topology: web-server\n  agent_control: server\n  ports:\n    web: 3000\n    server: 3001\ntasks:\n  dev:\n    - yarn dev\n",
		"byteplus-rtc.template.yaml":   "version: 1\nscene: voice-agent\nplatform: web\nagent_config: server/scenes/default.json\nrequired_files:\n  - package.json\n  - web/package.json\n  - web/src/App.tsx\n  - server/package.json\n  - server/app.js\n  - server/scenes/default.json\n  - byteplus-rtc.taskfile.yaml\n",
	}
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag:   tar.TypeXGlobalHeader,
		PAXRecords: map[string]string{"comment": "codeload fixture"},
	}); err != nil {
		return nil, err
	}
	for name, content := range files {
		body := []byte(content)
		if err := tw.WriteHeader(&tar.Header{
			Name:     "rtc-aigc-demo-fixture/" + name,
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(body); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

type result struct {
	stdout string
	stderr string
	code   int
}

// run executes convo-cli with args in dir, with extra env KEY=VAL entries.
func run(t *testing.T, dir string, env []string, args ...string) result {
	return runInput(t, dir, env, "", args...)
}

func runInput(t *testing.T, dir string, env []string, input string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(input)
	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return result{stdout: out.String(), stderr: errBuf.String(), code: code}
}

// envelope parses the JSON envelope from stdout (last JSON object).
func (r result) envelope(t *testing.T) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(r.stdout))
	var last map[string]any
	for {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			break
		}
		last = m
	}
	if last == nil {
		t.Fatalf("no JSON envelope in stdout: %q", r.stdout)
	}
	return last
}

func TestAuthLoginHelpExposesConsentAndStoreFlags(t *testing.T) {
	r := run(t, t.TempDir(), nil, "auth", "login", "--help")
	if r.code != 0 {
		t.Fatalf("auth login help exit %d: %s", r.code, r.stderr)
	}
	for _, flag := range []string{"--browser", "--store", "--force"} {
		if !strings.Contains(r.stdout, flag) {
			t.Fatalf("auth login help does not expose %s:\n%s", flag, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "--no-browser") {
		t.Fatalf("auth login help exposes removed --no-browser:\n%s", r.stdout)
	}
}

func TestAuthLoginManualRequiresSupportedAuthorization(t *testing.T) {
	r := run(t, t.TempDir(), nil, "auth", "login", "--browser=manual")
	if r.code == 0 || !strings.Contains(r.stdout, "byteplus-rtc.cli.invalid_flag") {
		t.Fatalf("manual login result: code=%d stdout=%s stderr=%s", r.code, r.stdout, r.stderr)
	}
}

func TestInitDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	r := run(t, dir, nil, "init", "--scene", "voice-agent", "--platform", "web", "--name", "demo", "--dry-run")
	if r.code != 0 {
		t.Fatalf("dry-run exit %d: %s", r.code, r.stderr)
	}
	env := r.envelope(t)
	if env["ok"] != true {
		t.Fatalf("expected ok=true: %v", env)
	}
	// Nothing should be written.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("dry-run wrote files: %v", entries)
	}
}

func TestInitGeneratesTree(t *testing.T) {
	dir := t.TempDir()
	r := run(t, dir, nil, "init", "./demo", "--scene", "voice-agent", "--platform", "web")
	if r.code != 0 {
		t.Fatalf("init exit %d: %s", r.code, r.stderr)
	}
	for _, f := range []string{"byteplus-rtc.config.yaml", "package.json", "web/src/App.tsx", "server/app.js", "byteplus-rtc.taskfile.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, "demo", f)); err != nil {
			t.Fatalf("expected %s: %v", f, err)
		}
	}
}

func TestInitUnknownTemplate(t *testing.T) {
	dir := t.TempDir()
	r := run(t, dir, nil, "init", "--scene", "nope", "--platform", "web")
	if r.code == 0 {
		t.Fatal("expected non-zero exit for unknown template")
	}
	env := r.envelope(t)
	errObj, _ := env["error"].(map[string]any)
	if errObj["code"] != "byteplus-rtc.template.not_found" {
		t.Fatalf("expected template.not_found, got %v", env)
	}
}

func TestConfigValidateFailsOnUnresolvedEnv(t *testing.T) {
	dir := setupProject(t)
	r := run(t, dir, []string{"RTC_APP_ID="}, "config", "validate")
	if r.code == 0 {
		t.Fatal("expected validation failure (unresolved ${RTC_APP_ID})")
	}
}

func TestExplainErrorKnownAndUnknown(t *testing.T) {
	dir := t.TempDir()
	// Known string code.
	r := run(t, dir, nil, "explain-error", "INVALID_TOKEN")
	if r.code != 0 {
		t.Fatalf("known code exit %d", r.code)
	}
	env := r.envelope(t)
	data, _ := env["data"].(map[string]any)
	if data["found"] != true || data["doctor_check"] != "token.valid" {
		t.Fatalf("unexpected explain data: %v", data)
	}
	// Unknown code → non-zero, unknown_code.
	r = run(t, dir, nil, "explain-error", "TOTALLY_MADE_UP")
	if r.code == 0 {
		t.Fatal("expected non-zero for unknown code")
	}
	// A negative code must remain positional after the argument terminator.
	r = run(t, dir, nil, "explain-error", "--format", "json", "--", "-1000")
	if r.code == 0 {
		t.Fatal("expected unknown negative code to return non-zero")
	}
	data, _ = r.envelope(t)["data"].(map[string]any)
	if data["query"] != "-1000" || data["found"] != false {
		t.Fatalf("negative code was not parsed as data: %v", data)
	}
	if hint, _ := data["hint"].(string); !strings.Contains(hint, `convo-cli docs search -- '-1000'`) || !strings.Contains(hint, "convo-cli doctor") {
		t.Fatalf("negative-code recovery hint is incomplete: %q", hint)
	}

	help := run(t, dir, nil, "explain-error", "--help")
	if help.code != 0 || !strings.Contains(help.stdout, "convo-cli explain-error -- -1000") {
		t.Fatalf("explain-error help omits negative-code syntax: code=%d stdout=%q stderr=%q", help.code, help.stdout, help.stderr)
	}
}

func TestFullFirstRunLoop(t *testing.T) {
	dir := setupProject(t)
	env := []string{"RTC_APP_KEY=secretkeyvalue12345", "RTC_APP_ID=app123456789012345678901"} // public-scan: allow; gitleaks:allow — synthetic test credentials

	// Fill app_id with a concrete value so ${ENV} resolves.
	if r := run(t, dir, env, "config", "set", "rtc.app_id", "app123456789012345678901"); r.code != 0 {
		t.Fatalf("config set: %s", r.stderr)
	}

	// doctor before token → FAIL (token missing).
	if r := run(t, dir, env, "doctor", "project"); r.code == 0 {
		t.Fatal("expected doctor FAIL before token issue")
	}

	// token issue --write.
	if r := run(t, dir, env, "token", "issue", "--write"); r.code != 0 {
		t.Fatalf("token issue: %s", r.stderr)
	}

	// --write must sync the token into .env.local so the web runtime (which
	// reads VITE_RTC_TOKEN) sees it — not just into byteplus-rtc.config.yaml.
	envBytes, err := os.ReadFile(filepath.Join(dir, ".env.local"))
	if err != nil {
		t.Fatalf("read .env.local: %v", err)
	}
	if !strings.Contains(string(envBytes), "VITE_RTC_TOKEN=001") {
		t.Fatalf(".env.local missing VITE_RTC_TOKEN after issue --write:\n%s", envBytes)
	}

	// doctor now PASS.
	if r := run(t, dir, env, "doctor"); r.code != 0 {
		t.Fatalf("expected doctor PASS after token, exit %d: %s", r.code, r.stderr)
	}

	// token check valid.
	if r := run(t, dir, env, "token", "check"); r.code != 0 {
		t.Fatalf("token check should pass: %s", r.stderr)
	}

	// project readiness passes (folds in the former `test smoke` checks).
	if r := run(t, dir, env, "doctor", "project"); r.code != 0 {
		t.Fatalf("doctor project should pass: %s", r.stderr)
	}
}

// setupProject creates a minimal non-agent project fixture for command tests.
// It deliberately does not retain a scene that is absent from the published catalog.
func setupProject(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	if err := config.Save(config.Default("demo", "test-scene", "web"), config.Path(base)); err != nil {
		t.Fatalf("save fixture config: %v", err)
	}
	taskfile := "version: 1\nscene: test-scene\nplatform: web\ntasks:\n  dev:\n    - 'true'\n"
	if err := os.WriteFile(filepath.Join(base, "byteplus-rtc.taskfile.yaml"), []byte(taskfile), 0o644); err != nil {
		t.Fatalf("write fixture taskfile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write fixture package: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(base, "src"), 0o755); err != nil {
		t.Fatalf("create fixture source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "src", "main.js"), []byte("export {};\n"), 0o644); err != nil {
		t.Fatalf("write fixture source: %v", err)
	}
	return base
}

func TestAgentPreflightAndResourceSetupDryRun(t *testing.T) {
	dir := t.TempDir()
	if err := config.Save(config.Default("preflight", "voice-agent", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	preview := run(t, dir, nil, "agent", "resources", "--write", "--dry-run", "--format", "json")
	if preview.code != 0 {
		t.Fatalf("resource preview: %s", preview.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.local")); !os.IsNotExist(err) {
		t.Fatal("resource dry-run wrote dotenv")
	}
	request := `{"Config":{"ASRConfig":{"Provider":"Google","ProviderParams":{}},"LLMConfig":{"Provider":"OpenAI"},"TTSConfig":{"Provider":"OpenAI","ProviderParams":{}},"AvatarConfig":{"Enabled":true,"Provider":"Akool","ProviderParams":{"AvatarId":"avatar"}}}}`
	env := []string{"OPENAI_API_KEY=fixture-openai", "GOOGLE_CREDENTIALS_JSON={}", "AKOOL_API_KEY="}
	rejected := runInput(t, dir, env, request, "agent", "check", "--runtime-request-stdin", "--format", "json")
	if rejected.code == 0 || !strings.Contains(rejected.stdout, "AKOOL_API_KEY") {
		t.Fatalf("missing avatar check: %s", rejected.stdout)
	}
	env = append(env, "AKOOL_API_KEY=fixture-akool")
	accepted := runInput(t, dir, env, request, "agent", "check", "--runtime-request-stdin", "--format", "json")
	if accepted.code != 0 || !strings.Contains(accepted.stdout, `"ready": true`) {
		t.Fatalf("preflight: %s %s", accepted.stdout, accepted.stderr)
	}
	for _, secret := range []string{"fixture-openai", "fixture-akool"} {
		if strings.Contains(accepted.stdout, secret) {
			t.Fatal("preflight exposed a credential")
		}
	}
}
