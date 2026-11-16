// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"convo-cli/internal/config"
	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/openapi"
)

type fakeVoiceChatAgentClient struct {
	starts []openapi.StartVoiceChatRequest
	stops  []openapi.StopVoiceChatRequest
}

func (f *fakeVoiceChatAgentClient) StartVoiceChat(_ context.Context, request openapi.StartVoiceChatRequest) (*openapi.StartVoiceChatResult, error) {
	f.starts = append(f.starts, request)
	return &openapi.StartVoiceChatResult{TaskID: request.TaskID}, nil
}

func (f *fakeVoiceChatAgentClient) StopVoiceChat(_ context.Context, request openapi.StopVoiceChatRequest) error {
	f.stops = append(f.stops, request)
	return nil
}

func TestRuntimeAgentCommandsBypassServerGuardWithoutPersistingState(t *testing.T) {
	t.Setenv("BYTEPLUS_MODEL_ENDPOINT_ID", "endpoint")
	t.Setenv("BYTEPLUS_MODEL_API_KEY", "model-secret")
	t.Setenv("BYTEPLUS_SEED_SPEECH_APP_ID", "speech-app")
	t.Setenv("BYTEPLUS_SEED_SPEECH_ACCESS_TOKEN", "speech-secret")
	dir := t.TempDir()
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })

	cfg := config.Default("demo", "voice-agent", "web")
	cfg.Agent.TaskID = "persisted-task"
	if err := config.Save(cfg, config.Path(dir)); err != nil {
		t.Fatal(err)
	}
	taskfile := "version: 2\nscene: voice-agent\nplatform: web\nruntime:\n  topology: web-server\n  agent_control: server\n"
	if err := os.WriteFile(filepath.Join(dir, "byteplus-rtc.taskfile.yaml"), []byte(taskfile), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := &fakeVoiceChatAgentClient{}
	oldFactory, oldDryRun, oldFormat := newVoiceChatAgentClient, flagDryRun, flagFormat
	newVoiceChatAgentClient = func(*config.Config) voiceChatAgentClient { return fake }
	flagDryRun = false
	flagFormat = "json"
	t.Cleanup(func() {
		newVoiceChatAgentClient = oldFactory
		flagDryRun = oldDryRun
		flagFormat = oldFormat
	})

	startRequest := openapi.StartVoiceChatRequest{
		AppID: "aabbccddeeff001122334455", RoomID: "runtime-room", TaskID: "runtime-task",
		Config: json.RawMessage(`{"ASRConfig":{"Provider":"BytePlus","ProviderParams":{}},"LLMConfig":{"Mode":"BytePlusArk"},"TTSConfig":{"Provider":"byteplus_Bidirectional_streaming","ProviderParams":{}}}`), AgentConfig: map[string]any{"UserID": "agent"},
	}
	startJSON, _ := json.Marshal(startRequest)
	start := newAgentStartCmd()
	start.SetArgs([]string{"--runtime-request-stdin"})
	start.SetIn(strings.NewReader(string(startJSON)))
	if err := start.Execute(); err != nil {
		t.Fatal(err)
	}

	stopRequest := openapi.StopVoiceChatRequest{
		AppID: startRequest.AppID, RoomID: startRequest.RoomID, TaskID: startRequest.TaskID,
	}
	stopJSON, _ := json.Marshal(stopRequest)
	stop := newAgentStopCmd()
	stop.SetArgs([]string{"--runtime-request-stdin"})
	stop.SetIn(strings.NewReader(string(stopJSON)))
	if err := stop.Execute(); err != nil {
		t.Fatal(err)
	}

	if len(fake.starts) != 1 || fake.starts[0].TaskID != "runtime-task" || fake.starts[0].BusinessID != meta.BusinessID {
		t.Fatalf("runtime starts = %+v", fake.starts)
	}
	if len(fake.stops) != 1 || fake.stops[0].TaskID != "runtime-task" {
		t.Fatalf("runtime stops = %+v", fake.stops)
	}
	loaded, _, err := config.LoadNearest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Agent.TaskID != "persisted-task" {
		t.Fatalf("runtime commands changed agent.task_id to %q", loaded.Agent.TaskID)
	}
	if _, err := os.Stat(agentStatePath(dir)); !os.IsNotExist(err) {
		t.Fatalf("runtime commands wrote legacy agent state: %v", err)
	}
}

func TestRuntimeAgentRequestRejectsMissingIdentity(t *testing.T) {
	err := validateRuntimeStopRequest(openapi.StopVoiceChatRequest{AppID: "app"})
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.openapi.invalid_request" {
		t.Fatalf("error = %v", err)
	}

	err = validateRuntimeStartRequest(openapi.StartVoiceChatRequest{
		AppID:       "app",
		RoomID:      "room",
		TaskID:      "task",
		Config:      json.RawMessage(`{"ASRConfig":{"Provider":"BytePlus","ProviderParams":{}},"LLMConfig":{"Mode":"BytePlusArk"},"TTSConfig":{"Provider":"byteplus_Bidirectional_streaming","ProviderParams":{}}}`),
		AgentConfig: map[string]any{},
	})
	typed, ok = errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.openapi.invalid_request" || !strings.Contains(typed.Message, "AgentConfig.UserID") {
		t.Fatalf("error = %v", err)
	}
}

func TestRuntimeStopRequestNeedsOnlyIdentity(t *testing.T) {
	err := validateRuntimeStopRequest(openapi.StopVoiceChatRequest{
		AppID: "app", RoomID: "room", TaskID: "task",
	})
	if err != nil {
		t.Fatalf("identity-only StopVoiceChat request rejected: %v", err)
	}
}
