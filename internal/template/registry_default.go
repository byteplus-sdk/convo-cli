// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package template

import "convo-cli/internal/product"

const (
	bytePlusVoiceAgentRemoteCommit    = "6c326ab0c01544fe0f85d9abf178f755d6919a13"
	bytePlusVoiceAgentCandidateSHA256 = "587339a4ddf2e499b295ba8aac689e7b890365b8306040ed52d5bec272ec5d12"
)

// The URL and checksum are variables so local/E2E tests can serve the exact
// candidate archive before its reviewed Demo commit is published to GitHub.
var (
	bytePlusVoiceAgentRemoteURL    string
	bytePlusVoiceAgentRemoteSHA256 = bytePlusVoiceAgentCandidateSHA256
)

type bytePlusRegistryProvider struct{}

func (bytePlusRegistryProvider) Templates() []Template {
	profile := product.Current()
	return []Template{{
		Scene:       "voice-agent",
		Platform:    "web",
		Title:       "BytePlus Voice AI demo (web)",
		Description: "Full RTC_AIGC_Demo with a React web app, companion server, resource-driven VoiceChat, and server-managed agent lifecycle.",
		SDK:         SDK{Name: profile.SDKName, Version: profile.SDKVersion},
		Available:   true,
		Default:     true,
		Remote: &RemoteSource{
			Repository: "byteplus-sdk/RTC_AIGC_Demo",
			URL:        bytePlusVoiceAgentRemoteURL,
			Commit:     bytePlusVoiceAgentRemoteCommit,
			SHA256:     bytePlusVoiceAgentRemoteSHA256,
		},
	}}
}

func init() { registerRegistryProvider(product.BytePlusID, bytePlusRegistryProvider{}) }
