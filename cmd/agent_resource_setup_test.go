// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"convo-cli/internal/auth"
	"convo-cli/internal/config"
	projectenv "convo-cli/internal/env"
	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/openapi"
)

func TestResourceSelectorsPreserveDotenvContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.local")
	original := "# RTC and provider settings\r\nRTC_APP_ID=app\r\nRTC_APP_KEY='kept-secret'\r\nBYTEPLUS_MODEL_ENDPOINT_ID=old # previous\r\nCUSTOM_VALUE=keep\r\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeAgentResourceSelectors(path, map[string]string{"BYTEPLUS_MODEL_ENDPOINT_ID": "ep-new", "BYTEPLUS_MODEL_API_KEY_ID": "123"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	text := string(raw)
	for _, part := range []string{"# RTC and provider settings\r\n", "RTC_APP_KEY='kept-secret'\r\n", "CUSTOM_VALUE=keep\r\n", "BYTEPLUS_MODEL_ENDPOINT_ID=ep-new\r\n", "BYTEPLUS_MODEL_API_KEY_ID=123\r\n"} {
		if !strings.Contains(text, part) {
			t.Fatalf("dotenv entry changed: %q", part)
		}
	}
}
func TestResourceSelectionRequiresExplicitConfirmation(t *testing.T) {
	options := []agentResourceOption{{ID: "key-1", Name: "first"}, {ID: "key-2", Name: "second"}}
	var prompt bytes.Buffer
	selected, err := chooseAgentResource(bufio.NewReader(strings.NewReader("2\n")), &prompt, "model API key", "", options, true)
	if err != nil || selected != "key-2" {
		t.Fatalf("selected=%s error=%v", selected, err)
	}
	if _, err := chooseAgentResource(bufio.NewReader(strings.NewReader("")), &prompt, "model API key", "", options, false); err == nil {
		t.Fatal("selection required")
	}
}

func TestResourceSelectionByName(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		options           []agentResourceOption
	}{
		{"numeric name", "1\n", "ep-a", []agentResourceOption{{ID: "ep-a", Name: "Support"}, {ID: "ep-b", Name: "1"}}},
		{"name", "Support\n", "ep-support", []agentResourceOption{{ID: "ep-support", Name: "Support"}}},
		{"number", "1\n", "ep-support", []agentResourceOption{{ID: "ep-support", Name: "Support"}}},
		{"duplicate", "Support\n", "", []agentResourceOption{{ID: "ep-a", Name: "Support"}, {ID: "ep-b", Name: "Support"}}},
		{"duplicate number", "2\n", "ep-b", []agentResourceOption{{ID: "ep-a", Name: "Support"}, {ID: "ep-b", Name: "Support"}}},
		{"unnamed", "1\n", "ep-a", []agentResourceOption{{ID: "ep-a"}}},
		{"blank unnamed", "\n", "", []agentResourceOption{{ID: "ep-a"}}},
		{"invalid", "missing\n", "", []agentResourceOption{{ID: "ep-a", Name: "Support"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var prompt bytes.Buffer
			got, err := chooseAgentResource(bufio.NewReader(strings.NewReader(tc.input)), &prompt, "model service", "", tc.options, true)
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("got=%q err=%v", got, err)
			}
			if tc.name == "name" && strings.Contains(prompt.String(), "ep-support") {
				t.Fatalf("prompt=%s", prompt.String())
			}
			if tc.name == "duplicate number" && !strings.Contains(prompt.String(), "Support (ep-b)") {
				t.Fatalf("prompt=%s", prompt.String())
			}
		})
	}
}

func TestResourceSelectionRecoveryDetails(t *testing.T) {
	for _, tc := range []struct {
		name, selected string
		options        []agentResourceOption
		code           string
	}{
		{"selection", "", []agentResourceOption{{ID: "speech-1", Name: "Support speech"}}, "byteplus-rtc.dev.selection_required"},
		{"empty", "", nil, "byteplus-rtc.agent.missing_config"},
		{"stale", "gone", []agentResourceOption{{ID: "speech-1", Name: "Support speech"}}, "byteplus-rtc.agent.missing_config"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := chooseAgentResource(bufio.NewReader(strings.NewReader("")), io.Discard, "speech application", tc.selected, tc.options, false)
			typed, ok := errs.As(err)
			if !ok || typed.Code != tc.code {
				t.Fatalf("error=%v", err)
			}
			if typed.Details["console_url"] != "https://console.byteplus.com/voice/app" || typed.Details["selection_flag"] != "--speech-app-id" {
				t.Fatalf("details=%v", typed.Details)
			}
			if tc.name == "selection" {
				candidates := typed.Details["candidates"].([]agentResourceOption)
				if candidates[0].DisplayName != "Support speech" || candidates[0].ID != "speech-1" {
					t.Fatalf("candidates=%v", candidates)
				}
			}
		})
	}
}

