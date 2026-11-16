// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package product_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryUsesOrdinaryBytePlusBuild(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, directory := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			if strings.HasSuffix(path, "_reference.go") {
				t.Errorf("unexpected reference implementation: %s", path)
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, line := range strings.Split(string(raw), "\n") {
				if strings.HasPrefix(line, "//go:build ") && strings.Contains(line, "reference") {
					t.Errorf("unexpected reference build selection: %s", path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
