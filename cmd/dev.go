// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"convo-cli/internal/affordance"
	"convo-cli/internal/config"
	projectenv "convo-cli/internal/env"
	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/nodeproject"
	"convo-cli/internal/openapi"
	"convo-cli/internal/product"
	"convo-cli/internal/telemetry"
	"convo-cli/internal/template"
)

var (
	newDevConsoleClient = func(cfg *config.Config) (devConsoleClient, error) {
		contract, err := currentDevSetupContract()
		if err != nil {
			return nil, err
		}
		return contract.NewConsoleClient(cfg), nil
	}
	rtcInputIsTerminal = func(input io.Reader) bool {
		file, ok := input.(*os.File)
		if !ok {
			return false
		}
		return term.IsTerminal(int(file.Fd()))
	}
	makeRTCSelectorRaw = func(input io.Reader) (func(), error) {
		file, ok := input.(*os.File)
		if !ok {
			return func() {}, nil
		}
		state, err := term.MakeRaw(int(file.Fd()))
		if err != nil {
			return nil, err
		}
		return func() { _ = term.Restore(int(file.Fd()), state) }, nil
	}
	writeDevProjectEnv = projectenv.Write
)

type devConsoleClient interface {
	DescribeApps(context.Context, int, int) ([]openapi.RtcApp, error)
	DescribeAppKeys(context.Context, string) (openapi.AppKeys, error)
}

type devSetupOptions struct {
	AppID       string
	Reconfigure bool
}

type devResourceApp struct {
	AppID string `json:"app_id"`
	Name  string `json:"name,omitempty"`
}

func noAvailableAppHint() string {
	contract, err := currentDevSetupContract()
	if err != nil {
		return "create an RTC application or verify the current account/project permissions"
	}
	return contract.NoAvailableAppHint()
}

