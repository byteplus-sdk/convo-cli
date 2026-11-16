// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package openapi

import "encoding/json"

// Request and response types for the BytePlus RTC conversational AI API.
// Config carries ASR, TTS, and LLM parameters; AgentConfig carries agent identity.

// StartVoiceChatRequest launches an agent into a room.
type StartVoiceChatRequest struct {
	AppID       string          `json:"AppId"`
	BusinessID  string          `json:"BusinessId,omitempty"`
	RoomID      string          `json:"RoomId"`
	TaskID      string          `json:"TaskId"`
	Config      json.RawMessage `json:"Config,omitempty"`
	AgentConfig map[string]any  `json:"AgentConfig,omitempty"`
}

// StartVoiceChatResult is the StartVoiceChat Result payload.
type StartVoiceChatResult struct {
	TaskID string `json:"TaskId"`
}

// StopVoiceChatRequest ends a running task.
type StopVoiceChatRequest struct {
	AppID  string `json:"AppId"`
	RoomID string `json:"RoomId"`
	TaskID string `json:"TaskId"`
}
