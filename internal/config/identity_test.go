// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"

	"convo-cli/internal/errs"
)

func TestValidateIdentity(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"simple", "room-01", false},
		{"user default", "user-01", false},
		{"alnum", "Room123", false},
		{"allowed symbols", "a_b.c-d:e", false},
		{"with space", "meeting room 1", false},
		{"max length", strings.Repeat("a", 128), false},
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"too long", strings.Repeat("a", 129), true},
		{"disallowed slash", "room/01", true},
		{"disallowed unicode", "δωμάτιο01", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateIdentity("--room-id", tc.value)
			if tc.wantErr != (err != nil) {
				t.Fatalf("ValidateIdentity(%q): wantErr=%v got err=%v", tc.value, tc.wantErr, err)
			}
			if err != nil {
				te, ok := errs.As(err)
				if !ok || te.Code != "byteplus-rtc.config.invalid_identity" {
					t.Fatalf("expected byteplus-rtc.config.invalid_identity, got %v", err)
				}
				if te.Param != "--room-id" {
					t.Fatalf("expected param --room-id, got %q", te.Param)
				}
			}
		})
	}
}
