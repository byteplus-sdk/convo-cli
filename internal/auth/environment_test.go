// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"convo-cli/internal/errs"
	"convo-cli/internal/openapi"
)

type credentialProviderFunc func(context.Context) (openapi.Credential, error)

func (f credentialProviderFunc) Credentials(ctx context.Context) (openapi.Credential, error) {
	return f(ctx)
}

func TestEnvironmentProviderLoadsCompleteCredential(t *testing.T) {
	t.Setenv("TEST_AK", "ak")
	t.Setenv("TEST_SK", "sk")
	t.Setenv("TEST_ST", "sts")
	credential, err := (EnvironmentProvider{AccessKeyEnv: "TEST_AK", SecretKeyEnv: "TEST_SK", SessionTokenEnv: "TEST_ST"}).Credentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccessKeyID != "ak" || credential.SecretAccessKey != "sk" || credential.SessionToken != "sts" {
		t.Fatalf("credential = %+v", credential)
	}
}

func TestEnvironmentProviderDoesNotLeakPartialSecret(t *testing.T) {
	t.Setenv("TEST_AK", "sensitive-ak")
	t.Setenv("TEST_SK", "")
	_, err := (EnvironmentProvider{AccessKeyEnv: "TEST_AK", SecretKeyEnv: "TEST_SK", ProductName: "test"}).Credentials(context.Background())
	if err == nil || strings.Contains(err.Error(), "sensitive-ak") {
		t.Fatalf("unsafe error = %v", err)
	}
	typed, ok := errs.As(err)
	if !ok || typed.Code != "byteplus-rtc.auth.not_authenticated" {
		t.Fatalf("error = %#v", err)
	}
}

func TestFallbackProviderOnlyFallsBackForNotAuthenticated(t *testing.T) {
	fallbackCalls := 0
	fallback := credentialProviderFunc(func(context.Context) (openapi.Credential, error) {
		fallbackCalls++
		return openapi.Credential{AccessKeyID: "fallback"}, nil
	})
	provider := FallbackProvider{
		Primary: credentialProviderFunc(func(context.Context) (openapi.Credential, error) {
			return openapi.Credential{}, errs.New("byteplus-rtc.auth.not_authenticated", errs.TypeAuth, "missing")
		}),
		Fallback: fallback,
	}
	credential, err := provider.Credentials(context.Background())
	if err != nil || credential.AccessKeyID != "fallback" || fallbackCalls != 1 {
		t.Fatalf("credential=%+v calls=%d err=%v", credential, fallbackCalls, err)
	}

	provider.Primary = credentialProviderFunc(func(context.Context) (openapi.Credential, error) {
		return openapi.Credential{}, errors.New("corrupt")
	})
	_, err = provider.Credentials(context.Background())
	if err == nil || fallbackCalls != 1 {
		t.Fatalf("fallback used for unsafe error: calls=%d err=%v", fallbackCalls, err)
	}
}
