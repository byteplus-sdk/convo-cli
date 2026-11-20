// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"convo-cli/internal/errs"
	"convo-cli/internal/template"
)

func TestDevPortEnvironmentReachesNodeListenersAndProxy(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	for _, automatic := range []bool{false, true} {
		t.Run(strconv.FormatBool(automatic), func(t *testing.T) {
			t.Setenv("NODE_OPTIONS", "")
			t.Setenv("SERVER_PORT", "1")
			ports := template.Ports{Web: reserveThenReleasePort(t), Server: reserveThenReleasePort(t)}
			if automatic {
				occupied, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer occupied.Close()
				ports.Server = occupied.Addr().(*net.TCPAddr).Port
				ports, err = autoSelectDevPorts(ports)
				if err != nil {
					t.Fatal(err)
				}
			}
			env, cleanup, err := prepareDevPortTaskEnv(t.TempDir(), ports)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			root := t.TempDir()
			for _, role := range []string{"server", "web"} {
				dir := filepath.Join(root, role)
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				script := `const http = require('node:http');
const web = require('node:path').basename(process.cwd()) === 'web';
const port = Number(web ? process.env.PORT : process.env.SERVER_PORT || 3001);
const server = http.createServer((req, res) => {
  if (web) {
    http.get('http://127.0.0.1:' + (process.env.SERVER_PORT || 3001) + req.url, upstream => upstream.pipe(res)).on('error', () => { res.statusCode = 502; res.end(); });
  } else res.end(JSON.stringify({port: server.address().port}));
});
server.listen(port, '127.0.0.1', () => console.log(server.address().port));`
				cmd := exec.Command(node, "-e", script)
				cmd.Dir, cmd.Env = dir, append(os.Environ(), env...)
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				cmd.Stderr = os.Stderr
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
				line := make(chan string, 1)
				go func() { value, _ := bufio.NewReader(stdout).ReadString('\n'); line <- strings.TrimSpace(value) }()
				want := ports.Server
				if role == "web" {
					want = ports.Web
				}
				select {
				case got := <-line:
					if got != strconv.Itoa(want) {
						t.Fatalf("%s listens on %s, want %d", role, got, want)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("listener startup timed out")
				}
			}
			client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
			resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(ports.Web) + "/runtimeConfig")
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			client.CloseIdleConnections()
			var result struct {
				Port int `json:"port"`
			}
			if err != nil || json.Unmarshal(raw, &result) != nil || result.Port != ports.Server {
				t.Fatalf("proxy response %s (%v), want server %d", raw, err, ports.Server)
			}
		})
	}
}

func TestResolveDevPortsRejectsInvalidTaskfilePort(t *testing.T) {
	tf := &template.Taskfile{Version: 2}
	tf.Runtime.Ports.Web = 70000
	_, err := resolveDevPorts(tf, 0, 0)
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.template.contract_invalid" || typed.Param != "runtime.ports.web" {
		t.Fatalf("error = %v, want invalid runtime.ports.web", err)
	}
}

func TestPrepareDevPortTaskEnvWritesNodeRoleBridge(t *testing.T) {
	t.Setenv("NODE_OPTIONS", "")
	env, cleanup, err := prepareDevPortTaskEnv(t.TempDir(), template.Ports{Web: 3002, Server: 3001})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if !strings.Contains(strings.Join(env, "\n"), "\nSERVER_PORT=3001\n") {
		t.Fatalf("missing backend/proxy port: %v", env)
	}
	if !strings.Contains(strings.Join(env, "\n"), "\nPORT=3002\n") {
		t.Fatalf("missing root launcher port: %v", env)
	}
	var option string
	for _, value := range env {
		if strings.HasPrefix(value, "NODE_OPTIONS=--require=") {
			option = strings.TrimPrefix(value, "NODE_OPTIONS=--require=")
		}
	}
	path, err := strconv.Unquote(option)
	if err != nil {
		t.Fatalf("NODE_OPTIONS require path %q: %v", option, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, expected := range []string{
		`role === "web"`,
		`process.env.PORT = process.env.CONVO_CLI_WEB_PORT`,
		`process.env.REACT_APP_AIGC_PROXY_HOST = "http://127.0.0.1:" + process.env.CONVO_CLI_SERVER_PORT`,
		`role === "server"`,
		`process.env.PORT = process.env.CONVO_CLI_SERVER_PORT`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("Node port bridge missing %q:\n%s", expected, script)
		}
	}
}

func TestDevPortAvailableRejectsIPv6Listener(t *testing.T) {
	listener, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if devPortAvailable(port) {
		t.Fatalf("port %d reported available while IPv6 listener is active", port)
	}
}

func TestResolveDevPortsRejectsDuplicateOverrides(t *testing.T) {
	tf := &template.Taskfile{Version: 2}
	_, err := resolveDevPorts(tf, 3002, 3002)
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.cli.invalid_flag" {
		t.Fatalf("error = %v, want invalid port flags", err)
	}
}

func TestAutoPortsAvoidWildcardListeners(t *testing.T) {
	for _, network := range []string{"tcp4", "tcp6"} {
		t.Run(network, func(t *testing.T) {
			web, err := net.Listen(network, ":0")
			if err != nil {
				t.Skip(err)
			}
			defer web.Close()
			server, err := net.Listen(network, ":0")
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			occupied := template.Ports{Web: web.Addr().(*net.TCPAddr).Port, Server: server.Addr().(*net.TCPAddr).Port}
			if err := preflightDevPorts(occupied); err == nil {
				t.Fatal("wildcard listener passed preflight")
			}
			selected, err := autoSelectDevPorts(occupied)
			if err != nil {
				t.Fatal(err)
			}
			if selected.Web == occupied.Web || selected.Server == occupied.Server || selected.Web == selected.Server {
				t.Fatalf("occupied=%+v selected=%+v", occupied, selected)
			}
			if err := preflightDevPorts(selected); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAIGCDemoLauncherReceivesDistinctWebAndServerPorts(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "server"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"package.json": `{"name":"byteplus-rtc-aigc-demo","scripts":{"setup":"npm install","dev":"node scripts/dev.cjs"}}`, "server/cliRuntime.js": "module.exports = {};"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	values, cleanup, err := prepareDevPortTaskEnv(dir, template.Ports{Web: 3000, Server: 3007})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	got := strings.Join(values, "\n")
	if !strings.Contains(got, "\nPORT=3000\n") || !strings.HasSuffix(got, "SERVER_PORT=3007") || strings.Contains(got, "NODE_OPTIONS=") {
		t.Fatalf("unexpected launcher environment: %v", values)
	}
}
