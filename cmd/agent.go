// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"convo-cli/internal/affordance"
	"convo-cli/internal/config"
	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/openapi"
	"convo-cli/internal/product"
	"convo-cli/internal/template"
)

const runtimeAgentRequestLimit = 2 << 20

type voiceChatAgentClient interface {
	StartVoiceChat(context.Context, openapi.StartVoiceChatRequest) (*openapi.StartVoiceChatResult, error)
	StopVoiceChat(context.Context, openapi.StopVoiceChatRequest) error
}

var newVoiceChatAgentClient = func(cfg *config.Config) voiceChatAgentClient {
	client := openapi.NewAgentClient(newAuthenticatedOpenAPIClient(cfg))
	return client
}

func newAgentCmd() *cobra.Command {
	// Hidden advanced/legacy command: in server-managed projects the web
	// app controls the agent lifecycle (the companion server delegates via the
	// hidden `--runtime-request-stdin` path), so the happy path never runs
	// `convo-cli agent` by hand. It stays registered and runnable for legacy
	// CLI-managed projects, but is kept off the browsable `--help` list.
	cmd := &cobra.Command{
		Use:    "agent",
		Short:  "Orchestrate the conversational AI agent in legacy CLI-managed projects",
		Hidden: true,
	}
	cmd.AddCommand(newAgentStartCmd(), newAgentStopCmd(), newAgentStatusCmd(), newAgentCheckCmd())
	if resources, err := currentAgentResourceContract(); err == nil {
		if resourceCmd := resources.Command(); resourceCmd != nil {
			cmd.AddCommand(resourceCmd)
		}
	}
	affordance.Attach(cmd, affordance.Affordance{
		When:   []string{"Bringing the AI agent into the room for a legacy CLI-managed voice-agent scene"},
		Avoid:  []string{"Non-agent projects; v2 server-managed projects use the web app"},
		Prereq: []string{"auth login complete; agent.config_file contains the console template"},
		Examples: []string{
			meta.BinName + " agent start",
			meta.BinName + " agent start --dry-run",
			meta.BinName + " agent stop",
		},
	})
	return cmd
}

