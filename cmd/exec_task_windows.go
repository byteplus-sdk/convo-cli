// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package cmd

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// buildTaskCmd bypasses Go's CommandLineToArgvW-compatible quoting because
// cmd.exe uses different parsing rules. The outer quotes delimit the command
// string consumed by /C; cmdline itself remains unchanged.
func buildTaskCmd(shell, cmdline string) *exec.Cmd {
	cmd := exec.Command(shell)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: shell + ` /D /C "` + cmdline + `"`,
	}
	return cmd
}

func configureDevProcess(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
}

type devWindowsProcess struct {
	handle  windows.Handle
	created int64
}

type devProcessTree struct {
	root      *os.Process
	processes map[uint32]devWindowsProcess
	initErr   error
}

func openDevWindowsProcess(pid uint32) (devWindowsProcess, error) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return devWindowsProcess{}, err
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		_ = windows.CloseHandle(handle)
		return devWindowsProcess{}, err
	}
	return devWindowsProcess{handle: handle, created: created.Nanoseconds()}, nil
}

func newDevProcessTree(cmd *exec.Cmd) *devProcessTree {
	p := &devProcessTree{root: cmd.Process, processes: make(map[uint32]devWindowsProcess)}
	process, err := openDevWindowsProcess(uint32(cmd.Process.Pid))
	p.initErr = err
	if err == nil {
		p.processes[uint32(cmd.Process.Pid)] = process
	}
	return p
}

func (p *devProcessTree) refresh() error {
	if p.initErr != nil {
		return p.initErr
	}
	var captured windows.Filetime
	windows.GetSystemTimeAsFileTime(&captured)
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	var processes []windows.ProcessEntry32
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		processes = append(processes, entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return err
	}
	return p.discover(processes, captured.Nanoseconds())
}

func (p *devProcessTree) discover(entries []windows.ProcessEntry32, captured int64) error {
	for changed := true; changed; {
		changed = false
		for _, entry := range entries {
			if _, known := p.processes[entry.ProcessID]; known {
				continue
			}
			parent, owned := p.processes[entry.ParentProcessID]
			if !owned {
				continue
			}
			process, err := openDevWindowsProcess(entry.ProcessID)
			if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_ACCESS_DENIED) {
				continue
			}
			if err != nil {
				return err
			}
			if process.created < parent.created || process.created > captured {
				_ = windows.CloseHandle(process.handle)
				continue
			}
			p.processes[entry.ProcessID] = process
			changed = true
		}
	}
	return nil
}

func (p *devProcessTree) alive() bool {
	for _, process := range p.processes {
		if state, err := windows.WaitForSingleObject(process.handle, 0); err != nil || state == uint32(windows.WAIT_TIMEOUT) {
			return true
		}
	}
	return false
}

func (p *devProcessTree) terminate() {
	_ = windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(p.root.Pid))
}

func (p *devProcessTree) kill() {
	_ = p.refresh()
	for pid := range p.processes {
		handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
		if err == nil {
			_ = windows.TerminateProcess(handle, 1)
			_ = windows.CloseHandle(handle)
		}
	}
	_ = p.root.Kill()
}

func (p *devProcessTree) close() {
	for pid, process := range p.processes {
		_ = windows.CloseHandle(process.handle)
		delete(p.processes, pid)
	}
}
