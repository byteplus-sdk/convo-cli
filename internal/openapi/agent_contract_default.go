// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"encoding/json"

	"convo-cli/internal/product"
)

type bytePlusVoiceChatWireContract struct{}

type bytePlusStartVoiceChatRequest struct {
	AppID       string          `json:"AppId"`
	RoomID      string          `json:"RoomId"`
	TaskID      string          `json:"TaskId"`
	Config      json.RawMessage `json:"Config"`
	AgentConfig map[string]any  `json:"AgentConfig"`
}

type bytePlusStopVoiceChatRequest struct {
	AppID  string `json:"AppId"`
	RoomID string `json:"RoomId"`
	TaskID string `json:"TaskId"`
}

func (bytePlusVoiceChatWireContract) EncodeStart(request StartVoiceChatRequest) any {
	return bytePlusStartVoiceChatRequest{
		AppID: request.AppID, RoomID: request.RoomID, TaskID: request.TaskID,
		Config: request.Config, AgentConfig: request.AgentConfig,
	}
}

func (bytePlusVoiceChatWireContract) EncodeStop(request StopVoiceChatRequest) any {
	return bytePlusStopVoiceChatRequest{AppID: request.AppID, RoomID: request.RoomID, TaskID: request.TaskID}
}

func init() {
	registerVoiceChatWireContract(product.BytePlusID, bytePlusVoiceChatWireContract{})
}
