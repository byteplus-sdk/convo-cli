// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package product_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"unicode"
)

func TestBytePlusEntrypointsUseEnglish(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, entry := range []string{"README.md", "package.json", "AGENTS.md", "CONTRIBUTING.md", "SECURITY.md", "SUPPORT.md", "RELEASING.md", "docs", "skills", "internal/errorcodes/byteplus-web.yaml", "internal/errorcodes/byteplus-voice-agent.yaml"} {
		err := filepath.WalkDir(filepath.Join(root, entry), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, r := range string(raw) {
				if unicode.Is(unicode.Han, r) {
					t.Errorf("%s contains non-English documentation", path)
					break
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Name string            `json:"name"`
		Bin  map[string]string `json:"bin"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "@byteplus/convo-cli" || len(manifest.Bin) != 1 || manifest.Bin["convo-cli"] != "scripts/run.js" {
		t.Fatalf("unexpected distribution identity: %+v", manifest)
	}
}
