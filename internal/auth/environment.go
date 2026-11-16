// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"os"
	"strings"

	"convo-cli/internal/errs"
	"convo-cli/internal/openapi"
)

// EnvironmentProvider loads non-interactive OpenAPI credentials at call time.
// It stores nothing and never includes credential values in errors.
type EnvironmentProvider struct {
	AccessKeyEnv    string
	SecretKeyEnv    string
	SessionTokenEnv string
	ProductName     string
}

func (p EnvironmentProvider) Credentials(context.Context) (openapi.Credential, error) {
	accessKey := strings.TrimSpace(os.Getenv(p.AccessKeyEnv))
	secretKey := strings.TrimSpace(os.Getenv(p.SecretKeyEnv))
	if accessKey == "" || secretKey == "" {
		name := strings.TrimSpace(p.ProductName)
		if name == "" {
			name = "OpenAPI"
		}
		return openapi.Credential{}, errs.New("byteplus-rtc.auth.not_authenticated", errs.TypeAuth,
			"%s credentials are missing or incomplete", name).
			WithHint("sign in or set %s and %s together in the local environment; never pass secrets as command arguments", p.AccessKeyEnv, p.SecretKeyEnv)
	}
	return openapi.Credential{
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
		SessionToken:    strings.TrimSpace(os.Getenv(p.SessionTokenEnv)),
	}, nil
}

// FallbackProvider tries the fallback only when the primary source is simply
// unauthenticated. Invalid or unsafe stored credentials fail closed.
type FallbackProvider struct {
	Primary  openapi.CredentialProvider
	Fallback openapi.CredentialProvider
}

func (p FallbackProvider) Credentials(ctx context.Context) (openapi.Credential, error) {
	credential, err := p.Primary.Credentials(ctx)
	if err == nil {
		return credential, nil
	}
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.auth.not_authenticated" || p.Fallback == nil {
		return openapi.Credential{}, err
	}
	return p.Fallback.Credentials(ctx)
}
