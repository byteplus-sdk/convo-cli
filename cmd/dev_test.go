// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"convo-cli/internal/config"
	projectenv "convo-cli/internal/env"
	"convo-cli/internal/errs"
	"convo-cli/internal/openapi"
	"convo-cli/internal/product"
	"convo-cli/internal/telemetry"
	"convo-cli/internal/template"
)

type fakeRTCAppClient struct {
	apps         []openapi.RtcApp
	listErr      error
	appKey       string
	appKeyErr    error
	requestedID  string
	listRequests int
}

func (f *fakeRTCAppClient) DescribeApps(context.Context, int, int) ([]openapi.RtcApp, error) {
	f.listRequests++
	return f.apps, f.listErr
}

func (f *fakeRTCAppClient) DescribeAppKeys(_ context.Context, appID string) (openapi.AppKeys, error) {
	f.requestedID = appID
	return openapi.AppKeys{AppID: appID, AppKey: f.appKey}, f.appKeyErr
}

func TestDevSetupFlags(t *testing.T) {
	cmd := newDevCmd()
	for _, name := range []string{"app-id", "reconfigure", "web-port", "server-port", "auto-port"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing dev flag %q", name)
		}
	}
	if cmd.Flags().Lookup("list-resources") != nil {
		t.Fatal("dev --list-resources should not be exposed")
	}

}

func TestDevPortFlagsRejectZero(t *testing.T) {
	cmd := newDevCmd()
	cmd.SetArgs([]string{"--web-port", "0"})
	err := cmd.Execute()
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.cli.invalid_flag" || typed.Param != "--web-port" {
		t.Fatalf("error = %v, want invalid --web-port", err)
	}
}

func TestDevCredentialBootstrapRunsWhenEitherCredentialIsMissing(t *testing.T) {
	cfg := config.Default("demo", "test-scene", "web")
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	if !config.CanBootstrapRTCAppCredentials(cfg, config.Validate(cfg), false) {
		t.Fatal("both missing credentials should trigger bootstrap")
	}

	t.Setenv("RTC_APP_ID", "aabbccddeeff001122334455")
	if !config.CanBootstrapRTCAppCredentials(cfg, config.Validate(cfg), false) {
		t.Fatal("missing AppKey should trigger bootstrap")
	}

	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "existing-key")
	if !config.CanBootstrapRTCAppCredentials(cfg, config.Validate(cfg), false) {
		t.Fatal("missing AppID should trigger bootstrap")
	}

	t.Setenv("RTC_APP_ID", "aabbccddeeff001122334455")
	t.Setenv("RTC_APP_KEY", "existing-key")
	if config.CanBootstrapRTCAppCredentials(cfg, config.Validate(cfg), false) {
		t.Fatal("complete credentials should not trigger bootstrap")
	}
	if !config.CanBootstrapRTCAppCredentials(cfg, config.Validate(cfg), true) {
		t.Fatal("forced reconfiguration should trigger bootstrap")
	}
}

func TestDevCredentialBootstrapRejectsOtherConfigErrors(t *testing.T) {
	cfg := config.Default("demo", "test-scene", "web")
	cfg.RTC.RoomID = ""
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	if config.CanBootstrapRTCAppCredentials(cfg, config.Validate(cfg), false) {
		t.Fatal("bootstrap must not hide unrelated config errors")
	}
}

