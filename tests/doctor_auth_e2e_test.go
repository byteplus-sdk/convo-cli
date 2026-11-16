// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDoctorRejectsExpiredUnrefreshableCredentials(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	writeAuthFixture(t, filepath.Join(home, "auth.json"), time.Now().Add(-time.Hour))
	r := run(t, dir, []string{"CONVO_CLI_HOME=" + home}, "doctor", "cli")
	if r.code == 0 || !strings.Contains(r.stdout, `"status": "FAIL"`) {
		t.Fatalf("expired auth: %s %s", r.stdout, r.stderr)
	}
}

func writeAuthFixture(t *testing.T, path string, expiry time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	sts := `{"AccessKeyId":"fixture-ak","SecretAccessKey":"fixture-sk","SessionToken":"fixture-session"}`
	raw, err := json.Marshal(map[string]any{"version": 1, "store": "file", "token": map[string]string{"access_token": sts, "expires_at": expiry.UTC().Format(time.RFC3339)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
