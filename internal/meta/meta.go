// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package meta is the single source of truth for the CLI's identity.
//
// Every help text, error hint, skill and affordance resolves the binary name
// from BinName rather than hard-coding a string. Release builds may override
// the identity with ldflags.
package meta

import "convo-cli/internal/product"

// These values may be overridden at build time via -ldflags, e.g.
//
//	go build -ldflags "-X convo-cli/internal/meta.BinName=custom-cli"
var (
	// BinName is the invoked command name / binary name.
	BinName = product.Current().BinaryName

	// ConfigFileName is the canonical unified config file name selected with the
	// product build.
	ConfigFileName = product.Current().ConfigFileName

	// BusinessID is the stable RTC attribution tag for the selected product.
	BusinessID = product.Current().BusinessID

	// UserAgentProduct is the product token used for RTC request attribution.
	UserAgentProduct = product.Current().UserAgentProduct

	// Version is the CLI version; injected via ldflags in release builds.
	Version = "0.0.1-dev"

	// ReleaseMarker gives cross-platform postflight an unambiguous binary image
	// marker that cannot be confused with a version inside embedded Skill text.
	// The trailing semicolon is a required boundary: release builds inject
	// "CONVO_CLI_RELEASE_VERSION=<Version>;".
	ReleaseMarker = "CONVO_CLI_RELEASE_VERSION=0.0.1-dev;"

	// Commit is the build commit; injected via ldflags in release builds.
	Commit = "unknown"

	// BuildDate is the build timestamp; injected via ldflags in release builds.
	BuildDate = "unknown"
)