func newDevCmd() *cobra.Command {
	var (
		reconfigure    bool
		appID          string
		webPort        int
		serverPort     int
		autoPort       bool
		startupTimeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Start local dev (runs the template's `dev` task)",
		RunE: func(c *cobra.Command, args []string) error {
			setupContract, err := currentDevSetupContract()
			if err != nil {
				return err
			}
			if startupTimeout <= 0 {
				return errs.New("byteplus-rtc.cli.invalid_flag", errs.TypeValidation, "--startup-timeout must be positive").WithParam("--startup-timeout")
			}
			options := devSetupOptions{AppID: appID, Reconfigure: reconfigure}
			if err := setupContract.ValidateOptions(options); err != nil {
				return err
			}
			if err := validateDevPortOverrides(
				c.Flags().Changed("web-port"), webPort,
				c.Flags().Changed("server-port"), serverPort,
			); err != nil {
				return err
			}
			cfg, path, err := config.LoadNearest(".")
			if err != nil {
				return err
			}
			dir := projectDir(path)
			restoreEnv, err := loadProjectEnv(dir)
			if err != nil {
				return err
			}
			defer restoreEnv()

			forceSetup := reconfigure || strings.TrimSpace(appID) != ""
			if forceSetup && strings.TrimSpace(cfg.RTC.AppID) != "${RTC_APP_ID}" {
				return errs.New("byteplus-rtc.dev.app_setup_failed", errs.TypePrecondition,
					"RTC application setup requires rtc.app_id to reference ${RTC_APP_ID}").
					WithHint("set rtc.app_id to ${RTC_APP_ID} before retrying")
			}
			rep := config.Validate(cfg)
			credentialBootstrapRequired := setupContract.CredentialBootstrapRequired(cfg, rep, forceSetup)
			devSetupRequired := credentialBootstrapRequired
			if devSetupRequired && !flagDryRun {
				restoreCredentials, setupErr := bootstrapDevRTCAppCredentialsWithOptions(
					c.Context(), cfg, dir, c.InOrStdin(), c.ErrOrStderr(),
					options,
				)
				if setupErr != nil {
					return setupErr
				}
				defer restoreCredentials()
				rep = config.Validate(cfg)
			}

			// Config must be complete; otherwise route to doctor. Dry-run reports
			// the eligible bootstrap without contacting OpenAPI or writing files.
			if rep.HasErrors() && !(flagDryRun && devSetupRequired) {
				return errs.New("byteplus-rtc.dev.config_incomplete", errs.TypePrecondition,
					"config is incomplete").
					WithHint("run `%s doctor` to see what to fix", meta.BinName)
			}

			tf, err := template.LoadTaskfile(dir)
			if err != nil {
				return err
			}
			if len(tf.Tasks.Dev) == 0 {
				return errs.New("byteplus-rtc.template.not_found", errs.TypeNotFound,
					"taskfile has no `dev` task").WithHint("regenerate with `%s init`", meta.BinName)
			}
			ports, err := resolveDevPorts(tf, webPort, serverPort)
			if err != nil {
				return err
			}
			if autoPort {
				ports, err = autoSelectDevPorts(ports)
				if err != nil {
					return err
				}
			}

			// Legacy voice-agent projects keep their explicit, CLI-managed start
			// flow. Full-stack v2 projects start and stop through the web/server.
			if cfg.Project.Scene == "voice-agent" && !tf.ServerManagedAgent() && cfg.Agent.TaskID == "" {
				out().Warn("no agent running — run `%s agent start` first so the web app can connect to it (dev does not auto-start the billed AI task)", meta.BinName)
			}

			if flagDryRun {
				out().Progress("dry-run: not executing")
				result := map[string]any{
					"dir": dir, "dev_task": tf.Tasks.Dev, "install_task": tf.Tasks.Install,
				}
				for key, value := range devRuntimeData(dir, "dry-run", ports) {
					result[key] = value
				}
				if devSetupRequired {
					bootstrap := map[string]any{
						"required":       true,
						"reconfigure":    reconfigure,
						"list_action":    openapi.DescribeAppsAction,
						"app_key_action": openapi.DescribeAppKeysAction,
						"version":        openapi.RTCAppManagementVersion,
						"list_version":   openapi.RTCAppListVersion,
					}
					result["credential_bootstrap"] = bootstrap
				}
				return out().Data(result)
			}
			if err := preflightDevPorts(ports); err != nil {
				return err
			}

			if err := checkDevToolchain(c.Context(), dir); err != nil {
				return err
			}
			if len(tf.Tasks.Install) > 0 {
				missing, err := nodeproject.NeedsInstall(dir, tf.Runtime.Topology == "web-server")
				if err != nil {
					return devServiceError("check project dependencies: %s", err)
				}
				if missing {
					out().Progress("installing dependencies…")
					if err := runTasksWithEnv(dir, tf.Tasks.Install, nil); err != nil {
						return err
					}
				}
			}
			// Installation can take minutes, so close the check/use window before
			// launching the actual dev processes.
			if autoPort {
				ports, err = autoSelectDevPorts(ports)
			} else {
				err = preflightDevPorts(ports)
			}
			if err != nil {
				return err
			}

			var taskEnv []string
			if tf.ServerManagedAgent() {
				taskEnv, err = serverManagedTaskEnv(cfg, dir)
				if err != nil {
					return err
				}
			}
			portEnv, cleanupPortEnv, err := prepareDevPortTaskEnv(ports)
			if err != nil {
				return err
			}
			defer cleanupPortEnv()
			taskEnv = append(taskEnv, portEnv...)

			out().Progress("starting dev server (%v) in %s", tf.Tasks.Dev, dir)
			reported := false
			reportStarted := func() error {
				if ports.Web == 0 && ports.Server == 0 {
					return nil
				}
				status := "starting"
				if tf.Runtime.Topology == "web-server" {
					status = "ready"
				}
				if err := out().Data(devRuntimeData(dir, status, ports)); err != nil {
					return err
				}
				reported = true
				return nil
			}
			var runErr error
			if tf.Runtime.Topology == "web-server" {
				runErr = runDevServices(c.Context(), dir, tf.Tasks.Dev, taskEnv, ports, startupTimeout, reportStarted)
			} else {
				runErr = runTasksWithEnvAfterStart(dir, tf.Tasks.Dev, taskEnv, reportStarted)
			}
			if err := runErr; err != nil {
				if reported {
					if typed, ok := errs.As(err); ok {
						out().Warn("%s", typed.Message)
						return typed.Reported()
					}
				}
				return err
			}
			if reported {
				return nil
			}
			return out().Data(map[string]any{"dir": dir, "status": "dev exited"})
		},
	}
	cmd.Flags().BoolVar(&reconfigure, "reconfigure", false, "select the RTC application again")
	cmd.Flags().StringVar(&appID, "app-id", "", "select an RTC application by public AppID")
	cmd.Flags().IntVar(&webPort, "web-port", 0, "override the template web port")
	cmd.Flags().IntVar(&serverPort, "server-port", 0, "override the template server port")
	cmd.Flags().BoolVar(&autoPort, "auto-port", false, "replace occupied template ports with available local ports")
	cmd.Flags().DurationVar(&startupTimeout, "startup-timeout", 2*time.Minute, "maximum wait for web and server readiness")
	setupContract, err := currentDevSetupContract()
	if err != nil {
		panic(err)
	}
	if err := setupContract.ConfigureDevCommand(cmd); err != nil {
		panic(err)
	}
	affordance.Attach(cmd, setupContract.DevAffordance())
	return cmd
}

func checkDevToolchain(ctx context.Context, dir string) error {
	checks, err := nodeproject.CheckEngines(ctx, dir)
	if err != nil {
		return devServiceError("read project toolchain requirements: %s", err)
	}
	for _, check := range checks {
		if check.Err != nil {
			return devServiceError("%s", check.Err).
				WithHint("install %s matching %s from package.json, then retry", check.Tool, check.Requirement)
		}
	}
	return nil
}

func queryDevApps(ctx context.Context, client devConsoleClient) ([]openapi.RtcApp, error) {
	listCtx, cancelList := context.WithTimeout(ctx, 15*time.Second)
	apps, err := client.DescribeApps(listCtx, 0, 0)
	cancelList()
	if err != nil {
		return nil, mapDevRTCAppError(err)
	}
	return availableRTCApps(apps), nil
}

func mapDevRTCAppError(err error) error {
	return mapOpenAPIError(err)
}

func bootstrapDevRTCAppCredentials(
	ctx context.Context,
	cfg *config.Config,
	dir string,
	input io.Reader,
	prompt io.Writer,
) (func(), error) {
	return bootstrapDevRTCAppCredentialsWithOptions(ctx, cfg, dir, input, prompt, devSetupOptions{})
}

func bootstrapDevRTCAppCredentialsWithOptions(
	ctx context.Context,
	cfg *config.Config,
	dir string,
	input io.Reader,
	prompt io.Writer,
	options devSetupOptions,
) (func(), error) {
	interactive := rtcInputIsTerminal(input)
	client, err := newDevConsoleClient(cfg)
	if err != nil {
		return nil, err
	}
	setupContract, err := currentDevSetupContract()
	if err != nil {
		return nil, err
	}
	selectorReader := bufio.NewReader(input)
	refreshCredentials := options.Reconfigure || strings.TrimSpace(options.AppID) != "" ||
		strings.TrimSpace(os.Getenv("RTC_APP_ID")) == "" || strings.TrimSpace(os.Getenv("RTC_APP_KEY")) == ""
	var selected openapi.RtcApp
	if refreshCredentials {
		requestedAppID := strings.TrimSpace(options.AppID)
		if requestedAppID != "" && setupContract.TrustExplicitRTCAppID() {
			selected = openapi.RtcApp{AppID: requestedAppID}
		} else {
			apps, queryErr := queryDevApps(ctx, client)
			if queryErr != nil {
				return nil, queryErr
			}
			if len(apps) == 0 {
				return nil, errs.New("byteplus-rtc.dev.app_setup_failed", errs.TypeNotFound,
					"DescribeApps returned no status=1 RTC applications").
					WithHint("%s", noAvailableAppHint())
			}

			if requestedAppID == "" && !options.Reconfigure {
				requestedAppID = strings.TrimSpace(os.Getenv("RTC_APP_ID"))
			}
			switch {
			case requestedAppID != "":
				selected, err = selectRTCAppByID(apps, requestedAppID)
			case len(apps) == 1:
				selected = apps[0]
			case interactive:
				selected, err = selectRTCAppWithReader(input, selectorReader, prompt, apps)
			default:
				return nil, devAppSelectionRequiredError(apps,
					"multiple active RTC applications are available; choose one")
			}
		}
		if err != nil {
			return nil, err
		}
		if viteAppID := strings.TrimSpace(os.Getenv("VITE_RTC_APP_ID")); viteAppID != "" && viteAppID != "${RTC_APP_ID}" && viteAppID != selected.AppID {
			return nil, errs.New("byteplus-rtc.dev.app_setup_failed", errs.TypePrecondition,
				"VITE_RTC_APP_ID conflicts with the selected AppID").
				WithHint("unset VITE_RTC_APP_ID or align it with %s before retrying", selected.AppID)
		}
	}

	if cfg.Project.Scene == "voice-agent" {
		if err := validateDevScene(dir, cfg.Agent.ConfigFile); err != nil {
			return nil, err
		}
	}

	updates := map[string]string{}
	if refreshCredentials {
		keyCtx, cancelKey := context.WithTimeout(ctx, 15*time.Second)
		keys, err := client.DescribeAppKeys(keyCtx, selected.AppID)
		cancelKey()
		if err != nil {
			return nil, mapDevRTCAppError(err)
		}
		if strings.TrimSpace(keys.AppKey) == "" {
			return nil, errs.New("byteplus-rtc.dev.app_setup_failed", errs.TypeNotFound,
				"DescribeAppKeys returned no primary AppKey for the selected AppID").
				WithHint("verify access to AppID %s, then retry `%s dev`", selected.AppID, meta.BinName)
		}
		updates = rtcAppCredentialUpdates(selected.AppID, keys.AppKey)
	}

	path := projectenv.Path(dir)
	var snapshotPaths []string
	if refreshCredentials {
		snapshotPaths = append(snapshotPaths, path)
	}

	snapshots, err := snapshotDevFiles(snapshotPaths)
	if err != nil {
		return nil, err
	}
	restore, err := applyTemporaryEnv(updates)
	if err != nil {
		return nil, err
	}
	var written []string
	if refreshCredentials {
		written, err = writeDevProjectEnv(path, updates)
		if err != nil {
			restore()
			return nil, rollbackDevSetup(err, snapshots)
		}
	}

	if refreshCredentials {
		name := selected.Name
		if name == "" {
			name = "unnamed"
		}
		fmt.Fprintf(prompt, "selected RTC application: %s (%s)\n", name, selected.AppID)
		fmt.Fprintf(prompt, "wrote %d credential key(s) to %s (AppKey masked)\n", len(written), path)
	}
	return restore, nil
}

func selectRTCAppByID(apps []openapi.RtcApp, appID string) (openapi.RtcApp, error) {
	appID = strings.TrimSpace(appID)
	for _, app := range apps {
		if app.AppID == appID {
			return app, nil
		}
	}
	return openapi.RtcApp{}, devAppSelectionRequiredError(apps,
		fmt.Sprintf("--app-id %q is not an active RTC application for the signed-in account", appID)).
		WithParam("--app-id")
}

func devAppSelectionRequiredError(apps []openapi.RtcApp, message string) *errs.Error {
	candidates := make([]devResourceApp, 0, len(apps))
	for _, app := range apps {
		candidates = append(candidates, devResourceApp{AppID: app.AppID, Name: app.Name})
	}
	return errs.New("byteplus-rtc.dev.selection_required", errs.TypePrecondition, "%s", message).
		WithParam("--app-id").
		WithDetails(map[string]any{"apps": candidates}).
		WithHint("choose an app_id from error.details.apps and retry `%s dev --app-id <id>`; never paste secrets into chat or command arguments", meta.BinName)
}

func validateDevScene(dir, configFile string) error {
	if strings.TrimSpace(configFile) == "" {
		configFile = "server/scenes/default.json"
	}
	path, err := config.ProjectFilePath(dir, configFile)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation,
			"cannot read agent scene %q: %s", configFile, err).
			WithHint("run `%s doctor` to inspect the configured scene", meta.BinName)
	}
	if err := config.ValidateServerAgentConfigJSON(raw); err != nil {
		return errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation,
			"agent scene %q is invalid: %s", configFile, err).
			WithHint("run `%s doctor` to inspect the configured scene", meta.BinName)
	}
	return nil
}

