// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package product owns the build-selected product contract. Scalar values live
// in Profile; consumer packages own behavior interfaces that use the profile.
package product

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// SelectedID is the build-selected profile identifier. Release builds may
// override it with -ldflags. Unknown identifiers fail closed in Current.
var SelectedID = BytePlusID

// NpmRegistryURLOverride is set by distribution builds that publish outside
// the profile's public registry.
var NpmRegistryURLOverride string
var NpmPackageOverride string

// Capabilities describes supported command flows. It must not be used for
// request encoding or resource-discovery behavior; consumer-owned interfaces
// handle those differences.
type Capabilities struct {
	EnvironmentCredentials  bool
	RTCAppBootstrap         bool
	AgentResourceDiscovery  bool
	SecretBackedSceneConfig bool
	StrictOpenAPIProfile    bool
	RemoteTemplate          bool
	RemoteRTCJoinToken      bool
}

// Profile contains immutable scalar contracts that are known to vary between
// product implementations. It intentionally contains no maps, slices, or
// function fields, so callers receive an independent value copy.
type Profile struct {
	ID                     string
	DisplayName            string
	BinaryName             string
	ConfigFileName         string
	TemplateManifestName   string
	TaskfileName           string
	ErrorCodePrefix        string
	CLIPathEnv             string
	ProjectDirEnv          string
	BusinessIDEnv          string
	OpenAPIUserAgentEnv    string
	ProjectEnvFileEnv      string
	BusinessID             string
	UserAgentProduct       string
	ProjectRegion          string
	OpenAPIEndpoint        string
	OpenAPIService         string
	OpenAPIRegion          string
	VoiceChatVersion       string
	AgentUserIDField       string
	AgentTargetUserIDField string
	SDKName                string
	SDKVersion             string
	DocsMCPEndpoint        string
	NpmPackage             string
	NpmRegistryURL         string
	OfficialSkillName      string
	StateDirName           string
	StateDirEnv            string
	UpdateNotifierEnv      string
	SkillsNotifierEnv      string
	RetiredSkills          string
	AccessKeyEnv           string
	SecretKeyEnv           string
	SessionTokenEnv        string
	AuthStorageNamespace   string
	AuthHomeEnv            string
	AuthDirName            string
	OAuthAuthorizeEndpoint string
	OAuthTokenEndpoint     string
	OAuthLocalClientID     string
	OAuthScope             string
	OAuthRefreshSkew       time.Duration
	Capabilities           Capabilities
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Profile{}
)

func register(profile Profile) {
	if err := validate(profile); err != nil {
		panic(err)
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[profile.ID]; exists {
		panic(fmt.Sprintf("product profile %q is already registered", profile.ID))
	}
	registry[profile.ID] = profile
}

// Current returns a copy of the selected immutable profile. It panics for an
// invalid build selection because continuing with partial defaults could send
// credentials or signed requests to the wrong service.
func Current() Profile {
	profile, ok := Lookup(SelectedID)
	if !ok {
		panic(fmt.Sprintf("unknown product profile %q", SelectedID))
	}
	if override := strings.TrimSpace(NpmPackageOverride); override != "" {
		profile.NpmPackage = override
	}
	if override := strings.TrimSpace(NpmRegistryURLOverride); override != "" {
		profile.NpmRegistryURL = override
	}
	return profile
}

// Lookup returns a copy of a registered profile.
func Lookup(id string) (Profile, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	profile, ok := registry[strings.TrimSpace(id)]
	return profile, ok
}

// RegisteredIDs returns a sorted snapshot for architecture tests and build
// diagnostics.
func RegisteredIDs() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func validate(profile Profile) error {
	required := map[string]string{
		"id": profile.ID, "display name": profile.DisplayName, "binary name": profile.BinaryName,
		"template manifest name": profile.TemplateManifestName, "taskfile name": profile.TaskfileName,
		"error code prefix": profile.ErrorCodePrefix, "CLI path env": profile.CLIPathEnv,
		"project dir env": profile.ProjectDirEnv, "business ID env": profile.BusinessIDEnv,
		"OpenAPI user agent env": profile.OpenAPIUserAgentEnv, "project env file env": profile.ProjectEnvFileEnv,
		"config file name": profile.ConfigFileName, "business id": profile.BusinessID,
		"user agent product": profile.UserAgentProduct, "project region": profile.ProjectRegion,
		"openapi endpoint": profile.OpenAPIEndpoint, "openapi service": profile.OpenAPIService,
		"openapi region": profile.OpenAPIRegion, "voicechat version": profile.VoiceChatVersion,
		"agent user id field": profile.AgentUserIDField, "agent target user id field": profile.AgentTargetUserIDField,
		"sdk name": profile.SDKName, "sdk version": profile.SDKVersion, "docs mcp endpoint": profile.DocsMCPEndpoint,
		"npm package": profile.NpmPackage, "npm registry URL": profile.NpmRegistryURL,
		"official skill name": profile.OfficialSkillName, "state dir name": profile.StateDirName,
		"state dir env": profile.StateDirEnv, "update notifier env": profile.UpdateNotifierEnv,
		"skills notifier env": profile.SkillsNotifierEnv,
		"access key env":      profile.AccessKeyEnv, "secret key env": profile.SecretKeyEnv,
		"session token env": profile.SessionTokenEnv, "auth storage namespace": profile.AuthStorageNamespace,
		"auth home env": profile.AuthHomeEnv, "auth directory name": profile.AuthDirName,
		"oauth authorize endpoint": profile.OAuthAuthorizeEndpoint, "oauth token endpoint": profile.OAuthTokenEndpoint,
		"oauth local client id": profile.OAuthLocalClientID, "oauth scope": profile.OAuthScope,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("product profile %s is required", field)
		}
	}
	if profile.OAuthRefreshSkew <= 0 {
		return fmt.Errorf("product profile oauth refresh skew must be positive")
	}
	return nil
}
