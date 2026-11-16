// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillsfs

import (
	"testing"
	"testing/fstest"

	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"byted-bp-convo-guide/SKILL.md":                      {Data: []byte("---\nname: byted-bp-convo-guide\ndescription: voice agent\n---\n\n# body\n")},
		"byted-bp-convo-guide/references/quickstart.md":      {Data: []byte("# quickstart\n")},
		"byted-bp-convo-guide/references/troubleshooting.md": {Data: []byte("# trouble\n")},
		"example-skill/SKILL.md":                             {Data: []byte("---\nname: example-skill\ndescription: example skill\n---\n")},
		"example-skill/references/lifecycle.md":              {Data: []byte("# lifecycle\n")},
		// A stray dir with no SKILL.md must be ignored by List/ensureSkill.
		"not-a-skill/README.md": {Data: []byte("nope")},
	}
}

func TestListReportsNameDescriptionVersionReferences(t *testing.T) {
	r := New(testFS())
	skills, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d: %+v", len(skills), skills)
	}
	if skills[0].Name != "byted-bp-convo-guide" || skills[1].Name != "example-skill" {
		t.Fatalf("unexpected/unsorted names: %+v", skills)
	}
	voiceAgent := skills[0]
	if voiceAgent.Description != "voice agent" {
		t.Fatalf("description = %q", voiceAgent.Description)
	}
	if voiceAgent.Version != meta.Version {
		t.Fatalf("version = %q, want %q", voiceAgent.Version, meta.Version)
	}
	if len(voiceAgent.References) != 2 || voiceAgent.References[0] != "byted-bp-convo-guide/references/quickstart.md" {
		t.Fatalf("references = %v", voiceAgent.References)
	}
}

func TestReadSkillAndReferenceByteIdentical(t *testing.T) {
	fsys := testFS()
	r := New(fsys)

	got, err := r.ReadSkill("byted-bp-convo-guide")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(fsys["byted-bp-convo-guide/SKILL.md"].Data) {
		t.Fatal("SKILL.md bytes differ from source")
	}

	ref, cleaned, err := r.ReadReference("byted-bp-convo-guide", "references/quickstart.md")
	if err != nil {
		t.Fatal(err)
	}
	if cleaned != "references/quickstart.md" {
		t.Fatalf("cleaned = %q", cleaned)
	}
	if string(ref) != string(fsys["byted-bp-convo-guide/references/quickstart.md"].Data) {
		t.Fatal("reference bytes differ from source")
	}

	// Slash-form via SplitArg.
	name, rest := SplitArg("byted-bp-convo-guide/references/quickstart.md")
	if name != "byted-bp-convo-guide" || rest != "references/quickstart.md" {
		t.Fatalf("SplitArg = %q,%q", name, rest)
	}
}

func TestRejectsUnsafeAndUnknown(t *testing.T) {
	r := New(testFS())
	cases := []struct {
		name, ref, wantCode string
	}{
		{"byted-bp-convo-guide", "../../etc/passwd", "byteplus-rtc.skills.invalid_path"},
		{"byted-bp-convo-guide", "/etc/passwd", "byteplus-rtc.skills.invalid_path"},
		{"byted-bp-convo-guide", `..\evil`, "byteplus-rtc.skills.invalid_path"},
		// Windows-style "..\" embedded mid-path (not just as a prefix) must also be rejected.
		{"byted-bp-convo-guide", `references\..\..\etc\passwd`, "byteplus-rtc.skills.invalid_path"},
		{"byted-bp-convo-guide", "references/missing.md", "byteplus-rtc.skills.not_found"},
		{"no-such-skill", "references/x.md", "byteplus-rtc.skills.unknown_skill"},
		{"not-a-skill", "README.md", "byteplus-rtc.skills.unknown_skill"},
	}
	for _, c := range cases {
		_, _, err := r.ReadReference(c.name, c.ref)
		typed, ok := errs.As(err)
		if !ok || typed.Code != c.wantCode {
			t.Fatalf("ReadReference(%q,%q): err=%v, want %s", c.name, c.ref, err, c.wantCode)
		}
	}

	// A bare unknown skill name and traversal in the name are rejected too.
	if _, err := r.ReadSkill(".."); err == nil {
		t.Fatal("ReadSkill(\"..\") should fail")
	}
	if _, err := r.ReadSkill("not-a-skill"); err == nil {
		t.Fatal("ReadSkill(\"not-a-skill\") should fail (no SKILL.md)")
	}
}

func TestListPathOneLayer(t *testing.T) {
	r := New(testFS())
	entries, listed, err := r.ListPath("byted-bp-convo-guide/references")
	if err != nil {
		t.Fatal(err)
	}
	if listed != "byted-bp-convo-guide/references" || len(entries) != 2 {
		t.Fatalf("listed=%q entries=%v", listed, entries)
	}
}
