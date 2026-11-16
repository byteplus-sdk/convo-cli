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

func hydrateOtherRuntimeProviders(voice map[string]any) error {
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
	if llm, ok := voice["LLMConfig"].(map[string]any); ok && llm["Provider"] != "BytePlus" && llm["Mode"] != "BytePlusArk" {
		key := "CUSTOM_LLM_API_KEY"
		address, _ := llm["Url"].(string)
		u, _ := url.Parse(address)
		if llm["Provider"] == "OpenAI" || (u != nil && u.Scheme == "https" && u.Host == "api.openai.com") {
			key = "OPENAI_API_KEY"
		}
		if u != nil && u.Scheme == "https" && u.Host == "generativelanguage.googleapis.com" {
			key = "GOOGLE_LLM_API_KEY"
		}
		if key == "CUSTOM_LLM_API_KEY" && strings.TrimSpace(os.Getenv(key)) == "" && strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
			key = "OPENAI_API_KEY"
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
