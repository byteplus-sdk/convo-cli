// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"convo-cli/internal/affordance"
	"convo-cli/internal/auth"
	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/product"
)

const authRefreshSkew = 15 * time.Minute

const (
	authBrowserAsk  = "ask"
	authBrowserOpen = "open"
)

var (
	authOpenBrowser   = auth.OpenBrowser
	authHTTPClient    = http.DefaultClient
	authRandomURLSafe = auth.RandomURLSafe
	authIsTerminal    = func(reader io.Reader) bool {
		file, ok := reader.(*os.File)
		return ok && term.IsTerminal(int(file.Fd()))
	}
	// These strings remain linker-overridable for the repository E2E fixture.
	// init fills only empty production builds from the selected profile.
	oauthAuthorizeEndpoint string
	oauthTokenEndpoint     string
)

func init() {
	profile := product.Current()
	if oauthAuthorizeEndpoint == "" {
		oauthAuthorizeEndpoint = profile.OAuthAuthorizeEndpoint
	}
	if oauthTokenEndpoint == "" {
		oauthTokenEndpoint = profile.OAuthTokenEndpoint
	}
}

func currentAuthRefreshSkew() time.Duration {
	if skew := product.Current().OAuthRefreshSkew; skew > 0 {
		return skew
	}
	return authRefreshSkew
}

type authLoginResult struct {
	Authenticated   bool   `json:"authenticated"`
	TokenLocation   string `json:"token_location"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	HasRefreshToken bool   `json:"has_refresh_token"`
	ClientID        string `json:"client_id"`
	Scope           string `json:"scope"`
}

type authStatusResult struct {
	Authenticated   bool   `json:"authenticated"`
	TokenLocation   string `json:"token_location"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	Expired         bool   `json:"expired"`
	Refreshed       bool   `json:"refreshed"`
	HasAccessToken  bool   `json:"has_access_token"`
	HasRefreshToken bool   `json:"has_refresh_token"`
	TokenType       string `json:"token_type,omitempty"`
	ClientID        string `json:"client_id,omitempty"`
	Scope           string `json:"scope,omitempty"`
	ObtainedAt      string `json:"obtained_at,omitempty"`
}

