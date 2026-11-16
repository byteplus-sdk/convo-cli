// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"convo-cli/internal/meta"
)

// --- synthetic unit tests (no repo content) ---

func testTree() *cobra.Command {
	root := &cobra.Command{Use: "convo-cli"}
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().Bool("dry-run", false, "")

	agent := &cobra.Command{Use: "agent"}
	start := &cobra.Command{Use: "start", Run: func(*cobra.Command, []string) {}}
	start.Flags().Bool("yes", false, "")
	agent.AddCommand(start)

	token := &cobra.Command{Use: "token"}
	issue := &cobra.Command{Use: "issue", Run: func(*cobra.Command, []string) {}}
	issue.Flags().Bool("write", false, "")
	issue.Flags().String("room-id", "", "")
	token.AddCommand(issue)

	root.AddCommand(agent, token)
	return root
}

func TestValidateAcceptsRealCommands(t *testing.T) {
	cmds := []Command{
		{Raw: "convo-cli agent start --yes", Args: []string{"agent", "start", "--yes"}},
		{Raw: "convo-cli agent start --dry-run", Args: []string{"agent", "start", "--dry-run"}},
		{Raw: "convo-cli token issue --room-id room-2 --write", Args: []string{"token", "issue", "--room-id", "room-2", "--write"}},
		{Raw: "convo-cli token issue --format json", Args: []string{"token", "issue", "--format", "json"}},
		{Raw: "convo-cli agent start --yes -- -value", Args: []string{"agent", "start", "--yes", "--", "-value"}},
	}
	problems, unver := Validate(testTree(), cmds)
	if len(problems) != 0 || len(unver) != 0 {
		t.Fatalf("expected clean, got problems=%v unver=%v", problems, unver)
	}
}

func TestValidateFlagsDrift(t *testing.T) {
	cmds := []Command{
		{Raw: "convo-cli env write --asr-access-token X", Args: []string{"env", "write", "--asr-access-token", "X"}},
		{Raw: "convo-cli config set agent.llm.endpoint_id X", Args: []string{"config", "set", "agent.llm.endpoint_id", "X"}},
		{Raw: "convo-cli token issue --bogus", Args: []string{"token", "issue", "--bogus"}},
	}
	problems, _ := Validate(testTree(), cmds)
	if len(problems) != 3 {
		t.Fatalf("expected 3 problems, got %d: %v", len(problems), problems)
	}
}

func TestValidatePlaceholderIsUnverifiable(t *testing.T) {
	cmds := []Command{{Raw: "convo-cli <command>", Args: []string{"<command>"}}}
	problems, unver := Validate(testTree(), cmds)
	if len(problems) != 0 || len(unver) != 1 {
		t.Fatalf("expected 1 unverifiable, got problems=%v unver=%v", problems, unver)
	}
}

func TestHarvestFencedInlineAndEnvPrefix(t *testing.T) {
	dir := t.TempDir()
	md := "# doc\n\n```bash\nCONVO_CLI_NO_SKILLS_NOTICE=1 convo-cli doctor --format json\nconvo-cli token issue \\\n  --write\n```\n\nRun `convo-cli agent status` then `convo-cli doctor`.\n"
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	cmds, err := HarvestDirForBinary(dir, "convo-cli")
	if err != nil {
		t.Fatal(err)
	}
	// doctor (env-prefixed), token issue (continuation), agent status, doctor.
	if len(cmds) != 4 {
		t.Fatalf("expected 4 harvested, got %d: %+v", len(cmds), cmds)
	}
	// The continuation must be joined into one command.
	var joined bool
	for _, c := range cmds {
		if c.Args[0] == "token" && contains(c.Args, "--write") {
			joined = true
		}
	}
	if !joined {
		t.Fatalf("continuation not joined: %+v", cmds)
	}
}

// reasons collects the Reason field of every problem for easy assertion.
func reasons(ps []Problem) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Reason
	}
	return out
}

func countReason(ps []Problem, substr string) int {
	n := 0
	for _, p := range ps {
		if strings.Contains(p.Reason, substr) {
			n++
		}
	}
	return n
}

