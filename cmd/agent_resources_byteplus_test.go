// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"convo-cli/internal/auth"
	"convo-cli/internal/openapi"
)

func TestBytePlusAgentResourceDryRunHasNoPrerequisiteOrNetworkWork(t *testing.T) {
	contract := bytePlusAgentResourceContract{}
	cmd := &cobra.Command{Use: "start"}
	contract.ConfigureStart(cmd)
	cleanup, err := contract.PrepareStart(context.Background(), nil, cmd, true)
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
}

func TestBytePlusRuntimeStartInjectsCredentialsInsideCLI(t *testing.T) {
	t.Setenv("BYTEPLUS_MODEL_ENDPOINT_ID", "endpoint-1")
	t.Setenv("BYTEPLUS_MODEL_API_KEY", "model-secret")
	t.Setenv("BYTEPLUS_SEED_SPEECH_APP_ID", "speech-1")
	t.Setenv("BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN", "speech-secret")
	req := openapi.StartVoiceChatRequest{Config: json.RawMessage(`{
		"ASRConfig":{"Provider":"BytePlus","ProviderParams":{}},
		"LLMConfig":{"Mode":"BytePlusArk"},
		"TTSConfig":{"Provider":"byteplus_Bidirectional_streaming","ProviderParams":{"ResourceId":"seed-tts-2.0"}}
	}`)}
	if err := (bytePlusAgentResourceContract{}).PrepareRuntimeStart(context.Background(), nil, &req, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(req.Config, &got); err != nil {
		t.Fatal(err)
	}
	asr := got["ASRConfig"].(map[string]any)["ProviderParams"].(map[string]any)
	llm := got["LLMConfig"].(map[string]any)
	ttsParams := got["TTSConfig"].(map[string]any)["ProviderParams"].(map[string]any)
	tts := ttsParams["app"].(map[string]any)
	if ttsParams["ResourceId"] != "seed-tts-2.0" {
		t.Fatalf("runtime TTS resource changed: %s", req.Config)
	}
	if asr["AccessToken"] != "speech-secret" || llm["APIKey"] != "model-secret" || tts["token"] != "speech-secret" {
		t.Fatalf("runtime provider credentials were not injected: %s", req.Config)
	}
}

func TestBytePlusAgentResourceStartAcceptsLocalSecretsWithoutResourceLookup(t *testing.T) {
	t.Setenv("BYTEPLUS_MODEL_ENDPOINT_ID", "endpoint-1")
	t.Setenv("BYTEPLUS_MODEL_API_KEY", "model-secret")
	t.Setenv("BYTEPLUS_SEED_SPEECH_APP_ID", "speech-1")
	t.Setenv("BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN", "speech-secret")

	contract := bytePlusAgentResourceContract{}
	cmd := &cobra.Command{Use: "start"}
	contract.ConfigureStart(cmd)
	cleanup, err := contract.PrepareStart(context.Background(), nil, cmd, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("BYTEPLUS_MODEL_API_KEY"); got != "model-secret" {
		t.Fatalf("BYTEPLUS_MODEL_API_KEY = %q", got)
	}
	cleanup()
}

func TestBytePlusRuntimeStartPreservesDemoProviderSelection(t *testing.T) {
	t.Setenv("BYTEPLUS_MODEL_ENDPOINT_ID", "env-endpoint")
	t.Setenv("BYTEPLUS_MODEL_API_KEY", "model-secret")
	t.Setenv("BYTEPLUS_SEED_SPEECH_APP_ID", "speech-1")
	t.Setenv("BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN", "speech-secret")
	req := openapi.StartVoiceChatRequest{Config: json.RawMessage(`{"LLMConfig":{"Provider":"BytePlus","EndPointId":"selected-endpoint"},"TTSConfig":{"Provider":"BytePlus","ProviderParams":{"ResourceId":"seed-tts-2.0","audio":{"voice_type":"selected-voice"}}}}`)}
	if err := (bytePlusAgentResourceContract{}).PrepareRuntimeStart(context.Background(), nil, &req, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(req.Config, &got); err != nil {
		t.Fatal(err)
	}
	llm := got["LLMConfig"].(map[string]any)
	params := got["TTSConfig"].(map[string]any)["ProviderParams"].(map[string]any)
	app, _ := params["app"].(map[string]any)
	if llm["EndPointId"] != "selected-endpoint" || llm["APIKey"] != "model-secret" || app["token"] != "speech-secret" || params["ResourceId"] != "seed-tts-2.0" {
		t.Fatal("Demo provider credentials or selected resource were not preserved")
	}
}

func TestRuntimeOpenAIProvidersDoNotRequireBytePlusResources(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-secret")
	t.Setenv("AKOOL_API_KEY", "akool-secret")
	req := openapi.StartVoiceChatRequest{Config: json.RawMessage(`{"LLMConfig":{"Provider":"OpenAI","ModelName":"gpt-4o"},"TTSConfig":{"Provider":"OpenAI","ProviderParams":{"Voice":"alloy"}},"AvatarConfig":{"Enabled":true,"Provider":"Akool","ProviderParams":{"AvatarId":"selected-avatar"}}}`)}
	if err := (bytePlusAgentResourceContract{}).PrepareRuntimeStart(context.Background(), nil, &req, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(req.Config, &got); err != nil {
		t.Fatal(err)
	}
	if got["LLMConfig"].(map[string]any)["APIKey"] != "openai-secret" {
		t.Fatal("OpenAI key missing")
	}
	avatar := got["AvatarConfig"].(map[string]any)["ProviderParams"].(map[string]any)
	if avatar["ApiKey"] != "akool-secret" || avatar["AvatarId"] != "selected-avatar" {
		t.Fatal("Akool credentials missing")
	}
}

func TestRuntimeMissingAvatarCredentialNamesTheField(t *testing.T) {
	t.Setenv("AKOOL_API_KEY", "")
	req := openapi.StartVoiceChatRequest{Config: json.RawMessage(`{"AvatarConfig":{"Enabled":true,"Provider":"Akool","ProviderParams":{"AvatarId":"avatar"}}}`)}
	err := (bytePlusAgentResourceContract{}).PrepareRuntimeStart(context.Background(), nil, &req, false)
	if err == nil || !strings.Contains(err.Error(), "AKOOL_API_KEY") {
		t.Fatalf("error=%v", err)
	}
}

func TestAgentPreflightPreservesFlagSelectedResourceContext(t *testing.T) {
	t.Cleanup(overrideAuthGlobals(t))
	t.Cleanup(auth.SetTokenStoreForTest(auth.NewMemoryTokenStore()))
	t.Setenv("BYTEPLUS_ACCESS_KEY_ID", "fixture-ak")
	t.Setenv("BYTEPLUS_SECRET_ACCESS_KEY", "fixture-sk")
	previous := map[string]string{
		"BYTEPLUS_MODEL_ENDPOINT_ID":        "old-endpoint",
		"BYTEPLUS_MODEL_API_KEY_ID":         "11",
		"BYTEPLUS_PROJECT_NAME":             "old-project",
		"BYTEPLUS_MODEL_API_KEY":            "old-key",
		"BYTEPLUS_SEED_SPEECH_APP_ID":       "old-speech",
		"BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN": "old-speech-secret",
	}
	for key, value := range previous {
		t.Setenv(key, value)
	}
	modelCalls, speechCalls := 0, 0
	authHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body string
		switch req.URL.Query().Get("Action") {
		case "GetRawApiKey":
			modelCalls++
			var request struct {
				ID      int    `json:"Id"`
				Project string `json:"ProjectName"`
			}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.ID != 22 || request.Project != "new-project" {
				t.Errorf("credential lookup used key %d in project %q", request.ID, request.Project)
			}
			body = `{"Result":{"ApiKey":"new-key"}}`
		case "ApplicationCredentials":
			speechCalls++
			if req.URL.Query().Get("AppID") != "new-speech" {
				t.Error("credential lookup used previous speech application")
			}
			body = `{"data":{"credentials":[{"access_token":"new-speech-secret"}]}}`
		default:
			t.Fatalf("unexpected action %q", req.URL.Query().Get("Action"))
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	contract := bytePlusAgentResourceContract{}
	cmd := &cobra.Command{Use: "check"}
	contract.ConfigureStart(cmd)
	for key, value := range map[string]string{bytePlusModelEndpointFlag: "new-endpoint", bytePlusModelAPIKeyFlag: "22", bytePlusSpeechAppFlag: "new-speech", bytePlusProjectFlag: "new-project"} {
		if err := cmd.Flags().Set(key, value); err != nil {
			t.Fatal(err)
		}
	}
	cleanup, err := contract.PrepareStart(context.Background(), nil, cmd, false)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	req := openapi.StartVoiceChatRequest{Config: json.RawMessage(`{"LLMConfig":{"Provider":"BytePlus"},"ASRConfig":{"Provider":"BytePlus","ProviderParams":{}}}`)}
	if err := contract.PrepareRuntimeStart(context.Background(), nil, &req, false); err != nil {
		t.Fatal(err)
	}
	var got struct {
		LLM struct {
			Endpoint string `json:"EndPointId"`
			Key      string `json:"APIKey"`
		} `json:"LLMConfig"`
		ASR struct {
			Params struct {
				App   string `json:"AppId"`
				Token string `json:"AccessToken"`
			} `json:"ProviderParams"`
		} `json:"ASRConfig"`
	}
	if err := json.Unmarshal(req.Config, &got); err != nil {
		t.Fatal(err)
	}
	if got.LLM.Endpoint != "new-endpoint" || got.LLM.Key != "new-key" || got.ASR.Params.App != "new-speech" || got.ASR.Params.Token != "new-speech-secret" {
		t.Fatal("preflight lost the selected provider configuration")
	}
	if modelCalls != 2 || speechCalls != 1 {
		t.Fatalf("credential lookups: model=%d speech=%d", modelCalls, speechCalls)
	}
	cleanup()
	for key, value := range previous {
		if os.Getenv(key) != value {
			t.Errorf("%s was not restored", key)
		}
	}
}