func newAuthCmd() *cobra.Command {
	profile := product.Current()
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate and inspect " + profile.DisplayName + " Signin status",
	}
	cmd.AddCommand(newAuthLoginCmd(), newAuthRefreshCmd(), newAuthStatusCmd(), newAuthLogoutCmd())
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"Before Console/OpenAPI-backed commands which need a Signin token"},
		Examples: []string{meta.BinName + " auth login", meta.BinName + " auth status"},
	})
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	profile := product.Current()
	var (
		browserMode     string
		credentialStore string
		projectAuth     bool
		force           bool
		timeout         time.Duration
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in with " + profile.DisplayName + " Signin using Authorization Code + PKCE",
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateAuthBrowserMode(browserMode); err != nil {
				return err
			}
			if timeout <= 0 {
				return errs.New("byteplus-rtc.cli.invalid_flag", errs.TypeValidation,
					"authorization timeout must be greater than zero").WithParam("--timeout")
			}
			if credentialStore != "" {
				if err := auth.ValidateStoreMode(credentialStore); err != nil {
					return err
				}
			}
			var projectSetup *projectAuthSetup
			if projectAuth {
				if credentialStore != "" && credentialStore != auth.StoreFile {
					return errs.New("byteplus-rtc.cli.invalid_flag", errs.TypeValidation, "--project-auth requires --store=file")
				}
				credentialStore = auth.StoreFile
				var err error
				projectSetup, err = prepareProjectAuth(flagDryRun)
				if err != nil {
					return err
				}
			}
			if flagDryRun {
				return out().Data(map[string]any{"status": "would_login", "project_auth": projectAuth})
			}
			if projectSetup != nil {
				defer auth.SetFileTokenStoreForCommand(projectSetup.tokenPath)()
			} else if credentialStore != "" {
				restoreStore, err := auth.SetTokenStoreModeForCommand(credentialStore)
				if err != nil {
					return err
				}
				defer restoreStore()
			}
			if !force {
				token, reusable, refreshed, err := reusableAuthLogin(c.Context())
				if err != nil {
					return err
				}
				if reusable {
					if credentialStore != "" {
						if err := auth.PersistStoreMode(credentialStore); err != nil {
							return err
						}
					}
					if projectSetup != nil {
						if err := projectSetup.commit(); err != nil {
							return err
						}
					}
					if refreshed {
						out().Progress("refreshed existing Signin login in %s", auth.TokenLocation())
					} else {
						out().Progress("already logged in; reusing token in %s", auth.TokenLocation())
					}
					return out().Data(newAuthLoginResult(token))
				}
			}
			_, err := resolveAuthBrowserMode(c, browserMode)
			if err != nil {
				return err
			}
			if err := auth.PreflightTokenStore(); err != nil {
				return err
			}
			clientID := profile.OAuthLocalClientID
			verifier, challenge, err := auth.NewPKCEPair()
			if err != nil {
				return errs.Wrap(err, "byteplus-rtc.cli.internal", "generate PKCE pair: %s", err)
			}
			state, err := authRandomURLSafe(32)
			if err != nil {
				return errs.Wrap(err, "byteplus-rtc.cli.internal", "generate OAuth state: %s", err)
			}
			nonce, err := authRandomURLSafe(16)
			if err != nil {
				return errs.Wrap(err, "byteplus-rtc.cli.internal", "generate OAuth nonce: %s", err)
			}

			callbackServer, err := auth.StartCallbackServer(auth.DefaultRedirectURI)
			if err != nil {
				return err
			}
			redirectURI := callbackServer.RedirectURI
			defer func() {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = callbackServer.Shutdown(shutdownCtx)
			}()

			loginURL, err := auth.BuildAuthorizeURL(auth.AuthorizeParams{
				AuthorizeEndpoint: oauthAuthorizeEndpoint,
				ClientID:          clientID,
				RedirectURI:       redirectURI,
				Scope:             profile.OAuthScope,
				State:             state,
				Nonce:             nonce,
				CodeChallenge:     challenge,
			})
			if err != nil {
				return err
			}

			if err := authOpenBrowser(loginURL); err != nil {
				out().Warn("unable to open browser automatically: %s", err)
				out().Progress("open this URL manually:\n%s", loginURL)
			} else {
				out().Progress("browser opened for %s Signin login", profile.DisplayName)
			}
			waitCtx, cancel := context.WithTimeout(c.Context(), timeout)
			defer cancel()
			var callback auth.CallbackResult
			select {
			case callback = <-callbackServer.Results:
			case <-waitCtx.Done():
				return errs.New("byteplus-rtc.auth.authorization_failed", errs.TypeAuth,
					"login timed out waiting for callback on %s", callbackServer.RedirectURI)
			}
			if err := auth.CallbackAuthorizationError(callback); err != nil {
				return err
			}
			if callback.Code == "" {
				return errs.New("byteplus-rtc.auth.authorization_failed", errs.TypeAuth,
					"authorization callback did not include code")
			}
			if callback.State != state {
				return errs.New("byteplus-rtc.auth.authorization_failed", errs.TypeAuth,
					"authorization state mismatch")
			}
			authorizationCode := callback.Code
			exchangeCtx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
			defer cancel()
			token, err := auth.ExchangeCodeWithScope(exchangeCtx, authHTTPClient, oauthTokenEndpoint, clientID, redirectURI, authorizationCode, verifier, profile.OAuthScope)
			if err != nil {
				return err
			}
			if err := auth.SaveToken(token); err != nil {
				return err
			}
			if projectSetup != nil {
				if err := projectSetup.commit(); err != nil {
					return err
				}
			}
			out().Progress("logged in; token stored in %s", auth.TokenLocation())
			if force {
				out().Progress("after changing accounts, run `%s agent resources --write`, stop the running dev process, and run `%s dev --reconfigure` to refresh RTC credentials; exported access keys remain in effect", meta.BinName, meta.BinName)
			}

			return out().Data(newAuthLoginResult(token))
		},
	}
	cmd.Flags().StringVar(&browserMode, "browser", authBrowserAsk, "authorization mode: ask or open")
	cmd.Flags().StringVar(&credentialStore, "store", "", "credential store: file or keyring (default: persisted choice, initially file)")
	cmd.Flags().BoolVar(&projectAuth, "project-auth", false, "store credentials in the project and remember the choice")
	cmd.Flags().BoolVar(&force, "force", false, "reauthorize even when a usable Signin token is stored")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "time to wait for browser authorization")
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"Establishing Signin credentials for Console/OpenAPI-backed commands"},
		Avoid:    []string{"Offline-only flows such as token check"},
		Prereq:   []string{"An interactive terminal or an explicit --browser=open choice"},
		Examples: []string{meta.BinName + " auth login", meta.BinName + " auth login --store=keyring --browser=open"},
	})
	return cmd
}