// TestScanFileSecretAndBannedAreCodeScoped locks in the three review fixes:
//   - pure-hex hashes (git SHA-1 / SHA-256) are not secrets;
//   - a real base64 blob in a code block still is;
//   - banned commands and secrets are only scanned inside fenced code blocks, so a
//     prose "don't use …" note or a hash in text never trips the gate;
//   - placeholder suppression is scoped to the matched token — a real key on an
//     "example" line is caught, only a <…>/$VAR wrapper is exempt.
func TestScanFileSecretAndBannedAreCodeScoped(t *testing.T) {
	md := strings.Join([]string{
		"# doc",
		"",
		"The legacy flow used `config set agent.llm.endpoint_id X` — do not use it.", // prose/inline → exempt
		"Commit " + strings.Repeat("a", 40) + " in text is fine.",                    // prose hash → exempt
		"",
		"```bash",
		"# pin commit " + strings.Repeat("b", 40),                                    // 40-hex SHA-1 → ok
		"# sha256 " + strings.Repeat("c", 64),                                        // 64-hex checksum → ok
		"convo-cli token issue --token QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVphYmNksv==", // base64 blob → secret
		"convo-cli config set agent.llm.endpoint_id X",                               // banned in code → flagged
		"convo-cli env write --token AKLTexampleKEYvalue1234567 # example",           // real key on example line → flagged
		"convo-cli env write --token <AKLTexampleKEYvalue1234567>",                   // placeholder-wrapped → exempt
		"```",
	}, "\n")

	problems := scanFile("doc.md", map[string]bool{}, []byte(md))

	if got := countReason(problems, "banned legacy command"); got != 1 {
		t.Fatalf("banned: want 1 (code only), got %d: %v", got, reasons(problems))
	}
	if got := countReason(problems, "possible secret literal"); got != 2 {
		t.Fatalf("secret: want 2 (base64 blob + bare AK key), got %d: %v", got, reasons(problems))
	}
}

func TestCheckSkillsEnforcesPublishingMetadataAndLicense(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "Byted-BP-Convo-Guide")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: Byted-BP-Convo-Guide\ndescription: voice agent\n---\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	problems, err := CheckSkills(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"only lowercase letters", "missing frontmatter version", "missing Skill root LICENSE"} {
		if countReason(problems, want) != 1 {
			t.Fatalf("missing problem %q: %v", want, reasons(problems))
		}
	}
}

func TestCheckSkillsRejectsInvalidVersionAndLicense(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "byted-product-example")
	if err := os.MkdirAll(filepath.Join(skillDir, "LICENSE"), 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: byted-product-example\ndescription: example\nversion: latest\n---\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	problems, err := CheckSkills(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"valid SemVer", "not a non-empty regular file"} {
		if countReason(problems, want) != 1 {
			t.Fatalf("missing problem %q: %v", want, reasons(problems))
		}
	}
}

func TestValidSkillVersion(t *testing.T) {
	for _, version := range []string{"0.1.0", "0.1.0-dev", "1.2.3-rc.4", "1.2.3+build.7"} {
		if !validSkillVersion(version) {
			t.Errorf("validSkillVersion(%q) = false", version)
		}
	}
	for _, version := range []string{"v1.2.3", "1.2", "latest", "1.2.3-01", "1.2.3-alpha.01"} {
		if validSkillVersion(version) {
			t.Errorf("validSkillVersion(%q) = true", version)
		}
	}
}

func TestSkillNamePublishingFormat(t *testing.T) {
	for _, name := range []string{"byted-las-asr", "byted-bp-convo-guide"} {
		if !skillName.MatchString(name) {
			t.Errorf("skillName rejects %q", name)
		}
	}
	for _, name := range []string{"voice-agent", "byted-product", "byted-BP-convo-guide"} {
		if skillName.MatchString(name) {
			t.Errorf("skillName accepts %q", name)
		}
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestHarvestUsesConfiguredBinaryName(t *testing.T) {
	for _, binaryName := range []string{meta.BinName, "custom.cli"} {
		t.Run(binaryName, func(t *testing.T) {
			dir := t.TempDir()
			markdown := "```bash\nNOTICE=0 " + binaryName + " doctor --bogus\n```\nRun `" + binaryName + " agent start`.\nRun `other-cli doctor`."
			if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(markdown), 0o600); err != nil {
				t.Fatal(err)
			}
			var commands []Command
			var err error
			if binaryName == meta.BinName {
				commands, err = HarvestDir(dir)
			} else {
				commands, err = HarvestDirForBinary(dir, binaryName)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(commands) != 2 {
				t.Fatalf("commands = %+v", commands)
			}
			if commands[0].Raw != binaryName+" doctor --bogus" {
				t.Fatalf("raw command = %q", commands[0].Raw)
			}
			problems, _ := Validate(testTree(), commands)
			if len(problems) == 0 {
				t.Fatal("invalid harvested command passed validation")
			}
		})
	}
}
