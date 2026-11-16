// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/template"
)

type devServiceState struct {
	server bool
	web    bool
}

func runDevServices(ctx context.Context, dir string, tasks, env []string, ports template.Ports, timeout time.Duration, ready func() error) error {
	if ports.Web == 0 || ports.Server == 0 {
		return errs.New("byteplus-rtc.template.contract_invalid", errs.TypeValidation, "web-server runtime requires both web and server ports")
	}
	if err := runTasksWithEnv(dir, tasks[:len(tasks)-1], env); err != nil {
		return err
	}
	logs := newDevCompilationLog(dir, os.Stderr)
	var entryModules []string
	if logs.vite {
		var err error
		entryModules, err = viteEntryModulePaths(dir)
		if err != nil {
			return devServiceError("read Vite entry modules: %s", err)
		}
	}
	command := tasks[len(tasks)-1]
	if logs.vite && logs.typecheckRequired {
		if !logs.typecheckScript {
			return devServiceError("web/package.json must define a typecheck script for the Vite TypeScript project")
		}
		command = "npm --prefix web run typecheck && (" + command + ")"
	}
	shell, _ := taskShellCommand(runtime.GOOS, command)
	cmd := buildTaskCmd(shell, command)
	configureDevProcess(cmd)
	cmd.Dir, cmd.Env = dir, append(os.Environ(), env...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, logs, logs
	cmd.WaitDelay = 2 * time.Second
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	if err := cmd.Start(); err != nil {
		return devServiceError("start dev task: %s", err)
	}
	tree := newDevProcessTree(cmd)
	defer tree.close()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	stop := func(reason error) error {
		if err := stopDevTask(cmd, waited, tree); err != nil {
			return err
		}
		return reason
	}
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	states := probeDevServices(probeCtx, ports, logs, entryModules)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	startupDone := deadline.C
	var announced devServiceState
	var unhealthySince time.Time
	var rebuildSince time.Time
	isReady := false
	for {
		select {
		case err := <-waited:
			waited <- err
			if err != nil {
				return stop(devServiceError("dev task exited: %s", err))
			}
			return stop(devServiceError("dev task exited while core services were expected to remain running"))
		case <-ctx.Done():
			return stop(devServiceError("dev canceled: %s", ctx.Err()))
		case <-signals:
			if !isReady {
				return stop(devServiceError("dev stopped before web and server became ready"))
			}
			return stop(nil)
		case <-startupDone:
			return stop(devServiceError("web and server did not become ready within %s", timeout).
				WithDetails(map[string]any{"web_ready": announced.web, "server_ready": announced.server}))
		case state := <-states:
			if err := tree.refresh(); err != nil {
				return stop(devServiceError("inspect dev processes: %s", err))
			}
			compiled, compileFailed, compiling := logs.state()
			if compileFailed && !isReady {
				return stop(devServiceError("web compilation failed"))
			}
			if isReady && compiling && state.server {
				if rebuildSince.IsZero() {
					rebuildSince = time.Now()
				}
				if time.Since(rebuildSince) >= timeout {
					return stop(devServiceError("web rebuild did not finish within %s", timeout))
				}
				unhealthySince = time.Time{}
				continue
			}
			if !compiling {
				rebuildSince = time.Time{}
			}
			state.web = state.web && compiled
			if state.server && !announced.server {
				out().Progress("server_ready: http://localhost:%d", ports.Server)
			}
			if state.web && !announced.web {
				out().Progress("web_ready: http://localhost:%d", ports.Web)
			}
			announced = state
			if state.server && state.web {
				unhealthySince = time.Time{}
				if !isReady {
					deadline.Stop()
					startupDone = nil
					if err := ready(); err != nil {
						return stop(err)
					}
					isReady = true
				}
			} else if isReady {
				if unhealthySince.IsZero() {
					unhealthySince = time.Now()
				} else if time.Since(unhealthySince) >= 5*time.Second {
					return stop(devServiceError("core services failed readiness checks for 5s").
						WithDetails(map[string]any{"web_ready": state.web, "server_ready": state.server}))
				}
			}
		}
	}
}

func devServiceError(format string, args ...any) *errs.Error {
	return errs.New("byteplus-rtc.dev.task_failed", errs.TypeIO, format, args...).
		WithHint("check the service logs above; run `%s doctor`", meta.BinName)
}

func stopDevTask(cmd *exec.Cmd, waited <-chan error, tree *devProcessTree) error {
	return stopDevTaskWithin(cmd, waited, tree, 20*time.Second)
}

func stopDevTaskWithin(cmd *exec.Cmd, waited <-chan error, tree *devProcessTree, grace time.Duration) error {
	discoveryErr := tree.refresh()
	tree.terminate()
	deadline := time.NewTimer(grace)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	exited := false
	for {
		if exited && !tree.alive() {
			if discoveryErr != nil {
				return devServiceError("inspect dev processes during shutdown: %s", discoveryErr)
			}
			return nil
		}
		select {
		case <-waited:
			waited = nil
			exited = true
		case <-ticker.C:
		case <-deadline.C:
			tree.kill()
			if !exited {
				<-waited
			}
			return devServiceError("dev shutdown exceeded %s", grace)
		}
	}
}

func probeDevServices(ctx context.Context, ports template.Ports, logs *devCompilationLog, entryModules []string) <-chan devServiceState {
	states := make(chan devServiceState)
	go func() {
		transport := &http.Transport{DisableKeepAlives: true}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			logs.recoverVite(func(path string) bool {
				return probeViteModule(ctx, client, ports.Web, path)
			})
			state := devServiceState{
				server: probeDevEndpoint(ctx, client, ports.Server, "/health", "health"),
				web: probeDevEndpoint(ctx, client, ports.Web, "/", "page") &&
					probeDevEndpoint(ctx, client, ports.Web, "/runtimeConfig", "config") &&
					probeViteEntryModules(ctx, client, ports.Web, entryModules),
			}
			select {
			case states <- state:
			case <-ctx.Done():
				return
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
	return states
}

func probeDevEndpoint(ctx context.Context, client *http.Client, port int, path, kind string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+path, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	if kind == "page" {
		return true
	}
	var body struct {
		OK     bool `json:"ok"`
		Result struct {
			AppID string `json:"appId"`
		} `json:"Result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil {
		return false
	}
	return (kind == "health" && body.OK) || (kind == "config" && body.Result.AppID != "")
}
