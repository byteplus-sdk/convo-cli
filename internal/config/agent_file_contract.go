// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"sync"

	"convo-cli/internal/product"
)

// serverSceneValidationContract validates a persisted server scene.
type serverSceneValidationContract interface {
	Validate(*ServerScene) error
}

var (
	serverSceneValidationMu        sync.RWMutex
	serverSceneValidationContracts = map[string]serverSceneValidationContract{}
)

func registerServerSceneValidationContract(id string, contract serverSceneValidationContract) {
	if id == "" || contract == nil {
		panic("server scene validation contract requires an id and implementation")
	}
	serverSceneValidationMu.Lock()
	defer serverSceneValidationMu.Unlock()
	if _, exists := serverSceneValidationContracts[id]; exists {
		panic("server scene validation contract already registered: " + id)
	}
	serverSceneValidationContracts[id] = contract
}

func currentServerSceneValidationContract() serverSceneValidationContract {
	id := product.Current().ID
	serverSceneValidationMu.RLock()
	contract := serverSceneValidationContracts[id]
	serverSceneValidationMu.RUnlock()
	if contract == nil {
		panic("server scene validation contract is not registered for product " + id)
	}
	return contract
}
