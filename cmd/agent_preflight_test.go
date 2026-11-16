// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"

	"convo-cli/internal/errs"
)

type speechCheckTransport func(*http.Request) (*http.Response, error)

func (f speechCheckTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSpeechPreflightChecksAuthorizationAndHandshake(t *testing.T) {
	for _, status := range []int{101, 403, 401} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			previous := speechPreflightHTTP
			t.Cleanup(func() { speechPreflightHTTP = previous })
			speechPreflightHTTP = &http.Client{Transport: speechCheckTransport(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("X-Api-Access-Key") != "speech-secret" || req.Header.Get("X-Api-Resource-Id") != "seed-tts-2.0" {
					t.Fatal("incorrect resource or credentials")
				}
				digest := sha1.Sum([]byte(req.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
				return &http.Response{StatusCode: status, Header: http.Header{"Upgrade": []string{"websocket"}, "Sec-Websocket-Accept": []string{base64.StdEncoding.EncodeToString(digest[:])}}, Body: io.NopCloser(strings.NewReader("secret upstream body"))}, nil
			})}
			err := checkSpeechAccess(context.Background(), "TTS", map[string]any{"ProviderParams": map[string]any{"ResourceId": "seed-tts-2.0", "app": map[string]any{"appid": "app", "token": "speech-secret"}}})
			if status == 101 {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			typed, ok := errs.As(err)
			if !ok || typed.Code != "byteplus-rtc.openapi.request_failed" {
				t.Fatalf("error=%v", err)
			}
			if strings.Contains(err.Error(), "speech-secret") || strings.Contains(err.Error(), "upstream body") {
				t.Fatal("credential leaked")
			}
		})
	}
}
