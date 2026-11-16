// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package template

import (
	"testing"

	"convo-cli/internal/product"
)

func TestBytePlusVoiceAgentTemplateUsesPinnedOfficialDemo(t *testing.T) {
	previous := product.SelectedID
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() { product.SelectedID = previous })
	tmpl, ok := Find("voice-agent", "web")
	if !ok || !tmpl.Available || !tmpl.Default || tmpl.Remote == nil || tmpl.dir != "" {
		t.Fatalf("BytePlus template = %+v", tmpl)
	}
	if tmpl.SDK.Name != "@byteplus/rtc" || tmpl.SDK.Version != product.BytePlusSDKVersion {
		t.Fatalf("BytePlus SDK = %+v", tmpl.SDK)
	}
	if tmpl.Remote.Repository != "byteplus-sdk/RTC_AIGC_Demo" ||
		tmpl.Remote.Commit != bytePlusVoiceAgentRemoteCommit ||
		tmpl.Remote.SHA256 != bytePlusVoiceAgentRemoteSHA256 {
		t.Fatalf("BytePlus remote source is not pinned: %+v", tmpl.Remote)
	}
}

func TestRegistryReturnsDefensiveCopiesAndAvailableEntries(t *testing.T) {
	all := List()
	available := Available()
	if len(all) == 0 || len(available) == 0 {
		t.Fatal("published registry must expose an available template")
	}
	for _, tmpl := range available {
		if !tmpl.Available {
			t.Fatalf("Available returned reserved template: %+v", tmpl)
		}
	}
	originalScene := all[0].Scene
	all[0].Scene = "mutated"
	again := List()
	if again[0].Scene != originalScene {
		t.Fatalf("List exposed registry backing storage: %+v", again[0])
	}
}
