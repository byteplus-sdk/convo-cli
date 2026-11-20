// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"convo-cli/internal/config"
	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/openapi"
	"convo-cli/internal/product"
)

const (
	bytePlusModelEndpointFlag = "model-endpoint-id"
	bytePlusModelAPIKeyFlag   = "model-api-key-id"
	bytePlusSpeechAppFlag     = "speech-app-id"
	bytePlusProjectFlag       = "resource-project"
)

type bytePlusAgentResourceContract struct{}

func (bytePlusAgentResourceContract) Command() *cobra.Command {
	var projectName string
	var write bool
	cmd := &cobra.Command{
		Use:   "resources",
		Short: "Find model services, API keys and speech applications by name",
		RunE: func(c *cobra.Command, args []string) error {
			if write {
				return configureResourceCommand(c, projectName)
			}
			client := newBytePlusResourceClient(nil)
			endpoints, err := client.ListModelEndpoints(c.Context(), projectName)
			if err != nil {
				return agentResourceFailure(err, "model service", projectName)
			}
			apiKeys, err := client.ListModelAPIKeys(c.Context(), projectName)
			if err != nil {
				return agentResourceFailure(err, "model API key", projectName)
			}
			result := map[string]any{
				"model_endpoints": endpoints, "model_api_keys": apiKeys,
				"query": map[string]any{"project": projectName, "region": product.Current().OpenAPIRegion, "endpoint_status": "Running"},
				"note":  "Present choices by display_name; map each selection to its id and save with agent resources --write",
			}
			speechApps, speechErr := client.ListSpeechApps(c.Context())
			if speechErr != nil {
				result["speech_apps"] = []openapi.BytePlusSpeechApp{}
				failure := agentResourceFailure(speechErr, "speech application", projectName)
				result["speech_discovery"] = "failed"
				typed, _ := errs.As(failure)
				result["speech_error"] = typed
				out().Warn("%s", failure)
			} else {
				result["speech_apps"] = speechApps
			}
			choices := agentResourceChoices(endpoints, apiKeys, speechApps, projectName)
			result["choices"] = choices
			guidance := map[string]any{}
			for key, label := range map[string]string{"model_endpoints": "model service", "model_api_keys": "model API key", "speech_apps": "speech application"} {
				status := "available"
				if len(choices[key]) == 0 {
					status = "empty"
				}
				if key == "speech_apps" && speechErr != nil {
					status = "failed"
				}
				guidance[key] = map[string]any{"status": status, "resource": label, "guidance": agentResourceGuideFor(label)}
			}
			result["discovery"] = guidance
			return out().Data(result)
		},
	}
	cmd.Flags().StringVar(&projectName, bytePlusProjectFlag, "default", "BytePlus resource project name")
	cmd.Flags().BoolVar(&write, "write", false, "configure Flexible resources and an optional Realtime OpenAI API key in .env.local")
	cmd.Flags().String(bytePlusModelEndpointFlag, "", "select a ModelArk endpoint ID")
	cmd.Flags().String(bytePlusModelAPIKeyFlag, "", "select a ModelArk API key resource ID")
	cmd.Flags().String(bytePlusSpeechAppFlag, "", "select a Seed Speech application ID")
	return cmd
}

func (bytePlusAgentResourceContract) ConfigureStart(cmd *cobra.Command) {
	cmd.Flags().String(bytePlusModelEndpointFlag, "", "ModelArk running endpoint ID (or BYTEPLUS_MODEL_ENDPOINT_ID)")
	cmd.Flags().String(bytePlusModelAPIKeyFlag, "", "ModelArk API key resource ID, not the secret key (or BYTEPLUS_MODEL_API_KEY_ID)")
	cmd.Flags().String(bytePlusSpeechAppFlag, "", "Seed Speech application ID used by ASR and TTS (or BYTEPLUS_SEED_SPEECH_APP_ID)")
	cmd.Flags().String(bytePlusProjectFlag, "", "BytePlus resource project name (default: BYTEPLUS_PROJECT_NAME or default)")
}

