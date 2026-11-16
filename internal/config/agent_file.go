// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ServerScene is the configuration contract shared by the remote voice-agent
// template, doctor, provision, and the agent runtime bridge. AgentConfig.UserID
// is part of the persisted scene contract; only TargetUserID is runtime-only.
type ServerScene struct {
	SceneConfig map[string]any `json:"SceneConfig"`
	VoiceChat   struct {
		Config      json.RawMessage `json:"Config"`
		AgentConfig map[string]any  `json:"AgentConfig"`
	} `json:"VoiceChat"`
}

// ParseServerSceneJSON decodes and validates a scene configuration. The old
// top-level Config + AgentConfig shape is intentionally not accepted.
func ParseServerSceneJSON(raw []byte) (*ServerScene, error) {
	var scene ServerScene
	if err := json.Unmarshal(raw, &scene); err != nil {
		return nil, fmt.Errorf("not valid JSON: %w", err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("must be a JSON object")
	}
	if _, ok := root["SceneConfig"]; !ok || scene.SceneConfig == nil {
		return nil, fmt.Errorf("must contain a SceneConfig object")
	}
	if _, ok := root["VoiceChat"]; !ok {
		return nil, fmt.Errorf("must contain a VoiceChat object")
	}
	if isEmptyRawJSON(scene.VoiceChat.Config) || scene.VoiceChat.AgentConfig == nil {
		return nil, fmt.Errorf("VoiceChat must contain Config and AgentConfig objects")
	}
	if err := currentServerSceneValidationContract().Validate(&scene); err != nil {
		return nil, err
	}
	return &scene, nil
}

// ValidateServerAgentConfigJSON applies the shared scene contract.
func ValidateServerAgentConfigJSON(raw []byte) error {
	_, err := ParseServerSceneJSON(raw)
	return err
}

func isJSONObject(value any) bool {
	_, ok := value.(map[string]any)
	return ok
}

func isEmptyRawJSON(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "" || s == "null"
}
