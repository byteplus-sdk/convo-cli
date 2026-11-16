// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"html"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var viteScriptTag = regexp.MustCompile(`(?is)<script\b[^>]*>`)
var viteScriptAttribute = regexp.MustCompile(`(?is)\s(type|src)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)

func viteEntryModulePaths(dir string) ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "web", "index.html"))
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, tag := range viteScriptTag.FindAllString(string(raw), -1) {
		attributes := map[string]string{}
		for _, match := range viteScriptAttribute.FindAllStringSubmatch(tag, -1) {
			attributes[strings.ToLower(match[1])] = html.UnescapeString(match[2] + match[3] + match[4])
		}
		if !strings.EqualFold(attributes["type"], "module") || attributes["src"] == "" {
			continue
		}
		source, err := url.Parse(attributes["src"])
		if err != nil {
			return nil, fmt.Errorf("invalid module source: %w", err)
		}
		if source.IsAbs() || source.Host != "" {
			continue
		}
		if strings.ContainsAny(source.Path, "\\\x00\r\n") {
			return nil, fmt.Errorf("invalid local module source")
		}
		for _, segment := range strings.Split(source.Path, "/") {
			if segment == ".." {
				return nil, fmt.Errorf("module source must stay inside the web project")
			}
		}
		source.Path = path.Clean("/" + strings.TrimPrefix(source.Path, "/"))
		source.RawPath, source.Fragment = "", ""
		paths = append(paths, source.String())
	}
	return paths, nil
}

func probeViteEntryModules(ctx context.Context, client *http.Client, port int, paths []string) bool {
	for _, module := range paths {
		if !probeViteModule(ctx, client, port, module) {
			return false
		}
	}
	return true
}

func probeViteModule(ctx context.Context, client *http.Client, port int, module string) bool {
	endpoint, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(port) + module)
	if err != nil {
		return false
	}
	query := endpoint.Query()
	query.Set("import", "")
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return false
	}
	response, err := client.Do(req)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	return err == nil && response.StatusCode == http.StatusOK && (contentType == "text/javascript" || contentType == "application/javascript")
}
