// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsDevShutdown(t *testing.T) {
	for _, mode := range []string{"cleanup", "timeout", "launcher-exit"} {
		t.Run(mode, func(t *testing.T) { runWindowsShutdownCase(t, mode) })
	}
}

func TestWindowsDemoWatcherShutdown(t *testing.T) {
	if os.Getenv("CONVO_TEST_DEMO_DIR") == "" {
		t.Skip("CONVO_TEST_DEMO_DIR is not set")
	}
	for _, mode := range []string{"node-cleanup", "node-timeout"} {
		t.Run(mode, func(t *testing.T) { runWindowsShutdownCase(t, mode) })
	}
}

func runWindowsShutdownCase(t *testing.T, mode string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, "-test.run=^TestWindowsDevShutdownDriver$", "-test.timeout=30s")
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = append(os.Environ(), "CONVO_SHUTDOWN_TEST_DIR="+t.TempDir(), "CONVO_SHUTDOWN_TEST_MODE="+mode)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE, HideWindow: true}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shutdown driver: %v\n%s", err, output)
	}
}

func TestWindowsDevShutdownDriver(t *testing.T) {
	dir := os.Getenv("CONVO_SHUTDOWN_TEST_DIR")
	if dir == "" {
		return
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	mode := os.Getenv("CONVO_SHUTDOWN_TEST_MODE")
	unrelated := exec.Command(self, "-test.run=^TestWindowsDevShutdownHelper$")
	unrelated.Env = append(os.Environ(), "CONVO_SHUTDOWN_TEST_ROLE=unrelated")
	configureDevProcess(unrelated)
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unrelated.Process.Kill(); _ = unrelated.Wait() }()
	waitWindowsShutdownFile(t, dir, "unrelated-ready")

	command := fmt.Sprintf(`"%s" -test.run=^TestWindowsDevShutdownHelper$`, self)
	if strings.HasPrefix(mode, "node-") {
		node, err := exec.LookPath("node")
		if err != nil {
			t.Fatal(err)
		}
		watcher := filepath.Join(os.Getenv("CONVO_TEST_DEMO_DIR"), "scripts", "server-dev.cjs")
		if _, err := os.Stat(watcher); err != nil {
			t.Fatal(err)
		}
		command = fmt.Sprintf(`"%s" "%s"`, node, watcher)
		service := `const fs = require('node:fs');
const path = require('node:path');
const dir = process.env.CONVO_SHUTDOWN_TEST_DIR;
fs.appendFileSync(path.join(dir, 'starts'), 'started\n');
const server = require('node:net').createServer();
server.listen(0, '127.0.0.1', () => {
 fs.writeFileSync(path.join(dir, 'service-port'), String(server.address().port));
 fs.writeFileSync(path.join(dir, 'service-ready'), 'yes');
});
process.on('SIGBREAK', () => {
 if (process.env.CONVO_SHUTDOWN_TEST_MODE.endsWith('timeout')) return;
 setTimeout(() => server.close(() => {
  fs.writeFileSync(path.join(dir, 'cleaned'), 'yes');
  process.exit(0);
 }), 700);
});`
		if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte(service), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := buildTaskCmd("cmd.exe", command)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CONVO_SHUTDOWN_TEST_ROLE=launcher")
	commandLine := cmd.SysProcAttr.CmdLine
	configureDevProcess(cmd)
	if cmd.SysProcAttr.CmdLine != commandLine {
		t.Fatal("process configuration changed the shell command line")
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	tree := newDevProcessTree(cmd)
	defer tree.close()
	defer tree.kill()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	waitWindowsShutdownFile(t, dir, "service-ready")
	if err := tree.refresh(); err != nil {
		t.Fatal(err)
	}
	if len(tree.processes) < 3 {
		t.Fatalf("expected shell, launcher and service, got %d processes", len(tree.processes))
	}
	if mode == "launcher-exit" {
		if err := os.WriteFile(filepath.Join(dir, "exit-launcher"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-waited:
			waited <- err
		case <-time.After(5 * time.Second):
			t.Fatal("launcher did not exit")
		}
	}
	rawPort, err := os.ReadFile(filepath.Join(dir, "service-port"))
	if err != nil {
		t.Fatal(err)
	}
	address := "127.0.0.1:" + string(rawPort)
	grace := 5 * time.Second
	if strings.HasSuffix(mode, "timeout") {
		grace = 500 * time.Millisecond
	}
	touchErrors := make(chan error, 1)
	if mode == "node-cleanup" {
		touch := time.AfterFunc(200*time.Millisecond, func() { touchErrors <- os.Chtimes(filepath.Join(dir, "app.js"), time.Now(), time.Now()) })
		defer touch.Stop()
	}
	started := time.Now()
	err = stopDevTaskWithin(cmd, waited, tree, grace)
	if strings.HasSuffix(mode, "timeout") {
		if err == nil || time.Since(started) < grace {
			t.Fatalf("shutdown did not enforce the grace period: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "cleaned")); !os.IsNotExist(err) {
			t.Fatal("uncooperative service unexpectedly completed cleanup")
		}
	} else {
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "cleaned")); err != nil {
			t.Fatalf("shutdown returned before service cleanup: %v", err)
		}
	}
	if mode == "node-cleanup" {
		select {
		case err := <-touchErrors:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("file change did not run during shutdown")
		}
	}
	if strings.HasPrefix(mode, "node-") {
		starts, err := os.ReadFile(filepath.Join(dir, "starts"))
		if err != nil || string(starts) != "started\n" {
			t.Fatalf("backend restarted during shutdown: %q, %v", starts, err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for tree.alive() {
		if time.Now().After(deadline) {
			t.Fatal("shutdown left a service running")
		}
		time.Sleep(20 * time.Millisecond)
	}
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatalf("service port was not released: %v", err)
	}
	_ = listener.Close()
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(unrelated.Process.Pid))
	if err != nil {
		t.Fatalf("unrelated process was terminated: %v", err)
	}
	defer windows.CloseHandle(handle)
	if state, err := windows.WaitForSingleObject(handle, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("unrelated process was affected: state=%d err=%v", state, err)
	}
}

func TestWindowsDevShutdownHelper(t *testing.T) {
	role := os.Getenv("CONVO_SHUTDOWN_TEST_ROLE")
	if role == "" {
		return
	}
	dir := os.Getenv("CONVO_SHUTDOWN_TEST_DIR")
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt)
	if role == "launcher" {
		self, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		child := exec.Command(self, "-test.run=^TestWindowsDevShutdownHelper$")
		child.Env = append(os.Environ(), "CONVO_SHUTDOWN_TEST_ROLE=service")
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		ticker := time.NewTicker(20 * time.Millisecond)
		for {
			select {
			case <-signals:
				os.Exit(0)
			case <-ticker.C:
				if _, err := os.Stat(filepath.Join(dir, "exit-launcher")); err == nil {
					os.Exit(0)
				}
			}
		}
	}
	var listener net.Listener
	if role == "service" {
		var err error
		listener, err = net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		if err := os.WriteFile(filepath.Join(dir, "service-port"), []byte(strconv.Itoa(port)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, role+"-ready"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for range signals {
		if role == "unrelated" {
			os.Exit(93)
		}
		if os.Getenv("CONVO_SHUTDOWN_TEST_MODE") == "timeout" {
			continue
		}
		time.Sleep(700 * time.Millisecond)
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "cleaned"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
	}
}

func waitWindowsShutdownFile(t *testing.T, dir, name string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", name)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestWindowsDevDiscoveryRejectsReusedParentPID(t *testing.T) {
	self := uint32(os.Getpid())
	current, err := openDevWindowsProcess(self)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(current.handle)
	parent := self + 1
	tree := &devProcessTree{processes: map[uint32]devWindowsProcess{parent: {created: current.created + 1}}}
	if err := tree.discover([]windows.ProcessEntry32{{ProcessID: self, ParentProcessID: parent}}, current.created+2); err != nil {
		t.Fatal(err)
	}
	if process, ok := tree.processes[self]; ok {
		_ = windows.CloseHandle(process.handle)
		t.Fatal("an older process was adopted through a reused parent PID")
	}
}

func TestWindowsDevDiscoveryRejectsProcessesCreatedAfterSnapshot(t *testing.T) {
	self := uint32(os.Getpid())
	current, err := openDevWindowsProcess(self)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(current.handle)
	parent := self + 1
	tree := &devProcessTree{processes: map[uint32]devWindowsProcess{parent: {created: current.created - 2}}}
	if err := tree.discover([]windows.ProcessEntry32{{ProcessID: self, ParentProcessID: parent}}, current.created-1); err != nil {
		t.Fatal(err)
	}
	if process, ok := tree.processes[self]; ok {
		_ = windows.CloseHandle(process.handle)
		t.Fatal("a PID reused after the snapshot was adopted")
	}
}

func TestWindowsDevDiscoveryIgnoresInaccessibleStaleParent(t *testing.T) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if windows.UTF16ToString(entry.ExeFile[:]) != "csrss.exe" {
			continue
		}
		self := uint32(os.Getpid())
		current, err := openDevWindowsProcess(self)
		if err != nil {
			t.Fatal(err)
		}
		tree := &devProcessTree{processes: map[uint32]devWindowsProcess{self: current}}
		defer tree.close()
		entry.ParentProcessID = self
		if err := tree.discover([]windows.ProcessEntry32{entry}, time.Now().UnixNano()); err != nil {
			t.Fatal(err)
		}
		if _, ok := tree.processes[entry.ProcessID]; ok {
			t.Fatal("unrelated protected process was adopted")
		}
		return
	}
	t.Skip("protected console process unavailable")
}
