// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"convo-cli/internal/errs"
	"convo-cli/internal/template"
)

func TestDevServiceHelperProcess(t *testing.T) {
	mode := os.Getenv("GO_WANT_DEV_SERVICE_HELPER")
	if mode == "" {
		return
	}
	if mode == "compile-error" {
		_, _ = os.Stderr.WriteString("webpack compiled with 2 errors\n")
	}
	if mode == "vite-error" {
		_, _ = os.Stderr.WriteString("[web] VITE v8.3.5 ready in 12 ms\n[web] 10:00:00 [vite] Internal server error: missing import\n")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if mode == "unhealthy" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/runtimeConfig", func(w http.ResponseWriter, r *http.Request) {
		if mode == "bad-proxy" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"Result":{"appId":"test-app"}}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<!doctype html><html></html>"))
	})
	for _, key := range []string{"CONVO_CLI_WEB_PORT", "SERVER_PORT"} {
		listener, err := net.Listen("tcp", "127.0.0.1:"+os.Getenv(key))
		if err != nil {
			os.Exit(2)
		}
		go func() { _ = http.Serve(listener, mux) }()
		if mode == "crash" && key == "SERVER_PORT" {
			go func() { time.Sleep(time.Second); _ = listener.Close() }()
		}
	}
	if mode == "exit" {
		time.Sleep(time.Second)
		os.Exit(7)
	}
	select {}
}

func TestViteCompilationHandlesPrefixedLogs(t *testing.T) {
	log := &devCompilationLog{dest: io.Discard, required: true, vite: true, typecheckRequired: true}
	if ready, _, compiling := log.state(); ready || !compiling {
		t.Fatal("Vite became ready before its startup log")
	}
	for _, chunk := range []string{"[web] \x1b[32mVITE v8.3.5\x1b[0m rea", "dy in 83 ms\n"} {
		_, _ = log.Write([]byte(chunk))
	}
	if ready, failed, _ := log.state(); !ready || failed {
		t.Fatalf("Vite startup: ready=%v failed=%v", ready, failed)
	}
	_, _ = log.Write([]byte("[server] Internal server error: unrelated\n"))
	if _, failed, _ := log.state(); failed {
		t.Fatal("backend log treated as Vite compilation failure")
	}
	_, _ = log.Write([]byte("[web] 12:00:00 [vite] Pre-transform error: missing import\n"))
	if ready, failed, _ := log.state(); ready || !failed {
		t.Fatalf("Vite transform failure: ready=%v failed=%v", ready, failed)
	}
	_, _ = log.Write([]byte("[web] VITE v8.3.5 ready in 20 ms\n"))
	if ready, failed, _ := log.state(); !ready || failed {
		t.Fatalf("Vite restart: ready=%v failed=%v", ready, failed)
	}
}

func TestViteHTTPReadinessRequiresSuccessfulStartup(t *testing.T) {
	for _, mode := range []string{"vite-error", "vite-pending"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("GO_WANT_DEV_SERVICE_HELPER", mode)
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "web"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "web", "package.json"), []byte(`{"devDependencies":{"vite":"8.3.5"}}`), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "web", "index.html"), []byte("<html></html>"), 0o644); err != nil {
				t.Fatal(err)
			}
			ports := template.Ports{Web: reserveThenReleasePort(t), Server: reserveThenReleasePort(t)}
			ready := false
			err := runDevServices(context.Background(), dir, []string{testHelperCommand(t, "TestDevServiceHelperProcess")}, devPortEnv(ports), ports, time.Second, func() error { ready = true; return nil })
			if err == nil || ready {
				t.Fatalf("Vite startup: ready=%v err=%v", ready, err)
			}
			waitForDevPortsReleased(t, ports.Web, ports.Server)
		})
	}
}

