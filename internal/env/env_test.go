// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package env

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"convo-cli/internal/meta"
)

func TestLoadIntoProcessSetsUnsetAndWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(Path(root), []byte("CONVO_CLI_TEST_APPID=app-24-chars-000000000000\nCONVO_CLI_TEST_KEY=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Run from a nested subdir to exercise the walk-up.
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	os.Unsetenv("CONVO_CLI_TEST_APPID")
	os.Unsetenv("CONVO_CLI_TEST_KEY")
	t.Cleanup(func() { os.Unsetenv("CONVO_CLI_TEST_APPID"); os.Unsetenv("CONVO_CLI_TEST_KEY") })

	set, err := LoadIntoProcess(sub)
	if err != nil {
		t.Fatalf("LoadIntoProcess: %v", err)
	}
	if len(set) != 2 {
		t.Fatalf("set = %v, want both keys", set)
	}
	if os.Getenv("CONVO_CLI_TEST_APPID") != "app-24-chars-000000000000" || os.Getenv("CONVO_CLI_TEST_KEY") != "secret" {
		t.Fatalf("env not loaded: appid=%q key=%q", os.Getenv("CONVO_CLI_TEST_APPID"), os.Getenv("CONVO_CLI_TEST_KEY"))
	}
}

func TestLoadIntoProcessRealEnvWins(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(Path(root), []byte("CONVO_CLI_TEST_WINS=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONVO_CLI_TEST_WINS", "from-shell") // real env already set
	if _, err := LoadIntoProcess(root); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("CONVO_CLI_TEST_WINS"); got != "from-shell" {
		t.Fatalf("real env must win, got %q", got)
	}
}

func TestLoadIntoProcessMissingFileIsNoError(t *testing.T) {
	set, err := LoadIntoProcess(t.TempDir())
	if err != nil || set != nil {
		t.Fatalf("missing .env.local: set=%v err=%v", set, err)
	}
}

func TestWriteUsesBinaryNameInHeader(t *testing.T) {
	previous := meta.BinName
	meta.BinName = "convo-cli"
	t.Cleanup(func() { meta.BinName = previous })

	path := Path(t.TempDir())
	if _, err := Write(path, map[string]string{"RTC_APP_ID": "app-id"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	header := "# Written by `convo-cli env write`."
	if !strings.HasPrefix(string(raw), header) {
		t.Fatalf("env header = %q", raw)
	}
}

// TestLoadDotenvSemantics pins parity with Node dotenv / Vite (the same file the
// web runtime reads): quotes are stripped, `export ` is dropped, and inline
// comments on unquoted values are removed — so a quoted RTC_APP_KEY signs tokens
// with the same value the frontend uses, not a quote-wrapped mismatch.
func TestLoadDotenvSemantics(t *testing.T) {
	root := t.TempDir()
	content := "RTC_APP_KEY=\"abc secret\"\n" + // matched double quotes stripped, inner spaces kept
		"RTC_APP_ID='app-24-chars-000000000000'\n" + // single quotes stripped
		"export RTC_ROOM_ID=room-1\n" + // export prefix dropped
		"RTC_USER_ID=user-1 # trailing comment\n" + // inline comment removed
		"RTC_TAB_COMMENT=user-2\t# tab comment\n" + // tab-delimited inline comment removed
		"RTC_QUOTED_COMMENT=\"value\" # note\n" + // comment after closing quote ignored
		"RTC_HASH_COMMENT=ab#cd\n"
	if err := os.WriteFile(Path(root), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := Load(Path(root))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := map[string]string{
		"RTC_APP_KEY":        "abc secret",
		"RTC_APP_ID":         "app-24-chars-000000000000",
		"RTC_ROOM_ID":        "room-1",
		"RTC_USER_ID":        "user-1",
		"RTC_TAB_COMMENT":    "user-2",
		"RTC_QUOTED_COMMENT": "value",
		"RTC_HASH_COMMENT":   "ab",
	}
	for k, w := range want {
		if got := values[k]; got != w {
			t.Errorf("%s = %q, want %q", k, got, w)
		}
	}
}

func TestWriteReplacesExistingFileWithPrivatePermissions(t *testing.T) {
	path := Path(t.TempDir())
	if err := os.WriteFile(path, []byte("RTC_APP_ID=existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(path, map[string]string{"RTC_APP_KEY": "secret"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("env permissions = %04o", info.Mode().Perm())
	}
	values, err := Load(path)
	if err != nil || values["RTC_APP_ID"] != "existing" || values["RTC_APP_KEY"] != "secret" {
		t.Fatalf("env values = %v, error = %v", values, err)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".env-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files = %v, error = %v", matches, err)
	}
}

func TestWritePreservesQuotedAndCommentValues(t *testing.T) {
	path := Path(t.TempDir())
	values := map[string]string{
		"PADDED": "  padded  ",
		"HASH":   "value # literal",
		"QUOTED": "'quoted'",
		"SINGLE": "it's # literal",
	}
	if _, err := Write(path, values); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range values {
		if got[key] != value {
			t.Errorf("%s = %q, want %q", key, got[key], value)
		}
	}
}

func TestWriteRejectsLineInjection(t *testing.T) {
	for _, values := range []map[string]string{
		{"KEY\nINJECTED": "value"},
		{"KEY": "value\nINJECTED=secret"},
	} {
		path := Path(t.TempDir())
		if _, err := Write(path, values); err == nil {
			t.Fatal("expected validation error")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("unexpected output file: %v", err)
		}
	}
}

func TestWriteFillsTemplateAndMigratesSparseProject(t *testing.T) {
	for _, initial := range []string{"", "MODEL=old\nPROJECT=default\n"} {
		dir := t.TempDir()
		path := Path(dir)
		template := "# RTC\r\nAPP=\r\n\r\n# Model\r\nMODEL=\r\nPORT=3001\r\n"
		if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte(template), 0600); err != nil {
			t.Fatal(err)
		}
		if initial != "" {
			if err := os.WriteFile(path, []byte(initial), 0600); err != nil {
				t.Fatal(err)
			}
		}
		fields := map[string]string{"APP": "app", "MODEL": "new", "PROJECT": "default"}
		for range 2 {
			if _, err := Write(path, fields); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := "# RTC\nAPP=app\n\n# Model\nMODEL=new\nPORT=3001\nPROJECT=default\n"
			if strings.ReplaceAll(string(raw), "\r\n", "\n") != want {
				t.Fatalf("unexpected layout: %q", raw)
			}
		}
	}
}

func TestWriteUpdatesCommentedSecrets(t *testing.T) {
	path := Path(t.TempDir())
	if err := os.WriteFile(path, []byte("export KEY=old # keep\nCUSTOM='untouched'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "one#two", "two # three"} {
		if _, err := Write(path, map[string]string{"KEY": value}); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil || got["KEY"] != value {
			t.Fatal("updated value did not round-trip")
		}
		raw, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(raw), " # keep\nCUSTOM='untouched'\n") {
			t.Fatal("local formatting lost")
		}
	}
}

func TestWritePreservesDotenvDelimitersAndEscapes(t *testing.T) {
	cases := []struct{ initial, wantOld, update string }{
		{"old#note", "old", "old#note"},
		{"`old # data` # note", "old # data", "new'q#z"},
		{`"old\nvalue" # note`, "old\nvalue", `new'\n#z`},
		{`"quote\"and#hash" # note`, `quote\"and#hash`, `two'quotes"#end`},
	}
	for _, tc := range cases {
		path := Path(t.TempDir())
		if err := os.WriteFile(path, []byte("KEY="+tc.initial+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		values, err := Load(path)
		if err != nil || values["KEY"] != tc.wantOld {
			t.Fatalf("initial %q, got %q, err %v", tc.initial, values["KEY"], err)
		}
		var previous string
		for range 2 {
			if _, err := Write(path, map[string]string{"KEY": tc.update}); err != nil {
				t.Fatal(err)
			}
			values, err := Load(path)
			if err != nil || values["KEY"] != tc.update {
				t.Fatalf("%q produced %q, err %v", tc.update, values["KEY"], err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(string(raw), "#note\n") && !strings.HasSuffix(string(raw), "# note\n") || previous != "" && previous != string(raw) {
				t.Fatalf("unexpected output %q", raw)
			}
			previous = string(raw)
		}
	}
}

func TestTemplateDuplicateKeepsExistingLocalValue(t *testing.T) {
	dir := t.TempDir()
	path := Path(dir)
	if err := os.WriteFile(path, []byte("MODEL=local\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte("MODEL=first\nPORT=3001\nMODEL=last\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(path, map[string]string{"APP": "new"}); err != nil {
		t.Fatal(err)
	}
	values, err := Load(path)
	if err != nil || values["MODEL"] != "local" {
		t.Fatal("existing value was replaced by template default")
	}
}