func TestBootstrapRTCAppCredentialsSelectsFetchesAndWritesEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(projectenv.Path(dir), []byte("OTHER=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	t.Setenv("VITE_RTC_APP_ID", "${RTC_APP_ID}")
	fake := &fakeRTCAppClient{
		apps: []openapi.RtcApp{
			{AppID: "111111111111111111111111", Name: "One", Status: "1"},
			{AppID: "222222222222222222222222", Name: "Two", Status: "1"},
		},
		appKey: "selected-secret",
	}
	oldFactory, oldTerminal := newDevConsoleClient, rtcInputIsTerminal
	newDevConsoleClient = func(*config.Config) (devConsoleClient, error) { return fake, nil }
	rtcInputIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() {
		newDevConsoleClient = oldFactory
		rtcInputIsTerminal = oldTerminal
	})

	var prompt bytes.Buffer
	restore, err := bootstrapDevRTCAppCredentials(
		context.Background(), config.Default("demo", "test-scene", "web"), dir,
		strings.NewReader("\x1b[B \r"), &prompt,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if fake.requestedID != "222222222222222222222222" {
		t.Fatalf("DescribeAppKeys AppId = %q", fake.requestedID)
	}
	values, err := projectenv.Load(projectenv.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if values["RTC_APP_ID"] != fake.requestedID || values["VITE_RTC_APP_ID"] != fake.requestedID {
		t.Fatalf("AppID updates = %+v", values)
	}
	if values["RTC_APP_KEY"] != "selected-secret" || values["OTHER"] != "value" {
		t.Fatalf("env values = %+v", values)
	}
	if os.Getenv("RTC_APP_ID") != fake.requestedID || os.Getenv("RTC_APP_KEY") != "selected-secret" {
		t.Fatal("selected credentials were not applied to the dev process")
	}
	if strings.Contains(prompt.String(), "selected-secret") {
		t.Fatal("prompt leaked AppKey")
	}
	if !strings.Contains(prompt.String(), "↑/↓ move · space select · enter confirm") {
		t.Fatal("selector did not render keyboard instructions")
	}
}

func TestBootstrapRTCAppCredentialsOnlyOffersActiveApps(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	t.Setenv("VITE_RTC_APP_ID", "${RTC_APP_ID}")
	fake := &fakeRTCAppClient{
		apps: []openapi.RtcApp{
			{AppID: "disabled-app", Name: "Disabled", Status: "0"},
			{AppID: "active-app", Name: "Active", Status: "1"},
			{AppID: "other-app", Name: "Other", Status: "2"},
		},
		appKey: "selected-secret",
	}
	oldFactory, oldTerminal := newDevConsoleClient, rtcInputIsTerminal
	newDevConsoleClient = func(*config.Config) (devConsoleClient, error) { return fake, nil }
	rtcInputIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() {
		newDevConsoleClient = oldFactory
		rtcInputIsTerminal = oldTerminal
	})

	var prompt bytes.Buffer
	restore, err := bootstrapDevRTCAppCredentials(
		context.Background(), config.Default("demo", "test-scene", "web"), dir,
		strings.NewReader(" \r"), &prompt,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if fake.requestedID != "active-app" {
		t.Fatalf("DescribeAppKeys AppId = %q, want active-app", fake.requestedID)
	}
	if strings.Contains(prompt.String(), "Disabled") || strings.Contains(prompt.String(), "Other") {
		t.Fatalf("selector rendered a non-active app: %q", prompt.String())
	}
}

func TestBytePlusExplicitAppIDSkipsAppListing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	t.Setenv("VITE_RTC_APP_ID", "${RTC_APP_ID}")
	fake := &fakeRTCAppClient{
		listErr: errors.New("listing is unavailable"),
		appKey:  "selected-secret",
	}
	oldFactory, oldProduct := newDevConsoleClient, product.SelectedID
	newDevConsoleClient = func(*config.Config) (devConsoleClient, error) { return fake, nil }
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() {
		newDevConsoleClient = oldFactory
		product.SelectedID = oldProduct
	})

	restore, err := bootstrapDevRTCAppCredentialsWithOptions(
		context.Background(), config.Default("demo", "test-scene", "web"), dir,
		strings.NewReader(""), &bytes.Buffer{}, devSetupOptions{AppID: "explicit-app"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if fake.listRequests != 0 {
		t.Fatalf("DescribeApps calls = %d, want 0", fake.listRequests)
	}
	if fake.requestedID != "explicit-app" {
		t.Fatalf("DescribeAppKeys AppId = %q", fake.requestedID)
	}
}

func TestMapDevRTCAppPermissionErrorsExplainRequiredPermission(t *testing.T) {
	for _, code := range []string{"AccessDenied", "LackPolicy"} {
		t.Run(code, func(t *testing.T) {
			err := mapDevRTCAppError(&openapi.APIError{Action: "DescribeAppKeys", Code: code, Message: "not authorized"})
			typed, ok := errs.As(err)
			if !ok {
				t.Fatalf("error = %v", err)
			}
			if !strings.Contains(typed.Hint, "rtc:DescribeAppKeys") || !strings.Contains(typed.Hint, "rtc:DescribeApps") {
				t.Fatalf("hint %q does not mention rtc:DescribeAppKeys", typed.Hint)
			}
		})
	}
}

func TestSelectRTCAppKeyboardInteraction(t *testing.T) {
	apps := []openapi.RtcApp{
		{AppID: "one", Name: "One", Status: "1"},
		{AppID: "two", Name: "Two", Status: "1"},
		{AppID: "three", Name: "Three", Status: "1"},
	}

	t.Run("arrow up wraps then space selects and enter confirms", func(t *testing.T) {
		var output bytes.Buffer
		selected, err := selectRTCApp(strings.NewReader("\x1b[A \r"), &output, apps)
		if err != nil {
			t.Fatal(err)
		}
		if selected.AppID != "three" {
			t.Fatalf("selected AppID = %q", selected.AppID)
		}
	})

	t.Run("enter requires a space selection", func(t *testing.T) {
		var output bytes.Buffer
		selected, err := selectRTCApp(strings.NewReader("\r\x1b[B \r"), &output, apps)
		if err != nil {
			t.Fatal(err)
		}
		if selected.AppID != "two" {
			t.Fatalf("selected AppID = %q", selected.AppID)
		}
		if !strings.Contains(output.String(), "select an application with space before confirming") {
			t.Fatal("selector did not explain that space is required")
		}
	})
}

func TestBootstrapDevRTCAppCredentialsReturnsAppCandidates(t *testing.T) {
	oldFactory, oldTerminal := newDevConsoleClient, rtcInputIsTerminal
	fake := &fakeRTCAppClient{apps: []openapi.RtcApp{
		{AppID: "one", Name: "One", Status: "1"},
		{AppID: "two", Name: "Two", Status: "1"},
	}}
	newDevConsoleClient = func(*config.Config) (devConsoleClient, error) { return fake, nil }
	rtcInputIsTerminal = func(io.Reader) bool { return false }
	t.Cleanup(func() {
		newDevConsoleClient = oldFactory
		rtcInputIsTerminal = oldTerminal
	})

	_, err := bootstrapDevRTCAppCredentials(
		context.Background(), config.Default("demo", "test-scene", "web"), t.TempDir(),
		strings.NewReader("1\n"), &bytes.Buffer{},
	)
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.dev.selection_required" || typed.Details["apps"] == nil {
		t.Fatalf("error = %v", err)
	}
}

func TestDevDryRunReportsBootstrapWithoutOpenAPIOrWrite(t *testing.T) {
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := config.Save(config.Default("demo", "test-scene", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "byteplus-rtc.taskfile.yaml"), []byte("version: 1\nscene: test-scene\nplatform: web\ntasks:\n  dev:\n    - 'false'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	oldFactory, oldDryRun := newDevConsoleClient, flagDryRun
	called := false
	newDevConsoleClient = func(*config.Config) (devConsoleClient, error) {
		called = true
		return &fakeRTCAppClient{}, nil
	}
	flagDryRun = true
	t.Cleanup(func() {
		newDevConsoleClient = oldFactory
		flagDryRun = oldDryRun
	})

	if err := newDevCmd().Execute(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("dry-run called OpenAPI")
	}
	if _, err := os.Stat(projectenv.Path(dir)); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote .env.local: %v", err)
	}
}

func TestDevV2WebServerDryRunReportsDefaultPorts(t *testing.T) {
	taskfile := "version: 2\nscene: test-scene\nplatform: web\nruntime:\n  topology: web-server\ntasks:\n  dev:\n    - 'false'\n"
	setupDevPortProject(t, taskfile)
	previousDryRun := flagDryRun
	flagDryRun = true
	t.Cleanup(func() { flagDryRun = previousDryRun })

	stdout, err := captureDevStdout(newDevCmd().Execute)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data struct {
			Ports map[string]int    `json:"ports"`
			URLs  map[string]string `json:"urls"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout, &envelope); err != nil {
		t.Fatalf("decode dry-run output %q: %v", stdout, err)
	}
	if envelope.Data.Ports["web"] != 3000 || envelope.Data.Ports["server"] != 3001 {
		t.Fatalf("default v2 ports = %+v", envelope.Data.Ports)
	}
}

func TestDevRejectsOccupiedPortBeforeInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX lsof shim")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	port := listener.Addr().(*net.TCPAddr).Port

	taskfile := "version: 2\nscene: test-scene\nplatform: web\nruntime:\n  ports:\n    web: " + strconv.Itoa(port) + "\ntasks:\n  install:\n    - 'go run task-fixture.go install-ran'\n  dev:\n    - 'go run task-fixture.go dev-ran'\n"
	dir := setupDevPortProject(t, taskfile)
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const occupiedPID = 2147483647
	pid := strconv.Itoa(occupiedPID)
	lsof := "#!/bin/sh\ncase \"$*\" in\n  *'-d cwd'*) printf 'p" + pid + "\\nfcwd\\nn/tmp/other-project\\n' ;;\n  *) printf 'p" + pid + "\\ncnode\\n' ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(binDir, "lsof"), []byte(lsof), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	err = newDevCmd().Execute()
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.dev.port_in_use" {
		t.Fatalf("dev error = %v, want byteplus-rtc.dev.port_in_use", err)
	}
	occupant, ok := typed.Details["occupant"].(map[string]any)
	if !ok || occupant["pid"] != occupiedPID || occupant["process"] != "node" || occupant["directory"] != "/tmp/other-project" {
		t.Fatalf("port occupant details = %#v", typed.Details["occupant"])
	}
	if !strings.Contains(typed.Message, "node") || !strings.Contains(typed.Message, "/tmp/other-project") {
		t.Fatalf("port error does not identify occupant: %q", typed.Message)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "install-ran")); !os.IsNotExist(statErr) {
		t.Fatalf("install ran before port preflight: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "dev-ran")); !os.IsNotExist(statErr) {
		t.Fatalf("dev task ran with occupied port: %v", statErr)
	}
}

func TestDevFlagAndDotenvPortsReachDevTask(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = occupied.Close() })
	declaredPort := occupied.Addr().(*net.TCPAddr).Port
	webPort := reserveThenReleasePort(t)
	serverPort := reserveThenReleasePort(t)

	taskfile := "version: 2\nscene: test-scene\nplatform: web\nruntime:\n  ports:\n    web: " + strconv.Itoa(declaredPort) + "\n    server: 3001\ntasks:\n  dev:\n    - " + strconv.Quote(devPortHelperCommand(t)) + "\n"
	dir := setupDevPortProject(t, taskfile)

	t.Setenv("SERVER_PORT", "")
	if err := os.Unsetenv("SERVER_PORT"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectenv.Path(dir), []byte(fmt.Sprintf("PORT=%d\nSERVER_PORT=%d\n", declaredPort, serverPort)), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := newDevCmd()
	cmd.SetArgs([]string{"--web-port", strconv.Itoa(webPort)})
	stdout, err := captureDevStdout(cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data struct {
			Status string            `json:"status"`
			Ports  map[string]int    `json:"ports"`
			URLs   map[string]string `json:"urls"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout, &envelope); err != nil {
		t.Fatalf("decode dev output %q: %v", stdout, err)
	}
	if envelope.Data.Status != "starting" || envelope.Data.Ports["web"] != webPort || envelope.Data.Ports["server"] != serverPort {
		t.Fatalf("structured dev ports = %+v", envelope.Data)
	}
	if envelope.Data.URLs["web"] != "http://localhost:"+strconv.Itoa(webPort) || envelope.Data.URLs["server"] != "http://localhost:"+strconv.Itoa(serverPort) {
		t.Fatalf("structured dev URLs = %+v", envelope.Data.URLs)
	}
	if got := strings.TrimSpace(readFileForDevTest(t, filepath.Join(dir, "web-port"))); got != strconv.Itoa(webPort) {
		t.Fatalf("web process PORT = %q, want %d", got, webPort)
	}
	if got := strings.TrimSpace(readFileForDevTest(t, filepath.Join(dir, "server-port"))); got != strconv.Itoa(serverPort) {
		t.Fatalf("server process PORT = %q, want %d", got, serverPort)
	}
}

func TestDevImmediateTaskFailureDoesNotEmitSuccess(t *testing.T) {
	webPort := reserveThenReleasePort(t)
	serverPort := reserveThenReleasePort(t)
	failureCommand := "exit 7"
	if runtime.GOOS == "windows" {
		failureCommand = "exit /b 7"
	}
	taskfile := "version: 2\nscene: test-scene\nplatform: web\nruntime:\n  ports:\n    web: " + strconv.Itoa(webPort) + "\n    server: " + strconv.Itoa(serverPort) + "\ntasks:\n  dev:\n    - " + strconv.Quote(failureCommand) + "\n"
	setupDevPortProject(t, taskfile)

	stdout, err := captureDevStdout(newDevCmd().Execute)
	if err == nil {
		t.Fatal("dev unexpectedly succeeded")
	}
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.dev.task_failed" || typed.IsReported() {
		t.Fatalf("error = %#v, want unreported byteplus-rtc.dev.task_failed", err)
	}
	if len(stdout) != 0 {
		t.Fatalf("immediate task failure emitted success data: %q", stdout)
	}
}

func captureDevStdout(run func() error) ([]byte, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	previous := os.Stdout
	os.Stdout = writer
	runErr := run()
	_ = writer.Close()
	os.Stdout = previous
	raw, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if runErr != nil {
		return raw, runErr
	}
	return raw, readErr
}

func TestDevAutoPortReplacesOnlyOccupiedPort(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = occupied.Close() })
	occupiedPort := occupied.Addr().(*net.TCPAddr).Port
	serverPort := reserveThenReleasePort(t)

	taskfile := "version: 2\nscene: test-scene\nplatform: web\nruntime:\n  ports:\n    web: " + strconv.Itoa(occupiedPort) + "\n    server: " + strconv.Itoa(serverPort) + "\ntasks:\n  dev:\n    - " + strconv.Quote(devPortHelperCommand(t)) + "\n"
	dir := setupDevPortProject(t, taskfile)

	cmd := newDevCmd()
	cmd.SetArgs([]string{"--auto-port"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	webRaw, err := os.ReadFile(filepath.Join(dir, "web-port"))
	if err != nil {
		t.Fatal(err)
	}
	webPort, err := strconv.Atoi(string(webRaw))
	if err != nil || webPort == occupiedPort || webPort < 1 {
		t.Fatalf("auto-selected web port = %q, occupied = %d", webRaw, occupiedPort)
	}
	serverRaw, err := os.ReadFile(filepath.Join(dir, "server-port"))
	if err != nil {
		t.Fatal(err)
	}
	if string(serverRaw) != strconv.Itoa(serverPort) {
		t.Fatalf("unoccupied server port changed to %q, want %d", serverRaw, serverPort)
	}
}

func reserveThenReleasePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func setupDevPortProject(t *testing.T, taskfile string) string {
	t.Helper()
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := config.Save(config.Default("demo", "test-scene", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "byteplus-rtc.taskfile.yaml"), []byte(taskfile), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RTC_APP_ID", "aabbccddeeff001122334455")
	t.Setenv("RTC_APP_KEY", "secret")
	return dir
}

func devPortHelperCommand(t *testing.T) string {
	t.Helper()
	t.Setenv("GO_WANT_DEV_PORT_HELPER", "1")
	return testHelperCommand(t, "TestDevPortHelperProcess")
}

func testHelperCommand(t *testing.T, testName string) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(executable, `"`, `""`) + `" -test.run=^` + testName + `$`
	}
	return "'" + strings.ReplaceAll(executable, "'", "'\"'\"'") + "' -test.run=^" + testName + "$"
}

func TestDevPortHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_DEV_PORT_HELPER") != "1" {
		return
	}
	if err := os.WriteFile("web-port", []byte(os.Getenv("CONVO_CLI_WEB_PORT")), 0o644); err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile("server-port", []byte(os.Getenv("CONVO_CLI_SERVER_PORT")), 0o644); err != nil {
		os.Exit(2)
	}
	time.Sleep(350 * time.Millisecond)
	os.Exit(0)
}

func readFileForDevTest(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestDevBootstrapsCredentialsThenRunsTask(t *testing.T) {
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := config.Save(config.Default("demo", "test-scene", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	selectedAppID := "aabbccddeeff001122334455"
	t.Setenv("GO_WANT_DEV_CREDENTIALS_HELPER", "1")
	t.Setenv("WANT_RTC_APP_ID", selectedAppID)
	t.Setenv("WANT_RTC_APP_KEY", "secret")
	command := testHelperCommand(t, "TestDevCredentialsHelperProcess")
	taskfile := "version: 1\nscene: test-scene\nplatform: web\ntasks:\n  dev:\n    - " + strconv.Quote(command) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "byteplus-rtc.taskfile.yaml"), []byte(taskfile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectenv.Path(dir), []byte("RTC_APP_ID=\nRTC_APP_KEY=\nVITE_RTC_APP_ID=${RTC_APP_ID}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	t.Setenv("VITE_RTC_APP_ID", "${RTC_APP_ID}")
	fake := &fakeRTCAppClient{
		apps:   []openapi.RtcApp{{AppID: selectedAppID, Name: "Demo", Status: "1"}},
		appKey: "secret",
	}
	oldFactory, oldTerminal, oldDryRun := newDevConsoleClient, rtcInputIsTerminal, flagDryRun
	newDevConsoleClient = func(*config.Config) (devConsoleClient, error) { return fake, nil }
	rtcInputIsTerminal = func(io.Reader) bool { return true }
	flagDryRun = false
	t.Cleanup(func() {
		newDevConsoleClient = oldFactory
		rtcInputIsTerminal = oldTerminal
		flagDryRun = oldDryRun
	})

	cmd := newDevCmd()
	cmd.SetIn(strings.NewReader(" \r"))
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dev-ran")); err != nil {
		t.Fatalf("dev task did not run with selected credentials: %v", err)
	}
	values, err := projectenv.Load(projectenv.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if values["RTC_APP_ID"] != selectedAppID || values["RTC_APP_KEY"] != "secret" {
		t.Fatalf("persisted env = %+v", values)
	}
}

func TestDevCredentialsHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_DEV_CREDENTIALS_HELPER") != "1" {
		return
	}
	if got, want := os.Getenv("RTC_APP_ID"), os.Getenv("WANT_RTC_APP_ID"); got != want {
		t.Fatalf("RTC_APP_ID = %q, want %q", got, want)
	}
	if got, want := os.Getenv("RTC_APP_KEY"), os.Getenv("WANT_RTC_APP_KEY"); got != want {
		t.Fatalf("RTC_APP_KEY = %q, want %q", got, want)
	}
	if err := os.WriteFile("dev-ran", []byte("ran"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadProjectEnvUsesProcessPrecedenceAndRestores(t *testing.T) {
	dir := t.TempDir()
	processKey := "CONVO_CLI_TEST_PROCESS_PRECEDENCE"
	fileOnlyKey := "CONVO_CLI_TEST_FILE_ONLY"
	t.Setenv(processKey, "from-process")
	_ = os.Unsetenv(fileOnlyKey)
	t.Cleanup(func() { _ = os.Unsetenv(fileOnlyKey) })
	contents := processKey + "=from-file\n" + fileOnlyKey + "=loaded\n"
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	restore, err := loadProjectEnv(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(processKey); got != "from-process" {
		t.Fatalf("process env was replaced: %q", got)
	}
	if got := os.Getenv(fileOnlyKey); got != "loaded" {
		t.Fatalf("file env was not loaded: %q", got)
	}
	restore()
	if got := os.Getenv(processKey); got != "from-process" {
		t.Fatalf("process env changed after restore: %q", got)
	}
	if _, ok := os.LookupEnv(fileOnlyKey); ok {
		t.Fatal("file-only env remained set after restore")
	}
}

func TestBytePlusServerManagedTaskEnvUsesOnlyBytePlusPublicKeys(t *testing.T) {
	previous := product.SelectedID
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() { product.SelectedID = previous })
	stdinIsTTY, stdoutIsTTY := false, false
	telemetry.InitializeInvocation(telemetry.ResolveInvocationContextOptions{
		CLIName: "convo-cli", CLIVersion: "1.2.3", StdinIsTTY: &stdinIsTTY, StdoutIsTTY: &stdoutIsTTY,
	})
	t.Cleanup(func() {
		telemetry.InitializeInvocation(telemetry.ResolveInvocationContextOptions{
			CLIName: "convo-cli", CLIVersion: "1.2.3", StdinIsTTY: &stdinIsTTY, StdoutIsTTY: &stdoutIsTTY,
		})
	})
	t.Setenv("BYTEPLUS_ACCESS_KEY_ID", "")
	t.Setenv("BYTEPLUS_SECRET_ACCESS_KEY", "")

	taskEnv, err := serverManagedTaskEnv(config.Default("demo", "voice-agent", "web"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"BYTEPLUS_RTC_CLI_PATH", "BYTEPLUS_RTC_PROJECT_DIR", "BYTEPLUS_RTC_BUSINESS_ID"} {
		if taskEnvValue(taskEnv, key) == "" {
			t.Fatalf("missing %s in %v", key, taskEnv)
		}
	}
}

func TestServerManagedTaskEnvPreservesExplicitCredentials(t *testing.T) {
	t.Setenv("BYTEPLUS_ACCESS_KEY_ID", "EXPLICIT_AK")
	t.Setenv("BYTEPLUS_SECRET_ACCESS_KEY", "EXPLICIT_SK")
	dir := t.TempDir()
	taskEnv, err := serverManagedTaskEnv(config.Default("demo", "voice-agent", "web"), dir)
	if err != nil {
		t.Fatal(err)
	}
	if taskEnvValue(taskEnv, "BYTEPLUS_RTC_CLI_PATH") != "" {
		t.Fatalf("explicit credentials should select direct OpenAPI mode: %v", taskEnv)
	}
	if taskEnvValue(taskEnv, "AGENT_CONFIG_PATH") != filepath.Join(dir, "server/scenes/default.json") {
		t.Fatalf("task environment = %v", taskEnv)
	}
	if os.Getenv("BYTEPLUS_ACCESS_KEY_ID") != "EXPLICIT_AK" || os.Getenv("BYTEPLUS_SECRET_ACCESS_KEY") != "EXPLICIT_SK" {
		t.Fatal("explicit server credentials were changed")
	}
}

func TestServerManagedTaskEnvRejectsIncompleteLongTermCredentials(t *testing.T) {
	t.Setenv("BYTEPLUS_ACCESS_KEY_ID", "EXPLICIT_AK")
	t.Setenv("BYTEPLUS_SECRET_ACCESS_KEY", "")
	_, err := serverManagedTaskEnv(config.Default("demo", "voice-agent", "web"), t.TempDir())
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.auth.invalid_token" {
		t.Fatalf("error = %v", err)
	}
}

func taskEnvValue(values []string, key string) string {
	prefix := key + "="
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return ""
}

func TestRunTasksWithEnvInjectsOnlyIntoChild(t *testing.T) {
	const key = "CONVO_CLI_TEST_CHILD_ONLY"
	t.Setenv(key, "parent")
	t.Setenv("GO_WANT_TASK_ENV_HELPER", "1")
	command := testHelperCommand(t, "TestTaskEnvHelperProcess")
	if err := runTasksWithEnv(t.TempDir(), []string{command}, []string{key + "=available"}); err != nil {
		t.Fatal(err)
	}
	if os.Getenv(key) != "parent" {
		t.Fatal("task environment leaked into or replaced the CLI process environment")
	}
}

func TestTaskEnvHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_TASK_ENV_HELPER") != "1" {
		return
	}
	if got := os.Getenv("CONVO_CLI_TEST_CHILD_ONLY"); got != "available" {
		t.Fatalf("child task environment = %q, want available", got)
	}
}

func TestTaskShellCommandUsesCmdOnWindows(t *testing.T) {
	t.Setenv("ComSpec", `C:\Windows\System32\cmd.exe`)
	name, args := taskShellCommand("windows", "yarn dev")
	if name != `C:\Windows\System32\cmd.exe` {
		t.Fatalf("shell = %q, want ComSpec", name)
	}
	wantArgs := []string{"/D", "/C", "yarn dev"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %q, want %q", args, wantArgs)
	}
}

func TestTaskShellCommandPreservesQuotedCommandOnWindows(t *testing.T) {
	t.Setenv("ComSpec", `C:\Windows\System32\cmd.exe`)
	command := `"C:\Program Files\convo-cli\helper.exe" -test.run=^TestHelper$`
	_, args := taskShellCommand("windows", command)
	wantArgs := []string{"/D", "/C", command}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %q, want %q", args, wantArgs)
	}
}

func TestTaskShellCommandFallsBackToCmdExeOnWindows(t *testing.T) {
	t.Setenv("ComSpec", "")
	name, _ := taskShellCommand("windows", "yarn dev")
	if name != "cmd.exe" {
		t.Fatalf("shell = %q, want cmd.exe", name)
	}
}

func TestServerManagedProjectRoutesAllAgentCommandsToWeb(t *testing.T) {
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := config.Save(config.Default("demo", "voice-agent", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	taskfile := "version: 2\nscene: voice-agent\nplatform: web\nruntime:\n  topology: web-server\n  agent_control: server\n"
	if err := os.WriteFile(filepath.Join(dir, "byteplus-rtc.taskfile.yaml"), []byte(taskfile), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"start", "stop", "status"} {
		cmd := NewRootCmd()
		cmd.SetArgs([]string{"agent", action})
		err := cmd.Execute()
		typed, ok := errs.As(err)
		if !ok || typed.Code != "byteplus-rtc.agent.server_managed" {
			t.Fatalf("agent %s error = %v, want byteplus-rtc.agent.server_managed", action, err)
		}
		if typed.Hint == "" {
			t.Fatalf("agent %s should guide the user to the web app", action)
		}
	}
}

func TestV1ProjectKeepsCLIAgentControl(t *testing.T) {
	dir := t.TempDir()
	taskfile := "version: 1\nscene: voice-agent\nplatform: web\n"
	if err := os.WriteFile(filepath.Join(dir, "byteplus-rtc.taskfile.yaml"), []byte(taskfile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := requireCLIAgentControl(config.Default("demo", "voice-agent", "web"), dir); err != nil {
		t.Fatalf("v1 project should retain CLI agent control: %v", err)
	}
}

func TestDoctorProjectLoadsDotEnvForServerManagedProject(t *testing.T) {
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := config.Save(config.Default("demo", "voice-agent", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "server", "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server", "scenes", "default.json"), []byte(`{"SceneConfig":{"Name":"Default"},"VoiceChat":{"Config":{"ASRConfig":{"Provider":"BytePlus","ProviderParams":{}},"LLMConfig":{"Mode":"BytePlusArk","EndPointId":"ep-test","APIKey":"test-api-key"},"TTSConfig":{"Provider":"byteplus_Bidirectional_streaming","ProviderParams":{}}},"AgentConfig":{"UserID":"voice_agent"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	taskfile := "version: 2\nscene: voice-agent\nplatform: web\nsdk:\n  name: '@byteplus/rtc'\n  version: '4.69.3'\nruntime:\n  topology: web-server\n  agent_control: server\n"
	if err := os.WriteFile(filepath.Join(dir, "byteplus-rtc.taskfile.yaml"), []byte(taskfile), 0o644); err != nil {
		t.Fatal(err)
	}
	dotenv := "RTC_APP_ID=appid-from-file\nRTC_APP_KEY=appkey-from-file\nBYTEPLUS_ACCESS_KEY_ID=AKID\nBYTEPLUS_SECRET_ACCESS_KEY=SECRET\n" // public-scan: allow; gitleaks:allow — synthetic test credentials
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), []byte(dotenv), 0o600); err != nil {
		t.Fatal(err)
	}
	// Minimal scaffold entrypoints so doctor's file-presence check passes.
	for _, f := range []struct{ path, body string }{
		{"package.json", "{}"},
		{"web/package.json", "{}"},
		{"web/src/App.tsx", "export default function App(){return null}"},
		{"server/package.json", "{}"},
		{"server/app.js", "// server"},
	} {
		p := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cmd := newDoctorProjectCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor project should use .env.local: %v", err)
	}
}

func TestDoctorDoesNotCompleteMissingRTCAppCredentials(t *testing.T) {
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })
	if err := config.Save(config.Default("demo", "test-scene", "web"), config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	t.Setenv("VITE_RTC_APP_ID", "${RTC_APP_ID}")
	fake := &fakeRTCAppClient{}
	oldAppFactory, oldDryRun := newDevConsoleClient, flagDryRun
	newDevConsoleClient = func(*config.Config) (devConsoleClient, error) { return fake, nil }
	flagDryRun = false
	t.Cleanup(func() {
		newDevConsoleClient = oldAppFactory
		flagDryRun = oldDryRun
	})

	cmd := newDoctorCmd()
	cmd.SetErr(&bytes.Buffer{})
	err = cmd.Execute()
	if typed, ok := errs.As(err); !ok || typed.Code != "byteplus-rtc.doctor.failed" {
		t.Fatalf("doctor error = %v, want missing-credential diagnostics", err)
	}
	if fake.listRequests != 0 || fake.requestedID != "" {
		t.Fatalf("doctor performed completion: app=%d key=%q", fake.listRequests, fake.requestedID)
	}
	if _, err := os.Stat(projectenv.Path(dir)); !os.IsNotExist(err) {
		t.Fatalf("doctor wrote .env.local: %v", err)
	}
}

func TestDevStartsBeforeProviderResourceSelection(t *testing.T) {
	for _, interactive := range []bool{true, false} {
		t.Run(strconv.FormatBool(interactive), func(t *testing.T) {
			t.Cleanup(overrideAuthGlobals(t))
			previousTTY := rtcInputIsTerminal
			rtcInputIsTerminal = func(io.Reader) bool { return interactive }
			t.Cleanup(func() { rtcInputIsTerminal = previousTTY })
			for _, key := range []string{"BYTEPLUS_MODEL_ENDPOINT_ID", "BYTEPLUS_MODEL_API_KEY_ID", "BYTEPLUS_MODEL_API_KEY", "BYTEPLUS_SEED_SPEECH_APP_ID", "BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN"} {
				t.Setenv(key, "")
			}
			t.Setenv("BYTEPLUS_ACCESS_KEY_ID", "fixture-ak")
			t.Setenv("BYTEPLUS_SECRET_ACCESS_KEY", "fixture-sk")
			t.Setenv("OPENAI_API_KEY", "fixture-openai")
			t.Setenv("AWS_ACCESS_KEY_ID", "fixture-aws-ak")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "fixture-aws-sk")
			queries := 0
			authHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				queries++
				return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"ResponseMetadata":{"Error":{"Code":"AccessDenied","Message":"Resource access unavailable"}}}`))}, nil
			})}
			taskfile := "version: 2\nscene: test-scene\nplatform: web\nruntime:\n  topology: web-server\n  agent_control: server\ntasks:\n  dev:\n    - " + strconv.Quote(testHelperCommand(t, "TestDevServiceHelperProcess")) + "\n"
			setupDevPortProject(t, taskfile)
			t.Setenv("GO_WANT_DEV_SERVICE_HELPER", "exit")
			cmd := newDevCmd()
			cmd.SetIn(strings.NewReader(""))
			cmd.SetArgs([]string{"--web-port", strconv.Itoa(reserveThenReleasePort(t)), "--server-port", strconv.Itoa(reserveThenReleasePort(t))})
			cmd.SetErr(io.Discard)
			stdout, err := captureDevStdout(cmd.Execute)
			if typed, ok := errs.As(err); !ok || !typed.IsReported() {
				t.Fatalf("expected ready service followed by task exit: %v", err)
			}
			if !strings.Contains(string(stdout), `"status": "ready"`) {
				t.Fatalf("dev did not become ready: %s", stdout)
			}
			if queries != 0 {
				t.Fatalf("dev queried provider resources %d times", queries)
			}
		})
	}
}