func (bytePlusAgentResourceContract) PrepareStart(ctx context.Context, cfg *config.Config, cmd *cobra.Command, dryRun bool) (func(), error) {
	if dryRun {
		return func() {}, nil
	}
	selection := bytePlusResourceSelection{
		ModelEndpointID: flagValue(cmd, bytePlusModelEndpointFlag),
		ModelAPIKeyID:   flagValue(cmd, bytePlusModelAPIKeyFlag),
		SpeechAppID:     flagValue(cmd, bytePlusSpeechAppFlag),
		ProjectName:     flagValue(cmd, bytePlusProjectFlag),
	}
	selection = bytePlusSelectionFromEnv(selection)
	credentials, err := resolveBytePlusResourceCredentials(ctx, cfg, selection)
	if err != nil {
		return nil, err
	}
	return applyTemporaryEnv(map[string]string{
		"BYTEPLUS_MODEL_ENDPOINT_ID":        selection.ModelEndpointID,
		"BYTEPLUS_MODEL_API_KEY_ID":         selection.ModelAPIKeyID,
		"BYTEPLUS_PROJECT_NAME":             selection.ProjectName,
		"BYTEPLUS_MODEL_API_KEY":            credentials.ModelAPIKey,
		"BYTEPLUS_SEED_SPEECH_APP_ID":       selection.SpeechAppID,
		"BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN": credentials.SpeechToken,
	})
}

func (bytePlusAgentResourceContract) PrepareRuntimeStart(ctx context.Context, cfg *config.Config, request *openapi.StartVoiceChatRequest, options runtimeStartOptions) error {
	if options.dryRun {
		return nil
	}
	var voiceConfig map[string]any
	if err := json.Unmarshal(request.Config, &voiceConfig); err != nil {
		return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation,
			"runtime VoiceChat Config is invalid JSON: %s", err)
	}
	selection := bytePlusResourceSelection{}
	if llm, ok := voiceConfig["LLMConfig"].(map[string]any); ok {
		selection.ModelEndpointID, _ = llm["EndPointId"].(string)
	}
	selection = bytePlusSelectionFromEnv(selection)
	if err := hydrateOtherRuntimeProviders(voiceConfig, options.localScene); err != nil {
		return err
	}
	llm, _ := voiceConfig["LLMConfig"].(map[string]any)
	asr, _ := voiceConfig["ASRConfig"].(map[string]any)
	tts, _ := voiceConfig["TTSConfig"].(map[string]any)
	needModel := llm["Provider"] == "BytePlus" || llm["Mode"] == "BytePlusArk"
	needSpeech := asr["Provider"] == "BytePlus" || tts["Provider"] == "BytePlus" || tts["Provider"] == "byteplus_Bidirectional_streaming"
	credentials, err := resolveSelectedBytePlusResourceCredentials(ctx, cfg, selection, needModel, needSpeech)
	if err != nil {
		return err
	}
	setRuntimeProviderCredentials(voiceConfig, selection, credentials)
	request.Config, err = json.Marshal(voiceConfig)
	if err != nil {
		return errs.Wrap(err, "byteplus-rtc.cli.internal", "encode hydrated runtime VoiceChat Config: %s", err)
	}
	return nil
}

type bytePlusResourceCredentials struct {
	ModelAPIKey string
	SpeechToken string
}

func bytePlusSelectionFromEnv(selection bytePlusResourceSelection) bytePlusResourceSelection {
	selection.ModelEndpointID = firstNonEmptyString(selection.ModelEndpointID, os.Getenv("BYTEPLUS_MODEL_ENDPOINT_ID"))
	selection.ModelAPIKeyID = firstNonEmptyString(selection.ModelAPIKeyID, os.Getenv("BYTEPLUS_MODEL_API_KEY_ID"))
	selection.SpeechAppID = firstNonEmptyString(selection.SpeechAppID, os.Getenv("BYTEPLUS_SEED_SPEECH_APP_ID"))
	selection.ProjectName = firstNonEmptyString(selection.ProjectName, os.Getenv("BYTEPLUS_PROJECT_NAME"), "default")
	return selection
}

func resolveBytePlusResourceCredentials(ctx context.Context, cfg *config.Config, selection bytePlusResourceSelection) (bytePlusResourceCredentials, error) {
	return resolveSelectedBytePlusResourceCredentials(ctx, cfg, selection, true, true)
}

