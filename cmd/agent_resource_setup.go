// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"convo-cli/internal/config"
	projectenv "convo-cli/internal/env"
	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/openapi"
	"convo-cli/internal/product"
)

type agentResourceOption struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

func agentResourceOptions(options []agentResourceOption) []agentResourceOption {
	counts := map[string]int{}
	for _, option := range options {
		counts[strings.TrimSpace(option.Name)]++
	}
	for i := range options {
		name := strings.TrimSpace(options[i].Name)
		if name == "" {
			name = "Unnamed resource"
		}
		options[i].DisplayName = name
		if strings.TrimSpace(options[i].Name) == "" || counts[strings.TrimSpace(options[i].Name)] > 1 {
			options[i].DisplayName = fmt.Sprintf("%s (%s)", name, options[i].ID)
		}
	}
	return options
}

func chooseAgentResource(reader *bufio.Reader, prompt io.Writer, label, selected string, options []agentResourceOption, interactive bool) (string, error) {
	options = agentResourceOptions(options)
	guide := agentResourceGuideFor(label)
	details := map[string]any{"resource": label, "candidates": options, "selection_flag": guide.Flag, "documentation_url": guide.DocumentationURL, "console_url": guide.ConsoleURL}
	for _, o := range options {
		if selected != "" && o.ID == selected {
			return selected, nil
		}
	}
	if selected != "" {
		return "", errs.New("byteplus-rtc.agent.missing_config", errs.TypeValidation, "selected %s %q is unavailable in this project", label, selected).WithDetails(details).WithHint("run `%s agent resources` and choose an available resource by name", meta.BinName)
	}
	if len(options) == 0 {
		return "", errs.New("byteplus-rtc.agent.missing_config", errs.TypeNotFound, "no %s resources found", label).WithDetails(details).WithHint("%s Guide: %s Console: %s", guide.Action, guide.DocumentationURL, guide.ConsoleURL)
	}
	if !interactive {
		return "", errs.New("byteplus-rtc.dev.selection_required", errs.TypePrecondition, "select a %s resource", label).WithDetails(details).WithHint("present candidate display names to the user, map the chosen candidate to its id, then rerun `%s agent resources --write` with %s", meta.BinName, guide.Flag)
	}
	fmt.Fprintf(prompt, "Select %s:\n", label)
	for i, o := range options {
		fmt.Fprintf(prompt, "  %d. %s\n", i+1, o.DisplayName)
	}
	fmt.Fprint(prompt, "Selection (number or name): ")
	line, err := reader.ReadString('\n')
	if err != nil && !(err == io.EOF && line != "") {
		return "", errs.New("byteplus-rtc.dev.selection_required", errs.TypePrecondition, "%s selection was not completed", label)
	}
	choice := strings.TrimSpace(line)
	if index, err := strconv.Atoi(choice); err == nil && index >= 1 && index <= len(options) {
		return options[index-1].ID, nil
	}
	matches := []string{}
	for _, option := range options {
		if choice != "" && (choice == strings.TrimSpace(option.Name) || choice == option.DisplayName) {
			matches = append(matches, option.ID)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", errs.New("byteplus-rtc.dev.selection_required", errs.TypeValidation, "multiple %s resources have this name", label).
			WithDetails(details).WithHint("select the numbered entry for the intended resource")
	}
	return "", errs.New("byteplus-rtc.dev.selection_required", errs.TypeValidation, "invalid %s selection", label).WithDetails(details).WithHint("select a listed number or resource name")
}

func configureAgentResourceSelectors(ctx context.Context, cfg *config.Config, dir string, selection bytePlusResourceSelection, input io.Reader, prompt io.Writer) (map[string]string, error) {
	reader := bufio.NewReader(input)
	terminal, interactive := input.(*os.File)
	interactive = interactive && rtcInputIsTerminal(terminal)
	flexible := true
	if interactive {
		fmt.Fprint(prompt, "Configure Flexible resources? [Y/n] ")
		answer, err := reader.ReadString('\n')
		if err != nil {
			return nil, errs.Wrap(err, "byteplus-rtc.dev.selection_required", "read Flexible resource selection")
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "", "y", "yes":
		case "n", "no":
			flexible = false
		default:
			return nil, errs.New("byteplus-rtc.dev.selection_required", errs.TypeValidation, "enter yes or no")
		}
	}

	updates := map[string]string{}
	if flexible {
		var err error
		updates, err = selectFlexibleResources(ctx, cfg, selection, reader, prompt, interactive)
		if err != nil {
			return nil, err
		}
	}
	local, err := projectenv.Load(projectenv.Path(dir))
	if err != nil {
		return nil, err
	}
	if flexible && updates["BYTEPLUS_PROJECT_NAME"] == local["BYTEPLUS_PROJECT_NAME"] {
		if updates["BYTEPLUS_MODEL_API_KEY_ID"] == local["BYTEPLUS_MODEL_API_KEY_ID"] {
			updates["BYTEPLUS_MODEL_API_KEY"] = local["BYTEPLUS_MODEL_API_KEY"]
		}
		if updates["BYTEPLUS_SEED_SPEECH_APP_ID"] == local["BYTEPLUS_SEED_SPEECH_APP_ID"] {
			updates["BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN"] = local["BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN"]
		}
	}
	rtcID, rtcKey, err := resourceRTCApplication(ctx, cfg, local, selection.RTCAppID, input, reader, prompt)
	if err != nil {
		if typed, ok := errs.As(err); ok && !interactive && typed.Param == "--app-id" && typed.Code == "byteplus-rtc.dev.selection_required" {
			selection.RTCAppID = "<app-id>"
			typed.Details["retry_argv"] = selection.retryWriteArgs()
		}
		return nil, err
	}
	updates["RTC_APP_ID"], updates["RTC_APP_KEY"] = rtcID, rtcKey
	if cfg != nil && cfg.Agent.ConfigSource != config.AgentConfigSourceBrowser {
		updates["VITE_RTC_APP_ID"] = rtcID
	}
	if interactive {
		fmt.Fprint(prompt, "OpenAI API key for Realtime (Enter to skip): ")
		key, err := readResourcePassword(int(terminal.Fd()))
		fmt.Fprintln(prompt)
		if err != nil {
			return nil, errs.Wrap(err, "byteplus-rtc.dev.selection_required", "read Realtime API key")
		}
		if len(key) > 0 {
			updates["OPENAI_API_KEY"] = string(key)
		}
	}
	if _, err := projectenv.Write(projectenv.Path(dir), updates); err != nil {
		return nil, err
	}
	delete(updates, "BYTEPLUS_MODEL_API_KEY")
	delete(updates, "BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN")
	delete(updates, "RTC_APP_KEY")
	delete(updates, "OPENAI_API_KEY")
	return updates, nil
}

func resourceRTCApplication(ctx context.Context, cfg *config.Config, local map[string]string, requested string, input io.Reader, reader *bufio.Reader, prompt io.Writer) (string, string, error) {
	configured := ""
	if cfg != nil && cfg.RTC.AppID != "" && cfg.RTC.AppID != "${RTC_APP_ID}" {
		var missing []string
		configured, missing = config.ResolveEnv(cfg.RTC.AppID)
		if len(missing) > 0 || configured == "" || requested != "" && requested != configured {
			return "", "", errs.New("byteplus-rtc.dev.app_setup_failed", errs.TypeValidation, "set rtc.app_id to the selected RTC application or to ${RTC_APP_ID}")
		}
	}
	id := firstNonEmptyString(requested, configured, os.Getenv("RTC_APP_ID"), local["RTC_APP_ID"])
	if id != "" && id == local["RTC_APP_ID"] && local["RTC_APP_KEY"] != "" {
		return id, local["RTC_APP_KEY"], nil
	}
	client, err := newDevConsoleClient(cfg)
	if err != nil {
		return "", "", err
	}
	if id == "" {
		apps, err := queryDevApps(ctx, client)
		if err != nil {
			return "", "", err
		}
		switch {
		case len(apps) == 0:
			return "", "", errs.New("byteplus-rtc.dev.app_setup_failed", errs.TypeNotFound, "no active RTC applications are available").WithHint("%s", noAvailableAppHint())
		case len(apps) == 1:
			id = apps[0].AppID
			fmt.Fprintf(prompt, "Selected RTC application: %s (%s)\n", apps[0].Name, id)
		case rtcInputIsTerminal(input):
			app, err := selectRTCAppWithReader(input, reader, prompt, apps)
			if err != nil {
				return "", "", err
			}
			id = app.AppID
		default:
			return "", "", devAppSelectionRequiredError(apps, "select an active RTC application").WithHint("retry %s agent resources --write with --app-id <id>", meta.BinName)
		}
	}
	keyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	keys, err := client.DescribeAppKeys(keyCtx, id)
	if err != nil {
		var apiErr *openapi.APIError
		if cfg != nil && cfg.Agent.ConfigSource == config.AgentConfigSourceBrowser && errors.As(err, &apiErr) &&
			(apiErr.Code == "AccessDenied" || strings.HasPrefix(apiErr.Code, "AccessDenied.") || apiErr.Code == "LackPolicy") {
			fmt.Fprintln(prompt, "RTC AppKey access was denied. The browser Demo will use GetToken.")
			return id, "", nil
		}
		return "", "", mapDevRTCAppError(err)
	}
	if strings.TrimSpace(keys.AppKey) == "" {
		return "", "", errs.New("byteplus-rtc.dev.app_setup_failed", errs.TypeNotFound, "no primary AppKey returned for the selected RTC application")
	}
	return id, keys.AppKey, nil
}

var readResourcePassword = term.ReadPassword

func (selection bytePlusResourceSelection) retryWriteArgs() []string {
	retry := []string{meta.BinName, "agent", "resources", "--write",
		"--resource-project", selection.ProjectName,
		"--model-endpoint-id", firstNonEmptyString(selection.ModelEndpointID, "<model-endpoint-id>"),
		"--model-api-key-id", firstNonEmptyString(selection.ModelAPIKeyID, "<model-api-key-id>"),
		"--speech-app-id", firstNonEmptyString(selection.SpeechAppID, "<speech-app-id>"),
	}
	if selection.RTCAppID != "" {
		retry = append(retry, "--app-id", selection.RTCAppID)
	}
	return retry
}

func selectFlexibleResources(ctx context.Context, cfg *config.Config, selection bytePlusResourceSelection, reader *bufio.Reader, prompt io.Writer, interactive bool) (map[string]string, error) {
	client := newBytePlusResourceClient(cfg)
	endpoints, err := client.ListModelEndpoints(ctx, selection.ProjectName)
	if err != nil {
		return nil, agentResourceFailure(err, "model service", selection.ProjectName)
	}
	keys, err := client.ListModelAPIKeys(ctx, selection.ProjectName)
	if err != nil {
		return nil, agentResourceFailure(err, "model API key", selection.ProjectName)
	}
	apps, err := client.ListSpeechApps(ctx)
	if err != nil {
		return nil, agentResourceFailure(err, "speech application", selection.ProjectName)
	}
	choices := agentResourceChoices(endpoints, keys, apps, selection.ProjectName)
	selectionError := func(err error) error {
		typed, ok := errs.As(err)
		if ok {
			if typed.Details == nil {
				typed.Details = map[string]any{}
			}
			typed.Details["choices"] = choices
			typed.Details["project"] = selection.ProjectName
			typed.Details["region"] = product.Current().OpenAPIRegion
			typed.Details["retry_argv"] = selection.retryWriteArgs()
		}
		return err
	}
	endpoint, err := chooseAgentResource(reader, prompt, "model service", selection.ModelEndpointID, choices["model_endpoints"], interactive)
	if err != nil {
		return nil, selectionError(err)
	}
	selection.ModelEndpointID = endpoint
	key, err := chooseAgentResource(reader, prompt, "model API key", selection.ModelAPIKeyID, choices["model_api_keys"], interactive)
	if err != nil {
		return nil, selectionError(err)
	}
	selection.ModelAPIKeyID = key
	speech, err := chooseAgentResource(reader, prompt, "speech application", selection.SpeechAppID, choices["speech_apps"], interactive)
	if err != nil {
		return nil, selectionError(err)
	}
	return map[string]string{"BYTEPLUS_MODEL_ENDPOINT_ID": endpoint, "BYTEPLUS_MODEL_API_KEY_ID": key, "BYTEPLUS_SEED_SPEECH_APP_ID": speech, "BYTEPLUS_PROJECT_NAME": selection.ProjectName, "BYTEPLUS_MODEL_API_KEY": "", "BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN": ""}, nil
}

func configureResourceCommand(c *cobra.Command, project string) error {
	cfg, path, err := config.LoadNearest(".")
	if err != nil {
		return err
	}
	if flagDryRun {
		keys := []string{"RTC_APP_ID", "RTC_APP_KEY", "BYTEPLUS_MODEL_ENDPOINT_ID", "BYTEPLUS_MODEL_API_KEY_ID", "BYTEPLUS_SEED_SPEECH_APP_ID", "BYTEPLUS_PROJECT_NAME", "BYTEPLUS_MODEL_API_KEY", "BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN"}
		if cfg.Agent.ConfigSource != config.AgentConfigSourceBrowser {
			keys = append(keys, "VITE_RTC_APP_ID")
		}
		if rtcInputIsTerminal(c.InOrStdin()) {
			keys = append(keys, "OPENAI_API_KEY")
		}
		return out().Data(map[string]any{"dry_run": true, "path": projectenv.Path(projectDir(path)), "keys": keys, "conditionally_cleared_keys": []string{"BYTEPLUS_MODEL_API_KEY", "BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN"}})
	}
	selection := bytePlusResourceSelection{RTCAppID: flagValue(c, "app-id"), ModelEndpointID: flagValue(c, bytePlusModelEndpointFlag), ModelAPIKeyID: flagValue(c, bytePlusModelAPIKeyFlag), SpeechAppID: flagValue(c, bytePlusSpeechAppFlag), ProjectName: project}
	updates, err := configureAgentResourceSelectors(c.Context(), cfg, projectDir(path), selection, c.InOrStdin(), c.ErrOrStderr())
	if err != nil {
		return err
	}
	if updates != nil {
		out().Progress("resource selections saved; restart the running dev process to apply them")
	}
	return out().Data(map[string]any{"path": projectenv.Path(projectDir(path)), "resources": updates, "restart_required": updates != nil, "next_steps": []string{meta.BinName + " dev"}})
}

func agentResourceChoices(endpoints []openapi.BytePlusModelEndpoint, keys []openapi.BytePlusModelAPIKey, apps []openapi.BytePlusSpeechApp, project string) map[string][]agentResourceOption {
	endpointOptions, keyOptions, speechOptions := []agentResourceOption{}, []agentResourceOption{}, []agentResourceOption{}
	for _, e := range endpoints {
		endpointOptions = append(endpointOptions, agentResourceOption{ID: e.ID, Name: e.Name})
	}
	for _, k := range keys {
		keyOptions = append(keyOptions, agentResourceOption{ID: k.ID.String(), Name: k.Name})
	}
	for _, a := range apps {
		if a.ProjectName == "" || a.ProjectName == project {
			speechOptions = append(speechOptions, agentResourceOption{ID: a.AppID, Name: a.Name})
		}
	}
	return map[string][]agentResourceOption{
		"model_endpoints": agentResourceOptions(endpointOptions),
		"model_api_keys":  agentResourceOptions(keyOptions),
		"speech_apps":     agentResourceOptions(speechOptions),
	}
}
