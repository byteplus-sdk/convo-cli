// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"sync"

	"convo-cli/internal/product"
)

// voiceChatWireContract is owned by the OpenAPI consumer. Implementations
// encode only the product-specific request shape and action support; signing,
// transport, errors, and output remain shared.
type voiceChatWireContract interface {
	EncodeStart(StartVoiceChatRequest) any
	EncodeStop(StopVoiceChatRequest) any
}

var (
	voiceChatContractsMu sync.RWMutex
	voiceChatContracts   = map[string]voiceChatWireContract{}
)

func registerVoiceChatWireContract(id string, contract voiceChatWireContract) {
	if id == "" || contract == nil {
		panic("VoiceChat wire contract requires an id and implementation")
	}
	voiceChatContractsMu.Lock()
	defer voiceChatContractsMu.Unlock()
	if _, exists := voiceChatContracts[id]; exists {
		panic("VoiceChat wire contract already registered: " + id)
	}
	voiceChatContracts[id] = contract
}

func lookupVoiceChatWireContract(id string) voiceChatWireContract {
	voiceChatContractsMu.RLock()
	defer voiceChatContractsMu.RUnlock()
	return voiceChatContracts[id]
}

// EncodeStartVoiceChatRequest returns the exact selected wire shape for safe
// previews and fixtures without performing transport.
func EncodeStartVoiceChatRequest(request StartVoiceChatRequest) (any, error) {
	contract := lookupVoiceChatWireContract(product.Current().ID)
	if contract == nil {
		return nil, &APIError{Action: "StartVoiceChat", Message: "VoiceChat wire contract is not registered for the selected product"}
	}
	return contract.EncodeStart(request), nil
}