type devFileSnapshot struct {
	path    string
	existed bool
	data    []byte
	mode    os.FileMode
}

func snapshotDevFiles(paths []string) ([]devFileSnapshot, error) {
	snapshots := make([]devFileSnapshot, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				snapshots = append(snapshots, devFileSnapshot{path: path})
				continue
			}
			return nil, errs.New("byteplus-rtc.provision.write_failed", errs.TypeIO,
				"snapshot %q before setup: %s", path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, errs.New("byteplus-rtc.provision.write_failed", errs.TypeIO,
				"snapshot %q before setup: %s", path, err)
		}
		snapshots = append(snapshots, devFileSnapshot{
			path: path, existed: true, data: data, mode: info.Mode().Perm(),
		})
	}
	return snapshots, nil
}

func rollbackDevSetup(original error, snapshots []devFileSnapshot) error {
	var failures []string
	for index := len(snapshots) - 1; index >= 0; index-- {
		snapshot := snapshots[index]
		if !snapshot.existed {
			if err := os.Remove(snapshot.path); err != nil && !os.IsNotExist(err) {
				failures = append(failures, fmt.Sprintf("remove %s: %v", snapshot.path, err))
			}
			continue
		}
		if err := os.WriteFile(snapshot.path, snapshot.data, snapshot.mode); err != nil {
			failures = append(failures, fmt.Sprintf("restore %s: %v", snapshot.path, err))
		}
	}
	if len(failures) == 0 {
		return original
	}
	return errs.New("byteplus-rtc.provision.write_failed", errs.TypeIO,
		"setup failed: %v; rollback failed: %s", original, strings.Join(failures, "; ")).
		WithCause(original)
}

