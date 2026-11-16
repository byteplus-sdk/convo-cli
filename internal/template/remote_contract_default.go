// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package template

import (
	"fmt"
	"strings"

	"convo-cli/internal/meta"
	"convo-cli/internal/product"
)

type bytePlusRemoteTemplateContract struct{}

func (bytePlusRemoteTemplateContract) Prepare(files []RenderedFile) ([]RenderedFile, error) {
	prepared := make([]RenderedFile, len(files))
	copy(prepared, files)
	ignore := product.Current().AuthDirName + "/"
	foundIgnore := false
	for index := range prepared {
		if prepared[index].Path == ".gitignore" {
			foundIgnore = true
			content := prepared[index].Content
			if !strings.Contains("\n"+content+"\n", "\n"+ignore+"\n") {
				prepared[index].Content = strings.TrimRight(content, "\r\n") + "\n" + ignore + "\n"
				prepared[index].Bytes = len(prepared[index].Content)
			}
		}
	}
	if !foundIgnore {
		prepared = append(prepared, RenderedFile{Path: ".gitignore", Content: ignore + "\n", Bytes: len(ignore) + 1})
	}
	for index := range prepared {
		if prepared[index].Path != "README.md" {
			continue
		}
		prepared[index].Content = replaceREADMESection(prepared[index].Content, "RTC CLI integration", fmt.Sprintf("The companion server uses the authenticated CLI for RTC token issuance and VoiceChat Start/Stop.\n\nRun these commands in the generated project:\n\n```shell\n%[1]s auth login --browser=open\n%[1]s agent resources --write\n%[1]s dev\n```\n\nThe CLI selects an RTC application and stores its AppID and AppKey in the local .env.local file. Resource setup selects the model and speech resources. Keep credentials in the server environment.\n\nWhen BytePlus access keys are configured, the server signs OpenAPI requests directly. Otherwise it uses the CLI Signin session. RTC tokens use GetToken Version 2025-06-01.\n", meta.BinName))
		prepared[index].Content = replaceREADMESection(prepared[index].Content, "Introduction", "BytePlus RTC conversational AI combines real-time audio and video with ASR, LLM, and TTS providers. Configure the providers through the VoiceChat APIs or the demo controls.\n")
		prepared[index].Bytes = len(prepared[index].Content)
	}
	return prepared, nil
}

func replaceREADMESection(content, heading, body string) string {
	marker := "## " + heading + "\n"
	start := -1
	if strings.HasPrefix(content, marker) {
		start = 0
	} else if offset := strings.Index(content, "\n"+marker); offset >= 0 {
		start = offset + 1
	}
	if start < 0 {
		return content
	}
	bodyStart := start + len(marker)
	end := len(content)
	if offset := strings.Index(content[bodyStart:], "\n## "); offset >= 0 {
		end = bodyStart + offset + 1
	}
	return content[:bodyStart] + "\n" + strings.TrimSpace(body) + "\n\n" + content[end:]
}

func (bytePlusRemoteTemplateContract) Validate(files map[string]string, manifest templateManifest, taskfile Taskfile) error {
	if manifest.Scene != "voice-agent" || manifest.Platform != "web" {
		return contractError("BytePlus remote template must provide voice-agent/web")
	}
	if taskfile.SDK.Name != "@byteplus/rtc" || taskfile.SDK.Version != product.BytePlusSDKVersion {
		return contractError("BytePlus remote template must pin @byteplus/rtc %s", product.BytePlusSDKVersion)
	}
	server := files["server/app.js"]
	serverConfig := server
	if strings.Contains(server, "require('./config')") || strings.Contains(server, `require("./config")`) {
		serverConfig = files["server/config.js"]
	}
	for _, required := range []string{"BYTEPLUS_RTC_CLI_PATH", "BYTEPLUS_RTC_PROJECT_DIR", "BYTEPLUS_RTC_OPENAPI_USER_AGENT"} {
		if !strings.Contains(serverConfig, required) {
			return contractError("BytePlus voice-agent server config is missing CLI integration marker %q", required)
		}
	}
	for _, required := range []string{
		"--runtime-request-stdin", "GetToken", "2025-06-01", "2025-05-01",
		"BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN", "BYTEPLUS_MODEL_API_KEY",
	} {
		if !strings.Contains(server, required) {
			return contractError("BytePlus voice-agent server is missing CLI integration marker %q", required)
		}
	}
	for _, path := range []string{product.BytePlusTaskfileName, "server/app.js", "web/src/lib/useCommon.ts"} {
		if strings.Contains(strings.ToLower(files[path]), "botid") || strings.Contains(strings.ToLower(files[path]), "bot-id") {
			return contractError("BytePlus remote template must not expose BotID in %s", path)
		}
	}
	return nil
}

func init() { registerRemoteTemplateContract(product.BytePlusID, bytePlusRemoteTemplateContract{}) }
