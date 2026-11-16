// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"convo-cli/internal/product"
	"convo-cli/internal/skillscheck"
)

func resetOverrides() {
	ExecutableOverride = nil
	NpmInstallOverride = nil
	VerifyOverride = nil
	SkillsSyncOverride = nil
	SkillsAddOverride = nil
	SkillsRemoveOverride = nil
}

func TestDetectInstallMethod(t *testing.T) {
	t.Cleanup(resetOverrides)
	ExecutableOverride = func() (string, error) {
		return "/usr/local/lib/node_modules/@byteplus/convo-cli/scripts/convo-cli", nil
	}
	method, _, err := DetectInstallMethod()
	if err != nil || method != InstallNPM {
		t.Fatalf("method=%s err=%v", method, err)
	}
	ExecutableOverride = func() (string, error) { return "/usr/local/bin/convo-cli", nil }
	method, _, err = DetectInstallMethod()
	if err != nil || method != InstallManual {
		t.Fatalf("method=%s err=%v", method, err)
	}
}

func TestNpmPackageUsesSelectedProduct(t *testing.T) {
	previous := product.SelectedID
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() { product.SelectedID = previous })
	if got := NpmPackage(); got != "@byteplus/convo-cli" {
		t.Fatalf("NpmPackage()=%q", got)
	}
}

func TestOverrides(t *testing.T) {
	t.Cleanup(resetOverrides)
	called := 0
	NpmInstallOverride = func(context.Context, string, string) error { called++; return nil }
	VerifyOverride = func(context.Context, string, string) error { called++; return nil }
	SkillsSyncOverride = func(context.Context, string) (SkillsSyncResult, error) {
		called++
		return SkillsSyncResult{}, errors.New("offline")
	}
	if err := RunNpmInstall(context.Background(), "1.2.3", "/prefix/lib/node_modules/@byteplus/convo-cli/scripts/bin/convo-cli"); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBinary(context.Background(), "1.2.3", "/prefix/lib/node_modules/@byteplus/convo-cli/scripts/bin/convo-cli"); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncSkills(context.Background(), "1.2.3"); err == nil || called != 3 {
		t.Fatalf("called=%d err=%v", called, err)
	}
}

