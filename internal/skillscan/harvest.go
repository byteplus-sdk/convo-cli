// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package skillscan is the Skill quality gate. It harvests every CLI
// invocation from Skill/template markdown (fenced blocks and inline code spans),
// then validates each against the live cobra command tree so a Skill can never
// teach a command/subcommand/flag the binary does not have. It also runs
// metadata/link/secret/banned-command checks. Everything is a plain Go check so
// the command tree itself is the source of truth (design: reuse NewRootCmd()).
package skillscan

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"convo-cli/internal/meta"
)

// Command is one harvested CLI invocation (Args excludes the leading verb).
type Command struct {
	Raw        string
	SourceFile string
	Line       int
	Args       []string
}

var envAssign = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*=[^\s]+$`)

// HarvestDir walks root for *.md and returns every harvested CLI command.
func HarvestDir(root string) ([]Command, error) {
	return HarvestDirForBinary(root, meta.BinName)
}

func HarvestDirForBinary(root, binaryName string) ([]Command, error) {
	var out []Command
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		cmds, err := harvestFile(path, binaryName)
		if err != nil {
			return err
		}
		out = append(out, cmds...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SourceFile != out[j].SourceFile {
			return out[i].SourceFile < out[j].SourceFile
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

func harvestFile(path, binaryName string) ([]Command, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	inlineCommand := regexp.MustCompile("`((?:[A-Z_][A-Z0-9_]*=[^`\\s]+\\s+)*" + regexp.QuoteMeta(binaryName) + "\\s+[^`]*)`")
	lines := strings.Split(string(data), "\n")
	var out []Command
	inFence := false
	for i := 0; i < len(lines); i++ {
		raw := strings.TrimSpace(lines[i])
		if strings.HasPrefix(raw, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			// A fenced command line: join `\`-continuations, strip ENV= prefixes.
			if args, ok := fencedCommand(lines, &i, binaryName); ok {
				out = append(out, Command{Raw: strings.Join(append([]string{binaryName}, args...), " "),
					SourceFile: path, Line: i + 1, Args: args})
			}
			continue
		}
		// Outside fences: harvest inline-code command spans.
		for _, m := range inlineCommand.FindAllStringSubmatch(lines[i], -1) {
			if args, ok := parseCommand(m[1], binaryName); ok {
				out = append(out, Command{Raw: binaryName + " " + strings.Join(args, " "),
					SourceFile: path, Line: i + 1, Args: args})
			}
		}
	}
	return out, nil
}

// fencedCommand joins a possibly `\`-continued command starting at *i and returns
// its args if it matches binaryName.
func fencedCommand(lines []string, i *int, binaryName string) ([]string, bool) {
	line := strings.TrimSpace(lines[*i])
	joined := trimContinuation(line)
	for continues(line) && *i+1 < len(lines) {
		*i++
		line = strings.TrimSpace(lines[*i])
		joined += " " + trimContinuation(line)
	}
	return parseCommand(joined, binaryName)
}

// parseCommand strips leading ENV=val assignments and a trailing `# …` shell
// comment and returns the matching command arguments.
func parseCommand(s, binaryName string) ([]string, bool) {
	fields := strings.Fields(s)
	for len(fields) > 0 && envAssign.MatchString(fields[0]) {
		fields = fields[1:]
	}
	if len(fields) == 0 || fields[0] != binaryName {
		return nil, false
	}
	args := fields[1:]
	// Drop a trailing shell comment ("… # note").
	for i, f := range args {
		if f == "#" {
			args = args[:i]
			break
		}
	}
	return args, true
}

func continues(line string) bool { return strings.HasSuffix(strings.TrimRight(line, " \t"), `\`) }

func trimContinuation(line string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimRight(line, " \t"), `\`))
}
