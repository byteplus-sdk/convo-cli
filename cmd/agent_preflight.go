// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"convo-cli/internal/config"
	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/openapi"
)

var speechPreflightHTTP = &http.Client{Timeout: 8 * time.Second}

func newAgentCheckCmd() *cobra.Command {
	var runtimeStdin bool
	cmd := &cobra.Command{Use: "check", Short: "Check selected agent configuration and speech service access", RunE: func(c *cobra.Command, _ []string) error {
		cfg, path, err := config.LoadNearest(".")
		if err != nil {
			return err
		}
		var req openapi.StartVoiceChatRequest
		resources, err := currentAgentResourceContract()
		if err != nil {
			return err
		}
		if runtimeStdin {
			if err := decodeRuntimeAgentRequest(c.InOrStdin(), &req); err != nil {
				return err
			}
		} else {
			cleanup, err := resources.PrepareStart(c.Context(), cfg, c, flagDryRun)
			if err != nil {
				return err
			}
			defer cleanup()
			req, err = buildStartRequest(cfg, projectDir(path))
			if err != nil {
				return err
			}
		}
		if flagDryRun {
			return out().Data(map[string]any{"dry_run": true, "checks": []string{"configuration", "speech_access"}})
		}
		if err := resources.PrepareRuntimeStart(c.Context(), cfg, &req, false); err != nil {
			return err
		}
		var voice map[string]any
		if err := json.Unmarshal(req.Config, &voice); err != nil {
			return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation, "Config must be a JSON object")
		}
		for _, key := range []string{"ASRConfig", "LLMConfig", "TTSConfig"} {
			if value, ok := voice[key].(map[string]any); !ok || len(value) == 0 {
				return errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation, "%s is required", key)
			}
		}
		for _, kind := range []string{"ASR", "TTS"} {
			provider, _ := voice[kind+"Config"].(map[string]any)
			if provider["Provider"] != "BytePlus" && !(kind == "TTS" && provider["Provider"] == "byteplus_Bidirectional_streaming") {
				continue
			}
			if err := checkSpeechAccess(c.Context(), kind, provider); err != nil {
				return err
			}
		}
		return out().Data(map[string]any{"ready": true, "issues": []any{}})
	}}
	cmd.Flags().BoolVar(&runtimeStdin, "runtime-request-stdin", false, "read the selected StartVoiceChat configuration from stdin")
	if resources, err := currentAgentResourceContract(); err == nil {
		resources.ConfigureStart(cmd)
	}
	return cmd
}

func checkSpeechAccess(ctx context.Context, kind string, provider map[string]any) error {
	params, _ := provider["ProviderParams"].(map[string]any)
	var resource, appID, token, path string
	if kind == "ASR" {
		resource, _ = params["ApiResourceId"].(string)
		appID, _ = params["AppId"].(string)
		token, _ = params["AccessToken"].(string)
		path = "/api/v3/sauc/bigmodel"
		switch params["StreamMode"] {
		case float64(2):
			path += "_async"
		case float64(1):
			path += "_nostream"
		}
	} else {
		resource, _ = params["ResourceId"].(string)
		if resource == "" {
			resource, _ = params["resourceId"].(string)
		}
		app, _ := params["app"].(map[string]any)
		appID, _ = app["appid"].(string)
		token, _ = app["token"].(string)
		path = "/api/v3/tts/bidirection"
	}
	if resource == "" {
		return errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation, "%s resource ID is required", kind)
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return errs.Wrap(err, "byteplus-rtc.cli.internal", "create speech check nonce")
	}
	nonce := base64.StdEncoding.EncodeToString(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://voice.ap-southeast-1.bytepluses.com"+path, nil)
	if err != nil {
		return errs.Wrap(err, "byteplus-rtc.cli.internal", "create speech check request")
	}
	for k, v := range map[string]string{"Connection": "Upgrade", "Upgrade": "websocket", "Sec-WebSocket-Version": "13", "Sec-WebSocket-Key": nonce, "X-Api-App-Key": appID, "X-Api-Access-Key": token, "X-Api-Resource-Id": resource} {
		req.Header.Set(k, v)
	}
	res, err := speechPreflightHTTP.Do(req)
	if err != nil {
		return errs.New("byteplus-rtc.openapi.request_failed", errs.TypeIO, "%s connection check failed", kind).WithHint("check Seed Speech availability and network access")
	}
	defer res.Body.Close()
	digest := sha1.Sum([]byte(nonce + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if res.StatusCode == http.StatusSwitchingProtocols && strings.EqualFold(res.Header.Get("Upgrade"), "websocket") && res.Header.Get("Sec-WebSocket-Accept") == base64.StdEncoding.EncodeToString(digest[:]) {
		return nil
	}
	message := fmt.Sprintf("%s connection check returned HTTP %d", kind, res.StatusCode)
	if res.StatusCode == http.StatusForbidden {
		message = fmt.Sprintf("%s resource %s is not authorized; activate the service in Seed Speech", kind, resource)
	}
	return errs.New("byteplus-rtc.openapi.request_failed", errs.TypeIO, "%s", message).WithHint("verify the speech App ID, credentials, and service activation; retry `%s agent check`", meta.BinName)
}
