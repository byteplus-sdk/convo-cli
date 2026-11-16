// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package openapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"convo-cli/internal/errs"
)

const (
	BytePlusResourceEndpoint        = "https://open.byteplusapi.com"
	BytePlusArkEndpoint             = "https://ark.ap-southeast-1.byteplusapi.com"
	BytePlusArkService              = "ark"
	BytePlusArkVersion              = "2024-01-01"
	BytePlusSpeechService           = "speech_saas_prod"
	BytePlusSpeechListVersion       = "2021-11-22"
	BytePlusSpeechCredentialVersion = "2021-08-30"
)

type BytePlusResourceClient struct {
	Ark    *Client
	Speech *Client
}

type BytePlusModelEndpoint struct {
	ID             string `json:"Id"`
	Name           string `json:"Name"`
	ProjectName    string `json:"ProjectName"`
	Status         string `json:"Status"`
	ModelReference struct {
		FoundationModel struct {
			Name string `json:"Name"`
		} `json:"FoundationModel"`
	} `json:"ModelReference"`
}

type BytePlusModelAPIKey struct {
	ID          json.Number `json:"Id"`
	Name        string      `json:"Name"`
	ProjectName string      `json:"ProjectName"`
	Status      string      `json:"Status"`
}

type BytePlusSpeechApp struct {
	AppID       string `json:"appid"`
	Name        string `json:"name"`
	ProjectName string `json:"volc_project_name"`
	State       string `json:"state"`
}

func (c BytePlusResourceClient) ListModelEndpoints(ctx context.Context, project string) ([]BytePlusModelEndpoint, error) {
	body, _ := json.Marshal(map[string]any{
		"PageNumber": 1, "PageSize": 100, "ProjectName": project,
		"Filter": map[string]any{"Statuses": []string{"Running"}},
	})
	var result struct {
		Items []BytePlusModelEndpoint `json:"Items"`
	}
	if err := c.Ark.InvokeResult(ctx, InvokeOptions{Action: "ListEndpoints", Version: BytePlusArkVersion, Method: http.MethodPost, Body: body}, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (c BytePlusResourceClient) ListModelAPIKeys(ctx context.Context, project string) ([]BytePlusModelAPIKey, error) {
	body, _ := json.Marshal(map[string]any{
		"PageNumber": 1, "PageSize": 100, "ProjectName": project,
		"Filter": map[string]any{"Statuses": []string{"Active"}},
	})
	var result struct {
		Items []BytePlusModelAPIKey `json:"Items"`
	}
	if err := c.Ark.InvokeResult(ctx, InvokeOptions{Action: "ListApiKeys", Version: BytePlusArkVersion, Method: http.MethodPost, Body: body}, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (c BytePlusResourceClient) GetModelRawAPIKey(ctx context.Context, id, project string) (string, error) {
	var resourceID any = id
	if numericID, err := strconv.ParseInt(id, 10, 64); err == nil {
		resourceID = numericID
	}
	body, _ := json.Marshal(map[string]any{"Id": resourceID, "ProjectName": project})
	var result struct {
		APIKey string `json:"ApiKey"`
	}
	if err := c.Ark.InvokeResult(ctx, InvokeOptions{Action: "GetRawApiKey", Version: BytePlusArkVersion, Method: http.MethodPost, Body: body}, &result); err != nil {
		return "", err
	}
	return result.APIKey, nil
}

func (c BytePlusResourceClient) ListSpeechApps(ctx context.Context) ([]BytePlusSpeechApp, error) {
	response, err := c.Speech.Invoke(ctx, InvokeOptions{Action: "ListApplications", Version: BytePlusSpeechListVersion, Method: http.MethodGet})
	if err != nil {
		return nil, err
	}
	var result struct {
		Data struct {
			Applications []BytePlusSpeechApp `json:"applications"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, err
	}
	return result.Data.Applications, nil
}

func (c BytePlusResourceClient) GetSpeechAccessToken(ctx context.Context, appID string) (string, error) {
	query := url.Values{"AppID": []string{appID}}
	var response Response
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		response, err = c.Speech.Invoke(ctx, InvokeOptions{Action: "ApplicationCredentials", Version: BytePlusSpeechCredentialVersion, Method: http.MethodGet, Query: query})
		if err == nil {
			break
		}
		if attempt == 3 || !transientResourceError(err) {
			return "", err
		}
		timer := time.NewTimer(time.Duration(100*(1<<attempt)) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	var result struct {
		Data struct {
			Credentials []struct {
				AccessToken string `json:"access_token"`
			} `json:"credentials"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return "", err
	}
	if len(result.Data.Credentials) == 0 {
		return "", nil
	}
	return result.Data.Credentials[0].AccessToken, nil
}

func transientResourceError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.HTTPStatus >= http.StatusInternalServerError
	}
	var typed *errs.Error
	return errors.As(err, &typed) && typed.Code == "byteplus-rtc.openapi.request_failed"
}
