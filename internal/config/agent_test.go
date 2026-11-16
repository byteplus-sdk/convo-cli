// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"
)

const validServerAgentConfig = `{
 "SceneConfig": {"Name": "Default"},
 "VoiceChat": {
  "Config": {
   "ASRConfig": {"Provider": "BytePlus", "ProviderParams": {}},
   "LLMConfig": {"Mode": "BytePlusArk", "EndPointId": "ep-test", "APIKey": "${BYTEPLUS_MODEL_API_KEY}"},
   "TTSConfig": {"Provider": "byteplus_Bidirectional_streaming", "ProviderParams": {}}
  },
  "AgentConfig": {"UserID": "voice_agent"}
 }
}`

func TestValidateServerAgentConfigJSON(t *testing.T) {
	if err := ValidateServerAgentConfigJSON([]byte(validServerAgentConfig)); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for _, test := range []struct{ old, replacement, field string }{
		{`"UserID": "voice_agent"`, `"UserID": ""`, "VoiceChat.AgentConfig.UserID"},
		{`"Provider": "BytePlus"`, `"Provider": "unsupported"`, "ASRConfig"},
		{`"Mode": "BytePlusArk"`, `"Mode": "unsupported"`, "LLMConfig"},
		{`"EndPointId": "ep-test"`, `"EndPointId": ""`, "LLMConfig.EndPointId"},
		{`"APIKey": "${BYTEPLUS_MODEL_API_KEY}"`, `"APIKey": ""`, "APIKey"},
		{`"Provider": "byteplus_Bidirectional_streaming"`, `"Provider": "unsupported"`, "TTSConfig"},
	} {
		raw := strings.Replace(validServerAgentConfig, test.old, test.replacement, 1)
		if err := ValidateServerAgentConfigJSON([]byte(raw)); err == nil || !strings.Contains(err.Error(), test.field) {
			t.Errorf("validation error = %v, want %s", err, test.field)
		}
	}
}

func TestValidateServerAgentConfigJSONDoesNotExposeAPIKey(t *testing.T) {
	sentinel := strings.Repeat("x", 37)
	raw := strings.Replace(validServerAgentConfig, "${BYTEPLUS_MODEL_API_KEY}", sentinel, 1)
	raw = strings.Replace(raw, `"EndPointId": "ep-test"`, `"EndPointId": ""`, 1)
	if err := ValidateServerAgentConfigJSON([]byte(raw)); err == nil || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("validation error = %v", err)
	}
}

func TestVoiceAgentDefaultsSeeded(t *testing.T) {
	c := Default("demo", "voice-agent", "web")
	// TargetUserID is intentionally left empty so it auto-follows rtc.user_id
	// (agent start falls back to rtc.user_id when target is unset).
	if c.Agent.UserID != "voice_agent" || c.Agent.ConfigFile != "server/scenes/default.json" || c.Agent.TargetUserID != "" {
		t.Fatalf("voice-agent defaults not seeded: %+v", c.Agent)
	}
	if c.Features.MCP {
		t.Fatal("voice-agent quickstart must not enable a business capability by default")
	}
	// A non-agent config must NOT get agent defaults.
	vc := Default("demo", "test-scene", "web")
	if vc.Agent.ConfigFile != "" {
		t.Fatalf("non-agent scene should not seed agent config: %+v", vc.Agent)
	}
}

func TestVoiceAgentValidationWarnsOnUnsetConfigFile(t *testing.T) {
	t.Setenv("RTC_APP_ID", "app")
	c := Default("demo", "voice-agent", "web")
	c.Agent.ConfigFile = "" // unset -> runtime falls back to the default scene
	rep := Validate(c)
	// Unset config_file must not be an error: `agent start` still runs via the
	// The default scene fallback remains available, so validation only warns.
	if rep.HasErrors() {
		t.Fatalf("unset config_file should not error; got %+v", rep.Findings)
	}
	found := false
	for _, f := range rep.Findings {
		if f.Field == "agent.config_file" && f.Severity == SevWarn {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected warn on agent.config_file; got %+v", rep.Findings)
	}
}

func TestVoiceAgentValidationPasses(t *testing.T) {
	t.Setenv("RTC_APP_ID", "app")
	c := Default("demo", "voice-agent", "web")
	c.Agent.ConfigSource = AgentConfigSourceDefault
	if rep := Validate(c); rep.HasErrors() {
		t.Fatalf("expected no errors, got %+v", rep.Findings)
	}
}

func TestTargetUserConsistencyWarn(t *testing.T) {
	t.Setenv("RTC_APP_ID", "app")
	c := Default("demo", "voice-agent", "web")
	c.Agent.TargetUserID = "someone-else" // != rtc.user_id (user-01)
	warned := false
	for _, f := range Validate(c).Findings {
		if f.Field == "agent.target_user_id" && f.Severity == SevWarn {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected target_user_id mismatch WARN")
	}
}

func TestAgentFieldSetGet(t *testing.T) {
	c := Default("demo", "voice-agent", "web")
	if err := c.Set("agent.config_file", "custom.json"); err != nil {
		t.Fatal(err)
	}
	v, err := c.Get("agent.config_file")
	if err != nil || v != "custom.json" {
		t.Fatalf("set/get agent field mismatch: %q %v", v, err)
	}
}

func TestAgentConfigSourceValidation(t *testing.T) {
	t.Setenv("RTC_APP_ID", "app")
	for _, source := range []string{"", AgentConfigSourceDefault, "unsupported"} {
		cfg := Default("demo", "voice-agent", "web")
		cfg.Agent.ConfigSource = source
		report := Validate(cfg)
		if report.HasErrors() != (source == "unsupported") {
			t.Errorf("source %q findings = %+v", source, report.Findings)
		}
	}
}
