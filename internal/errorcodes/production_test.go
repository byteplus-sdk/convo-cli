// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package errorcodes

import (
	"strings"
	"testing"
)

func TestProductionSources(t *testing.T) {
	kb, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range kb.entries {
		text := entry.Meaning + entry.Fix + entry.Source
		for _, forbidden := range []string{"{{bin}}"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s contains %q", entry.Code, forbidden)
			}
		}
		if entry.Verified == VerifiedYes && (!strings.HasPrefix(entry.Source, "convo-cli docs fetch ") || entry.VerifiedAt != "2026-10-15") {
			t.Errorf("%s has incomplete provenance: %+v", entry.Code, entry)
		}
	}
	audio, _ := kb.Lookup("AUDIO_MIXING_ERROR_PRELOAD_FAILED")
	mixing, _ := kb.Lookup("ERR_PARAM_INVALID")
	if audio.Meaning == mixing.Meaning {
		t.Fatal("numeric codes lost their enum-specific descriptions")
	}
}
