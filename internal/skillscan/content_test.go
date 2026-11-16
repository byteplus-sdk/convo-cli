// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscan_test

import (
	"os"
	"path/filepath"
	"testing"

	"convo-cli/cmd"
	"convo-cli/internal/product"
	"convo-cli/internal/skillscan"
)

// moduleRoot returns the repo root (two levels up from internal/skillscan).
func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

// allowedUnverifiable is the explicit, documented allowlist of harvested commands
// that cannot be statically resolved because a placeholder stands in the
// subcommand position. Each entry is a deliberate GENERIC reference, not a real
// invocation, and is reported (never silently skipped). A new entry MUST carry a
// reason here in review.
var allowedUnverifiable = map[string]bool{
	product.BytePlusBinaryName + " <command>": true,
	// Generic "run --help for any subcommand" guidance in convo-cli-shared.
	product.BytePlusBinaryName + " <command> --help": true,
	// Generic "any convo-cli command" reference in the skill-template prose.
	product.BytePlusBinaryName + " …": true,
}

// TestOfficialSkillCommandsAreAuthentic harvests every `convo-cli …` command from the
// official Skills and the template and validates it against the live command tree.
func TestOfficialSkillCommandsAreAuthentic(t *testing.T) {
	root := cmd.NewRootCmd()
	var all []skillscan.Command
	for _, dir := range []string{"skills", "skill-template"} {
		cmds, err := skillscan.HarvestDirForBinary(filepath.Join(moduleRoot(t), dir), product.BytePlusBinaryName)
		if err != nil {
			t.Fatalf("harvest %s: %v", dir, err)
		}
		all = append(all, cmds...)
	}
	if len(all) == 0 {
		t.Fatal("harvested zero commands — the harvester or content is wrong")
	}
	problems, unverifiable := skillscan.Validate(root, all)
	for _, p := range problems {
		t.Errorf("command authenticity: %s", p)
	}
	for _, u := range unverifiable {
		if !allowedUnverifiable[u.Raw] {
			t.Errorf("unverifiable command not in allowlist: %s", u)
		}
	}
}

// TestOfficialSkillsPassMetadataAndSafetyChecks runs the metadata/link/secret/
// banned-command gate over the official Skills and the template.
func TestOfficialSkillsPassMetadataAndSafetyChecks(t *testing.T) {
	root := moduleRoot(t)
	problems, err := skillscan.CheckSkills(filepath.Join(root, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := skillscan.CheckContentFiles(filepath.Join(root, "skill-template"), filepath.Join(root, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range append(problems, tpl...) {
		t.Errorf("skill check: %s", p)
	}
}
