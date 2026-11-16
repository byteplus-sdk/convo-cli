// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

// Command convo-cli is the BytePlus RTC conversational AI developer CLI.
// See internal/meta for the single identity source of truth.
package main

import (
	"os"

	"convo-cli/cmd"
	"convo-cli/internal/meta"
	"convo-cli/internal/telemetry"
)

func main() {
	telemetry.InitializeInvocation(telemetry.ResolveInvocationContextOptions{
		CLIName:    meta.UserAgentProduct,
		CLIVersion: meta.Version,
	})
	os.Exit(cmd.Execute())
}
