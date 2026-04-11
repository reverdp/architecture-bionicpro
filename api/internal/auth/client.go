package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"reports-api/internal/config"
)

const internalSecretHeader = "X-Internal-Auth"

type Client struct {
	cfg        config.Config
	httpClient *http.Client
}

type SessionIdentity struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
	Subject       string `json:"subject"`
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
	}
}

func (c *Client) ResolveSession(ctx context.Context, sessionID string) (*SessionIdentity, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("session cookie is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpointURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("create auth resolve request: %w", err)
	}

	req.Header.Set(internalSecretHeader, c.cfg.AuthInternalSecret)
	req.AddCookie(&http.Cookie{
		Name:  c.cfg.AuthSessionCookie,
		Value: sessionID,
		Path:  "/",
	})

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("resolve auth session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("resolve auth session failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var identity SessionIdentity
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&identity); err != nil {
		return nil, fmt.Errorf("decode auth resolve response: %w", err)
	}
	if !identity.Authenticated || identity.Username == "" {
		return nil, fmt.Errorf("session is not authenticated")
	}

	return &identity, nil
}

func (c *Client) endpointURL() string {
	base := strings.TrimRight(c.cfg.AuthInternalURL, "/")
	u, _ := url.Parse(base + "/internal/session/resolve")
	return u.String()
}
