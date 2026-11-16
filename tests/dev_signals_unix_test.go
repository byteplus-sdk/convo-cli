// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package tests

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"convo-cli/internal/config"
	"convo-cli/internal/product"
)

func TestNpmDevProcessGroupSignalsWaitForCleanup(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	launcher, err := os.ReadFile(filepath.Join("..", "scripts", "run.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			dir := t.TempDir()
			if err := config.Save(config.Default("fixture", "test-scene", "web"), config.Path(dir)); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(dir, "scripts", "bin"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(binPath, filepath.Join(dir, "scripts", "bin", "cli")); err != nil {
				t.Fatal(err)
			}
			for name, content := range map[string]string{
				"scripts/run.js":               string(launcher),
				"package.json":                 `{"name":"@fixture/cli","version":"0","bin":{"cli":"scripts/run.js"}}`,
				product.Current().TaskfileName: "version: 2\nscene: test-scene\nplatform: web\nruntime:\n  topology: web-server\n  ports:\n    web: 3000\n    server: 3001\ntasks:\n  dev:\n    - node service.cjs\n",
				"service.cjs": `const http = require('node:http');
const fs = require('node:fs');
const servers = [];
let stopping = false;
process.on('SIGTERM', () => {
  if (stopping) return;
  stopping = true;
  setTimeout(() => {
    fs.writeFileSync('cleaned', 'yes');
    servers.forEach(server => server.close());
  }, 700);
});
fs.writeFileSync('service-pid', String(process.pid));
for (const port of [process.env.SERVER_PORT, process.env.CONVO_CLI_WEB_PORT]) {
  const server = http.createServer((req, res) => {
    res.end(req.url === '/health' ? '{"ok":true}' : req.url === '/runtimeConfig' ? '{"Result":{"appId":"fixture"}}' : 'page');
  });
  server.listen(Number(port), '127.0.0.1');
  servers.push(server);
}`,
			} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			stdout, err := os.Create(filepath.Join(dir, "stdout"))
			if err != nil {
				t.Fatal(err)
			}
			defer stdout.Close()
			stderr, err := os.Create(filepath.Join(dir, "stderr"))
			if err != nil {
				t.Fatal(err)
			}
			defer stderr.Close()
			cmd := exec.Command(node, "scripts/run.js", "dev", "--auto-port", "--startup-timeout=10s", "--format=json")
			cmd.Dir, cmd.Stdout, cmd.Stderr = dir, stdout, stderr
			cmd.Env = append(os.Environ(), "RTC_APP_ID=fixture", "RTC_APP_KEY=fixture-key", "CONVO_CLI_HOME="+filepath.Join(dir, "auth"), "CONVO_CLI_NO_UPDATE_NOTIFIER=1", "CONVO_CLI_NO_SKILLS_NOTIFIER=1")
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			waited := make(chan error, 1)
			go func() { waited <- cmd.Wait() }()
			finished := false
			defer func() {
				if raw, err := os.ReadFile(filepath.Join(dir, "service-pid")); err == nil {
					if pid, err := strconv.Atoi(string(raw)); err == nil {
						if group, err := syscall.Getpgid(pid); err == nil {
							_ = syscall.Kill(-group, syscall.SIGKILL)
						}
					}
				}
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				if !finished {
					<-waited
				}
			}()
			var response struct {
				OK   bool `json:"ok"`
				Data struct {
					Status string         `json:"status"`
					Ports  map[string]int `json:"ports"`
				} `json:"data"`
			}
			deadline := time.Now().Add(15 * time.Second)
			for {
				select {
				case err := <-waited:
					finished = true
					logs, _ := os.ReadFile(stderr.Name())
					t.Fatalf("dev exited before readiness: %v %s", err, logs)
				default:
				}
				raw, _ := os.ReadFile(stdout.Name())
				if json.Unmarshal(raw, &response) == nil && response.OK && response.Data.Status == "ready" {
					break
				}
				if time.Now().After(deadline) {
					logs, _ := os.ReadFile(stderr.Name())
					t.Fatalf("dev did not become ready: %s %s", raw, logs)
				}
				time.Sleep(20 * time.Millisecond)
			}
			if err := syscall.Kill(-cmd.Process.Pid, sig); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-waited:
				finished = true
				if err != nil {
					t.Fatalf("launcher exit: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("launcher did not finish cleanup")
			}
			if _, err := os.Stat(filepath.Join(dir, "cleaned")); err != nil {
				t.Fatal("launcher exited before delayed service cleanup")
			}
			if len(response.Data.Ports) != 2 {
				t.Fatalf("unexpected ports: %v", response.Data.Ports)
			}
			for name, port := range response.Data.Ports {
				listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
				if err != nil {
					t.Fatalf("%s port was not released: %v", name, err)
				}
				_ = listener.Close()
			}
		})
	}
}
