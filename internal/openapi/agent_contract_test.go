// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"convo-cli/internal/product"
)

func TestBytePlusAgentClientUsesVersionAndSanitizedFixtures(t *testing.T) {
	previous := product.SelectedID
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() { product.SelectedID = previous })
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get("Version"); got != product.BytePlusVoiceChatVersion {
			t.Errorf("Version = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		fixture := "byteplus-start-voice-chat.json"
		if r.URL.Query().Get("Action") == "StopVoiceChat" {
			fixture = "byteplus-stop-voice-chat.json"
		}
		want, err := os.ReadFile(filepath.Join("testdata", fixture))
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != strings.TrimSpace(string(want)) {
			t.Errorf("body = %s, want %s", body, want)
		}
		_, _ = io.WriteString(w, `{"ResponseMetadata":{"RequestId":"request"},"Result":"ok"}`)
	}))
	defer server.Close()
	client := NewClient(staticProvider{credential: Credential{AccessKeyID: "ak", SecretAccessKey: "sk"}})
	client.Endpoint, client.Service, client.Region, client.HTTP = server.URL, "rtc", "ap-southeast-1", server.Client()
	agent := NewAgentClient(client)
	start := StartVoiceChatRequest{
		AppID: "app", BusinessID: "must-not-leak", RoomID: "room", TaskID: "task",
		Config:      json.RawMessage(`{"ASRConfig":{"Provider":"BytePlus"}}`),
		AgentConfig: map[string]any{"TargetUserID": []string{"user"}, "UserID": "agent"},
	}
	if _, err := agent.StartVoiceChat(context.Background(), start); err != nil {
		t.Fatal(err)
	}
	if err := agent.StopVoiceChat(context.Background(), StopVoiceChatRequest{AppID: "app", RoomID: "room", TaskID: "task"}); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("network requests = %d, want 2", requests)
	}
}

func TestBytePlusVoiceChatWireContractOmitsUnsupportedFields(t *testing.T) {
	contract := lookupVoiceChatWireContract(product.BytePlusID)
	if contract == nil {
		t.Fatal("unexpected BytePlus contract")
	}
	start := sampleStartReq()
	start.BusinessID = "must-not-leak"
	start.AgentConfig = map[string]any{"UserID": "agent", "TargetUserID": []string{"user"}}
	got, err := json.Marshal(contract.EncodeStart(start))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"AppId":"app","RoomId":"room","TaskId":"task","Config":{"ASRConfig":{"Provider":"BytePlus"},"LLMConfig":{"Mode":"BytePlusArk","EndPointId":"ep-test"}},"AgentConfig":{"TargetUserID":["user"],"UserID":"agent"}}`
	if string(got) != want {
		t.Fatalf("encoded start = %s, want %s", got, want)
	}
}

func TestMissingVoiceChatWireContractFailsClosed(t *testing.T) {
	client := AgentClient{}
	if err := client.requireContract("StartVoiceChat"); err == nil {
		t.Fatal("missing contract was accepted")
	}
}