func validateAuthBrowserMode(mode string) error {
	switch mode {
	case authBrowserAsk, authBrowserOpen:
		return nil
	default:
		return errs.New("byteplus-rtc.cli.invalid_flag", errs.TypeValidation,
			"invalid browser mode %q", mode).WithParam("--browser").
			WithHint("use --browser=ask or --browser=open")
	}
}

func resolveAuthBrowserMode(cmd *cobra.Command, mode string) (string, error) {
	if mode != authBrowserAsk {
		return mode, nil
	}
	if !authIsTerminal(cmd.InOrStdin()) {
		return "", errs.New("byteplus-rtc.auth.interaction_required", errs.TypePrecondition,
			"Signin authorization requires an explicit browser mode in a non-interactive terminal").
			WithParam("--browser").
			WithHint("rerun with `%s auth login --browser=open` after user consent", meta.BinName)
	}
	out().Progress("Signin requires authorization and may store refreshable credentials in %s", auth.TokenLocation())
	out().Progress("choose login mode: [o] open browser, [c] cancel")
	line, err := readInputLine(cmd.InOrStdin())
	if err != nil {
		return "", errs.Wrap(err, "byteplus-rtc.cli.internal", "read login choice: %s", err)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "o", "open":
		return authBrowserOpen, nil
	case "c", "cancel", "":
		return "", errs.New("byteplus-rtc.auth.authorization_failed", errs.TypeAuth, "Signin authorization canceled")
	default:
		return "", errs.New("byteplus-rtc.auth.authorization_failed", errs.TypeAuth,
			"unknown login choice %q", strings.TrimSpace(line)).
			WithHint("choose o or c")
	}
}

func reusableAuthLogin(ctx context.Context) (auth.TokenSet, bool, bool, error) {
	stored, err := auth.LoadToken()
	if errors.Is(err, os.ErrNotExist) {
		return auth.TokenSet{}, false, false, nil
	}
	if err != nil {
		if typed, ok := errs.As(err); ok && typed.Code == "byteplus-rtc.auth.storage_unavailable" {
			// Do not reuse credentials from unavailable or unsafe storage, but let
			// login continue so SaveToken can repair a recoverable auth file.
			return auth.TokenSet{}, false, false, nil
		}
		return auth.TokenSet{}, false, false, err
	}
	if strings.TrimSpace(stored.AccessToken) == "" {
		return auth.TokenSet{}, false, false, nil
	}
	refreshSkew := currentAuthRefreshSkew()
	if !stored.Expired(time.Now(), refreshSkew) {
		return stored, true, false, nil
	}

	refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	refreshed, didRefresh, err := auth.EnsureFreshTokenWithScope(refreshCtx, authHTTPClient, refreshSkew, oauthTokenEndpoint, product.Current().OAuthScope)
	if err == nil {
		return refreshed, true, didRefresh, nil
	}
	if typed, ok := errs.As(err); ok && typed.Code == "byteplus-rtc.auth.storage_unavailable" {
		return auth.TokenSet{}, false, false, err
	}
	return auth.TokenSet{}, false, false, nil
}

