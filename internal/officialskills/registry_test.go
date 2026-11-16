// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package officialskills

import (
	"strings"
	"testing"

	"convo-cli/internal/product"
	"convo-cli/internal/skillsfs"
)

func TestCurrentSelectsProductSkill(t *testing.T) {
	previous := product.SelectedID
	t.Cleanup(func() { product.SelectedID = previous })
	cases := []struct {
		productID string
		name      string
		command   string
	}{
		{product.BytePlusID, product.BytePlusOfficialSkill, "convo-cli"},
	}
	for _, test := range cases {
		product.SelectedID = test.productID
		reader := skillsfs.New(Current())
		skills, err := reader.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(skills) != 1 || skills[0].Name != test.name {
			t.Fatalf("product %s skills=%+v", test.productID, skills)
		}
		content, err := reader.ReadSkill(test.name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), test.command) {
			t.Fatalf("skill %s lacks command %q", test.name, test.command)
		}
	}
}
