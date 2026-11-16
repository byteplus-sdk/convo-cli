// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

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
			typed.Details["retry_argv"] = []string{meta.BinName, "agent", "resources", "--write",
				"--resource-project", selection.ProjectName,
				"--model-endpoint-id", firstNonEmptyString(selection.ModelEndpointID, "<model-endpoint-id>"),
				"--model-api-key-id", firstNonEmptyString(selection.ModelAPIKeyID, "<model-api-key-id>"),
				"--speech-app-id", firstNonEmptyString(selection.SpeechAppID, "<speech-app-id>"),
			}
		}
		return err
	}
	reader := bufio.NewReader(input)
	interactive := rtcInputIsTerminal(input)
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
	updates := map[string]string{"BYTEPLUS_MODEL_ENDPOINT_ID": endpoint, "BYTEPLUS_MODEL_API_KEY_ID": key, "BYTEPLUS_SEED_SPEECH_APP_ID": speech, "BYTEPLUS_PROJECT_NAME": selection.ProjectName, "BYTEPLUS_MODEL_API_KEY": "", "BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN": ""}
	if err := writeAgentResourceSelectors(projectenv.Path(dir), updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func writeAgentResourceSelectors(path string, updates map[string]string) error {
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return errs.Wrap(err, "byteplus-rtc.env.write_failed", "read local resource configuration")
	}
	newline := "\n"
	if strings.Contains(string(raw), "\r\n") {
		newline = "\r\n"
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(raw) == 0 {
		lines = nil
	}
	seen := map[string]bool{}
	for i, line := range lines {
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(key), "export "))
		if value, ok := updates[key]; ok {
			lines[i] = key + "=" + value
			seen[key] = true
		}
	}
	keys := []string{}
	for key, value := range updates {
		if strings.ContainsAny(key+value, "\r\n\x00'\"") {
			return errs.New("byteplus-rtc.env.write_failed", errs.TypeValidation, "resource selector must be a single-line ID")
		}
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, key+"="+updates[key])
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".resource-env-*")
	if err != nil {
		return errs.Wrap(err, "byteplus-rtc.env.write_failed", "create local resource configuration")
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0o600); err == nil {
		_, err = file.WriteString(strings.Join(lines, newline) + newline)
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		return errs.Wrap(err, "byteplus-rtc.env.write_failed", "save local resource configuration")
	}
	return nil
}

func configureResourceCommand(c *cobra.Command, project string) error {
	cfg, path, err := config.LoadNearest(".")
	if err != nil {
		return err
	}
	if flagDryRun {
		return out().Data(map[string]any{"dry_run": true, "path": projectenv.Path(projectDir(path)), "keys": []string{"BYTEPLUS_MODEL_ENDPOINT_ID", "BYTEPLUS_MODEL_API_KEY_ID", "BYTEPLUS_SEED_SPEECH_APP_ID", "BYTEPLUS_PROJECT_NAME", "BYTEPLUS_MODEL_API_KEY", "BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN"}, "cleared_keys": []string{"BYTEPLUS_MODEL_API_KEY", "BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN"}})
	}
	selection := bytePlusResourceSelection{ModelEndpointID: flagValue(c, bytePlusModelEndpointFlag), ModelAPIKeyID: flagValue(c, bytePlusModelAPIKeyFlag), SpeechAppID: flagValue(c, bytePlusSpeechAppFlag), ProjectName: project}
	updates, err := configureAgentResourceSelectors(c.Context(), cfg, projectDir(path), selection, c.InOrStdin(), c.ErrOrStderr())
	if err != nil {
		return err
	}
	delete(updates, "BYTEPLUS_MODEL_API_KEY")
	delete(updates, "BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN")
	out().Progress("resource selections saved; restart the running dev process to apply them")
	return out().Data(map[string]any{"path": projectenv.Path(projectDir(path)), "resources": updates, "restart_required": true, "next_steps": []string{meta.BinName + " dev"}})
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
