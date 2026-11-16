// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package docscontract

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode"
)

var (
	localMarkdownLink = regexp.MustCompile(`\[[^]]+\]\((#[^)]+|[^)#]+\.md(?:#[^)]+)?)\)`)
	chineseText       = regexp.MustCompile(`\p{Han}`)
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readDoc(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func markdownHeadingSlug(heading string) string {
	heading = strings.ToLower(strings.TrimSpace(strings.TrimLeft(heading, "#")))
	var slug strings.Builder
	lastDash := false
	for _, r := range heading {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_', r == '-':
			slug.WriteRune(r)
			lastDash = r == '-'
		case unicode.IsSpace(r):
			if slug.Len() > 0 && !lastDash {
				slug.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(slug.String(), "-")
}

func hasMarkdownAnchor(content, fragment string) bool {
	if strings.Contains(content, `id="`+fragment+`"`) {
		return true
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			if markdownHeadingSlug(trimmed) == fragment {
				return true
			}
		}
	}
	return false
}

func TestPublicDocumentationUsesEnglish(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{"README.md", "docs/voice-agent.md", "docs/automation.md", "docs/troubleshooting.md", "SUPPORT.md", "SECURITY.md", "CONTRIBUTING.md"} {
		if chineseText.MatchString(readDoc(t, root, rel)) {
			t.Errorf("%s must use English", rel)
		}
	}
}

func TestPublicDocumentationPinsCurrentSetupAndCredentialRules(t *testing.T) {
	root := repoRoot(t)
	readme := readDoc(t, root, "README.md")
	for _, marker := range []string{"RTC application", "AppID", "AppKey", "~/.convo-cli/auth.json", "byteplus-rtc.config.yaml"} {
		if !strings.Contains(readme, marker) {
			t.Errorf("README.md missing setup contract %q", marker)
		}
	}
	security := readDoc(t, root, "SECURITY.md")
	for _, marker := range []string{"$CONVO_CLI_HOME/auth.json", "file by default", "only after the user explicitly selects `--store=keyring`"} {
		if !strings.Contains(security, marker) {
			t.Errorf("SECURITY.md missing credential-store contract %q", marker)
		}
	}
	for _, reversed := range []string{"keyring by default", "tokens belong in the operating-system credential store"} {
		if strings.Contains(security, reversed) {
			t.Errorf("SECURITY.md contains reversed credential-store claim %q", reversed)
		}
	}
}

func TestPublicDocumentationLocalMarkdownLinksResolve(t *testing.T) {
	root := repoRoot(t)
	files := []string{
		"README.md",
		"docs/voice-agent.md",
		"docs/automation.md",
		"docs/troubleshooting.md",
		"SUPPORT.md",
		"SECURITY.md",
		"CONTRIBUTING.md",
	}
	for _, rel := range files {
		content := readDoc(t, root, rel)
		for _, match := range localMarkdownLink.FindAllStringSubmatch(content, -1) {
			parts := strings.SplitN(match[1], "#", 2)
			targetRel := parts[0]
			targetContent := content
			if targetRel != "" {
				path := filepath.Clean(filepath.Join(root, filepath.Dir(rel), filepath.FromSlash(targetRel)))
				data, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("%s links to missing %s: %v", rel, match[1], err)
					continue
				}
				targetContent = string(data)
			}
			if len(parts) == 2 && !hasMarkdownAnchor(targetContent, parts[1]) {
				t.Errorf("%s links to missing anchor %s", rel, match[1])
			}
		}
		if strings.Contains(content, "five-minute-quick-start") {
			t.Errorf("%s retains the removed quick-start anchor", rel)
		}
	}
}
