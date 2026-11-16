// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package officialskills selects the build product's embedded Skill content.
package officialskills

import (
	"fmt"
	"io/fs"
	"sync"

	"convo-cli/internal/product"
)

var (
	registryMu sync.RWMutex
	registry   = map[string]fs.FS{}
)

func register(productID string, content fs.FS) {
	if productID == "" || content == nil {
		panic("official Skill registration is incomplete")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[productID]; exists {
		panic(fmt.Sprintf("official Skills for product %q are already registered", productID))
	}
	registry[productID] = content
}

// Current returns the embedded Skill filesystem for the selected product.
func Current() fs.FS {
	productID := product.Current().ID
	registryMu.RLock()
	content, ok := registry[productID]
	registryMu.RUnlock()
	if !ok {
		panic(fmt.Sprintf("official Skills for product %q are not registered", productID))
	}
	return content
}
