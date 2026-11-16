// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package product

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type bytePlusContractFixture struct {
	SchemaVersion int `json:"schema_version"`
	Identity      struct {
		DisplayName      string `json:"display_name"`
		Binary           string `json:"binary"`
		ConfigFile       string `json:"config_file"`
		BusinessID       string `json:"business_id"`
		UserAgentProduct string `json:"user_agent_product"`
	} `json:"identity"`
	Config struct {
		ProjectRegion   string `json:"project_region"`
		OpenAPIEndpoint string `json:"openapi_endpoint"`
		OpenAPIService  string `json:"openapi_service"`
		OpenAPIRegion   string `json:"openapi_region"`
	} `json:"config"`
	Authentication struct {
		AuthorizeEndpoint string `json:"authorize_endpoint"`
		TokenEndpoint     string `json:"token_endpoint"`
		LocalClientID     string `json:"local_client_id"`
		Scope             string `json:"scope"`
		StorageNamespace  string `json:"storage_namespace"`
		HomeEnv           string `json:"home_env"`
		Directory         string `json:"directory"`
	} `json:"authentication"`
	OpenAPI struct {
		VoiceChatVersion       string `json:"voicechat_version"`
		AgentUserIDField       string `json:"agent_user_id_field"`
		AgentTargetUserIDField string `json:"agent_target_user_id_field"`
	} `json:"openapi"`
	Template struct {
		SDKName      string `json:"sdk_name"`
		SDKVersion   string `json:"sdk_version"`
		ManifestFile string `json:"manifest_file"`
		Taskfile     string `json:"taskfile"`
	} `json:"template"`
	PublicVocabulary struct {
		ErrorCodePrefix     string `json:"error_code_prefix"`
		CLIPathEnv          string `json:"cli_path_env"`
		ProjectDirEnv       string `json:"project_dir_env"`
		BusinessIDEnv       string `json:"business_id_env"`
		OpenAPIUserAgentEnv string `json:"openapi_user_agent_env"`
		ProjectEnvFileEnv   string `json:"project_env_file_env"`
	} `json:"public_vocabulary"`
	DocsMCPEndpoint string `json:"docs_mcp_endpoint"`
	Distribution    struct {
		NpmPackage     string `json:"npm_package"`
		NpmRegistryURL string `json:"npm_registry_url"`
	} `json:"distribution"`
	Skills struct {
		OfficialName      string `json:"official_name"`
		StateDirName      string `json:"state_dir_name"`
		StateDirEnv       string `json:"state_dir_env"`
		UpdateNotifierEnv string `json:"update_notifier_env"`
		SkillsNotifierEnv string `json:"skills_notifier_env"`
	} `json:"skills"`
}

func TestCurrentReturnsDefaultProfile(t *testing.T) {
	previous := SelectedID
	SelectedID = DefaultID
	t.Cleanup(func() { SelectedID = previous })
	profile := Current()
	if profile != defaultProfile {
		t.Fatalf("Current() = %+v, want %+v", profile, defaultProfile)
	}
	if ids := RegisteredIDs(); !reflect.DeepEqual(ids, []string{BytePlusID}) {
		t.Fatalf("registered IDs = %v", ids)
	}
}

func TestCurrentReturnsBytePlusProfile(t *testing.T) {
	previous := SelectedID
	SelectedID = BytePlusID
	t.Cleanup(func() { SelectedID = previous })
	if profile := Current(); profile != bytePlusProfile {
		t.Fatalf("Current() = %+v, want %+v", profile, bytePlusProfile)
	}
}

func TestCurrentAppliesDistributionOverrides(t *testing.T) {
	previousID, previousURL, previousName := SelectedID, NpmRegistryURLOverride, NpmPackageOverride
	SelectedID = BytePlusID
	NpmRegistryURLOverride = "https://registry.example.com/@scope%2Fpackage/latest"
	NpmPackageOverride = "@scope/package"
	t.Cleanup(func() {
		SelectedID, NpmRegistryURLOverride, NpmPackageOverride = previousID, previousURL, previousName
	})
	if got := Current().NpmPackage; got != NpmPackageOverride {
		t.Fatalf("package name=%q", got)
	}
	if got := Current().NpmRegistryURL; got != NpmRegistryURLOverride {
		t.Fatalf("registry URL=%q", got)
	}
}

func TestBytePlusContractFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "byteplus-contract.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture bytePlusContractFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 1 {
		t.Fatalf("schema version = %d", fixture.SchemaVersion)
	}
	p := bytePlusProfile
	got := []string{p.DisplayName, p.BinaryName, p.ConfigFileName, p.BusinessID, p.UserAgentProduct,
		p.ProjectRegion, p.OpenAPIEndpoint, p.OpenAPIService, p.OpenAPIRegion,
		p.OAuthAuthorizeEndpoint, p.OAuthTokenEndpoint, p.OAuthLocalClientID, p.OAuthScope, p.AuthStorageNamespace, p.AuthHomeEnv, p.AuthDirName,
		p.VoiceChatVersion, p.AgentUserIDField, p.AgentTargetUserIDField, p.SDKName, p.SDKVersion,
		p.TemplateManifestName, p.TaskfileName, p.ErrorCodePrefix, p.CLIPathEnv, p.ProjectDirEnv,
		p.BusinessIDEnv, p.OpenAPIUserAgentEnv, p.ProjectEnvFileEnv, p.DocsMCPEndpoint,
		p.NpmPackage, p.NpmRegistryURL, p.OfficialSkillName, p.StateDirName, p.StateDirEnv,
		p.UpdateNotifierEnv, p.SkillsNotifierEnv}
	want := []string{fixture.Identity.DisplayName, fixture.Identity.Binary, fixture.Identity.ConfigFile, fixture.Identity.BusinessID, fixture.Identity.UserAgentProduct,
		fixture.Config.ProjectRegion, fixture.Config.OpenAPIEndpoint, fixture.Config.OpenAPIService, fixture.Config.OpenAPIRegion,
		fixture.Authentication.AuthorizeEndpoint, fixture.Authentication.TokenEndpoint, fixture.Authentication.LocalClientID, fixture.Authentication.Scope, fixture.Authentication.StorageNamespace, fixture.Authentication.HomeEnv, fixture.Authentication.Directory,
		fixture.OpenAPI.VoiceChatVersion, fixture.OpenAPI.AgentUserIDField, fixture.OpenAPI.AgentTargetUserIDField, fixture.Template.SDKName, fixture.Template.SDKVersion,
		fixture.Template.ManifestFile, fixture.Template.Taskfile, fixture.PublicVocabulary.ErrorCodePrefix,
		fixture.PublicVocabulary.CLIPathEnv, fixture.PublicVocabulary.ProjectDirEnv, fixture.PublicVocabulary.BusinessIDEnv,
		fixture.PublicVocabulary.OpenAPIUserAgentEnv, fixture.PublicVocabulary.ProjectEnvFileEnv, fixture.DocsMCPEndpoint,
		fixture.Distribution.NpmPackage, fixture.Distribution.NpmRegistryURL, fixture.Skills.OfficialName,
		fixture.Skills.StateDirName, fixture.Skills.StateDirEnv, fixture.Skills.UpdateNotifierEnv,
		fixture.Skills.SkillsNotifierEnv}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BytePlus contract drifted:\n got: %q\nwant: %q", got, want)
	}
}

func TestCurrentFailsClosedForUnknownSelection(t *testing.T) {
	previous := SelectedID
	SelectedID = "unregistered"
	t.Cleanup(func() { SelectedID = previous })
	defer func() {
		if recover() == nil {
			t.Fatal("Current accepted an unknown profile")
		}
	}()
	_ = Current()
}

func TestProfileIsReturnedByValue(t *testing.T) {
	first := Current()
	first.DisplayName = "changed"
	if second := Current(); second.DisplayName != defaultProfile.DisplayName {
		t.Fatalf("profile mutation escaped: %+v", second)
	}
}

func TestValidateRejectsIncompleteAndUnsafeProfiles(t *testing.T) {
	missing := defaultProfile
	missing.OpenAPIEndpoint = ""
	if err := validate(missing); err == nil {
		t.Fatal("validate accepted missing endpoint")
	}
}
