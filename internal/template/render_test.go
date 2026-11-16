// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package template

import (
	"os"
	"path/filepath"
	"testing"

	"convo-cli/internal/config"
	"convo-cli/internal/errs"
)

// The published registry contains no non-instantiable placeholder rows.
func TestReservedSlotsNotRegistered(t *testing.T) {
	for _, slot := range [][2]string{{"video-call", "web"}, {"digital-human", "web"}} {
		if _, ok := Find(slot[0], slot[1]); ok {
			t.Fatalf("%s/%s should not be registered", slot[0], slot[1])
		}
	}
}

// The registry keeps its Available guard so a future not-yet-available entry is
// rejected before rendering.
func TestRenderNotAvailable(t *testing.T) {
	tmpl := Template{Scene: "future-scene", Platform: "web", Available: false}
	_, err := Render(tmpl, config.Default("x", "future-scene", "web"))
	e, ok := errs.As(err)
	if !ok || e.Code != "byteplus-rtc.template.not_found" {
		t.Fatalf("expected template.not_found for a not-available template, got %v", err)
	}
}

func TestWriteMaterializes(t *testing.T) {
	dir := t.TempDir()
	files := []RenderedFile{{Path: "src/main.js", Content: "export {};\n", Bytes: 11}}
	if err := Write(dir, files); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "main.js")); err != nil {
		t.Fatalf("expected src/main.js written: %v", err)
	}
}

func TestRenderAvailableTemplateWithMissingEmbeddedRootIsTyped(t *testing.T) {
	tmpl := Template{Scene: "test", Platform: "web", Available: true, dir: "missing"}
	_, err := Render(tmpl, config.Default("x", "test", "web"))
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.template.render_failed" {
		t.Fatalf("error=%v, want byteplus-rtc.template.render_failed", err)
	}
}

func TestWriteRejectsParentThatIsAFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "src"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Write(dir, []RenderedFile{{Path: "src/main.js", Content: "x"}})
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.template.render_failed" {
		t.Fatalf("error=%v, want byteplus-rtc.template.render_failed", err)
	}
}
