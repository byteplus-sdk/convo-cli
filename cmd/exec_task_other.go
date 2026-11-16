// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package cmd

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func buildTaskCmd(shell, cmdline string) *exec.Cmd {
	return exec.Command(shell, "-c", cmdline)
}

func configureDevProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

type devProcessTree struct {
	root   int
	groups map[int]bool
}

func newDevProcessTree(cmd *exec.Cmd) *devProcessTree {
	return &devProcessTree{root: cmd.Process.Pid, groups: map[int]bool{cmd.Process.Pid: true}}
}

func (p *devProcessTree) close() {}

type devProcess struct {
	pid, parent, group int
	zombie             bool
}

func devProcesses() ([]devProcess, error) {
	raw, err := exec.Command("ps", "-axo", "pid=,ppid=,pgid=,stat=").Output()
	if err != nil {
		return nil, err
	}
	var processes []devProcess
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 {
			continue
		}
		pid, e1 := strconv.Atoi(fields[0])
		parent, e2 := strconv.Atoi(fields[1])
		group, e3 := strconv.Atoi(fields[2])
		if e1 == nil && e2 == nil && e3 == nil {
			processes = append(processes, devProcess{pid, parent, group, strings.HasPrefix(fields[3], "Z")})
		}
	}
	return processes, nil
}

func (p *devProcessTree) refresh() error {
	processes, err := devProcesses()
	if err != nil {
		return err
	}
	owned := map[int]bool{p.root: true}
	for changed := true; changed; {
		changed = false
		for _, proc := range processes {
			if !owned[proc.pid] && (owned[proc.parent] || p.groups[proc.group]) {
				owned[proc.pid] = true
				changed = true
			}
		}
	}
	groups := make(map[int]bool)
	for _, proc := range processes {
		if !proc.zombie && owned[proc.pid] && (owned[proc.group] || p.groups[proc.group]) {
			groups[proc.group] = true
		}
	}
	p.groups = groups
	return nil
}

func (p *devProcessTree) alive() bool {
	processes, err := devProcesses()
	if err == nil {
		for _, proc := range processes {
			if !proc.zombie && p.groups[proc.group] {
				return true
			}
		}
		return false
	}
	for group := range p.groups {
		if err := syscall.Kill(-group, 0); err == nil || err == syscall.EPERM {
			return true
		}
	}
	return false
}

func (p *devProcessTree) terminate() {
	for group := range p.groups {
		_ = syscall.Kill(-group, syscall.SIGTERM)
	}
}

func (p *devProcessTree) kill() {
	for group := range p.groups {
		_ = syscall.Kill(-group, syscall.SIGKILL)
	}
}
