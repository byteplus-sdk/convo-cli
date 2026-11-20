// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"sync"

	"github.com/spf13/cobra"

	"convo-cli/internal/config"
	"convo-cli/internal/errs"
	"convo-cli/internal/openapi"
	"convo-cli/internal/product"
)

type runtimeStartOptions struct {
	dryRun     bool
	localScene bool
}

type agentResourceContract interface {
	Command() *cobra.Command
	ConfigureStart(*cobra.Command)
	PrepareStart(context.Context, *config.Config, *cobra.Command, bool) (func(), error)
	PrepareRuntimeStart(context.Context, *config.Config, *openapi.StartVoiceChatRequest, runtimeStartOptions) error
}

var (
	agentResourceContractsMu sync.RWMutex
	agentResourceContracts   = map[string]agentResourceContract{}
)

func registerAgentResourceContract(id string, contract agentResourceContract) {
	if id == "" || contract == nil {
		panic("agent resource contract requires an id and implementation")
	}
	agentResourceContractsMu.Lock()
	defer agentResourceContractsMu.Unlock()
	if _, exists := agentResourceContracts[id]; exists {
		panic("agent resource contract already registered: " + id)
	}
	agentResourceContracts[id] = contract
}

func currentAgentResourceContract() (agentResourceContract, error) {
	id := product.Current().ID
	agentResourceContractsMu.RLock()
	contract := agentResourceContracts[id]
	agentResourceContractsMu.RUnlock()
	if contract == nil {
		return nil, errs.New("byteplus-rtc.cli.internal", errs.TypeInternal,
			"agent resource contract is not registered for product %q", id)
	}
	return contract, nil
}
