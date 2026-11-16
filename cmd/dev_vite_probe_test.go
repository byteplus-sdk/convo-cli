// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestViteEntryModulePaths(t *testing.T) {
	for _, tc := range []struct {
		name, markup string
		want         []string
		fail         bool
	}{
		{"local", `<script type="module" src="./src/main.tsx"></script><script src='/other.js' type='module'></script>`, []string{"/src/main.tsx", "/other.js"}, false},
		{"attributes", `<SCRIPT TYPE=module SRC="/src/main.tsx?one=1&amp;two=2#x"></SCRIPT>`, []string{"/src/main.tsx?one=1&two=2"}, false},
		{"other", `<script src="/classic.js"></script><script type="module">import '/inline.js'</script><script type="module" src="https://example.com/app.js"></script>`, nil, false},
		{"traversal", `<script type="module" src="/%2e%2e/private.js"></script>`, nil, true},
		{"backslash", `<script type="module" src="/src\main.js"></script>`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "web"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "web", "index.html"), []byte(tc.markup), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := viteEntryModulePaths(dir)
			if (err != nil) != tc.fail || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("paths=%v err=%v", got, err)
			}
		})
	}
}

func TestViteEntryProbeRejectsTransformFailure(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusInternalServerError)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/src.ts" {
			requests.Add(1)
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			w.WriteHeader(int(status.Load()))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	_, rawPort, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	if probeViteEntryModules(context.Background(), server.Client(), port, []string{"/src.ts"}) {
		t.Fatal("failed module accepted")
	}
	status.Store(http.StatusOK)
	if !probeViteEntryModules(context.Background(), server.Client(), port, []string{"/src.ts"}) {
		t.Fatal("restored module rejected")
	}
	if requests.Load() != 2 {
		t.Fatalf("module requests=%d", requests.Load())
	}
}

func TestViteModuleProbeRejectsHTMLFallback(t *testing.T) {
	for _, contentType := range []string{"text/html", "text/javascript; charset=utf-8", "application/javascript"} {
		t.Run(contentType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !r.URL.Query().Has("import") {
					t.Error("module transform query missing")
				}
				w.Header().Set("Content-Type", contentType)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			_, rawPort, err := net.SplitHostPort(server.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(rawPort)
			if err != nil {
				t.Fatal(err)
			}
			if got := probeViteModule(context.Background(), server.Client(), port, "/src/missing.tsx"); got != (contentType != "text/html") {
				t.Fatalf("content type %s accepted=%v", contentType, got)
			}
		})
	}
}
