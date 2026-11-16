// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"net/http"
	"time"

	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/openapi"
)

type STSProvider struct {
	HTTPClient    *http.Client
	RefreshSkew   time.Duration
	TokenEndpoint string
	Scope         string
}

func (p STSProvider) Credentials(ctx context.Context) (openapi.Credential, error) {
	tokenEndpoint := p.TokenEndpoint
	if tokenEndpoint == "" {
		tokenEndpoint = DefaultTokenEndpoint
	}
	token, _, err := EnsureFreshTokenWithScope(ctx, p.HTTPClient, p.RefreshSkew, tokenEndpoint, p.Scope)
	if err != nil {
		return openapi.Credential{}, err
	}
	sts, err := ParseSTSCredential(token.AccessToken)
	if err != nil {
		return openapi.Credential{}, errs.New("byteplus-rtc.auth.invalid_token", errs.TypeAuth,
			"stored access_token is not a valid STS credential: %s", err).
			WithHint("run `%s auth login` again", meta.BinName)
	}
	return openapi.Credential{
		AccessKeyID:     sts.AccessKeyID,
		SecretAccessKey: sts.SecretAccessKey,
		SessionToken:    sts.SessionToken,
	}, nil
}