func TestNpmPrefix(t *testing.T) {
	for input, want := range map[string]string{
		"/usr/local/lib/node_modules/@byteplus/convo-cli/scripts/bin/convo-cli":                       "/usr/local",
		"/Users/me/.nvm/versions/node/v22/lib/node_modules/@byteplus/convo-cli/scripts/bin/convo-cli": "/Users/me/.nvm/versions/node/v22", // public-scan: allow — synthetic test path
		`C:\Users\me\AppData\Roaming\npm\node_modules\@byteplus\convo-cli\scripts\bin\convo-cli.exe`:  `C:\Users\me\AppData\Roaming\npm`,  // public-scan: allow — synthetic test path
	} {
		got, err := npmPrefix(input)
		if err != nil || got != want {
			t.Errorf("npmPrefix(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestRollbackBinaryAndCleanupBackup(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "convo-cli.exe")
	if err := os.WriteFile(binary, []byte("new"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary+".old", []byte("old"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := RollbackBinary(binary); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binary)
	if err != nil || string(data) != "old" {
		t.Fatalf("rollback data=%q err=%v", data, err)
	}
	if err := os.WriteFile(binary+".old", []byte("stale"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := CleanupBackup(binary); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(binary + ".old"); !os.IsNotExist(err) {
		t.Fatalf("backup remains: %v", err)
	}
	if err := CleanupBackup(binary); err != nil {
		t.Fatalf("cleanup must be idempotent: %v", err)
	}
}

func TestMaterializeSkillsUsesEmbeddedReleaseContent(t *testing.T) {
	root := t.TempDir()
	if err := materializeSkills(root); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"byted-bp-convo-guide"} {
		path := filepath.Join(root, "skills", name, "SKILL.md")
		if data, err := os.ReadFile(path); err != nil || len(data) == 0 {
			t.Fatalf("materialized %s: bytes=%d err=%v", path, len(data), err)
		}
	}
}

func TestMaterializeSkillsUsesBytePlusReleaseContent(t *testing.T) {
	previous := product.SelectedID
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() { product.SelectedID = previous })
	root := t.TempDir()
	if err := materializeSkills(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "skills", product.BytePlusOfficialSkill, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "convo-cli") {
		t.Fatalf("unexpected BytePlus Skill: %s", data)
	}

}

func TestSyncSkillsCopiesEmbeddedContentAndMarksVersion(t *testing.T) {
	t.Cleanup(resetOverrides)
	t.Setenv("CONVO_CLI_STATE_DIR", t.TempDir())

	var source string
	SkillsAddOverride = func(_ context.Context, gotSource string, args ...string) error {
		source = gotSource
		want := []string{"-g", "--copy", "-y"}
		if !slices.Equal(args, want) {
			t.Fatalf("skills add args=%q, want %q", args, want)
		}
		if _, err := os.Stat(filepath.Join(gotSource, "skills", "byted-bp-convo-guide", "SKILL.md")); err != nil {
			t.Fatalf("materialized skill unavailable during install: %v", err)
		}
		return nil
	}
	var removed []string
	SkillsRemoveOverride = func(_ context.Context, name string, args ...string) error {
		removed = append(removed, name)
		if !slices.Equal(args, []string{"-g", "-y"}) {
			t.Fatalf("skills remove name=%q args=%q", name, args)
		}
		return nil
	}

	if _, err := SyncSkills(context.Background(), "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("temporary source still exists after sync: %v", err)
	}
	wantRemoved := []string{"byted-bp-interactai-guide"}
	if !slices.Equal(removed, wantRemoved) {
		t.Fatalf("removed skills = %q, want %q", removed, wantRemoved)
	}
	state, err := os.ReadFile(filepath.Join(os.Getenv("CONVO_CLI_STATE_DIR"), "skills-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(state), `"version":"1.2.3"`) {
		t.Fatalf("unexpected skills state: %s", state)
	}
}

func TestSyncSkillsRemovesRetiredBytePlusSkill(t *testing.T) {
	t.Cleanup(resetOverrides)
	previous := product.SelectedID
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() { product.SelectedID = previous })
	t.Setenv("CONVO_CLI_STATE_DIR", t.TempDir())

	SkillsAddOverride = func(_ context.Context, source string, _ ...string) error {
		path := filepath.Join(source, "skills", product.BytePlusOfficialSkill, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("BytePlus Skill unavailable during install: %v", err)
		}
		return nil
	}
	var removed []string
	SkillsRemoveOverride = func(_ context.Context, name string, _ ...string) error {
		if name != "byted-bp-interactai-guide" {
			t.Fatalf("unexpected retired Skill cleanup: %s", name)
		}
		removed = append(removed, name)
		return nil
	}

	result, err := SyncSkills(context.Background(), "0.0.1-internal.2") // public-scan: allow — synthetic package version
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "byted-bp-interactai-guide" {
		t.Fatalf("retired Skills = %v", removed)
	}
	if result.Skill != product.BytePlusOfficialSkill || result.Action != SkillsActionInstalled || result.Version != "0.0.1-internal.2" { // public-scan: allow — synthetic package version
		t.Fatalf("unexpected sync result: %+v", result)
	}
}

func TestSyncSkillsLegacyRemovalFailureDoesNotAdvanceVersion(t *testing.T) {
	t.Cleanup(resetOverrides)
	stateDir := t.TempDir()
	t.Setenv("CONVO_CLI_STATE_DIR", stateDir)
	SkillsAddOverride = func(context.Context, string, ...string) error { return nil }
	var removed []string
	SkillsRemoveOverride = func(_ context.Context, name string, _ ...string) error {
		removed = append(removed, name)
		if name == "byted-bp-interactai-guide" {
			return errors.New("remove failed")
		}
		return nil
	}

	if _, err := SyncSkills(context.Background(), "2.0.0"); err == nil || !strings.Contains(err.Error(), "remove retired skill byted-bp-interactai-guide") {
		t.Fatalf("unexpected error: %v", err)
	}
	wantRemoved := []string{"byted-bp-interactai-guide"}
	if !slices.Equal(removed, wantRemoved) {
		t.Fatalf("removed skills = %q, want %q", removed, wantRemoved)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "skills-state.json")); !os.IsNotExist(err) {
		t.Fatalf("failed migration wrote state: %v", err)
	}
}

func TestSkillsRemoveOutputError(t *testing.T) {
	for _, output := range []string{
		"Failed to remove 1 skill(s)",
		"Could not remove skill from Codex: permission denied",
		"Could not scan directory /skills: permission denied",
	} {
		if skillsRemoveOutputError(output) == "" {
			t.Fatalf("failure output accepted: %q", output)
		}
	}
	for _, output := range []string{
		"Successfully removed 1 skill(s)",
		"No matching skills found for: byted-bp-interactai-guide",
	} {
		if reason := skillsRemoveOutputError(output); reason != "" {
			t.Fatalf("success output rejected: %q (%s)", output, reason)
		}
	}
}

func TestSkillsAddOnlyUnsupportedPromptScript(t *testing.T) {
	output := "■ Failed to install 1\n│\n│ ✗ byted-bp-convo-guide → PromptScript: PromptScript does not support global skill installation\n"
	if !skillsAddOnlyUnsupportedPromptScript(output) {
		t.Fatal("expected the single unsupported PromptScript target to be recoverable")
	}
	for _, output := range []string{
		"■ Failed to install 1\n│ ✗ byted-bp-convo-guide → Codex: permission denied\n",
		"■ Failed to install 2\n│ ✗ byted-bp-convo-guide → PromptScript: PromptScript does not support global skill installation\n│ ✗ byted-bp-convo-guide → Codex: permission denied\n",
	} {
		if skillsAddOnlyUnsupportedPromptScript(output) {
			t.Fatalf("unexpected recoverable installer output: %q", output)
		}
	}
}

func TestSyncSkillsFailureDoesNotAdvanceVersion(t *testing.T) {
	t.Cleanup(resetOverrides)
	stateDir := t.TempDir()
	t.Setenv("CONVO_CLI_STATE_DIR", stateDir)
	if err := skillscheck.MarkSynced("1.0.0"); err != nil {
		t.Fatal(err)
	}
	SkillsAddOverride = func(context.Context, string, ...string) error {
		return errors.New("installer failed")
	}

	if _, err := SyncSkills(context.Background(), "2.0.0"); err == nil {
		t.Fatal("expected sync failure")
	}
	state, err := os.ReadFile(filepath.Join(stateDir, "skills-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(state), `"version":"1.0.0"`) {
		t.Fatalf("sync failure advanced state: %s", state)
	}
}

func TestDetectInstallMethodPropagatesExecutableFailure(t *testing.T) {
	t.Cleanup(resetOverrides)
	ExecutableOverride = func() (string, error) { return "", errors.New("executable unavailable") }
	method, path, err := DetectInstallMethod()
	if method != InstallManual || path != "" || err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("method=%s path=%q err=%v", method, path, err)
	}
}

func TestBackupLifecycleHandlesAbsentCurrentBinary(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "convo-cli")
	if HasBackup(binary) {
		t.Fatal("backup unexpectedly exists")
	}
	if err := RollbackBinary(binary); err != nil {
		t.Fatalf("rollback without backup must be idempotent: %v", err)
	}
	if err := os.WriteFile(binary+".old", []byte("old"), 0o700); err != nil {
		t.Fatal(err)
	}
	if !HasBackup(binary) {
		t.Fatal("backup was not detected")
	}
	if err := RollbackBinary(binary); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(binary); err != nil || string(data) != "old" {
		t.Fatalf("restored data=%q err=%v", data, err)
	}
}

func TestVerifyBinaryOutputContract(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for _, tc := range []struct {
		name, body, expected, wantError string
	}{
		{name: "ok", body: `echo "convo-cli v1.2.3"`, expected: "1.2.3"},
		{name: "mismatch", body: `echo "convo-cli 2.0.0"`, expected: "1.2.3", wantError: "expected 1.2.3"},
		{name: "short", body: `echo "convo-cli"`, expected: "1.2.3", wantError: "unexpected version output"},
		{name: "failure", body: `echo "broken"; exit 4`, expected: "1.2.3", wantError: "broken"},
	} {
		err := VerifyBinary(context.Background(), tc.expected, write(tc.name, tc.body))
		if tc.wantError == "" && err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}
		if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
			t.Errorf("%s: error=%v, want containing %q", tc.name, err, tc.wantError)
		}
	}
}

func TestRunNpmInstallRejectsPathOutsideNpmPrefix(t *testing.T) {
	t.Cleanup(resetOverrides)
	err := RunNpmInstall(context.Background(), "1.2.3", filepath.Join(t.TempDir(), "convo-cli"))
	if err == nil || !strings.Contains(err.Error(), "cannot derive npm prefix") {
		t.Fatalf("error=%v", err)
	}
}

func TestRunSkillsRemoveOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	for _, tc := range []struct {
		name      string
		output    string
		exit      string
		wantError bool
		quiet     bool
	}{
		{"absent", "Found 71 unique installed skill(s)\nNo matching skills found for: retired-skill\n", "0", false, true},
		{"removed", "Successfully removed 1 skill(s)\n", "0", false, false},
		{"scan failure", "Could not scan directory /skills: permission denied\nNo matching skills found for: retired-skill\n", "0", true, false},
		{"removal failure", "Failed to remove 1 skill(s)\n", "0", true, false},
		{"process failure", "No matching skills found for: retired-skill\n", "1", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			script := "#!/bin/sh\nprintf '%s' \"$SKILLS_TEST_OUTPUT\"\nexit \"$SKILLS_TEST_EXIT\"\n"
			if err := os.WriteFile(filepath.Join(dir, "npx"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("SKILLS_TEST_OUTPUT", tc.output)
			t.Setenv("SKILLS_TEST_EXIT", tc.exit)
			capture, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			defer capture.Close()
			previous := os.Stderr
			os.Stderr = capture
			defer func() { os.Stderr = previous }()
			err = runSkillsRemove(context.Background(), "retired-skill", "-g", "-y")
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError = %v", err, tc.wantError)
			}
			got, err := os.ReadFile(capture.Name())
			if err != nil {
				t.Fatal(err)
			}
			want := tc.output
			if tc.quiet {
				want = ""
			}
			if string(got) != want {
				t.Fatalf("stderr = %q, want %q", got, want)
			}
		})
	}
}
