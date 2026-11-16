// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package template

import "convo-cli/internal/product"

const (
	bytePlusVoiceAgentRemoteCommit    = "c6b974902f2ef5af4ebd5be83aa5af89c8ee98e6"
	bytePlusVoiceAgentCandidateSHA256 = "3f9ed94dc92342748b616dfec4fed9f22c8d709da67785db8acbea0026f5fa2d"
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
