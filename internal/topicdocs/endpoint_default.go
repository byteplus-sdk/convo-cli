// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !topicdocs_e2e

package topicdocs

import "convo-cli/internal/product"

func newDefaultClient(version string, options ...Option) (*Client, error) {
	endpoint := product.Current().DocsMCPEndpoint
	if err := validateProductionEndpoint(endpoint); err != nil {
		return nil, err
	}
	return newClient(endpoint, version, nil, options...)
}