func resourceFixture(t *testing.T, failAction string, empty bool) {
	t.Helper()
	t.Cleanup(overrideAuthGlobals(t))
	t.Cleanup(auth.SetTokenStoreForTest(auth.NewMemoryTokenStore()))
	t.Setenv("BYTEPLUS_ACCESS_KEY_ID", "fixture-ak")
	t.Setenv("BYTEPLUS_SECRET_ACCESS_KEY", "fixture-sk")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("Action")
		if action == failAction {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"ResponseMetadata":{"Error":{"Code":"AccessDenied","Message":"Permission denied"}}}`)
			return
		}
		if empty {
			fmt.Fprint(w, `{"Result":{"Items":[]},"data":{"applications":[]}}`)
			return
		}
		switch action {
		case "ListEndpoints":
			fmt.Fprint(w, `{"Result":{"Items":[{"Id":"ep-support","Name":"Support","ProjectName":"default"}]}}`)
		case "ListApiKeys":
			fmt.Fprint(w, `{"Result":{"Items":[{"Id":123,"Name":"Support key","ProjectName":"default"}]}}`)
		case "GetRawApiKey":
			fmt.Fprint(w, `{"Result":{"ApiKey":"selected-key"}}`)
		case "ApplicationCredentials":
			fmt.Fprint(w, `{"data":{"credentials":[{"access_token":"selected-speech-secret"}]}}`)
		case "ListApplications":
			fmt.Fprint(w, `{"data":{"applications":[{"appid":"speech-1","name":"Support speech","volc_project_name":"default"},{"appid":"speech-other","name":"Other project","volc_project_name":"other"}]}}`)
		default:
			t.Errorf("unexpected action %s", action)
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	authHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.URL.Scheme, r.URL.Host = endpoint.Scheme, endpoint.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
}

func TestResourceDiscoveryAndSave(t *testing.T) {
	resourceFixture(t, "", false)
	previousTTY := rtcInputIsTerminal
	rtcInputIsTerminal = func(io.Reader) bool { return false }
	t.Cleanup(func() { rtcInputIsTerminal = previousTTY })
	dir := t.TempDir()
	selection := bytePlusResourceSelection{ProjectName: "default"}
	_, err := configureAgentResourceSelectors(context.Background(), nil, dir, selection, strings.NewReader(""), io.Discard)
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.dev.selection_required" {
		t.Fatalf("error=%v", err)
	}
	choices := typed.Details["choices"].(map[string][]agentResourceOption)
	if len(choices["speech_apps"]) != 1 {
		t.Fatalf("choices=%v", choices)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.local")); !os.IsNotExist(err) {
		t.Fatal("selection request wrote configuration")
	}
	selection.ModelEndpointID = choices["model_endpoints"][0].ID
	selection.ModelAPIKeyID = choices["model_api_keys"][0].ID
	selection.SpeechAppID = choices["speech_apps"][0].ID
	_, err = configureAgentResourceSelectors(context.Background(), nil, dir, selection, strings.NewReader(""), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".env.local"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"BYTEPLUS_MODEL_ENDPOINT_ID=ep-support", "BYTEPLUS_MODEL_API_KEY_ID=123", "BYTEPLUS_SEED_SPEECH_APP_ID=speech-1"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %s", want)
		}
	}
	selection.SpeechAppID = "deleted"
	_, err = configureAgentResourceSelectors(context.Background(), nil, dir, selection, strings.NewReader(""), io.Discard)
	if err == nil {
		t.Fatal("stale selection accepted")
	}
	after, _ := os.ReadFile(filepath.Join(dir, ".env.local"))
	if !bytes.Equal(data, after) {
		t.Fatal("failed selection changed configuration")
	}
}

func TestResourceDiscoveryStatusAndGuidance(t *testing.T) {
	for _, tc := range []struct {
		name, fail string
		empty      bool
		status     string
	}{
		{"available", "", false, "available"},
		{"empty", "", true, "empty"},
		{"speech failure", "ListApplications", false, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resourceFixture(t, tc.fail, tc.empty)
			previousFormat := flagFormat
			flagFormat = "json"
			t.Cleanup(func() { flagFormat = previousFormat })
			stdout := captureCommandStdout(t, func() error { return (bytePlusAgentResourceContract{}).Command().Execute() })
			var result struct {
				Data struct {
					Choices   map[string][]agentResourceOption `json:"choices"`
					Discovery map[string]struct {
						Status   string             `json:"status"`
						Guidance agentResourceGuide `json:"guidance"`
					} `json:"discovery"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatal(err)
			}
			if result.Data.Discovery["speech_apps"].Status != tc.status {
				t.Fatalf("output=%s", stdout)
			}
			if result.Data.Discovery["speech_apps"].Guidance.ConsoleURL != "https://console.byteplus.com/voice/app" {
				t.Fatalf("output=%s", stdout)
			}
			if !tc.empty && result.Data.Choices["model_endpoints"][0].DisplayName != "Support" {
				t.Fatalf("output=%s", stdout)
			}
		})
	}
}

