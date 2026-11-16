// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/product"
)

type agentResourceGuide struct {
	Flag             string `json:"selection_flag"`
	DocumentationURL string `json:"documentation_url"`
	ConsoleURL       string `json:"console_url"`
	Action           string `json:"action"`
}

func agentResourceGuideFor(resource string) agentResourceGuide {
	switch resource {
	case "model service":
		return agentResourceGuide{
			Flag:             "--" + bytePlusModelEndpointFlag,
			DocumentationURL: "https://docs.byteplus.com/en/docs/modelark/online-inference-standard",
			ConsoleURL:       "https://ai.byteplus.com/ark/region:ap-southeast-1/endpoint",
			Action:           "Check the selected project and region; create a model service in Online inference and wait until it is ready.",
		}
	case "model API key":
		return agentResourceGuide{
			Flag:             "--" + bytePlusModelAPIKeyFlag,
			DocumentationURL: "https://docs.byteplus.com/en/docs/modelark/api-key",
			ConsoleURL:       "https://ai.byteplus.com/ark/region:ap-southeast-1/apiKey",
			Action:           "Open API keys in the selected ModelArk project and create a named API key with access to the chosen model service.",
		}
	default:
		return agentResourceGuide{
			Flag:             "--" + bytePlusSpeechAppFlag,
			DocumentationURL: "https://docs.byteplus.com/en/docs/byteplusvoice/Speech_Console_Guide#E4ysfjvI",
			ConsoleURL:       "https://console.byteplus.com/voice/app",
			Action:           "Open the Seed Speech application console, create an application, and activate ASR and TTS. App ID and Access Token are available after activation.",
		}
	}
}

func agentResourceFailure(err error, resource, project string) error {
	typed, _ := errs.As(mapOpenAPIError(err))
	guide := agentResourceGuideFor(resource)
	typed.Message = fmt.Sprintf("Could not query %s resources: %s", resource, typed.Message)
	if typed.Details == nil {
		typed.Details = map[string]any{}
	}
	typed.Details["resource"] = resource
	typed.Details["project"] = project
	typed.Details["region"] = product.Current().OpenAPIRegion
	typed.Details["discovery_status"] = "failed"
	typed.Details["guidance"] = guide
	typed.Hint = fmt.Sprintf("%s Check account access and network connectivity, then retry `%s agent resources`. Guide: %s Console: %s", typed.Hint, meta.BinName, guide.DocumentationURL, guide.ConsoleURL)
	return typed
}
