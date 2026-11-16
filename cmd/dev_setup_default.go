// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/spf13/cobra"

	"convo-cli/internal/affordance"
	"convo-cli/internal/config"
	"convo-cli/internal/meta"
	"convo-cli/internal/openapi"
	"convo-cli/internal/product"
)

type bytePlusDevSetupContract struct{}

func (bytePlusDevSetupContract) ConfigureDevCommand(cmd *cobra.Command) error {
	cmd.Flags().Lookup("reconfigure").Usage = "select the RTC application again"
	return nil
}

func (bytePlusDevSetupContract) ValidateOptions(options devSetupOptions) error {

	return nil
}

func (bytePlusDevSetupContract) DevAffordance() affordance.Affordance {
	return affordance.Affordance{
		When:     []string{"Running a BytePlus scaffold; first run selects the RTC application"},
		Avoid:    []string{"Non-interactive automation while RTC AppID or AppKey is unset"},
		Prereq:   []string{"Valid non-credential config and Node toolchain; run auth login before interactive credential discovery"},
		Examples: []string{meta.BinName + " dev", meta.BinName + " dev --app-id <id>", meta.BinName + " dev --auto-port", meta.BinName + " dev --reconfigure", meta.BinName + " dev --dry-run"},
	}
}

func (bytePlusDevSetupContract) NewConsoleClient(cfg *config.Config) devConsoleClient {
	return openapi.NewConsoleClient(newAuthenticatedOpenAPIClient(cfg))
}

func (bytePlusDevSetupContract) CredentialBootstrapRequired(cfg *config.Config, report config.Report, force bool) bool {
	return config.CanBootstrapRTCAppCredentials(cfg, report, force)
}

func (bytePlusDevSetupContract) TrustExplicitRTCAppID() bool { return true }

func (bytePlusDevSetupContract) NoAvailableAppHint() string {
	return "create an RTC application or verify the current account/project permissions"
}

func (bytePlusDevSetupContract) InitNextSteps(scene string) []string {
	if scene != "voice-agent" {
		return []string{meta.BinName + " token issue --write", meta.BinName + " doctor", meta.BinName + " dev"}
	}
	return []string{
		meta.BinName + " auth login --browser=open",
		meta.BinName + " agent resources --write",
		meta.BinName + " dev              # join and control the agent from the Web app",
	}
}

func init() {
	registerDevSetupContract(product.BytePlusID, bytePlusDevSetupContract{})
}