func availableRTCApps(apps []openapi.RtcApp) []openapi.RtcApp {
	active := make([]openapi.RtcApp, 0, len(apps))
	for _, app := range apps {
		if app.IsActive() {
			active = append(active, app)
		}
	}
	return active
}

func selectRTCApp(input io.Reader, output io.Writer, apps []openapi.RtcApp) (openapi.RtcApp, error) {
	return selectRTCAppWithReader(input, bufio.NewReader(input), output, apps)
}

func selectRTCAppWithReader(input io.Reader, reader *bufio.Reader, output io.Writer, apps []openapi.RtcApp) (openapi.RtcApp, error) {
	if len(apps) == 0 {
		return openapi.RtcApp{}, errs.New("byteplus-rtc.dev.app_setup_failed", errs.TypeNotFound,
			"there are no RTC applications to select")
	}
	restoreTerminal, err := makeRTCSelectorRaw(input)
	if err != nil {
		return openapi.RtcApp{}, errs.Wrap(err, "byteplus-rtc.dev.app_setup_failed",
			"enable interactive RTC application selection: %s", err)
	}
	fmt.Fprint(output, "\x1b[?25l")
	defer func() {
		restoreTerminal()
		fmt.Fprint(output, "\x1b[?25h")
	}()

	cursor, selected := 0, -1
	rendered := false
	message := "press space to select, then enter to confirm"
	render := func() {
		lineCount := len(apps) + 3
		if rendered {
			fmt.Fprintf(output, "\x1b[%dA", lineCount)
		}
		fmt.Fprint(output, "\r\x1b[2Kselect an RTC application (status=1)\n")
		fmt.Fprint(output, "\r\x1b[2K↑/↓ move · space select · enter confirm\n")
		for index, app := range apps {
			pointer, check := " ", "[ ]"
			if index == cursor {
				pointer = "›"
			}
			if index == selected {
				check = "[x]"
			}
			name := strings.TrimSpace(app.Name)
			if name == "" {
				name = "unnamed"
			}
			fmt.Fprintf(output, "\r\x1b[2K%s %s %s · %s\n", pointer, check, name, app.AppID)
		}
		fmt.Fprintf(output, "\r\x1b[2K%s\n", message)
		rendered = true
	}
	render()

	for {
		key, readErr := readRTCSelectorKey(reader)
		if readErr != nil {
			if readErr != io.EOF {
				return openapi.RtcApp{}, errs.Wrap(readErr, "byteplus-rtc.dev.app_setup_failed",
					"read RTC application selection: %s", readErr)
			}
			return openapi.RtcApp{}, rtcAppSelectionCanceledError()
		}
		switch key {
		case rtcSelectorUp:
			cursor = (cursor - 1 + len(apps)) % len(apps)
			message = "press space to select, then enter to confirm"
			render()
		case rtcSelectorDown:
			cursor = (cursor + 1) % len(apps)
			message = "press space to select, then enter to confirm"
			render()
		case rtcSelectorSelect:
			selected = cursor
			message = "selected; press enter to confirm"
			render()
		case rtcSelectorConfirm:
			if selected >= 0 {
				return apps[selected], nil
			}
			message = "select an application with space before confirming"
			render()
		case rtcSelectorCancel:
			return openapi.RtcApp{}, rtcAppSelectionCanceledError()
		}
	}
}