func TestResourceQueryFailurePreservesCauseAndDoesNotWrite(t *testing.T) {
	for _, action := range []string{"ListEndpoints", "ListApiKeys", "ListApplications"} {
		t.Run(action, func(t *testing.T) {
			resourceFixture(t, action, false)
			dir := t.TempDir()
			_, err := configureAgentResourceSelectors(context.Background(), nil, dir, bytePlusResourceSelection{ProjectName: "default"}, strings.NewReader(""), io.Discard)
			typed, ok := errs.As(err)
			if !ok || typed.Details["discovery_status"] != "failed" || !strings.Contains(typed.Message, "Permission denied") || !strings.Contains(typed.Hint, "https://docs.byteplus.com/") {
				t.Fatalf("error=%v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, ".env.local")); !os.IsNotExist(err) {
				t.Fatal("failed query wrote configuration")
			}
		})
	}
}

func TestMissingResourcesSuggestsNameSelection(t *testing.T) {
	t.Setenv("BYTEPLUS_MODEL_API_KEY", "")
	t.Setenv("BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN", "")
	_, err := resolveBytePlusResourceCredentials(context.Background(), nil, bytePlusResourceSelection{})
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.agent.missing_config" {
		t.Fatalf("error=%v", err)
	}
	for _, name := range []string{"model service", "model API key", "speech application"} {
		if !strings.Contains(typed.Message, name) {
			t.Fatalf("message=%s", typed.Message)
		}
	}
	if !strings.Contains(typed.Hint, "choose them by name") {
		t.Fatalf("hint=%s", typed.Hint)
	}
}

func TestResourceSelectionErrorIncludesAllSafeChoicesAndRetry(t *testing.T) {
	restore := overrideAuthGlobals(t)
	defer restore()
	restoreStore := auth.SetTokenStoreForTest(auth.NewMemoryTokenStore())
	defer restoreStore()
	t.Setenv("BYTEPLUS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("BYTEPLUS_SECRET_ACCESS_KEY", "test-secret-key")
	authHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body string
		switch req.URL.Query().Get("Action") {
		case "ListEndpoints":
			body = `{"Result":{"Items":[{"Id":"ep-1","Name":"Model"}]}}`
		case "ListApiKeys":
			body = `{"Result":{"Items":[{"Id":123,"Name":"Key","ApiKey":"excluded-secret"}]}}`
		case "ListApplications":
			body = `{"data":{"applications":[{"appid":"speech-1","name":"Speech","volc_project_name":"project-a","token":"excluded-secret"},{"appid":"other","name":"Other","volc_project_name":"project-b"}]}}`
		default:
			t.Fatalf("unexpected API action %s", req.URL.Query().Get("Action"))
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	dir := t.TempDir()
	_, err := configureAgentResourceSelectors(context.Background(), nil, dir, bytePlusResourceSelection{ModelEndpointID: "ep-1", ProjectName: "project-a"}, strings.NewReader(""), io.Discard)
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.dev.selection_required" {
		t.Fatalf("selection error: %v", err)
	}
	raw, err := json.Marshal(typed.Details)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"model_endpoints", "model_api_keys", "speech_apps", "retry_argv", "ep-1", "speech-1", "project-a", "--model-api-key-id"} {
		if !strings.Contains(string(raw), required) {
			t.Errorf("missing %s: %s", required, raw)
		}
	}
	if strings.Contains(string(raw), "excluded-secret") || strings.Contains(string(raw), "project-b") {
		t.Fatalf("unsafe candidates: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.local")); !os.IsNotExist(err) {
		t.Fatalf("selection error wrote dotenv: %v", err)
	}
}

