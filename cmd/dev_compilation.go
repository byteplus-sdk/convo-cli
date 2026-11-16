// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"convo-cli/internal/nodeproject"
)

var devLogANSI = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
var devLogLocation = regexp.MustCompile(`:\d+(:\d+)?$`)

type devCompilationLog struct {
	mu                sync.Mutex
	dest              io.Writer
	pending           string
	required          bool
	ready             bool
	failed            bool
	typecheckRequired bool
	typechecked       bool
	vite              bool
	typecheckScript   bool
	webRoot           string
	errorFiles        map[string]bool
	errorGeneration   uint64
	errorFilePending  bool
}

func newDevCompilationLog(dir string, dest io.Writer) *devCompilationLog {
	log := &devCompilationLog{dest: dest}
	log.webRoot, _ = filepath.Abs(filepath.Join(dir, "web"))
	if resolved, err := filepath.EvalSymlinks(log.webRoot); err == nil {
		log.webRoot = resolved
	}
	pkg, err := nodeproject.Read(filepath.Join(dir, "web"))
	if err != nil {
		return log
	}
	log.vite = pkg.HasDependency("vite")
	log.required = log.vite || pkg.HasDependency("react-scripts")
	log.typecheckScript = pkg.Scripts["typecheck"] != ""
	if _, err := os.Stat(filepath.Join(dir, "web", "tsconfig.json")); err == nil {
		log.typecheckRequired = true
	}
	return log
}

func (l *devCompilationLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n, err := l.dest.Write(p)
	if l.required {
		l.pending += string(p)
		for {
			line, rest, ok := strings.Cut(l.pending, "\n")
			if !ok {
				break
			}
			l.pending = rest
			line = strings.TrimSpace(devLogANSI.ReplaceAllString(line, ""))
			line = strings.TrimSpace(strings.TrimPrefix(line, "[web]"))
			if l.vite {
				switch {
				case strings.Contains(line, "[vite]") && (strings.Contains(line, "Internal server error:") || strings.Contains(line, "Pre-transform error:")):
					l.failed, l.ready = true, false
					l.errorGeneration++
					l.errorFilePending = true
				case l.errorFilePending && strings.HasPrefix(line, "File: "):
					file := devLogLocation.ReplaceAllString(strings.TrimPrefix(line, "File: "), "")
					rel, err := filepath.Rel(l.webRoot, file)
					if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) && len(l.errorFiles) < 32 {
						if l.errorFiles == nil {
							l.errorFiles = make(map[string]bool)
						}
						l.errorFiles[(&url.URL{Path: "/" + filepath.ToSlash(rel)}).EscapedPath()] = true
						l.errorFilePending = false
					}
				case strings.HasPrefix(line, "VITE v") && strings.Contains(line, "ready in"):
					l.ready, l.failed, l.typechecked = true, false, true
					l.errorFiles, l.errorFilePending = nil, false
				}
				continue
			}
			switch {
			case line == "Compiling...", line == "Starting the development server...":
				l.ready, l.failed, l.typechecked = false, false, false
			case line == "No issues found.":
				l.typechecked = true
			case strings.HasPrefix(line, "Failed to compile"), strings.HasPrefix(line, "ERROR in "),
				strings.HasPrefix(line, "webpack compiled") && strings.Contains(line, "error"):
				l.failed, l.ready = true, false
			case line == "Compiled successfully!", line == "Compiled with warnings.", line == "webpack compiled successfully",
				strings.HasPrefix(line, "webpack compiled with") && strings.Contains(line, "warning"):
				l.ready, l.failed = true, false
			}
		}
		if len(l.pending) > 8192 {
			l.pending = l.pending[len(l.pending)-8192:]
		}
	}
	return n, err
}

func (l *devCompilationLog) state() (bool, bool, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ready := !l.required || (l.ready && (!l.typecheckRequired || l.typechecked))
	return ready, l.failed, l.required && !ready && !l.failed
}

func (l *devCompilationLog) recoverVite(probe func(string) bool) {
	l.mu.Lock()
	if !l.vite || !l.failed || l.errorFilePending || len(l.errorFiles) == 0 {
		l.mu.Unlock()
		return
	}
	generation := l.errorGeneration
	paths := make([]string, 0, len(l.errorFiles))
	for path := range l.errorFiles {
		paths = append(paths, path)
	}
	l.mu.Unlock()
	for _, path := range paths {
		if !probe(path) {
			return
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if generation == l.errorGeneration {
		l.failed, l.ready, l.errorFiles = false, true, nil
	}
}
