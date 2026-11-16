// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package template

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"convo-cli/internal/meta"
	"convo-cli/internal/product"
)

func TestPreparedTemplateIgnoresProjectCredentials(t *testing.T) {
	for _, existing := range []string{"", "node_modules/\n", ".env.local\r\n"} {
		files := []RenderedFile{{Path: ".gitignore", Content: existing, Bytes: len(existing)}}
		contract := bytePlusRemoteTemplateContract{}
		prepared, err := contract.Prepare(files)
		if err != nil {
			t.Fatal(err)
		}
		again, err := contract.Prepare(prepared)
		if err != nil || again[0].Content != prepared[0].Content {
			t.Fatal("ignore preparation must be idempotent")
		}
		dir := t.TempDir()
		if err := Write(dir, prepared); err != nil {
			t.Fatal(err)
		}
		git, err := exec.LookPath("git")
		if err != nil {
			t.Skip("git unavailable")
		}
		if output, err := exec.Command(git, "init", "-q", dir).CombinedOutput(); err != nil {
			t.Fatalf("git init: %s (%v)", output, err)
		}
		if err := os.Mkdir(filepath.Join(dir, ".convo-cli"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".convo-cli", "auth.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command(git, "-C", dir, "check-ignore", ".convo-cli/auth.json").CombinedOutput(); err != nil {
			t.Fatalf("credentials not ignored: %s (%v)", output, err)
		}
	}
}

func TestBytePlusRemoteTemplateContractRequiresCLIIntegration(t *testing.T) {
	if registeredRemoteTemplateContractCount() != 1 {
		t.Fatalf("remote template contract count = %d", registeredRemoteTemplateContractCount())
	}
	files := map[string]string{
		"server/app.js": strings.Join([]string{
			"BYTEPLUS_RTC_CLI_PATH", "BYTEPLUS_RTC_PROJECT_DIR", "BYTEPLUS_RTC_OPENAPI_USER_AGENT",
			"--runtime-request-stdin", "GetToken", "2025-06-01", "2025-05-01",
			"BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN", "BYTEPLUS_MODEL_API_KEY",
		}, "\n"),
		product.BytePlusTaskfileName: "runtime contract",
		"web/src/lib/useCommon.ts":   "room join",
	}
	manifest := templateManifest{Scene: "voice-agent", Platform: "web"}
	taskfile := Taskfile{SDK: SDK{Name: "@byteplus/rtc", Version: product.BytePlusSDKVersion}}
	contract := bytePlusRemoteTemplateContract{}
	if err := contract.Validate(files, manifest, taskfile); err != nil {
		t.Fatal(err)
	}
	delete(files, "server/app.js")
	if err := contract.Validate(files, manifest, taskfile); err == nil {
		t.Fatal("contract accepted a server without CLI integration")
	}
}

func TestBytePlusRemoteTemplateContractRejectsBotIDSurface(t *testing.T) {
	files := map[string]string{
		"server/app.js": strings.Join([]string{
			"BYTEPLUS_RTC_CLI_PATH", "BYTEPLUS_RTC_PROJECT_DIR", "BYTEPLUS_RTC_OPENAPI_USER_AGENT",
			"--runtime-request-stdin", "GetToken", "2025-06-01", "2025-05-01",
			"BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN", "BYTEPLUS_MODEL_API_KEY",
		}, "\n"),
		product.BytePlusTaskfileName: "--bot-id unsupported",
		"web/src/lib/useCommon.ts":   "room join",
	}
	err := (bytePlusRemoteTemplateContract{}).Validate(
		files,
		templateManifest{Scene: "voice-agent", Platform: "web"},
		Taskfile{SDK: SDK{Name: "@byteplus/rtc", Version: product.BytePlusSDKVersion}},
	)
	if err == nil {
		t.Fatal("contract accepted BotID surface")
	}
}

func TestBytePlusRemoteTemplateContractAcceptsExtractedServerConfig(t *testing.T) {
	files := remoteFixture()
	var markers []string
	for _, marker := range []string{"BYTEPLUS_RTC_CLI_PATH", "BYTEPLUS_RTC_PROJECT_DIR", "BYTEPLUS_RTC_OPENAPI_USER_AGENT"} {
		files["server/app.js"] = strings.ReplaceAll(files["server/app.js"], marker, "")
		markers = append(markers, marker)
	}
	files["server/app.js"] = "const { loadConfig } = require('./config');\n" + files["server/app.js"]
	files["server/config.js"] = strings.Join(markers, "\n")
	manifest := templateManifest{Scene: "voice-agent", Platform: "web"}
	taskfile := Taskfile{SDK: SDK{Name: "@byteplus/rtc", Version: product.BytePlusSDKVersion}}
	contract := bytePlusRemoteTemplateContract{}
	if err := contract.Validate(files, manifest, taskfile); err != nil {
		t.Fatal(err)
	}
	delete(files, "server/config.js")
	if err := contract.Validate(files, manifest, taskfile); err == nil {
		t.Fatal("missing imported config accepted")
	}
	files["server/config.js"] = strings.Join(markers, "\n")
	files["server/app.js"] = strings.ReplaceAll(files["server/app.js"], "--runtime-request-stdin", "")
	if err := contract.Validate(files, manifest, taskfile); err == nil {
		t.Fatal("missing runtime transport accepted")
	}
}

func TestBytePlusRemoteTemplatePreparationPreservesProjectDocumentation(t *testing.T) {
	originalBin := meta.BinName
	meta.BinName = "custom-cli"
	t.Cleanup(func() { meta.BinName = originalBin })
	setup := "## Setup and Configuration\n\nRequires Node.js 18 or later.\n\nnpm run dev:realtime\nOPENAI_API_KEY\nREACT_APP_AIGC_PROXY_HOST\nREACT_APP_MODEL_ENDPOINT_ID\n\n## Security and privacy\n\nSee [SECURITY.md](SECURITY.md).\n\n## Model and session settings\n\nSESSION_DURATION_SECONDS\n"
	original := "# Demo\n\nDemo Online: https://example.com/demo\n\n## RTC CLI integration\n\nPrevious setup instructions.\n\n## Introduction\n\nPrevious introduction.\n\n" + setup
	source := []RenderedFile{
		{Path: "README.md", Content: original, Bytes: len(original)},
		{Path: "server/app.js", Content: "unchanged", Bytes: 9},
	}
	prepared, err := (bytePlusRemoteTemplateContract{}).Prepare(source)
	if err != nil {
		t.Fatal(err)
	}
	readme := prepared[0]
	for _, command := range []string{"custom-cli auth login --browser=open", "custom-cli agent resources --write", "custom-cli dev"} {
		if !strings.Contains(readme.Content, command) {
			t.Errorf("README missing command %q", command)
		}
	}
	if !strings.HasPrefix(readme.Content, "# Demo\n\nDemo Online: https://example.com/demo\n") || !strings.HasSuffix(readme.Content, setup) || readme.Bytes != len(readme.Content) {
		t.Fatalf("prepared README lost project documentation: %+v", readme)
	}
	if strings.Contains(readme.Content, "Previous setup instructions.") || strings.Contains(readme.Content, "Previous introduction.") {
		t.Fatal("README retained replaced sections")
	}
	if source[0].Content != original || source[0].Bytes != len(original) {
		t.Fatal("preparation mutated the immutable source slice")
	}
	if prepared[1] != source[1] {
		t.Fatalf("unrelated file changed: %+v", prepared[1])
	}
}

func TestReplaceREADMESectionPreservesOtherSections(t *testing.T) {
	for _, tc := range []struct{ name, content, want string }{
		{"missing", "# Demo\n\nSetup notes.\n", "# Demo\n\nSetup notes.\n"},
		{"last section", "# Demo\n\n## Introduction\n\nOld.\n", "# Demo\n\n## Introduction\n\nNew.\n\n"},
		{"first section", "## Introduction\nOld.\n## Setup\nKeep.\n", "## Introduction\n\nNew.\n\n## Setup\nKeep.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := replaceREADMESection(tc.content, "Introduction", "New."); got != tc.want {
				t.Fatalf("README = %q, want %q", got, tc.want)
			}
		})
	}
}