func resolveSelectedBytePlusResourceCredentials(ctx context.Context, cfg *config.Config, selection bytePlusResourceSelection, needModel, needSpeech bool) (bytePlusResourceCredentials, error) {
	rawAPIKey := strings.TrimSpace(os.Getenv("BYTEPLUS_MODEL_API_KEY"))
	speechToken := strings.TrimSpace(os.Getenv("BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN"))
	if selection.ModelAPIKeyID != "" {
		rawAPIKey = ""
	}
	if selection.SpeechAppID != strings.TrimSpace(os.Getenv("BYTEPLUS_SEED_SPEECH_APP_ID")) {
		speechToken = ""
	}
	missing := []string{}
	if needModel {
		if selection.ModelEndpointID == "" || strings.HasPrefix(selection.ModelEndpointID, "Your ") {
			missing = append(missing, "model service")
		}
		if selection.ModelAPIKeyID == "" && rawAPIKey == "" {
			missing = append(missing, "model API key")
		}
	}
	if needSpeech && selection.SpeechAppID == "" {
		missing = append(missing, "speech application")
	}
	if len(missing) > 0 {
		return bytePlusResourceCredentials{}, errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation, "select the required resources: %s", strings.Join(missing, ", ")).WithDetails(map[string]any{"missing_resources": missing}).WithHint("run `%s agent resources` to find resources, then `%s agent resources --write` to choose them by name and save", meta.BinName, meta.BinName)
	}
	client := newBytePlusResourceClient(cfg)
	if needModel && rawAPIKey == "" {
		var err error
		rawAPIKey, err = client.GetModelRawAPIKey(ctx, selection.ModelAPIKeyID, selection.ProjectName)
		if err != nil {
			return bytePlusResourceCredentials{}, mapOpenAPIError(err)
		}
	}
	if needSpeech && speechToken == "" {
		var err error
		speechToken, err = client.GetSpeechAccessToken(ctx, selection.SpeechAppID)
		if err != nil {
			return bytePlusResourceCredentials{}, mapOpenAPIError(err)
		}
	}
	if (needModel && strings.TrimSpace(rawAPIKey) == "") || (needSpeech && strings.TrimSpace(speechToken) == "") {
		return bytePlusResourceCredentials{}, errs.New("byteplus-rtc.agent.missing_config", errs.TypeNotFound,
			"BytePlus resource credential exchange returned an empty credential").
			WithHint("verify the selected ModelArk API key and Seed Speech application permissions")
	}
	return bytePlusResourceCredentials{ModelAPIKey: rawAPIKey, SpeechToken: speechToken}, nil
}

func setRuntimeProviderCredentials(voiceConfig map[string]any, selection bytePlusResourceSelection, credentials bytePlusResourceCredentials) {
	if asr, ok := voiceConfig["ASRConfig"].(map[string]any); ok && asr["Provider"] == "BytePlus" {
		params, _ := asr["ProviderParams"].(map[string]any)
		if params == nil {
			params = map[string]any{}
			asr["ProviderParams"] = params
		}
		params["AppId"], params["AccessToken"] = selection.SpeechAppID, credentials.SpeechToken
	}
	if llm, ok := voiceConfig["LLMConfig"].(map[string]any); ok && (llm["Provider"] == "BytePlus" || llm["Mode"] == "BytePlusArk") {
		llm["EndPointId"], llm["APIKey"] = selection.ModelEndpointID, credentials.ModelAPIKey
	}
	if tts, ok := voiceConfig["TTSConfig"].(map[string]any); ok && (tts["Provider"] == "BytePlus" || tts["Provider"] == "byteplus_Bidirectional_streaming") {
		params, _ := tts["ProviderParams"].(map[string]any)
		if params == nil {
			params = map[string]any{}
			tts["ProviderParams"] = params
		}
		app, _ := params["app"].(map[string]any)
		if app == nil {
			app = map[string]any{}
			params["app"] = app
		}
		app["appid"], app["token"] = selection.SpeechAppID, credentials.SpeechToken
	}
}

type bytePlusResourceSelection struct {
	ModelEndpointID string
	ModelAPIKeyID   string
	SpeechAppID     string
	ProjectName     string
}

func newBytePlusResourceClient(cfg *config.Config) openapi.BytePlusResourceClient {
	base := newAuthenticatedOpenAPIClient(cfg)
	ark := *base
	ark.Endpoint, ark.Service, ark.Region = openapi.BytePlusArkEndpoint, openapi.BytePlusArkService, product.Current().OpenAPIRegion
	speech := *base
	speech.Endpoint, speech.Service, speech.Region = openapi.BytePlusResourceEndpoint, openapi.BytePlusSpeechService, product.Current().OpenAPIRegion
	return openapi.BytePlusResourceClient{Ark: &ark, Speech: &speech}
}

func flagValue(cmd *cobra.Command, name string) string {
	value, _ := cmd.Flags().GetString(name)
	return value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func init() { registerAgentResourceContract(product.BytePlusID, bytePlusAgentResourceContract{}) }
