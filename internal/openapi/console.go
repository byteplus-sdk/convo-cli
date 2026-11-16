// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Console setup OpenAPI versions. Each action pins its own Version, so —
// unlike AgentClient (DefaultVersion 2025-06-01) — ConsoleClient carries no
// single default and passes Version per call.
const (
	VersionDescribeApps     = "2020-12-01" // DescribeAppKeys
	RTCAppListVersion       = "2018-01-01"
	DescribeAppsAction      = "DescribeApps"
	DescribeAppKeysAction   = "DescribeAppKeys"
	RTCAppManagementVersion = VersionDescribeApps
	RTCAppStatusActive      = "1"
)

// ConsoleClient reads RTC applications and their AppKeys through signed OpenAPI calls.
type ConsoleClient struct {
	OpenAPI *Client
}

// NewConsoleClient builds a ConsoleClient over the given signed client.
func NewConsoleClient(client *Client) ConsoleClient {
	return ConsoleClient{OpenAPI: client}
}

// RtcApp is one entry of DescribeApps' AppList. Only the fields dev setup
// needs are modeled; unknown fields are ignored. AppKey is empty here (it comes
// from DescribeAppKeys).
type RtcApp struct {
	AppID  string `json:"AppId"`
	Name   string `json:"Name"`
	Status string `json:"-"`
}

// IsActive reports whether the console marks the RTC application as usable.
// Keep the service status mapping here so future wire values do not leak into
// command orchestration.
func (a RtcApp) IsActive() bool {
	return strings.TrimSpace(a.Status) == RTCAppStatusActive
}

func (a *RtcApp) UnmarshalJSON(data []byte) error {
	var raw struct {
		AppID          string          `json:"AppId"`
		Name           string          `json:"Name"`
		Status         json.RawMessage `json:"Status"`
		InstanceStatus json.RawMessage `json:"InstanceStatus"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.AppID, a.Name = raw.AppID, raw.Name
	status := raw.Status
	if len(status) == 0 || string(status) == "null" {
		status = raw.InstanceStatus
	}
	if len(status) == 0 || string(status) == "null" {
		return nil
	}
	if err := json.Unmarshal(status, &a.Status); err == nil {
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(status, &number); err != nil {
		return fmt.Errorf("decode RTC app status: %w", err)
	}
	a.Status = number.String()
	return nil
}

type describeAppsResult struct {
	AppList []RtcApp `json:"AppList"`
}

// DescribeApps lists the account's RTC apps under ProjectName=default and
// returns them so dev setup can resolve a target AppId. It pages through every
// result (advancing Offset by Limit until a short page) so accounts with more
// than one page of apps stay fully selectable — a bounded loop guards against a
// misbehaving backend that never returns a short page.
func (c ConsoleClient) DescribeApps(ctx context.Context, offset, limit int) ([]RtcApp, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var all []RtcApp
	seen := map[string]bool{}
	for pages := 0; pages < 100; pages++ {
		query := url.Values{}
		query.Set("Offset", strconv.Itoa(offset))
		query.Set("Limit", strconv.Itoa(limit))
		query.Set("ProjectName", "default")
		query.Set("UserName", "vcloud")
		query.Set("Platform", "0")
		query.Set("Keyword", "")
		var result describeAppsResult
		if err := c.OpenAPI.InvokeResult(ctx, InvokeOptions{
			Action:  DescribeAppsAction,
			Version: RTCAppListVersion,
			Method:  http.MethodGet,
			Query:   query,
		}, &result); err != nil {
			return nil, err
		}
		added := 0
		for _, app := range result.AppList {
			if id := strings.TrimSpace(app.AppID); id != "" {
				if seen[id] {
					continue
				}
				seen[id] = true
			}
			all = append(all, app)
			added++
		}
		// Stop on a short page, or when a full page contributed nothing new — a
		// backend that ignores Offset/Limit would otherwise loop to the page cap.
		if len(result.AppList) < limit || added == 0 {
			break
		}
		offset += limit
	}
	return all, nil
}

// AppKeys is the DescribeAppKeys Result: the primary AppKey plus a secondary.
// AppKey is a secret — callers MUST keep it env-only and never log it.
type AppKeys struct {
	AppID           string `json:"AppId"`
	AppKey          string `json:"AppKey"`
	SecondaryAppKey string `json:"SecondaryAppKey"`
}

// DescribeAppKeys fetches the AppKey for a given AppId. The Signin STS is
// sufficient — the console's MFA is a UI gate and does not apply at the OpenAPI
// layer.
func (c ConsoleClient) DescribeAppKeys(ctx context.Context, appID string) (AppKeys, error) {
	body, err := json.Marshal(map[string]string{"AppId": appID})
	if err != nil {
		return AppKeys{}, &APIError{Action: "DescribeAppKeys", Message: "marshal request: " + err.Error()}
	}
	var result AppKeys
	if err := c.OpenAPI.InvokeResult(ctx, InvokeOptions{
		Action:  "DescribeAppKeys",
		Version: VersionDescribeApps,
		Method:  http.MethodPost,
		Body:    body,
	}, &result); err != nil {
		return AppKeys{}, err
	}
	return result, nil
}
