// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDevShutdownWaitsForDetachedChildCleanup(t *testing.T) {
	testDevDetachedShutdown(t, "3000", 8*time.Second, false)
}

func TestDevShutdownKillsItsDetachedChildrenAfterDeadline(t *testing.T) {
	testDevDetachedShutdown(t, "ignore", 300*time.Millisecond, true)
}

func TestDevShutdownCleansObservedChildAfterLauncherExit(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	dir := t.TempDir()
	launcher := `const fs = require('node:fs');
const { spawn } = require('node:child_process');
const child = spawn(process.execPath, ['-e', "process.on('SIGTERM', () => process.exit(0)); require('node:fs').writeFileSync('child-ready', 'yes'); setInterval(() => {}, 1000);"], { detached: true, stdio: 'ignore' });
setInterval(() => { if (fs.existsSync('exit-parent')) process.exit(7); }, 20);`
	if err := os.WriteFile(filepath.Join(dir, "launcher.cjs"), []byte(launcher), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "launcher.cjs")
	configureDevProcess(cmd)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	tree := newDevProcessTree(cmd)
	defer tree.kill()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "child-ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			if err := tree.refresh(); err != nil {
				t.Fatal(err)
			}
			t.Fatal("child did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := tree.refresh(); err != nil {
		t.Fatal(err)
	}
	if len(tree.groups) < 2 {
		t.Fatal("detached child group was not discovered")
	}
	if err := os.WriteFile(filepath.Join(dir, "exit-parent"), []byte("yes"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-waited:
		if err == nil {
			t.Fatal("launcher exit was expected to fail")
		}
		waited <- err
	case <-time.After(5 * time.Second):
		t.Fatal("launcher did not exit")
	}
	if err := stopDevTaskWithin(cmd, waited, tree, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	if tree.alive() {
		t.Fatal("shutdown left the detached child alive")
	}
}

func testDevDetachedShutdown(t *testing.T, delay string, grace time.Duration, wantError bool) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	dir := t.TempDir()
	child := `const fs = require('node:fs');
let stopping = false;
process.on('SIGTERM', () => {
 if (stopping || process.argv[2] === 'ignore') return;
 stopping = true;
 setTimeout(() => { fs.writeFileSync('cleaned', 'yes'); process.exit(0); }, Number(process.argv[2]));
});
fs.writeFileSync('child-ready', 'yes');
setInterval(() => {}, 1000);`
	launcher := `const { spawn } = require('node:child_process');
const child = spawn(process.execPath, ['child.cjs', process.argv[2]], { detached: true, stdio: 'inherit' });
process.on('SIGTERM', () => { process.kill(-child.pid, 'SIGTERM'); process.exit(0); });`
	for name, content := range map[string]string{"child.cjs": child, "launcher.cjs": launcher} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := buildTaskCmd("sh", quoteShellArgument(node)+" launcher.cjs "+delay+"; echo task-finished")
	configureDevProcess(cmd)
	cmd.Dir = dir
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr, cmd.WaitDelay = &logs, &logs, 2*time.Second
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	tree := newDevProcessTree(cmd)
	defer tree.kill()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "child-ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = tree.refresh()
			t.Fatal("child did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	err = stopDevTaskWithin(cmd, waited, tree, grace)
	if (err != nil) != wantError {
		t.Fatalf("shutdown error=%v, want error=%v", err, wantError)
	}
	if !wantError {
		if _, err := os.Stat(filepath.Join(dir, "cleaned")); err != nil {
			t.Fatal("shutdown returned before child cleanup")
		}
	}
	deadline = time.Now().Add(3 * time.Second)
	for tree.alive() {
		if time.Now().After(deadline) {
			t.Fatal("shutdown left a child process")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