func newAuthLoginResult(token auth.TokenSet) authLoginResult {
	return authLoginResult{
		Authenticated:   true,
		TokenLocation:   auth.TokenLocation(),
		ExpiresAt:       token.ExpiresAt,
		HasRefreshToken: token.RefreshToken != "",
		ClientID:        token.ClientID,
		Scope:           token.Scope,
	}
}

func newAuthRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Refresh the stored Signin token",
		RunE: func(c *cobra.Command, args []string) error {
			stored, err := auth.LoadToken()
			if errors.Is(err, os.ErrNotExist) {
				return errs.New("byteplus-rtc.auth.not_authenticated", errs.TypeAuth,
					"not logged in").WithHint("run `%s auth login` first", meta.BinName)
			}
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
			defer cancel()
			refreshed, err := auth.RefreshTokenWithScope(ctx, authHTTPClient, oauthTokenEndpoint, stored, product.Current().OAuthScope)
			if err != nil {
				return err
			}
			if err := auth.SaveToken(refreshed); err != nil {
				return err
			}
			out().Progress("refreshed token in %s", auth.TokenLocation())
			return out().Data(authLoginResult{
				Authenticated:   true,
				TokenLocation:   auth.TokenLocation(),
				ExpiresAt:       refreshed.ExpiresAt,
				HasRefreshToken: refreshed.RefreshToken != "",
				ClientID:        refreshed.ClientID,
				Scope:           refreshed.Scope,
			})
		},
	}
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current Signin authentication status",
		RunE: func(c *cobra.Command, args []string) error {
			_, err := auth.LoadToken()
			if errors.Is(err, os.ErrNotExist) {
				return out().Data(authStatusResult{Authenticated: false, TokenLocation: auth.TokenLocation()})
			}
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
			defer cancel()
			token, refreshed, err := auth.EnsureFreshTokenWithScope(ctx, authHTTPClient, currentAuthRefreshSkew(), oauthTokenEndpoint, product.Current().OAuthScope)
			if err != nil {
				return err
			}
			return out().Data(authStatusResult{
				Authenticated:   true,
				TokenLocation:   auth.TokenLocation(),
				ExpiresAt:       token.ExpiresAt,
				Expired:         token.Expired(time.Now(), 0),
				Refreshed:       refreshed,
				HasAccessToken:  token.AccessToken != "",
				HasRefreshToken: token.RefreshToken != "",
				TokenType:       token.TokenType,
				ClientID:        token.ClientID,
				Scope:           token.Scope,
				ObtainedAt:      token.ObtainedAt,
			})
		},
	}
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear stored Signin tokens",
		RunE: func(c *cobra.Command, args []string) error {
			if err := auth.DeleteToken(); err != nil {
				return err
			}
			return out().Data(map[string]any{"authenticated": false, "token_location": auth.TokenLocation()})
		},
	}
}

func readInputLine(reader io.Reader) (string, error) {
	var value strings.Builder
	var one [1]byte
	for {
		n, err := reader.Read(one[:])
		if n == 1 {
			if one[0] == '\n' {
				return value.String(), nil
			}
			if one[0] != '\r' {
				_ = value.WriteByte(one[0])
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && value.Len() > 0 {
				return value.String(), nil
			}
			return value.String(), err
		}
		if n == 0 {
			return value.String(), fmt.Errorf("reader returned no data")
		}
	}
}