func TestViteRecoveryRequiresSuccessfulModuleResponses(t *testing.T) {
	root := t.TempDir()
	log := &devCompilationLog{dest: io.Discard, required: true, vite: true, webRoot: root}
	errorLog := "[web] 12:00:00 [vite] Internal server error: invalid source\n[web] File: " + filepath.Join(root, "src", "app.tsx") + ":2:3\n"
	_, _ = log.Write([]byte("VITE v8.3.5 ready in 1 ms\n" + errorLog))
	_, _ = log.Write([]byte("[web] 12:00:01 [vite] (client) hmr update /src/app.tsx\n"))
	log.recoverVite(func(path string) bool {
		if path != "/src/app.tsx" {
			t.Fatalf("unexpected probe: %s", path)
		}
		return false
	})
	if ready, failed, _ := log.state(); ready || !failed {
		t.Fatal("HMR notification cleared a persistent transform error")
	}
	log.recoverVite(func(string) bool {
		_, _ = log.Write([]byte(errorLog))
		return true
	})
	if ready, failed, _ := log.state(); ready || !failed {
		t.Fatal("a stale probe cleared a newer transform error")
	}
	log.recoverVite(func(string) bool { return true })
	if ready, failed, _ := log.state(); !ready || failed {
		t.Fatal("successful module transform did not restore readiness")
	}
	_, _ = log.Write([]byte("[vite] Internal server error: invalid source\nFile: " + filepath.Join(root, "..", "outside.ts") + "\n"))
	log.recoverVite(func(string) bool {
		t.Fatal("probed a file outside the web root")
		return true
	})
}

func TestViteTypecheckFailurePreventsServiceStartup(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm unavailable")
	}
	t.Setenv("GO_WANT_DEV_SERVICE_HELPER", "exit")
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"package.json":  `{"devDependencies":{"vite":"8.3.5"},"scripts":{"typecheck":"node -e \"process.exit(9)\""}}`,
		"tsconfig.json": `{}`,
		"index.html":    `<html></html>`,
	} {
		if err := os.WriteFile(filepath.Join(dir, "web", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ports := template.Ports{Web: reserveThenReleasePort(t), Server: reserveThenReleasePort(t)}
	ready := false
	command := testHelperCommand(t, "TestDevServiceHelperProcess")
	separator := "; "
	if runtime.GOOS == "windows" {
		separator = " & "
	}
	command += separator + "echo started > unexpected-start"
	err := runDevServices(context.Background(), dir, []string{command}, devPortEnv(ports), ports, 10*time.Second, func() error { ready = true; return nil })
	if err == nil || ready || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("typecheck: ready=%v err=%v", ready, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "unexpected-start")); !os.IsNotExist(err) {
		t.Fatalf("task ran after failed typecheck: %v", err)
	}
	waitForDevPortsReleased(t, ports.Web, ports.Server)
}

func TestDevRequiresSuccessfulCompilation(t *testing.T) {
	for _, mode := range []string{"compile-error", "compile-pending"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("GO_WANT_DEV_SERVICE_HELPER", mode)
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "web"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "web", "package.json"), []byte(`{"dependencies":{"react-scripts":"5.0.1"}}`), 0o644); err != nil {
				t.Fatal(err)
			}
			ports := template.Ports{Web: reserveThenReleasePort(t), Server: reserveThenReleasePort(t)}
			ready := false
			err := runDevServices(context.Background(), dir, []string{testHelperCommand(t, "TestDevServiceHelperProcess")}, devPortEnv(ports), ports, time.Second, func() error { ready = true; return nil })
			if err == nil || ready {
				t.Fatalf("compilation: ready=%v err=%v", ready, err)
			}
			if mode == "compile-error" && !strings.Contains(err.Error(), "compilation failed") {
				t.Fatalf("error: %v", err)
			}
			waitForDevPortsReleased(t, ports.Web, ports.Server)
		})
	}
}

func TestDevCompilationWaitsForTypecheckAndHandlesChunkedLogs(t *testing.T) {
	log := &devCompilationLog{dest: io.Discard, required: true, typecheckRequired: true}
	for _, chunk := range []string{"\x1b[32mwebpack comp", "iled successfully\x1b[0m\n"} {
		_, _ = log.Write([]byte(chunk))
	}
	if ready, failed, _ := log.state(); ready || failed {
		t.Fatalf("pending typecheck: %v %v", ready, failed)
	}
	_, _ = log.Write([]byte("No issues found.\n"))
	if ready, failed, _ := log.state(); !ready || failed {
		t.Fatalf("successful compilation: %v %v", ready, failed)
	}
	_, _ = log.Write([]byte("Compiling...\nwebpack compiled successfully\n"))
	if ready, failed, compiling := log.state(); ready || failed || !compiling {
		t.Fatalf("rebuild reused a previous typecheck: %v %v %v", ready, failed, compiling)
	}
	_, _ = log.Write([]byte("ERROR in src/App.tsx:20:3\n"))
	if ready, failed, _ := log.state(); ready || !failed {
		t.Fatalf("typecheck failure: %v %v", ready, failed)
	}
}

