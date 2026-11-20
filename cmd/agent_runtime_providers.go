// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"net/url"
	"os"
	"strings"

	"convo-cli/internal/errs"
)

func providerObject(parent map[string]any, key string) map[string]any {
	value, _ := parent[key].(map[string]any)
	if value == nil {
		value = map[string]any{}
		parent[key] = value
	}
	return value
}

func providerCredential(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" || strings.HasPrefix(value, "Your ") {
		return "", errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation, "%s is required", key).WithParam(key).WithHint("configure %s in .env.local", key)
	}
	return value, nil
}

func hydrateGatewayProvider(kind string, config map[string]any) error {
	module := strings.TrimSuffix(kind, "Config")
	invalid := func(field string) error {
		return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation, "invalid %s AI Gateway %s", module, field)
	}
	params, ok := config["ProviderParams"].(map[string]any)
	if !ok {
		if _, exists := config["ProviderParams"]; exists {
			return invalid("ProviderParams")
		}
		params = map[string]any{}
	}
	for key, value := range params {
		switch key {
		case "ExtraData", "ExtraHeader":
			if _, ok := value.(map[string]any); !ok {
				return invalid(key)
			}
		case "Voice":
			if module != "TTS" {
				return invalid(key)
			}
		case "OutputAudioPitchRate", "OutputAudioSpeedRate", "OutputAudioVolume":
			if _, ok := value.(float64); module != "TTS" || !ok {
				return invalid(key)
			}
		default:
			return invalid("ProviderParams")
		}
	}
	if voice, _ := params["Voice"].(string); module == "TTS" && strings.TrimSpace(voice) == "" {
		return invalid("Voice")
	}
	urlKey := module + "_GATEWAY_API_URL"
	address, err := providerCredential(urlKey)
	if err != nil {
		return err
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "wss" || u.Hostname() == "" || u.User != nil || strings.Contains(address, "#") {
		return errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation, "%s must be a secure WebSocket URL without credentials or a fragment", urlKey)
	}
	apiKey, err := providerCredential(module + "_GATEWAY_API_KEY")
	if err != nil {
		return err
	}
	params["URL"], params["APIKey"] = address, apiKey
	config["ProviderParams"] = params
	return nil
}

func hydrateOtherRuntimeProviders(voice map[string]any, localScene bool) error {
	set := func(target map[string]any, field, key string) error {
		value, err := providerCredential(key)
		if err != nil {
			return err
		}
		target[field] = value
		return nil
	}
	for _, kind := range []string{"ASRConfig", "TTSConfig"} {
		config, ok := voice[kind].(map[string]any)
		if !ok {
			continue
		}
		provider, _ := config["Provider"].(string)
		if provider == "BytePlus" || provider == "byteplus_Bidirectional_streaming" {
			continue
		}
		if provider == "ai_gateway" {
			if err := hydrateGatewayProvider(kind, config); err != nil {
				return err
			}
			continue
		}
		params := providerObject(config, "ProviderParams")
		switch provider {
		case "Amazon", "amazon":
			if err := set(params, "ID", "AWS_ACCESS_KEY_ID"); err != nil {
				return err
			}
			if err := set(params, "Secret", "AWS_SECRET_ACCESS_KEY"); err != nil {
				return err
			}
		case "Google", "google", "GoogleV2":
			if err := set(params, "CredentialsJSON", "GOOGLE_CREDENTIALS_JSON"); err != nil {
				return err
			}
			if !json.Valid([]byte(params["CredentialsJSON"].(string))) {
				return errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation, "GOOGLE_CREDENTIALS_JSON must contain valid JSON")
			}
			if provider == "GoogleV2" {
				if err := set(params, "RecognizerPath", "GOOGLE_RECOGNIZER_PATH"); err != nil {
					return err
				}
			}
		case "OpenAI", "openai":
			if err := set(params, "APIKey", "OPENAI_API_KEY"); err != nil {
				return err
			}
			if kind == "TTSConfig" {
				params["URL"] = firstNonEmptyString(os.Getenv("OPENAI_TTS_URL"), "https://api.openai.com/v1/audio/speech")
			}
		default:
			return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation, "unsupported %s provider %q", kind, provider)
		}
	}
	if !localScene {
		if err := validateBrowserLLMFields(voice); err != nil {
			return err
		}
	}
	if llm, ok := voice["LLMConfig"].(map[string]any); ok && llm["Provider"] != "BytePlus" && llm["Mode"] != "BytePlusArk" {
		key, err := runtimeLLMKey(llm, localScene)
		if err != nil {
			return err
		}
		if err := set(llm, "APIKey", key); err != nil {
			return err
		}
	}
	if avatar, ok := voice["AvatarConfig"].(map[string]any); ok && avatar["Enabled"] == true {
		if avatar["Provider"] == "Akool" {
			params := providerObject(avatar, "ProviderParams")
			if err := set(params, "ApiKey", "AKOOL_API_KEY"); err != nil {
				return err
			}
			id, _ := params["AvatarId"].(string)
			if strings.TrimSpace(id) == "" {
				return errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation, "Akool AvatarId is required")
			}
			if subtitles, ok := voice["SubtitleConfig"].(map[string]any); ok && subtitles["SubtitleMode"] != float64(1) {
				return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation, "Akool requires SubtitleConfig.SubtitleMode=1")
			}
		} else {
			target := avatar
			if avatar["ProviderParams"] != nil {
				target = providerObject(avatar, "ProviderParams")
			}
			if err := set(target, "AvatarAppID", "AVATAR_APP_ID"); err != nil {
				return err
			}
			if err := set(target, "AvatarToken", "AVATAR_TOKEN"); err != nil {
				return err
			}
		}
	}
	if search, ok := voice["WebSearchAgentConfig"].(map[string]any); ok && search["Enable"] == true {
		if err := set(search, "APIKey", "ASKECHO_API_KEY"); err != nil {
			return err
		}
		params := map[string]any{}
		if raw, ok := search["ParamsString"].(string); ok && raw != "" {
			if err := json.Unmarshal([]byte(raw), &params); err != nil {
				return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation, "WebSearchAgentConfig.ParamsString must contain a JSON object")
			}
		}
		if params == nil {
			params = map[string]any{}
		}
		bot := firstNonEmptyString(os.Getenv("ASKECHO_BOT_ID"))
		if bot != "" {
			params["bot_id"] = bot
		}
		if value, _ := params["bot_id"].(string); strings.TrimSpace(value) == "" {
			return errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation, "ASKECHO_BOT_ID is required")
		}
		raw, _ := json.Marshal(params)
		search["ParamsString"] = string(raw)
	}
	return nil
}