// buildStartRequest loads the selected scene from agent.config_file and injects
// the top-level identity params + AgentConfig.UserId/TargetUserId.
func buildStartRequest(cfg *config.Config, dir string) (openapi.StartVoiceChatRequest, error) {
	if cfg.Project.Scene != "voice-agent" {
		return openapi.StartVoiceChatRequest{}, errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation,
			"project scene is %q, not voice-agent", cfg.Project.Scene).
			WithHint("agent commands apply to voice-agent projects")
	}
	appID, missing := config.ResolveEnv(cfg.RTC.AppID)
	if len(missing) > 0 {
		return openapi.StartVoiceChatRequest{}, errs.New("byteplus-rtc.config.unresolved_env", errs.TypeValidation,
			"rtc.app_id references unset env %v", missing).WithParam("rtc.app_id").
			WithHint("run `%s dev` to select an RTC application, or set the missing values in .env.local", meta.BinName)
	}
	roomID := mustResolve(cfg.RTC.RoomID)
	if appID == "" || roomID == "" || cfg.RTC.UserID == "" {
		return openapi.StartVoiceChatRequest{}, errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation,
			"rtc.app_id/room_id/user_id are all required").
			WithHint("fill rtc.* in %s", meta.ConfigFileName)
	}

	cfgFile := cfg.Agent.ConfigFile
	if cfgFile == "" {
		cfgFile = "server/scenes/default.json"
	}
	tplPath, err := config.ProjectFilePath(dir, cfgFile)
	if err != nil {
		return openapi.StartVoiceChatRequest{}, err
	}
	raw, err := os.ReadFile(tplPath)
	if err != nil {
		return openapi.StartVoiceChatRequest{}, errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation,
			"cannot read agent template %q: %s", cfgFile, err).
			WithHint("copy the code example from Console > My Agents into %s", cfgFile)
	}
	resolvedRaw, missing := config.ResolveEnv(string(raw))
	if len(missing) > 0 {
		return openapi.StartVoiceChatRequest{}, errs.New("byteplus-rtc.config.unresolved_env", errs.TypeValidation,
			"agent config references unset env %v", missing).WithParam("agent.config_file").
			WithHint("set the missing values in the local environment or .env.local; never commit provider secrets")
	}
	tpl, err := config.ParseServerSceneJSON([]byte(resolvedRaw))
	if err != nil {
		return openapi.StartVoiceChatRequest{}, errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation,
			"%s is not a valid scene: %s", cfgFile, err).
			WithHint("fix SceneConfig and VoiceChat.Config/AgentConfig in %s", cfgFile)
	}

	agentCfg := tpl.VoiceChat.AgentConfig
	if agentCfg == nil {
		agentCfg = map[string]any{}
	}
	profile := product.Current()
	// Prefer the config's agent identity, but never overwrite the scene's
	// validated product-specific user field with an empty value: a blank value would
	// start a task the AI cannot join under.
	if strings.TrimSpace(cfg.Agent.UserID) != "" {
		agentCfg[profile.AgentUserIDField] = cfg.Agent.UserID
	}
	if userID, _ := agentCfg[profile.AgentUserIDField].(string); strings.TrimSpace(userID) == "" {
		return openapi.StartVoiceChatRequest{}, errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation,
			"agent.user_id is required to start the agent").
			WithParam("agent.user_id").
			WithHint("set agent.user_id (the AI's UserId) in %s", meta.ConfigFileName)
	}
	target := cfg.Agent.TargetUserID
	if target == "" {
		target = cfg.RTC.UserID
	}
	agentCfg[profile.AgentTargetUserIDField] = []string{target} // API currently supports one target

	taskID := cfg.Agent.TaskID
	if taskID == "" {
		taskID = "agent-" + roomID
	}
	return openapi.StartVoiceChatRequest{
		AppID: appID, BusinessID: profile.BusinessID, RoomID: roomID, TaskID: taskID,
		Config: tpl.VoiceChat.Config, AgentConfig: agentCfg,
	}, nil
}

func isEmptyJSON(raw json.RawMessage) bool {
	s := string(bytes.TrimSpace(raw))
	return s == "" || s == "{}" || s == "null"
}

// requireCLIAgentControl keeps the legacy agent commands available for v1
// projects while routing v2 full-stack projects to their companion server.
func requireCLIAgentControl(cfg *config.Config, dir string) error {
	if cfg.Project.Scene != "voice-agent" {
		return nil
	}
	taskfilePath := filepath.Join(dir, template.TaskfileName())
	if _, err := os.Stat(taskfilePath); err != nil {
		if os.IsNotExist(err) {
			return nil // pre-taskfile and hand-authored projects retain v1 behavior
		}
		return errs.Wrap(err, "byteplus-rtc.cli.internal", "inspect taskfile: %s", err)
	}
	tf, err := template.LoadTaskfile(dir)
	if err != nil {
		return err
	}
	if !tf.ServerManagedAgent() {
		return nil
	}
	return errs.New("byteplus-rtc.agent.server_managed", errs.TypePrecondition,
		"agent lifecycle is managed by this project's companion server").
		WithHint("run `%s dev`, then start or stop the AI from the web app", meta.BinName)
}

