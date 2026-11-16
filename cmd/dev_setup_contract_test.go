// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"convo-cli/internal/config"
	"convo-cli/internal/product"
)

func TestBytePlusDevSetupContractBootstrapsRTCWithoutBotDiscovery(t *testing.T) {
	previous := product.SelectedID
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() { product.SelectedID = previous })
	contract, err := currentDevSetupContract()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default("demo", "voice-agent", "web")
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	if !contract.CredentialBootstrapRequired(cfg, config.Validate(cfg), false) {
		t.Fatal("BytePlus setup did not enable RTC application bootstrap")
	}

}

func TestBytePlusDevHelpExposesRTCSetupWithoutBotSelector(t *testing.T) {
	previous := product.SelectedID
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() { product.SelectedID = previous })

	cmd := newDevCmd()
	if flag := cmd.Flags().Lookup("bot-id"); flag != nil {
		t.Fatalf("bot-id hidden = %v", flag != nil && flag.Hidden)
	}
	for _, name := range []string{"app-id", "reconfigure"} {
		flag := cmd.Flags().Lookup(name)
		if flag == nil || flag.Hidden {
			t.Fatalf("flag %q visible = %v", name, flag != nil && !flag.Hidden)
		}
	}
	var help bytes.Buffer
	cmd.SetOut(&help)
	cmd.SetErr(&help)
	if err := cmd.Help(); err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(help.String())
	if strings.Contains(lower, "bot") || !strings.Contains(lower, "reconfigure") || !strings.Contains(lower, "app-id") {
		t.Fatalf("BytePlus dev help leaked unsupported setup surface:\n%s", help.String())
	}
}