func validateBrowserLLMFields(voice map[string]any) error {
	for field := range voice {
		if field != "LLMConfig" && strings.EqualFold(field, "LLMConfig") {
			return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation, "use Config.LLMConfig for the LLM configuration")
		}
	}
	llm, _ := voice["LLMConfig"].(map[string]any)
	for field := range llm {
		for _, canonical := range []string{"Url", "Mode", "Provider"} {
			if field != canonical && strings.EqualFold(field, canonical) {
				return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation, "use LLMConfig.%s for this field", canonical)
			}
		}
	}
	if _, hasURL := llm["Url"]; hasURL && (llm["Provider"] == "BytePlus" || llm["Mode"] == "BytePlusArk") {
		return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation, "BytePlus LLM configuration uses EndPointId")
	}
	return nil
}

func runtimeLLMKey(llm map[string]any, localScene bool) (string, error) {
	if localScene {
		address, _ := llm["Url"].(string)
		u, _ := url.Parse(address)
		switch {
		case u != nil && u.Scheme == "https" && u.Host == "generativelanguage.googleapis.com":
			return "GOOGLE_LLM_API_KEY", nil
		case llm["Provider"] == "OpenAI" || (u != nil && u.Scheme == "https" && u.Host == "api.openai.com"):
			return "OPENAI_API_KEY", nil
		case strings.TrimSpace(os.Getenv("CUSTOM_LLM_API_KEY")) == "" && strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "":
			return "OPENAI_API_KEY", nil
		default:
			return "CUSTOM_LLM_API_KEY", nil
		}
	}

	if _, present := llm["Url"]; !present && llm["Provider"] == "OpenAI" {
		return "OPENAI_API_KEY", nil
	}
	address, _ := llm["Url"].(string)
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return "", errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation, "LLMConfig.Url must be an HTTPS URL without credentials")
	}
	if u.Port() == "" || u.Port() == "443" {
		switch strings.ToLower(u.Hostname()) {
		case "api.openai.com":
			return "OPENAI_API_KEY", nil
		case "generativelanguage.googleapis.com":
			return "GOOGLE_LLM_API_KEY", nil
		}
	}
	if address != strings.TrimSpace(os.Getenv("CUSTOM_LLM_URL")) {
		return "", errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation, "LLMConfig.Url must match the custom endpoint configured in CUSTOM_LLM_URL")
	}
	return "CUSTOM_LLM_API_KEY", nil
}
