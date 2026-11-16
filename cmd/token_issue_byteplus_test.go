// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"convo-cli/internal/openapi"
	"convo-cli/internal/product"
	"convo-cli/internal/token"
)

type bytePlusTokenTestProvider struct{}

func (bytePlusTokenTestProvider) Credentials(context.Context) (openapi.Credential, error) {
	return openapi.Credential{AccessKeyID: "ak", SecretAccessKey: "sk", SessionToken: "sts"}, nil
}

func TestBytePlusTokenIssueUsesRemoteGetTokenWithoutAppKey(t *testing.T) {
	want, err := token.Issue(token.IssueParams{
		AppID: "012345678901234567890123", AppKey: "test-key", RoomID: "room-1", UserID: "user-1",
		Publish: true, Subscribe: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("Action"); got != "GetToken" {
			t.Errorf("Action = %q", got)
		}
		if got := r.URL.Query().Get("Version"); got != bytePlusGetTokenVersion {
			t.Errorf("Version = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		var request map[string]string
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		if request["AppId"] != want.AppID || request["RoomId"] != want.RoomID || request["UserId"] != want.UserID {
			t.Errorf("request = %#v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Result": map[string]any{
			"Token": want.Token, "ExpireTime": want.ExpireAt.Unix(),
		}})
	}))
	defer server.Close()

	client := openapi.NewClient(bytePlusTokenTestProvider{})
	client.Endpoint, client.Service, client.Region, client.HTTP = server.URL, "rtc", "ap-southeast-1", server.Client()
	contract := bytePlusTokenIssueContract{client: client}
	got, err := contract.Issue(context.Background(), nil, token.IssueParams{
		AppID: want.AppID, RoomID: want.RoomID, UserID: want.UserID, Publish: true, Subscribe: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != want.Token || got.AppID != want.AppID || got.RoomID != want.RoomID || got.UserID != want.UserID {
		t.Fatalf("got = %+v", got)
	}
}

func TestBytePlusTokenIssueKeepsLocalAppKeyPath(t *testing.T) {
	contract := bytePlusTokenIssueContract{}
	got, err := contract.Issue(context.Background(), nil, token.IssueParams{
		AppID: "012345678901234567890123", AppKey: "test-key", RoomID: "room-1", UserID: "user-1",
		Publish: true, Subscribe: true,
	})
	if err != nil || got.Token == "" {
		t.Fatalf("token present=%v err=%v", got != nil && got.Token != "", err)
	}
}

func TestBytePlusTokenIssueHelpDocumentsRemoteCredentials(t *testing.T) {
	previous := product.SelectedID
	product.SelectedID = product.BytePlusID
	t.Cleanup(func() { product.SelectedID = previous })

	cmd := newTokenIssueCmd()
	var help bytes.Buffer
	cmd.SetOut(&help)
	cmd.SetErr(&help)
	if err := cmd.Help(); err != nil {
		t.Fatal(err)
	}
	got := help.String()
	if !strings.Contains(got, "Signin credentials or RTC_APP_KEY") ||
		!strings.Contains(got, "auth login or BytePlus AK/SK") {
		t.Fatalf("unexpected BytePlus token issue help:\n%s", got)
	}
}