type rtcSelectorKey int

const (
	rtcSelectorUnknown rtcSelectorKey = iota
	rtcSelectorUp
	rtcSelectorDown
	rtcSelectorSelect
	rtcSelectorConfirm
	rtcSelectorCancel
)

func readRTCSelectorKey(reader *bufio.Reader) (rtcSelectorKey, error) {
	value, err := reader.ReadByte()
	if err != nil {
		return rtcSelectorUnknown, err
	}
	switch value {
	case ' ':
		return rtcSelectorSelect, nil
	case '\r', '\n':
		return rtcSelectorConfirm, nil
	case 0x03:
		return rtcSelectorCancel, nil
	case 0x1b:
		leftBracket, err := reader.ReadByte()
		if err != nil {
			return rtcSelectorUnknown, err
		}
		if leftBracket != '[' {
			return rtcSelectorUnknown, nil
		}
		direction, err := reader.ReadByte()
		if err != nil {
			return rtcSelectorUnknown, err
		}
		switch direction {
		case 'A':
			return rtcSelectorUp, nil
		case 'B':
			return rtcSelectorDown, nil
		}
	}
	return rtcSelectorUnknown, nil
}

func rtcAppSelectionCanceledError() error {
	return errs.New("byteplus-rtc.dev.app_setup_failed", errs.TypePrecondition,
		"RTC application selection was canceled")
}

