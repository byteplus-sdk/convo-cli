// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import "testing"

func TestRuntimeGatewayProviders(t *testing.T) {
	for _, module := range []string{"ASR", "TTS"} {
		t.Run(module, func(t *testing.T) {
			kind := module + "Config"
			address := "wss://gateway.example/" + module
			t.Setenv(module+"_GATEWAY_API_URL", address)
			t.Setenv(module+"_GATEWAY_API_KEY", "test-key")
			params := map[string]any{"ExtraHeader": map[string]any{"locale": "en"}}
			if module == "TTS" {
				params["Voice"] = "selected"
				params["OutputAudioVolume"] = float64(0)
			}
			voice := map[string]any{kind: map[string]any{"Provider": "ai_gateway", "ProviderParams": params}}
			if err := hydrateOtherRuntimeProviders(voice, false); err != nil {
				t.Fatal(err)
			}
			if params["URL"] != address || params["APIKey"] != "test-key" || params["ExtraHeader"].(map[string]any)["locale"] != "en" {
				t.Fatal("missing gateway credentials or session headers")
			}
		})
	}
}

func TestRuntimeGatewayRejectsInvalidSettings(t *testing.T) {
	for _, tc := range []struct {
		name, module, address, key string
		params                     any
	}{
		{"invalid params", "ASR", "wss://example.org", "test-key", "invalid"},
		{"unexpected field", "ASR", "wss://example.org", "test-key", map[string]any{"URL": "wss://custom.example"}},
		{"invalid headers", "ASR", "wss://example.org", "test-key", map[string]any{"ExtraHeader": "invalid"}},
		{"missing voice", "TTS", "wss://example.org", "test-key", map[string]any{}},
		{"invalid volume", "TTS", "wss://example.org", "test-key", map[string]any{"Voice": "selected", "OutputAudioVolume": "loud"}},
		{"insecure URL", "ASR", "ws://example.org", "test-key", map[string]any{}},
		{"missing key", "ASR", "wss://example.org", "", map[string]any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.module+"_GATEWAY_API_URL", tc.address)
			t.Setenv(tc.module+"_GATEWAY_API_KEY", tc.key)
			voice := map[string]any{tc.module + "Config": map[string]any{"Provider": "ai_gateway", "ProviderParams": tc.params}}
			if err := hydrateOtherRuntimeProviders(voice, false); err == nil {
				t.Fatal("expected invalid gateway settings to fail")
			}
		})
	}
}

func TestRuntimeLLMCredentialFollowsEndpoint(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-test")
	t.Setenv("GOOGLE_LLM_API_KEY", "google-test")
	t.Setenv("CUSTOM_LLM_URL", "https://llm.example/api")
	for _, tc := range []struct {
		name            string
		config          map[string]any
		customKey, want string
	}{
		{"implicit OpenAI", map[string]any{"Provider": "OpenAI"}, "", "openai-test"},
		{"explicit OpenAI", map[string]any{"Mode": "CustomLLM", "Url": "https://api.openai.com/v1/chat/completions"}, "", "openai-test"},
		{"explicit Google", map[string]any{"Mode": "CustomLLM", "Url": "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"}, "", "google-test"},
		{"custom model", map[string]any{"Mode": "CustomLLM", "Url": "https://llm.example/api"}, "custom-test", "custom-test"},
		{"unconfigured destination", map[string]any{"Mode": "CustomLLM", "Url": "https://other.example/api"}, "custom-test", ""},
		{"missing custom key", map[string]any{"Mode": "CustomLLM", "Url": "https://llm.example/api"}, "", ""},
		{"OpenAI with custom origin", map[string]any{"Provider": "OpenAI", "Url": "https://llm.example/api"}, "", ""},
		{"BytePlus endpoint override", map[string]any{"Provider": "BytePlus", "Url": "https://llm.example/api"}, "", ""},
		{"noncanonical URL", map[string]any{"Provider": "OpenAI", "URL": "https://llm.example/api"}, "", ""},
		{"custom port", map[string]any{"Provider": "OpenAI", "Url": "https://api.openai.com:9443/api"}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CUSTOM_LLM_API_KEY", tc.customKey)
			err := hydrateOtherRuntimeProviders(map[string]any{"LLMConfig": tc.config}, false)
			if tc.want == "" {
				if err == nil || tc.config["APIKey"] != nil {
					t.Fatal("unexpected credential for endpoint")
				}
			} else if err != nil || tc.config["APIKey"] != tc.want {
				t.Fatal("failed to select configured endpoint credential")
			}
		})
	}
}

func TestLocalScenePreflightKeepsCustomKeyFallback(t *testing.T) {
	t.Setenv("CUSTOM_LLM_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "local-test")
	llm := map[string]any{"Mode": "CustomLLM", "Url": "https://llm.example/api"}
	if err := hydrateOtherRuntimeProviders(map[string]any{"LLMConfig": llm}, true); err != nil || llm["APIKey"] != "local-test" {
		t.Fatal("unexpected local scene credential")
	}
}
