// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package template

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/product"
)

// TaskfileName returns the selected product's per-project task contract file.
func TaskfileName() string { return product.Current().TaskfileName }

// Taskfile is the parsed module task contract.
type Taskfile struct {
	Version  int     `yaml:"version"`
	Scene    string  `yaml:"scene"`
	Platform string  `yaml:"platform"`
	SDK      SDK     `yaml:"sdk"`
	Runtime  Runtime `yaml:"runtime,omitempty"`
	Tasks    struct {
		PostCreate []string `yaml:"post_create"`
		Install    []string `yaml:"install"`
		Dev        []string `yaml:"dev"`
	} `yaml:"tasks"`
}

// Runtime describes which generated process owns runtime responsibilities.
// It was added in taskfile v2; zero values intentionally preserve v1 behavior.
type Runtime struct {
	Topology     string `yaml:"topology,omitempty"`
	AgentControl string `yaml:"agent_control,omitempty"`
	Ports        Ports  `yaml:"ports,omitempty"`
}

// Ports declares the local listeners owned by a web-server template.
type Ports struct {
	Web    int `yaml:"web,omitempty"`
	Server int `yaml:"server,omitempty"`
}

// ServerManagedAgent reports whether the companion server, rather than the
// CLI's agent subcommands, owns the conversational-agent lifecycle.
func (t *Taskfile) ServerManagedAgent() bool {
	return t != nil && t.Version >= 2 && t.Runtime.AgentControl == "server"
}

// LoadTaskfile reads the taskfile from a project directory.
func LoadTaskfile(dir string) (*Taskfile, error) {
	name := TaskfileName()
	path := filepath.Join(dir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.New("byteplus-rtc.template.not_found", errs.TypeNotFound,
				"%s not found in %q", name, dir).
				WithHint("run `%s init` inside a scaffolded project", meta.BinName)
		}
		return nil, errs.Wrap(err, "byteplus-rtc.cli.internal", "read taskfile: %s", err)
	}
	var tf Taskfile
	if err := yaml.Unmarshal(raw, &tf); err != nil {
		return nil, errs.New("byteplus-rtc.template.render_failed", errs.TypeIO,
			"parse %s: %s", name, err)
	}
	return &tf, nil
}
