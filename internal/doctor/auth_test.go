// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package doctor

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"convo-cli/internal/auth"
)

type authTransport func(*http.Request) (*http.Response, error)

func (f authTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCLIAuthChecksExpiryAndRefresh(t *testing.T) {
	sts, _ := json.Marshal(auth.STSCredential{AccessKeyID: "fixture-ak", SecretAccessKey: "fixture-sk", SessionToken: "fixture-session"})
	for _, tc := range []struct {
		name           string
		expired        bool
		response       string
		status         int
		networkFailure bool
		want           Status
	}{
		{"valid", false, "", 0, false, PASS},
		{"refreshed", true, `{"access_token":` + string(mustAuthJSON(string(sts))) + `,"expires_in":3600}`, 200, false, PASS},
		{"revoked", true, `{"error":"invalid_grant"}`, 400, false, FAIL},
		{"offline", true, "", 0, true, UNKNOWN},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restore := auth.SetTokenStoreForTest(auth.NewMemoryTokenStore())
			defer restore()
			expiry := time.Now().Add(time.Hour)
			if tc.expired {
				expiry = time.Now().Add(-time.Hour)
			}
			if err := auth.SaveToken(auth.TokenSet{AccessToken: string(sts), RefreshToken: "fixture-refresh", ClientID: "fixture-client", ExpiresAt: expiry.Format(time.RFC3339)}); err != nil {
				t.Fatal(err)
			}
			previous := http.DefaultClient
			defer func() { http.DefaultClient = previous }()
			requests := 0
			http.DefaultClient = &http.Client{Transport: authTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				if tc.networkFailure {
					return nil, errors.New("network unavailable")
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.response))}, nil
			})}
			check, ok := findCheck(RunCLI(), "cli.auth")
			if !ok || check.Status != tc.want {
				t.Fatalf("check=%+v want=%s", check, tc.want)
			}
			if (requests > 0) != tc.expired {
				t.Fatalf("refresh requests=%d expired=%v", requests, tc.expired)
			}
			if tc.name == "refreshed" {
				token, err := auth.LoadToken()
				if err != nil || token.Expired(time.Now(), 0) {
					t.Fatalf("refresh was not persisted: %v", err)
				}
			}
		})
	}
}

func mustAuthJSON(value string) []byte { raw, _ := json.Marshal(value); return raw }