func TestAIGCDemoUsesPackageScriptsAndBrowserConfig(t *testing.T) {
	dir := t.TempDir()
	pkg := filepath.Join(dir, "package.json")
	manifest := []byte(`{"name":"byteplus-rtc-aigc-demo","scripts":{"dev":"node scripts/dev.cjs","setup":"npm ci"}}`)
	if err := os.WriteFile(pkg, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "server"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server", "cliRuntime.js"), []byte("module.exports = {};"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, path, err := config.LoadNearest(filepath.Join(dir, "web"))
	if err != nil || filepath.Dir(path) != dir || cfg.Agent.ConfigSource != config.AgentConfigSourceBrowser || cfg.Agent.ConfigFile != "" {
		t.Fatalf("project discovery: %v, %s, %v", cfg, path, err)
	}
	tf, err := template.LoadTaskfile(dir)
	if err != nil || !tf.ServerManagedAgent() || !reflect.DeepEqual(tf.Tasks.Install, []string{"npm run setup"}) || !reflect.DeepEqual(tf.Tasks.Dev, []string{"npm run dev"}) {
		t.Fatalf("tasks: %v, %v", tf, err)
	}
	t.Setenv("BYTEPLUS_ACCESS_KEY_ID", "test")
	t.Setenv("BYTEPLUS_SECRET_ACCESS_KEY", "test")
	env, err := serverManagedTaskEnv(cfg, dir)
	if err != nil || taskEnvValue(env, "BYTEPLUS_RTC_CLI_PATH") == "" || taskEnvValue(env, "AGENT_CONFIG_PATH") != "" {
		t.Fatal("expected CLI runtime with browser-owned scene", err)
	}
	if _, err := os.Stat(config.Path(dir)); !os.IsNotExist(err) {
		t.Fatal("unexpected config file", err)
	}
	yamlCfg := config.Default("existing", "voice-agent", "web")
	if err := config.Save(yamlCfg, config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := config.LoadNearest(dir)
	if err != nil || loaded.Project.Name != "existing" {
		t.Fatal("expected explicit config to take precedence", err)
	}
}

func TestBrowserDemoFirstRunPersistsSelectedAppWithoutKeyLookup(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default("demo", "voice-agent", "web")
	cfg.UseBrowserAgent()
	t.Setenv("RTC_APP_ID", "")
	t.Setenv("RTC_APP_KEY", "")
	t.Setenv("VITE_RTC_APP_ID", "previous-app")
	fake := &fakeRTCAppClient{appKeyErr: errors.New("unexpected key lookup")}
	factory := newDevConsoleClient
	newDevConsoleClient = func(*config.Config) (devConsoleClient, error) { return fake, nil }
	t.Cleanup(func() { newDevConsoleClient = factory })
	const appID = "111111111111111111111111"
	restore, err := bootstrapDevRTCAppCredentialsWithOptions(context.Background(), cfg, dir, strings.NewReader(""), io.Discard, devSetupOptions{AppID: appID})
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	values, err := projectenv.Load(projectenv.Path(dir))
	if err != nil || values["RTC_APP_ID"] != appID || values["RTC_APP_KEY"] != "" || fake.requestedID != "" {
		t.Fatal("unexpected RTC first-run setup", err)
	}
	if config.CanBootstrapRTCAppCredentials(cfg, config.Validate(cfg), false) {
		t.Fatal("second run requested credentials again")
	}
}

func TestBrowserDemoChangingAppWithConfiguredKey(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default("demo", "voice-agent", "web")
	cfg.UseBrowserAgent()
	t.Setenv("RTC_APP_ID", "previous-app")
	t.Setenv("RTC_APP_KEY", "previous-app-key")
	path := projectenv.Path(dir)
	if _, err := projectenv.Write(path, map[string]string{"RTC_APP_ID": os.Getenv("RTC_APP_ID"), "RTC_APP_KEY": os.Getenv("RTC_APP_KEY")}); err != nil {
		t.Fatal(err)
	}
	initial, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = bootstrapDevRTCAppCredentialsWithOptions(context.Background(), cfg, dir, strings.NewReader(""), io.Discard, devSetupOptions{AppID: "next-app"})
	if typed, ok := errs.As(err); !ok || typed.Code != "byteplus-rtc.dev.app_setup_failed" {
		t.Fatalf("expected app selection error, got %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(initial) || os.Getenv("RTC_APP_ID") != "previous-app" {
		t.Fatal("app selection changed the existing credentials", err)
	}
	restore, err := bootstrapDevRTCAppCredentialsWithOptions(context.Background(), cfg, dir, strings.NewReader(""), io.Discard, devSetupOptions{AppID: "previous-app"})
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if os.Getenv("RTC_APP_KEY") != "previous-app-key" {
		t.Fatal("selecting the current application changed its local signing key")
	}
}
