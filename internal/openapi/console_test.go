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
	"strings"
	"testing"
)

func newConsoleTestClient(server *httptest.Server) ConsoleClient {
	client := NewClient(staticProvider{credential: Credential{
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
		SessionToken:    "session",
	}})
	client.Endpoint = server.URL
	client.HTTP = server.Client()
	return NewConsoleClient(client)
}

func TestDescribeAppsEncodesQueryAndDecodes(t *testing.T) {
	raw, err := os.ReadFile("../product/testdata/byteplus-contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		OpenAPI struct {
			AppManagement map[string]string `json:"app_management"`
		} `json:"openapi"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	contract := fixture.OpenAPI.AppManagement
	if contract["list_action"] != DescribeAppsAction || contract["list_version"] != RTCAppListVersion || contract["list_method"] != http.MethodGet || contract["app_key_action"] != DescribeAppKeysAction || contract["app_key_version"] != VersionDescribeApps || contract["app_key_method"] != http.MethodPost {
		t.Fatal("RTC application contract mismatch")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		q := r.URL.Query()
		if q.Get("Action") != "DescribeApps" || q.Get("Version") != RTCAppListVersion {
			t.Errorf("action/version = %q/%q", q.Get("Action"), q.Get("Version"))
		}
		for k, want := range map[string]string{"UserName": "vcloud", "Platform": "0", "Keyword": "", "ProjectName": "default"} {
			if !q.Has(k) || q.Get(k) != want {
				t.Errorf("%s = %q, want %q", k, q.Get(k), want)
			}
		}
		if q.Get("Offset") != "0" || q.Get("Limit") != "100" {
			t.Errorf("Offset/Limit = %q/%q", q.Get("Offset"), q.Get("Limit"))
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("request not signed")
		}
		_, _ = io.WriteString(w, `{"ResponseMetadata":{"RequestId":"rid"},"Result":{"AppList":[{"AppId":"app-1","Name":"first","Status":"1"},{"AppId":"app-2","Name":"second","InstanceStatus":1}]}}`)
	}))
	defer srv.Close()

	apps, err := newConsoleTestClient(srv).DescribeApps(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("DescribeApps: %v", err)
	}
	if len(apps) != 2 || apps[0].Status != "1" || apps[1].Name != "second" || apps[1].Status != "1" {
		t.Fatalf("apps = %+v", apps)
	}
}

func TestDescribeAppsPaginatesAndDeduplicates(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		switch requests {
		case 1:
			if q.Get("Offset") != "0" || q.Get("Limit") != "2" {
				t.Fatalf("first page Offset/Limit = %q/%q", q.Get("Offset"), q.Get("Limit"))
			}
			_, _ = io.WriteString(w, `{"Result":{"AppList":[{"AppId":"app-1","Status":"1"},{"AppId":"app-2","Status":"1"}]}}`)
		case 2:
			if q.Get("Offset") != "2" || q.Get("Limit") != "2" {
				t.Fatalf("second page Offset/Limit = %q/%q", q.Get("Offset"), q.Get("Limit"))
			}
			// app-2 repeats across the page boundary; a full page keeps paging.
			_, _ = io.WriteString(w, `{"Result":{"AppList":[{"AppId":"app-2","Status":"1"},{"AppId":"app-3","Status":"1"}]}}`)
		case 3:
			if q.Get("Offset") != "4" {
				t.Fatalf("third page Offset = %q", q.Get("Offset"))
			}
			_, _ = io.WriteString(w, `{"Result":{"AppList":[]}}`) // short page → stop
		default:
			t.Fatalf("unexpected request %d", requests)
		}
	}))
	defer srv.Close()

	apps, err := newConsoleTestClient(srv).DescribeApps(context.Background(), 0, 2)
	if err != nil {
		t.Fatalf("DescribeApps: %v", err)
	}
	if len(apps) != 3 || apps[0].AppID != "app-1" || apps[2].AppID != "app-3" {
		t.Fatalf("apps = %+v (want app-1,app-2,app-3 deduplicated)", apps)
	}
}

func TestRtcAppIsActiveUsesConsoleStatusMapping(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   bool
	}{
		{status: "1", want: true},
		{status: " 1 ", want: true},
		{status: "0", want: false},
		{status: "Running", want: false},
	} {
		if got := (RtcApp{Status: tc.status}).IsActive(); got != tc.want {
			t.Errorf("status %q: IsActive() = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestDescribeAppKeysPostsAppIDAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Query().Get("Version") != VersionDescribeApps {
			t.Errorf("version = %q", r.URL.Query().Get("Version"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"AppId":"app-1"`) {
			t.Errorf("body missing AppId: %s", body)
		}
		_, _ = io.WriteString(w, `{"ResponseMetadata":{"RequestId":"rid"},"Result":{"AppId":"app-1","AppKey":"0123456789abcdef0123456789abcdef","SecondaryAppKey":"secondary"}}`) // public-scan: allow; gitleaks:allow — synthetic test credential
	}))
	defer srv.Close()

	keys, err := newConsoleTestClient(srv).DescribeAppKeys(context.Background(), "app-1")
	if err != nil {
		t.Fatalf("DescribeAppKeys: %v", err)
	}
	if keys.AppKey != "0123456789abcdef0123456789abcdef" || keys.SecondaryAppKey != "secondary" {
		t.Fatalf("keys = %+v", keys)
	}
}

func TestConsoleClientAPIErrorIsParseable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"ResponseMetadata":{"RequestId":"rid","Error":{"Code":"InvalidParameter","Message":"bad AppId"}}}`)
	}))
	defer srv.Close()

	_, err := newConsoleTestClient(srv).DescribeAppKeys(context.Background(), "nope")
	ae, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err = %T, want *APIError", err)
	}
	if ae.Code != "InvalidParameter" || ae.RequestID != "rid" {
		t.Fatalf("APIError = %+v", ae)
	}
}

func TestDescribeAppsExcludesCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"Result":{"AppList":[{"AppId":"app-1","Name":"demo","Status":1,"AppKey":"secret-one","SecondaryAppKey":"secret-two"}]}}`)
	}))
	defer srv.Close()
	apps, err := newConsoleTestClient(srv).DescribeApps(context.Background(), 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(apps)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "AppKey") {
		t.Fatal("application candidates contain credentials")
	}
	if len(apps) != 1 || apps[0].Name != "demo" || !apps[0].IsActive() {
		t.Fatalf("apps = %+v", apps)
	}
}
