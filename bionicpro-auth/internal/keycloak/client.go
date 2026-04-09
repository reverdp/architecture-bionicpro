package keycloak

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bionicpro-auth/internal/config"
)

type Client struct {
	httpClient *http.Client
	cfg        config.Config
}

type TokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	IDToken          string `json:"id_token"`
	SessionState     string `json:"session_state"`
	Scope            string `json:"scope"`
}

type TokenClaims struct {
	Sub               string   `json:"sub"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	GivenName         string   `json:"given_name"`
	FamilyName        string   `json:"family_name"`
	Email             string   `json:"email"`
	EmailVerified     bool     `json:"email_verified"`
	Scope             string   `json:"scope"`
	Audience          any      `json:"aud"`
	AllowedOrigins    []string `json:"allowed-origins"`
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		cfg:        cfg,
	}
}

func (c *Client) BuildAuthorizationURL(state, codeChallenge, identityProvider string) string {
	values := url.Values{}
	values.Set("client_id", c.cfg.ClientID)
	values.Set("response_type", "code")
	values.Set("scope", c.cfg.Scopes)
	values.Set("redirect_uri", c.cfg.RedirectURL)
	values.Set("state", state)
	values.Set("code_challenge", codeChallenge)
	values.Set("code_challenge_method", "S256")
	if identityProvider != "" {
		values.Set("kc_idp_hint", identityProvider)
	}

	return fmt.Sprintf("%s/realms/%s/protocol/openid-connect/auth?%s", strings.TrimRight(c.cfg.KeycloakPublic, "/"), c.cfg.Realm, values.Encode())
}

func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (c *Client) ExchangeCode(ctx context.Context, code, verifier string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", c.cfg.RedirectURL)
	form.Set("code_verifier", verifier)

	return c.tokenRequest(ctx, form)
}

func (c *Client) Refresh(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)
	form.Set("refresh_token", refreshToken)

	return c.tokenRequest(ctx, form)
}

func (c *Client) Logout(ctx context.Context, refreshToken string) error {
	form := url.Values{}
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)
	form.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.logoutEndpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create logout request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("logout request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("logout failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func ParseSubject(accessToken string) string {
	claims, err := ParseClaims(accessToken)
	if err != nil {
		return ""
	}
	return claims.Sub
}

func ParseClaims(accessToken string) (*TokenClaims, error) {
	parts := strings.Split(accessToken, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid token format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode token payload: %w", err)
	}

	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("decode token claims: %w", err)
	}
	return &claims, nil
}

func (c *Client) tokenRequest(ctx context.Context, form url.Values) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenEndpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	return &tokenResp, nil
}

func (c *Client) tokenEndpoint() string {
	return fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", strings.TrimRight(c.cfg.KeycloakInternal, "/"), c.cfg.Realm)
}

func (c *Client) logoutEndpoint() string {
	return fmt.Sprintf("%s/realms/%s/protocol/openid-connect/logout", strings.TrimRight(c.cfg.KeycloakInternal, "/"), c.cfg.Realm)
}
