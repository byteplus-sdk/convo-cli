// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package errorcodes

import (
	"os"
	"testing"
	"unicode"

	"gopkg.in/yaml.v3"
)

func TestBytePlusGuidanceUsesEnglish(t *testing.T) {
	for _, name := range []string{"byteplus-web.yaml", "byteplus-voice-agent.yaml"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var data file
		if err := yaml.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		for _, entry := range data.Codes {
			for _, text := range []string{entry.Meaning, entry.Fix, entry.Via} {
				for _, r := range text {
					if unicode.Is(unicode.Han, r) {
						t.Errorf("%s: code %s contains non-English guidance", name, entry.Code)
						break
					}
				}
			}
		}
	}
}
