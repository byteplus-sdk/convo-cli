// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package template

import (
	"sync"

	"convo-cli/internal/errs"
	"convo-cli/internal/product"
)

// remoteTemplateContract validates the product-owned portion of an otherwise
// shared immutable remote template contract.
type remoteTemplateContract interface {
	Prepare([]RenderedFile) ([]RenderedFile, error)
	Validate(map[string]string, templateManifest, Taskfile) error
}

var (
	remoteTemplateContractsMu sync.RWMutex
	remoteTemplateContracts   = map[string]remoteTemplateContract{}
)

func registerRemoteTemplateContract(id string, contract remoteTemplateContract) {
	if id == "" || contract == nil {
		panic("remote template contract requires an id and implementation")
	}
	remoteTemplateContractsMu.Lock()
	defer remoteTemplateContractsMu.Unlock()
	if _, exists := remoteTemplateContracts[id]; exists {
		panic("remote template contract already registered: " + id)
	}
	remoteTemplateContracts[id] = contract
}

func validateSelectedRemoteTemplate(files map[string]string, manifest templateManifest, taskfile Taskfile) error {
	contract, err := selectedRemoteTemplateContract()
	if err != nil {
		return err
	}
	return contract.Validate(files, manifest, taskfile)
}

func prepareSelectedRemoteTemplate(files []RenderedFile) ([]RenderedFile, error) {
	contract, err := selectedRemoteTemplateContract()
	if err != nil {
		return nil, err
	}
	return contract.Prepare(files)
}

func selectedRemoteTemplateContract() (remoteTemplateContract, error) {
	id := product.Current().ID
	remoteTemplateContractsMu.RLock()
	contract := remoteTemplateContracts[id]
	remoteTemplateContractsMu.RUnlock()
	if contract == nil {
		return nil, errs.New("byteplus-rtc.cli.internal", errs.TypeInternal,
			"remote template contract is not registered for product %q", id)
	}
	return contract, nil
}

func registeredRemoteTemplateContractCount() int {
	remoteTemplateContractsMu.RLock()
	defer remoteTemplateContractsMu.RUnlock()
	return len(remoteTemplateContracts)
}