func newAgentStartCmd() *cobra.Command {
	var runtimeRequestStdin bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the AI agent (StartVoiceChat) into the configured room",
		RunE: func(c *cobra.Command, args []string) error {
			cfg, path, err := config.LoadNearest(".")
			if err != nil {
				return err
			}
			dir := projectDir(path)
			if runtimeRequestStdin {
				var req openapi.StartVoiceChatRequest
				if err := decodeRuntimeAgentRequest(c.InOrStdin(), &req); err != nil {
					return err
				}
				resources, err := currentAgentResourceContract()
				if err != nil {
					return err
				}
				if err := resources.PrepareRuntimeStart(c.Context(), cfg, &req, runtimeStartOptions{dryRun: flagDryRun}); err != nil {
					return err
				}
				// The companion server is local project code and may be modified;
				// enforce CLI attribution at the final trusted OpenAPI boundary.
				req.BusinessID = product.Current().BusinessID
				if err := validateRuntimeStartRequest(req); err != nil {
					return err
				}
				if flagDryRun {
					out().Progress("dry-run: previewing runtime StartVoiceChat request, not calling")
					result, err := agentStartDryRunResult(req)
					if err != nil {
						return mapOpenAPIError(err)
					}
					return out().Data(result)
				}
				out().Progress("calling StartVoiceChat through CLI runtime…")
				res, err := newVoiceChatAgentClient(cfg).StartVoiceChat(c.Context(), req)
				if err != nil {
					return mapOpenAPIError(err)
				}
				return out().Data(map[string]any{
					"task_id": res.TaskID, "room_id": req.RoomID, "status": "running",
				})
			}
			if err := requireCLIAgentControl(cfg, dir); err != nil {
				return err
			}
			resources, err := currentAgentResourceContract()
			if err != nil {
				return err
			}
			cleanupResources, err := resources.PrepareStart(c.Context(), cfg, c, flagDryRun)
			if err != nil {
				return err
			}
			defer cleanupResources()
			req, err := buildStartRequest(cfg, dir)
			if err != nil {
				return err
			}
			if flagDryRun {
				out().Progress("dry-run: previewing StartVoiceChat request, not calling")
				result, err := agentStartDryRunResult(req)
				if err != nil {
					return mapOpenAPIError(err)
				}
				return out().Data(result)
			}
			out().Progress("calling StartVoiceChat…")
			res, err := newVoiceChatAgentClient(cfg).StartVoiceChat(c.Context(), req)
			if err != nil {
				return mapOpenAPIError(err)
			}
			cfg.Agent.TaskID = res.TaskID
			if err := config.Save(cfg, path); err != nil {
				return err
			}
			_ = saveAgentState(dir, agentState{
				TaskID: res.TaskID, RoomID: req.RoomID, Status: "running", StartedAt: time.Now(),
			})
			out().Progress("agent started (TaskId=%s) — next: `%s dev` to open the web app; `%s agent stop` to end (AI tasks are billed)", res.TaskID, meta.BinName, meta.BinName)
			return out().Data(map[string]any{
				"task_id": res.TaskID, "room_id": req.RoomID, "status": "running",
				"next_steps": []string{meta.BinName + " dev", meta.BinName + " agent stop"},
			})
		},
	}
	cmd.Flags().BoolVar(&runtimeRequestStdin, "runtime-request-stdin", false,
		"read a server-managed StartVoiceChat request from stdin")
	_ = cmd.Flags().MarkHidden("runtime-request-stdin")
	if resources, err := currentAgentResourceContract(); err == nil {
		resources.ConfigureStart(cmd)
	}
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"After config + credentials are ready, to bring the AI into the room"},
		Prereq:   []string{"doctor PASS; signed in with `" + meta.BinName + " auth login`; agent.config_file is configured"},
		Examples: []string{meta.BinName + " agent start", meta.BinName + " agent start --dry-run"},
	})
	return cmd
}

func agentStartDryRunResult(req openapi.StartVoiceChatRequest) (map[string]any, error) {
	preview := req
	if product.Current().Capabilities.SecretBackedSceneConfig {
		preview = openapi.SanitizeStartVoiceChatRequest(req)
	}
	wire, err := openapi.EncodeStartVoiceChatRequest(preview)
	if err != nil {
		return nil, err
	}
	return map[string]any{"dry_run": true, "request": wire}, nil
}

