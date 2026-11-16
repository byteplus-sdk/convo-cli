// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"convo-cli/internal/affordance"
	"convo-cli/internal/config"
	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/openapi"
	"convo-cli/internal/product"
	"convo-cli/internal/token"
)

const bytePlusGetTokenVersion = "2025-06-01"

type bytePlusTokenIssueContract struct {
	client *openapi.Client
}

func (bytePlusTokenIssueContract) CommandShort() string {
	return "Issue an RTC join token using Signin credentials or RTC_APP_KEY"
}

func (bytePlusTokenIssueContract) CommandAffordance() affordance.Affordance {
	return affordance.Affordance{
		When:   []string{"Right after filling config, to unblock room join"},
		Prereq: []string{"rtc.app_id/room_id/user_id set; auth login or BytePlus AK/SK available"},
		Examples: []string{
			meta.BinName + " token issue --write",
			meta.BinName + " token issue --room-id room-2 --user-id alice --write",
		},
	}
}

func (contract bytePlusTokenIssueContract) Issue(ctx context.Context, cfg *config.Config, params token.IssueParams) (*token.IssueResult, error) {
	if params.AppKey != "" {
		return token.Issue(params)
	}
	body, _ := json.Marshal(map[string]string{
		"AppId":  params.AppID,
		"RoomId": params.RoomID,
		"UserId": params.UserID,
	})
	client := contract.client
	if client == nil {
		client = newAuthenticatedOpenAPIClient(cfg)
	}
	var result struct {
		Token      string `json:"Token"`
		ExpireTime int64  `json:"ExpireTime"`
	}
	if err := client.InvokeResult(ctx, openapi.InvokeOptions{
		Action: "GetToken", Version: bytePlusGetTokenVersion, Method: http.MethodPost, Body: body,
	}, &result); err != nil {
		return nil, mapOpenAPIError(err)
	}
	if result.Token == "" {
		return nil, errs.New("byteplus-rtc.openapi.request_failed", errs.TypeIO,
			"BytePlus GetToken response did not contain a token")
	}
	issued := &token.IssueResult{
		Token: result.Token, AppID: params.AppID, RoomID: params.RoomID, UserID: params.UserID,
		ExpireAt: time.Unix(result.ExpireTime, 0), Publish: true, Subscribe: true,
	}
	if info, err := token.Inspect(result.Token); err == nil {
		issued.AppID, issued.RoomID, issued.UserID = info.AppID, info.RoomID, info.UserID
		issued.Publish, issued.Subscribe = info.Publish, info.Subscribe
		if !info.ExpireAt.IsZero() {
			issued.ExpireAt = info.ExpireAt
		}
	}
	return issued, nil
}

func init() { registerTokenIssueContract(product.BytePlusID, bytePlusTokenIssueContract{}) }
