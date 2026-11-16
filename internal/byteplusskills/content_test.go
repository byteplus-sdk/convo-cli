// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package byteplusskills

import (
	"path/filepath"
	"testing"

	"convo-cli/internal/skillscan"
)

func TestOfficialSkillQuality(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	problems, err := skillscan.CheckSkills(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Fatalf("BytePlus Skill problems: %+v", problems)
	}
}
