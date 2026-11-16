// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBytePlusResourceClientUsesPinnedActionsAndKeepsSecretsOutOfLists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("Action") {
		case "ListEndpoints":
			_, _ = io.WriteString(w, `{"Result":{"Items":[{"Id":"ep-1","Name":"model","Status":"Running"}]}}`)
		case "ListApiKeys":
			_, _ = io.WriteString(w, `{"Result":{"Items":[{"Id":7,"Name":"key-name","Key":"must-not-decode","Status":"Active"}]}}`)
		case "GetRawApiKey":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request["Id"] != float64(7) {
				t.Errorf("Id = %#v, want numeric 7", request["Id"])
			}
			_, _ = io.WriteString(w, `{"Result":{"ApiKey":"raw-secret"}}`)
		case "ListApplications":
			_, _ = io.WriteString(w, `{"status":"success","data":{"applications":[{"appid":"speech-1","name":"speech"}]}}`)
		case "ApplicationCredentials":
			if r.URL.Query().Get("AppID") != "speech-1" {
				t.Errorf("AppID = %q", r.URL.Query().Get("AppID"))
			}
			_, _ = io.WriteString(w, `{"status":"success","data":{"credentials":[{"access_token":"speech-secret"}]}}`)
		default:
			http.Error(w, "unexpected action", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	newClient := func(service string) *Client {
		client := NewClient(staticProvider{credential: Credential{AccessKeyID: "ak", SecretAccessKey: "sk", SessionToken: "sts"}})
		client.Endpoint, client.Service, client.Region, client.HTTP = server.URL, service, "ap-southeast-1", server.Client()
		return client
	}
	client := BytePlusResourceClient{Ark: newClient(BytePlusArkService), Speech: newClient(BytePlusSpeechService)}
	ctx := context.Background()
	endpoints, err := client.ListModelEndpoints(ctx, "default")
	if err != nil || len(endpoints) != 1 || endpoints[0].ID != "ep-1" {
		t.Fatalf("endpoints=%+v err=%v", endpoints, err)
	}
	keys, err := client.ListModelAPIKeys(ctx, "default")
	if err != nil || len(keys) != 1 || keys[0].ID.String() != "7" {
		t.Fatalf("keys=%+v err=%v", keys, err)
	}
	raw, err := client.GetModelRawAPIKey(ctx, "7", "default")
	if err != nil || raw != "raw-secret" {
		t.Fatalf("raw present=%v err=%v", raw != "", err)
	}
	apps, err := client.ListSpeechApps(ctx)
	if err != nil || len(apps) != 1 || apps[0].AppID != "speech-1" {
		t.Fatalf("apps=%+v err=%v", apps, err)
	}
	token, err := client.GetSpeechAccessToken(ctx, "speech-1")
	if err != nil || token != "speech-secret" {
		t.Fatalf("token present=%v err=%v", token != "", err)
	}
}

func TestBytePlusSpeechCredentialRetriesTransientServerFailures(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"status":"success","data":{"credentials":[{"access_token":"speech-secret"}]}}`)
	}))
	defer server.Close()

	client := NewClient(staticProvider{credential: Credential{AccessKeyID: "ak", SecretAccessKey: "sk"}})
	client.Endpoint, client.Service, client.Region, client.HTTP = server.URL, BytePlusSpeechService, "ap-southeast-1", server.Client()
	token, err := (BytePlusResourceClient{Speech: client}).GetSpeechAccessToken(context.Background(), "speech-1")
	if err != nil || token != "speech-secret" {
		t.Fatalf("token present=%v err=%v", token != "", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}
