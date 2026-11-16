// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeStartVoiceChatRequestRedactsNestedSecretsWithoutMutation(t *testing.T) {
	secret := "must-not-appear"
	req := StartVoiceChatRequest{
		AppID: "app", Config: json.RawMessage(`{"ASRConfig":{"ProviderParams":{"AccessToken":"must-not-appear","SecretKey":"must-not-appear"}},"LLMConfig":{"APIKey":"must-not-appear"},"TTSConfig":{"ProviderParams":{"app":{"token":"must-not-appear"}}}}`),
		AgentConfig: map[string]any{"UserID": "agent", "ServerMessageSignatureForRTS": secret, "RefreshToken": secret},
	}
	got := SanitizeStartVoiceChatRequest(req)
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), secret) || strings.Count(string(raw), redactedValue) < 6 {
		t.Fatalf("sanitized request leaked a secret: %s", raw)
	}
	if !strings.Contains(string(req.Config), secret) || req.AgentConfig["ServerMessageSignatureForRTS"] != secret {
		t.Fatal("sanitizer mutated the invocation request")
	}
}