func TestDevDetectsServiceLossWhileWatcherLives(t *testing.T) {
	t.Setenv("GO_WANT_DEV_SERVICE_HELPER", "crash")
	ports := template.Ports{Web: reserveThenReleasePort(t), Server: reserveThenReleasePort(t)}
	ready := false
	err := runDevServices(context.Background(), t.TempDir(), []string{testHelperCommand(t, "TestDevServiceHelperProcess")}, devPortEnv(ports), ports, 3*time.Second, func() error { ready = true; return nil })
	if err == nil || !ready || !strings.Contains(err.Error(), "failed readiness") {
		t.Fatalf("service loss: ready=%v err=%v", ready, err)
	}
	waitForDevPortsReleased(t, ports.Web, ports.Server)
}

func TestDevWaitsForBothServicesAndProxy(t *testing.T) {
	for _, mode := range []string{"unhealthy", "bad-proxy"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("GO_WANT_DEV_SERVICE_HELPER", mode)
			web, server := reserveThenReleasePort(t), reserveThenReleasePort(t)
			taskfile := "version: 2\nscene: test-scene\nplatform: web\nruntime:\n  topology: web-server\n  ports:\n    web: " + strconv.Itoa(web) + "\n    server: " + strconv.Itoa(server) + "\ntasks:\n  dev:\n    - " + strconv.Quote(testHelperCommand(t, "TestDevServiceHelperProcess")) + "\n"
			setupDevPortProject(t, taskfile)
			cmd := newDevCmd()
			cmd.SetArgs([]string{"--startup-timeout", "700ms"})
			stdout, err := captureDevStdout(cmd.Execute)
			typed, ok := errs.As(err)
			if !ok || typed.Code != "byteplus-rtc.dev.task_failed" || typed.IsReported() || !strings.Contains(typed.Message, "within 700ms") {
				t.Fatalf("unexpected failure: %#v", err)
			}
			if len(stdout) != 0 {
				t.Fatalf("unready services emitted success: %s", stdout)
			}
			waitForDevPortsReleased(t, web, server)
		})
	}
}

func TestDevReportsReadyAndPropagatesLaterFailure(t *testing.T) {
	t.Setenv("GO_WANT_DEV_SERVICE_HELPER", "exit")
	web, server := reserveThenReleasePort(t), reserveThenReleasePort(t)
	taskfile := "version: 2\nscene: test-scene\nplatform: web\nruntime:\n  topology: web-server\n  ports:\n    web: " + strconv.Itoa(web) + "\n    server: " + strconv.Itoa(server) + "\ntasks:\n  dev:\n    - " + strconv.Quote(testHelperCommand(t, "TestDevServiceHelperProcess")) + "\n"
	setupDevPortProject(t, taskfile)
	stdout, err := captureDevStdout(newDevCmd().Execute)
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.dev.task_failed" || !typed.IsReported() {
		t.Fatalf("later failure: %#v", err)
	}
	var result struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil || result.Data.Status != "ready" {
		t.Fatalf("readiness response: %s (%v)", stdout, err)
	}
}

func TestDevRejectsTaskExitBeforeReadiness(t *testing.T) {
	t.Setenv("GO_WANT_DEV_PORT_HELPER", "1")
	ports := template.Ports{Web: reserveThenReleasePort(t), Server: reserveThenReleasePort(t)}
	ready := false
	err := runDevServices(context.Background(), t.TempDir(), []string{testHelperCommand(t, "TestDevPortHelperProcess")}, devPortEnv(ports), ports, 2*time.Second, func() error { ready = true; return nil })
	if err == nil || ready {
		t.Fatalf("early successful exit: ready=%v err=%v", ready, err)
	}
}

func TestDevEndpointRejectsUnhealthyAndUnrelatedResponses(t *testing.T) {
	for _, tc := range []struct {
		kind, body string
		status     int
		want       bool
	}{
		{"health", `{"ok":true}`, 200, true},
		{"health", `{"ok":false}`, 200, false},
		{"health", `{"ok":true}`, 503, false},
		{"config", `{"Result":{"appId":"app"}}`, 200, true},
		{"config", `<html>fallback</html>`, 200, false},
		{"config", `{"Result":{}}`, 200, false},
		{"page", "compile failed", 500, false},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		port := server.Listener.Addr().(*net.TCPAddr).Port
		got := probeDevEndpoint(context.Background(), server.Client(), port, "/", tc.kind)
		server.Close()
		if got != tc.want {
			t.Errorf("%+v: got %v", tc, got)
		}
	}
}

func waitForDevPortsReleased(t *testing.T, ports ...int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for _, port := range ports {
		for !devPortAvailable(port) {
			if time.Now().After(deadline) {
				t.Fatalf("dev left port %d occupied", port)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
