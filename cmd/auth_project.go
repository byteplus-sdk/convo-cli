// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"convo-cli/internal/auth"
	"convo-cli/internal/config"
	"convo-cli/internal/errs"
	"convo-cli/internal/product"
)

type projectAuthSetup struct {
	configPath string
	tokenPath  string
}

func prepareProjectAuth(dryRun bool) (*projectAuthSetup, error) {
	_, path, err := config.LoadNearest(".")
	if err != nil {
		return nil, err
	}
	profile := product.Current()
	dir := filepath.Dir(path)
	home := filepath.Join(dir, profile.AuthDirName)
	if override := strings.TrimSpace(os.Getenv(profile.AuthHomeEnv)); override != "" {
		absolute, err := filepath.Abs(override)
		if err != nil || absolute != home {
			return nil, errs.New("byteplus-rtc.cli.invalid_flag", errs.TypeValidation,
				"--project-auth conflicts with %s", profile.AuthHomeEnv).
				WithHint("unset %s before selecting project authentication", profile.AuthHomeEnv)
		}
	}
	setup := &projectAuthSetup{configPath: path, tokenPath: filepath.Join(home, "auth.json")}
	if dryRun {
		return setup, nil
	}
	if err := (auth.FileTokenStore{Path: setup.tokenPath}).Preflight(); err != nil {
		return nil, err
	}
	ignorePath := filepath.Join(dir, ".gitignore")
	if info, err := os.Lstat(ignorePath); err == nil && !info.Mode().IsRegular() {
		return nil, errs.New("byteplus-rtc.auth.storage_unavailable", errs.TypeAuth, ".gitignore must be a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, errs.New("byteplus-rtc.auth.storage_unavailable", errs.TypeAuth, "inspect .gitignore: %s", err)
	}
	raw, err := os.ReadFile(ignorePath)
	if err != nil && !os.IsNotExist(err) {
		return nil, errs.New("byteplus-rtc.auth.storage_unavailable", errs.TypeAuth, "read .gitignore: %s", err)
	}
	rule := "/" + profile.AuthDirName + "/"
	content := strings.TrimRight(string(raw), "\r\n")
	if !strings.HasSuffix(content, "\n"+rule) && content != rule {
		if content != "" {
			content += "\n"
		}
		if err := os.WriteFile(ignorePath, []byte(content+rule+"\n"), 0o644); err != nil {
			return nil, errs.New("byteplus-rtc.auth.storage_unavailable", errs.TypeAuth, "write .gitignore: %s", err)
		}
	}
	return setup, nil
}

func (s *projectAuthSetup) commit() error {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return err
	}
	cfg.Auth.Project = true
	return config.Save(cfg, s.configPath)
}