func applyTemporaryEnv(updates map[string]string) (func(), error) {
	type previousValue struct {
		value  string
		exists bool
	}
	previous := make(map[string]previousValue, len(updates))
	applied := make([]string, 0, len(updates))
	restore := func() {
		for index := len(applied) - 1; index >= 0; index-- {
			key := applied[index]
			old := previous[key]
			if old.exists {
				_ = os.Setenv(key, old.value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}
	for key, value := range updates {
		old, exists := os.LookupEnv(key)
		previous[key] = previousValue{value: old, exists: exists}
		if err := os.Setenv(key, value); err != nil {
			restore()
			return nil, errs.Wrap(err, "byteplus-rtc.cli.internal", "set %s for dev: %s", key, err)
		}
		applied = append(applied, key)
	}
	return restore, nil
}

// loadProjectEnv overlays .env.local without replacing exported process
// variables. The returned function restores the process environment exactly,
// which keeps repeated command executions and dry-runs isolated.
func loadProjectEnv(dir string) (func(), error) {
	values, err := projectenv.Load(projectenv.Path(dir))
	if err != nil {
		return nil, err
	}
	var added []string
	for key, value := range values {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			for _, addedKey := range added {
				_ = os.Unsetenv(addedKey)
			}
			return nil, errs.Wrap(err, "byteplus-rtc.cli.internal", "load %s: %s", projectenv.FileName, err)
		}
		added = append(added, key)
	}
	return func() {
		for _, key := range added {
			_ = os.Unsetenv(key)
		}
	}, nil
}

// serverManagedTaskEnv selects one of the two companion-server control modes:
// direct Node signing for explicit long-term AK/SK, or CLI-backed agent
// start/stop where Signin credentials remain inside a short-lived CLI process.
func serverManagedTaskEnv(cfg *config.Config, dir string) ([]string, error) {
	profile := product.Current()
	configFile := strings.TrimSpace(cfg.Agent.ConfigFile)
	if configFile == "" {
		configFile = "server/scenes/default.json"
	}
	if !filepath.IsAbs(configFile) {
		configFile = filepath.Join(dir, configFile)
	}
	configFile, err := filepath.Abs(configFile)
	if err != nil {
		return nil, errs.Wrap(err, "byteplus-rtc.cli.internal", "resolve agent config path: %s", err)
	}
	projectDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, errs.Wrap(err, "byteplus-rtc.cli.internal", "resolve project path: %s", err)
	}
	taskEnv := []string{
		"AGENT_CONFIG_PATH=" + configFile,
		profile.BusinessIDEnv + "=" + profile.BusinessID,
		profile.ProjectDirEnv + "=" + projectDir,
	}
	if userAgent, ok := telemetry.GetInvocationUserAgent(); ok {
		if slash := strings.IndexByte(userAgent, '/'); slash > 0 {
			userAgent = profile.UserAgentProduct + userAgent[slash:]
		}
		taskEnv = append(taskEnv, profile.OpenAPIUserAgentEnv+"="+userAgent)
	}

	accessKey := strings.TrimSpace(os.Getenv(profile.AccessKeyEnv))
	secretKey := strings.TrimSpace(os.Getenv(profile.SecretKeyEnv))
	if accessKey != "" || secretKey != "" {
		if accessKey == "" || secretKey == "" {
			return nil, errs.New("byteplus-rtc.auth.invalid_token", errs.TypeAuth,
				"%s and %s must be set together", profile.AccessKeyEnv, profile.SecretKeyEnv).
				WithHint("set both server credentials or unset them and run `%s auth login`", meta.BinName)
		}
		return taskEnv, nil
	}

	executable, err := os.Executable()
	if err != nil {
		return nil, errs.Wrap(err, "byteplus-rtc.cli.internal", "locate %s executable: %s", meta.BinName, err)
	}
	return append(taskEnv, profile.CLIPathEnv+"="+executable), nil
}

// taskShellCommand returns the platform shell invocation used for taskfile
// commands. Task entries are shell command lines rather than argv arrays, so
// Windows must use cmd.exe instead of requiring a Unix compatibility layer.
func taskShellCommand(goos, cmdline string) (string, []string) {
	if goos == "windows" {
		shell := strings.TrimSpace(os.Getenv("ComSpec"))
		if shell == "" {
			shell = "cmd.exe"
		}
		return shell, []string{"/D", "/C", cmdline}
	}
	return "sh", []string{"-c", cmdline}
}

// runTasksWithEnv executes each command through the native platform shell in
// dir, adding taskEnv only to child processes and streaming output to stderr so
// stdout remains data-only.
func runTasksWithEnv(dir string, cmds, taskEnv []string) error {
	return runTasksWithEnvAfterStart(dir, cmds, taskEnv, nil)
}

// runTasksWithEnvAfterStart delays the optional start notification briefly so
// shell lookup errors and immediately failing template commands remain normal
// typed failures instead of being hidden behind a premature success envelope.
func runTasksWithEnvAfterStart(dir string, cmds, taskEnv []string, afterStart func() error) error {
	for _, cmdline := range cmds {
		shell, _ := taskShellCommand(runtime.GOOS, cmdline)
		cmd := buildTaskCmd(shell, cmdline)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), taskEnv...)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		signals := make(chan os.Signal, 2)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		if err := cmd.Start(); err != nil {
			signal.Stop(signals)
			return errs.New("byteplus-rtc.dev.task_failed", errs.TypeIO,
				"task %q failed: %s", cmdline, err).
				WithHint("run `%s doctor`; check the task output above", meta.BinName)
		}

		// Keep the CLI alive while the child handles terminal shutdown and may
		// invoke this executable for CLI-backed agent stop. Forward signals that
		// target only the CLI; duplicate terminal delivery is coalesced by the server.
		done := make(chan struct{})
		go func() {
			for {
				select {
				case received := <-signals:
					_ = cmd.Process.Signal(received)
				case <-done:
					return
				}
			}
		}()
		waited := make(chan error, 1)
		go func() { waited <- cmd.Wait() }()
		var err error
		if afterStart != nil {
			select {
			case err = <-waited:
			case <-time.After(250 * time.Millisecond):
				if notifyErr := afterStart(); notifyErr != nil {
					_ = cmd.Process.Signal(syscall.SIGTERM)
					<-waited
					close(done)
					signal.Stop(signals)
					return notifyErr
				}
				afterStart = nil
				err = <-waited
			}
		} else {
			err = <-waited
		}
		close(done)
		signal.Stop(signals)
		if err != nil {
			return errs.New("byteplus-rtc.dev.task_failed", errs.TypeIO,
				"task %q failed: %s", cmdline, err).
				WithHint("run `%s doctor`; check the task output above", meta.BinName)
		}
	}
	return nil
}