func TestResourceReselectionClearsLocalCredentials(t *testing.T) {
	resourceFixture(t, "", false)
	dir := t.TempDir()
	path := projectenv.Path(dir)
	if err := os.WriteFile(path, []byte("RTC_APP_KEY=rtc-secret\nBYTEPLUS_MODEL_API_KEY=old-key\nBYTEPLUS_SEED_SPEECH_ACCESS_TOKEN=old-speech-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := configureAgentResourceSelectors(context.Background(), nil, dir, bytePlusResourceSelection{ProjectName: "default", ModelEndpointID: "ep-support", ModelAPIKeyID: "123", SpeechAppID: "speech-1"}, strings.NewReader(""), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	values, err := projectenv.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["BYTEPLUS_MODEL_API_KEY"] != "" || values["BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN"] != "" {
		t.Fatal("previous provider credentials remain active")
	}
	if values["RTC_APP_KEY"] != "rtc-secret" {
		t.Fatal("RTC credential changed")
	}
}

func TestSelectedResourcesOverridePreviousProviderCredentials(t *testing.T) {
	resourceFixture(t, "", false)
	t.Setenv("BYTEPLUS_MODEL_API_KEY", "prior-key")
	t.Setenv("BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN", "previous-speech-secret")
	t.Setenv("BYTEPLUS_SEED_SPEECH_APP_ID", "previous-app")
	got, err := resolveBytePlusResourceCredentials(context.Background(), nil, bytePlusResourceSelection{ProjectName: "default", ModelEndpointID: "ep-support", ModelAPIKeyID: "123", SpeechAppID: "speech-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ModelAPIKey != "selected-key" || got.SpeechToken != "selected-speech-secret" {
		t.Fatal("selected resources used previous credentials")
	}
}

func TestResourcePermissionRecovery(t *testing.T) {
	for _, apiErr := range []*openapi.APIError{
		{Action: "ApplicationCredentials", HTTPStatus: 403},
		{Action: "GetRawApiKey", Code: "AccessDenied"},
		{Action: "StartVoiceChat", Code: "AccessDenied.NoPermissionForApp"},
	} {
		typed, ok := errs.As(mapOpenAPIError(apiErr))
		if !ok || !strings.Contains(typed.Hint, "account") || !strings.Contains(typed.Hint, "auth login --force") {
			t.Fatalf("missing account recovery: %v", typed)
		}
	}
}

func TestResourceSaveReportsRestartAndKeepsFailedSelectionAtomic(t *testing.T) {
	resourceFixture(t, "", false)
	dir := t.TempDir()
	t.Chdir(dir)
	if err := config.Save(config.Default("resources", "voice-agent", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	before := []byte("BYTEPLUS_MODEL_API_KEY=prior-key\nBYTEPLUS_SEED_SPEECH_ACCESS_TOKEN=previous-speech-secret\n")
	if err := os.WriteFile(projectenv.Path(dir), before, 0600); err != nil {
		t.Fatal(err)
	}
	previousFormat := flagFormat
	flagFormat = "json"
	t.Cleanup(func() { flagFormat = previousFormat })
	command := func(speech string) error {
		c := (bytePlusAgentResourceContract{}).Command()
		c.SetArgs([]string{"--write", "--model-endpoint-id", "ep-support", "--model-api-key-id", "123", "--speech-app-id", speech})
		return c.Execute()
	}
	if err := command("unavailable"); err == nil {
		t.Fatal("unavailable resource accepted")
	}
	after, err := os.ReadFile(projectenv.Path(dir))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed selection changed credentials")
	}
	stdout := captureCommandStdout(t, func() error { return command("speech-1") })
	var result struct {
		Data struct {
			RestartRequired bool              `json:"restart_required"`
			NextSteps       []string          `json:"next_steps"`
			Resources       map[string]string `json:"resources"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Data.RestartRequired || len(result.Data.NextSteps) != 1 || result.Data.NextSteps[0] != meta.BinName+" dev" || result.Data.Resources["BYTEPLUS_SEED_SPEECH_APP_ID"] != "speech-1" {
		t.Fatalf("unexpected result: %s", stdout)
	}
	if strings.Contains(stdout, "prior-key") || strings.Contains(stdout, "previous-speech-secret") {
		t.Fatal("resource save exposed credentials")
	}
}
