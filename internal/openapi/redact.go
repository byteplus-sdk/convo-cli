// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"encoding/json"
	"strings"
)

const redactedValue = "[REDACTED]"

var sensitiveJSONKeys = map[string]bool{
	"accesskeyid": true, "secretaccesskey": true, "secretkey": true, "sessiontoken": true,
	"access_token": true, "accesstoken": true, "refreshtoken": true, "idtoken": true, "signintoken": true, "token": true,
	"apikey": true, "appkey": true, "appsecret": true, "clientsecret": true, "secret": true, "credentials": true, "credentialsjson": true,
	"authorization": true, "servermessagesignature": true, "servermessagesignatureforrts": true,
}

func SanitizeStartVoiceChatRequest(req StartVoiceChatRequest) StartVoiceChatRequest {
	out := req
	if len(req.Config) > 0 {
		var value any
		if json.Unmarshal(req.Config, &value) == nil {
			redactJSON(value)
			if raw, err := json.Marshal(value); err == nil {
				out.Config = raw
			}
		}
	}
	if req.AgentConfig != nil {
		raw, _ := json.Marshal(req.AgentConfig)
		var copied map[string]any
		if json.Unmarshal(raw, &copied) == nil {
			redactJSON(copied)
			out.AgentConfig = copied
		}
	}
	return out
}

func redactJSON(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", ""))
			if sensitiveJSONKeys[normalized] {
				current[key] = redactedValue
				continue
			}
			redactJSON(child)
		}
	case []any:
		for _, child := range current {
			redactJSON(child)
		}
	}
}
