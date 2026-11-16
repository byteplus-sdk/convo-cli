// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"sync"

	"convo-cli/internal/affordance"
	"convo-cli/internal/config"
	"convo-cli/internal/errs"
	"convo-cli/internal/product"
	"convo-cli/internal/token"
)

type tokenIssueContract interface {
	CommandShort() string
	CommandAffordance() affordance.Affordance
	Issue(context.Context, *config.Config, token.IssueParams) (*token.IssueResult, error)
}

var (
	tokenIssueContractsMu sync.RWMutex
	tokenIssueContracts   = map[string]tokenIssueContract{}
)

func registerTokenIssueContract(id string, contract tokenIssueContract) {
	if id == "" || contract == nil {
		panic("token issue contract requires an id and implementation")
	}
	tokenIssueContractsMu.Lock()
	defer tokenIssueContractsMu.Unlock()
	if _, exists := tokenIssueContracts[id]; exists {
		panic("token issue contract already registered: " + id)
	}
	tokenIssueContracts[id] = contract
}

func currentTokenIssueContract() (tokenIssueContract, error) {
	id := product.Current().ID
	tokenIssueContractsMu.RLock()
	contract := tokenIssueContracts[id]
	tokenIssueContractsMu.RUnlock()
	if contract == nil {
		return nil, errs.New("byteplus-rtc.cli.internal", errs.TypeInternal,
			"token issue contract is not registered for product %q", id)
	}
	return contract, nil
}
