// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"sync"

	"github.com/spf13/cobra"

	"convo-cli/internal/affordance"
	"convo-cli/internal/config"
	"convo-cli/internal/errs"
	"convo-cli/internal/product"
)

// devSetupContract is owned by command orchestration. It decides when and how
// the project setup flow discovers resources; persistence and rollback remain
// in the shared command implementation.
type devSetupContract interface {
	ConfigureDevCommand(*cobra.Command) error
	ValidateOptions(devSetupOptions) error
	DevAffordance() affordance.Affordance
	NewConsoleClient(*config.Config) devConsoleClient
	CredentialBootstrapRequired(*config.Config, config.Report, bool) bool
	TrustExplicitRTCAppID() bool
	NoAvailableAppHint() string
	InitNextSteps(string) []string
}

var (
	devSetupContractsMu sync.RWMutex
	devSetupContracts   = map[string]devSetupContract{}
)

func registerDevSetupContract(id string, contract devSetupContract) {
	if id == "" || contract == nil {
		panic("dev setup contract requires an id and implementation")
	}
	devSetupContractsMu.Lock()
	defer devSetupContractsMu.Unlock()
	if _, exists := devSetupContracts[id]; exists {
		panic("dev setup contract already registered: " + id)
	}
	devSetupContracts[id] = contract
}

func currentDevSetupContract() (devSetupContract, error) {
	id := product.Current().ID
	devSetupContractsMu.RLock()
	contract := devSetupContracts[id]
	devSetupContractsMu.RUnlock()
	if contract == nil {
		return nil, errs.New("byteplus-rtc.cli.internal", errs.TypeInternal,
			"project setup contract is not registered for product %q", id)
	}
	return contract, nil
}