func newAgentStopCmd() *cobra.Command {
	var runtimeRequestStdin bool
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the running AI agent (StopVoiceChat)",
		RunE: func(c *cobra.Command, args []string) error {
			cfg, path, err := config.LoadNearest(".")
			if err != nil {
				return err
			}
			if runtimeRequestStdin {
				var req openapi.StopVoiceChatRequest
				if err := decodeRuntimeAgentRequest(c.InOrStdin(), &req); err != nil {
					return err
				}
				if err := validateRuntimeStopRequest(req); err != nil {
					return err
				}
				if flagDryRun {
					out().Progress("dry-run: previewing runtime StopVoiceChat request, not calling")
					return out().Data(map[string]any{"dry_run": true, "request": req})
				}
				out().Progress("calling StopVoiceChat through CLI runtime…")
				if err := newVoiceChatAgentClient(cfg).StopVoiceChat(c.Context(), req); err != nil {
					return mapOpenAPIError(err)
				}
				return out().Data(map[string]any{"task_id": req.TaskID, "status": "stopped"})
			}
			if err := requireCLIAgentControl(cfg, projectDir(path)); err != nil {
				return err
			}
			taskID := cfg.Agent.TaskID
			if taskID == "" {
				return errs.New("byteplus-rtc.agent.no_task", errs.TypePrecondition,
					"no running agent task to stop").
					WithHint("run `%s agent start` first", meta.BinName)
			}
			if flagDryRun {
				out().Progress("dry-run: would StopVoiceChat TaskId=%s", taskID)
				return out().Data(map[string]any{"dry_run": true, "task_id": taskID})
			}
			out().Progress("calling StopVoiceChat…")
			if err := newVoiceChatAgentClient(cfg).StopVoiceChat(c.Context(), openapi.StopVoiceChatRequest{
				AppID: mustResolve(cfg.RTC.AppID), RoomID: mustResolve(cfg.RTC.RoomID), TaskID: taskID,
			}); err != nil {
				return mapOpenAPIError(err)
			}
			cfg.Agent.TaskID = ""
			_ = config.Save(cfg, path)
			clearAgentState(projectDir(path))
			return out().Data(map[string]any{"task_id": taskID, "status": "stopped"})
		},
	}
	cmd.Flags().BoolVar(&runtimeRequestStdin, "runtime-request-stdin", false,
		"read a server-managed StopVoiceChat request from stdin")
	_ = cmd.Flags().MarkHidden("runtime-request-stdin")
	return cmd
}

func decodeRuntimeAgentRequest(input io.Reader, target any) error {
	raw, err := io.ReadAll(io.LimitReader(input, runtimeAgentRequestLimit+1))
	if err != nil {
		return errs.Wrap(err, "byteplus-rtc.cli.internal", "read runtime agent request: %s", err)
	}
	if len(raw) > runtimeAgentRequestLimit {
		return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation,
			"runtime agent request exceeds %d bytes", runtimeAgentRequestLimit)
	}
	if len(bytes.TrimSpace(raw)) == 0 || json.Unmarshal(raw, target) != nil {
		return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation,
			"runtime agent request must be valid JSON")
	}
	return nil
}

func validateRuntimeStartRequest(req openapi.StartVoiceChatRequest) error {
	if strings.TrimSpace(req.AppID) == "" || strings.TrimSpace(req.RoomID) == "" || strings.TrimSpace(req.TaskID) == "" {
		return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation,
			"runtime StartVoiceChat request requires AppId, RoomId, and TaskId")
	}
	if isEmptyJSON(req.Config) || req.AgentConfig == nil {
		return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation,
			"runtime StartVoiceChat request requires Config and AgentConfig")
	}
	userIDField := product.Current().AgentUserIDField
	userID, ok := req.AgentConfig[userIDField].(string)
	if !ok || strings.TrimSpace(userID) == "" {
		return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation,
			"runtime StartVoiceChat request requires AgentConfig.%s", userIDField)
	}
	return nil
}

func validateRuntimeStopRequest(req openapi.StopVoiceChatRequest) error {
	if strings.TrimSpace(req.AppID) == "" || strings.TrimSpace(req.RoomID) == "" || strings.TrimSpace(req.TaskID) == "" {
		return errs.New("byteplus-rtc.openapi.invalid_request", errs.TypeValidation,
			"runtime StopVoiceChat request requires AppId, RoomId, and TaskId")
	}
	return nil
}

func newAgentStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the local agent task state",
		RunE: func(c *cobra.Command, args []string) error {
			cfg, path, err := config.LoadNearest(".")
			if err != nil {
				return err
			}
			if err := requireCLIAgentControl(cfg, projectDir(path)); err != nil {
				return err
			}
			st, ok := loadAgentState(projectDir(path))
			if !ok {
				return out().Data(map[string]any{"running": false})
			}
			return out().Data(map[string]any{
				"running": st.Status == "running", "task_id": st.TaskID,
				"room_id": st.RoomID, "started_at": st.StartedAt, "status": st.Status,
			})
		},
	}
}

// mapOpenAPIError converts an *openapi.APIError into a typed CLI error.
func mapOpenAPIError(err error) error {
	if _, ok := errs.As(err); ok {
		return err
	}
	ae, ok := err.(*openapi.APIError)
	if !ok {
		return errs.Wrap(err, "byteplus-rtc.openapi.request_failed", "%s", err.Error())
	}
	if ae.HTTPStatus == 403 || strings.HasPrefix(ae.Code, "AccessDenied") || ae.Code == "LackPolicy" {
		hint := "Ask the account owner or IAM administrator to grant access to the selected resources."
		switch ae.Action {
		case "DescribeApps", "DescribeAppKeys", "StartVoiceChat", "StopVoiceChat":
			hint = fmt.Sprintf("Use an RTC AppID accessible to the account; ask its owner or IAM administrator to grant rtc:DescribeApps and rtc:DescribeAppKeys access. Stop the running dev process, then run `%s dev --reconfigure` to select the application and refresh its credentials.", meta.BinName)
		case "ApplicationCredentials", "ListApplications":
			hint = "Ask the account owner or IAM administrator for access to the Seed Speech application and activation of ASR and TTS; verify the application at https://console.byteplus.com/voice/app."
		case "GetRawApiKey", "ListApiKeys", "ListEndpoints":
			hint = "Ask the account owner or IAM administrator for ModelArk project access; select a model service and API key from that project."
		}
		code, kind := "byteplus-rtc.openapi.api_error", errs.TypePrecondition
		if ae.Code == "" {
			code, kind = "byteplus-rtc.openapi.request_failed", errs.TypeIO
		}
		return errs.New(code, kind, "%s", ae.Error()).WithHint("%s To change the account, run `%s auth login --force --browser=open`, then `%s agent resources --write`; restart the dev process after updating configuration. Check exported BYTEPLUS_ACCESS_KEY_ID and BYTEPLUS_SECRET_ACCESS_KEY when using environment credentials.", hint, meta.BinName, meta.BinName)
	}
	if ae.Code != "" {
		return errs.New("byteplus-rtc.openapi.api_error", errs.TypePrecondition, "%s", ae.Error()).
			WithHint("run `%s explain-error %s` or `%s doctor` to triage", meta.BinName, ae.Code, meta.BinName)
	}
	return errs.New("byteplus-rtc.openapi.request_failed", errs.TypeIO, "%s", ae.Error()).
		WithHint("check network/credentials, then `%s doctor`", meta.BinName)
}

// --- local agent runtime state (project-dir .convo-cli/agent.json) ---

type agentState struct {
	TaskID    string    `json:"task_id"`
	RoomID    string    `json:"room_id"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
}

func agentStatePath(dir string) string { return filepath.Join(dir, ".convo-cli", "agent.json") }

func saveAgentState(dir string, st agentState) error {
	p := agentStatePath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	return os.WriteFile(p, b, 0o644)
}

func loadAgentState(dir string) (agentState, bool) {
	b, err := os.ReadFile(agentStatePath(dir))
	if err != nil {
		return agentState{}, false
	}
	var st agentState
	if json.Unmarshal(b, &st) != nil {
		return agentState{}, false
	}
	return st, true
}

func clearAgentState(dir string) { _ = os.Remove(agentStatePath(dir)) }
