// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package nodeproject

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
)

type Package struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Engines         map[string]string `json:"engines"`
	Scripts         map[string]string `json:"scripts"`
}

func Read(dir string) (Package, error) {
	var pkg Package
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return pkg, err
	}
	err = json.Unmarshal(raw, &pkg)
	return pkg, err
}

func (p Package) HasDependency(name string) bool {
	return p.Dependencies[name] != "" || p.DevDependencies[name] != ""
}

func NeedsInstall(dir string, fullStack bool) (bool, error) {
	dirs := []string{dir}
	if fullStack {
		dirs = append(dirs, filepath.Join(dir, "web"), filepath.Join(dir, "server"))
	}
	for _, packageDir := range dirs {
		pkg, err := Read(packageDir)
		if os.IsNotExist(err) && !fullStack {
			_, err = os.Stat(filepath.Join(packageDir, "node_modules"))
			if os.IsNotExist(err) {
				return true, nil
			}
			return false, err
		}
		if err != nil {
			return false, fmt.Errorf("read %s: %w", filepath.Join(packageDir, "package.json"), err)
		}
		for _, dependencies := range []map[string]string{pkg.Dependencies, pkg.DevDependencies} {
			for name := range dependencies {
				_, err := os.Stat(filepath.Join(packageDir, "node_modules", filepath.FromSlash(name), "package.json"))
				if os.IsNotExist(err) {
					return true, nil
				}
				if err != nil {
					return false, err
				}
			}
		}
	}
	return false, nil
}

type EngineCheck struct {
	Tool        string
	Requirement string
	Version     string
	Err         error
}

func CheckEngines(ctx context.Context, dir string) ([]EngineCheck, error) {
	pkg, err := Read(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return checkEngines(ctx, pkg, toolVersion), nil
}

func checkEngines(ctx context.Context, pkg Package, readVersion func(context.Context, string) (string, error)) []EngineCheck {
	var checks []EngineCheck
	for _, tool := range []string{"node", "npm"} {
		requirement := strings.TrimSpace(pkg.Engines[tool])
		if requirement == "" {
			continue
		}
		check := EngineCheck{Tool: tool, Requirement: requirement}
		constraint, err := semver.NewConstraint(requirement)
		if err != nil {
			check.Err = fmt.Errorf("invalid %s engine requirement %q", tool, requirement)
		} else {
			check.Version, check.Err = readVersion(ctx, tool)
			if check.Err == nil {
				version, err := semver.NewVersion(check.Version)
				if err != nil {
					check.Err = fmt.Errorf("%s did not return a valid version", tool)
				} else if !constraint.Check(version) {
					check.Err = fmt.Errorf("%s %s does not satisfy %s", tool, version, requirement)
				}
			}
		}
		checks = append(checks, check)
	}
	return checks
}

func toolVersion(ctx context.Context, tool string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, "--version")
	if runtime.GOOS == "windows" && tool == "npm" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/D", "/C", "npm --version")
	}
	raw, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot read %s version: %w", tool, err)
	}
	version, err := semver.NewVersion(strings.TrimSpace(string(raw)))
	if err != nil {
		return "", fmt.Errorf("%s did not return a valid version", tool)
	}
	return version.String(), nil
}
