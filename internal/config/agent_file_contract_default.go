// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"convo-cli/internal/product"
)

type bytePlusServerSceneValidationContract struct{}

func (bytePlusServerSceneValidationContract) Validate(scene *ServerScene) error {
	profile := product.Current()
	userID, ok := scene.VoiceChat.AgentConfig[profile.AgentUserIDField].(string)
	if !ok || strings.TrimSpace(userID) == "" {
		return fmt.Errorf("VoiceChat.AgentConfig.%s is required", profile.AgentUserIDField)
	}
	var value map[string]any
	if err := json.Unmarshal(scene.VoiceChat.Config, &value); err != nil || value == nil {
		return fmt.Errorf("VoiceChat.Config must be an object")
	}
	asr, ok := value["ASRConfig"].(map[string]any)
	if !ok || asr["Provider"] != "BytePlus" || !isJSONObject(asr["ProviderParams"]) {
		return fmt.Errorf("VoiceChat.Config.ASRConfig must use a BytePlus ProviderParams object")
	}
	llm, ok := value["LLMConfig"].(map[string]any)
	if !ok || llm["Mode"] != "BytePlusArk" {
		return fmt.Errorf("VoiceChat.Config.LLMConfig must use BytePlusArk mode")
	}
	endpointID, _ := llm["EndPointId"].(string)
	apiKey, _ := llm["APIKey"].(string)
	if strings.TrimSpace(endpointID) == "" || strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("VoiceChat.Config.LLMConfig.EndPointId and APIKey are required")
	}
	tts, ok := value["TTSConfig"].(map[string]any)
	if !ok || tts["Provider"] != "byteplus_Bidirectional_streaming" || !isJSONObject(tts["ProviderParams"]) {
		return fmt.Errorf("VoiceChat.Config.TTSConfig must use byteplus_Bidirectional_streaming ProviderParams")
	}
	return nil
}

func init() {
	registerServerSceneValidationContract(product.BytePlusID, bytePlusServerSceneValidationContract{})
}
