// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package product

import "time"

const (
	BytePlusID               = "byteplus"
	BytePlusBinaryName       = "convo-cli"
	BytePlusConfigFileName   = "byteplus-rtc.config.yaml"
	BytePlusTemplateManifest = "byteplus-rtc.template.yaml"
	BytePlusTaskfileName     = "byteplus-rtc.taskfile.yaml"
	BytePlusUserAgentProduct = "convo-cli"
	BytePlusOpenAPIEndpoint  = "https://rtc.ap-southeast-1.byteplusapi.com"
	BytePlusVoiceChatVersion = "2025-05-01"
	BytePlusSDKVersion       = "4.69.3"
	BytePlusNpmPackage       = "@byteplus/convo-cli"
	BytePlusNpmRegistryURL   = "https://registry.npmjs.org/@byteplus%2Fconvo-cli/latest"
	BytePlusOfficialSkill    = "byted-bp-convo-guide"
)

var bytePlusProfile = Profile{
	ID:                     BytePlusID,
	DisplayName:            "BytePlus",
	BinaryName:             BytePlusBinaryName,
	ConfigFileName:         BytePlusConfigFileName,
	TemplateManifestName:   BytePlusTemplateManifest,
	TaskfileName:           BytePlusTaskfileName,
	ErrorCodePrefix:        "byteplus-rtc",
	CLIPathEnv:             "BYTEPLUS_RTC_CLI_PATH",
	ProjectDirEnv:          "BYTEPLUS_RTC_PROJECT_DIR",
	BusinessIDEnv:          "BYTEPLUS_RTC_BUSINESS_ID",
	OpenAPIUserAgentEnv:    "BYTEPLUS_RTC_OPENAPI_USER_AGENT",
	ProjectEnvFileEnv:      "BYTEPLUS_RTC_ENV_FILE",
	BusinessID:             "byteplus_rtc_cli",
	UserAgentProduct:       BytePlusUserAgentProduct,
	ProjectRegion:          "ap-southeast-1",
	OpenAPIEndpoint:        BytePlusOpenAPIEndpoint,
	OpenAPIService:         "rtc",
	OpenAPIRegion:          "ap-southeast-1",
	VoiceChatVersion:       BytePlusVoiceChatVersion,
	AgentUserIDField:       "UserID",
	AgentTargetUserIDField: "TargetUserID",
	SDKName:                "@byteplus/rtc",
	SDKVersion:             BytePlusSDKVersion,
	DocsMCPEndpoint:        "https://topic.bytedance.com/mcp/byteplus-rtc",
	NpmPackage:             BytePlusNpmPackage,
	NpmRegistryURL:         BytePlusNpmRegistryURL,
	OfficialSkillName:      BytePlusOfficialSkill,
	RetiredSkills:          "byted-bp-interactai-guide",
	StateDirName:           "convo-cli",
	StateDirEnv:            "CONVO_CLI_STATE_DIR",
	UpdateNotifierEnv:      "CONVO_CLI_NO_UPDATE_NOTIFIER",
	SkillsNotifierEnv:      "CONVO_CLI_NO_SKILLS_NOTIFIER",
	AccessKeyEnv:           "BYTEPLUS_ACCESS_KEY_ID",
	SecretKeyEnv:           "BYTEPLUS_SECRET_ACCESS_KEY",
	SessionTokenEnv:        "BYTEPLUS_SESSION_TOKEN",
	AuthStorageNamespace:   "byteplus",
	AuthHomeEnv:            "CONVO_CLI_HOME",
	AuthDirName:            ".convo-cli",
	OAuthAuthorizeEndpoint: "https://signin.byteplus.com/authorize/oauth/authorize",
	OAuthTokenEndpoint:     "https://signin.byteplus.com/authorize/oauth/token",
	OAuthLocalClientID:     DefaultLocalClientID,
	OAuthScope:             DefaultOAuthScope,
	OAuthRefreshSkew:       time.Minute,
	Capabilities: Capabilities{
		EnvironmentCredentials:  true,
		RTCAppBootstrap:         true,
		AgentResourceDiscovery:  true,
		SecretBackedSceneConfig: true,
		StrictOpenAPIProfile:    true,
		RemoteTemplate:          true,
		RemoteRTCJoinToken:      true,
	},
}

func init() { register(bytePlusProfile) }
