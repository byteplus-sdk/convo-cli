// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package meta

import "testing"

func TestBusinessIDIsStableAcrossBinaryAliases(t *testing.T) {
	previous := BinName
	BinName = "custom-cli"
	t.Cleanup(func() { BinName = previous })

	if BusinessID != "byteplus_rtc_cli" {
		t.Fatalf("BusinessID = %q, want byteplus_rtc_cli", BusinessID)
	}
}
