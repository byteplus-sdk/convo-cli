// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package template

import "convo-cli/internal/product"

const (
	bytePlusVoiceAgentRemoteCommit    = "26b9259d9009577403317cc0c2c53cabb188c2bf"
	bytePlusVoiceAgentCandidateSHA256 = "11662bc03bd69e3dbba56b44d49bdac01f3fd9c69875aa3cf91719f144774a46"
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
