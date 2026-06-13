package cloudaccount

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	envAPIURL        = "ANONBIRD_CLOUD_API_URL"
	envInternalToken = "ANONBIRD_CLOUD_INTERNAL_TOKEN"
	envTimeout       = "ANONBIRD_CLOUD_ACCOUNT_TIMEOUT"
)

type Principal struct {
	AccountID string `json:"account_id"`
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Domain    string `json:"domain"`
}

type Resolver interface {
	Resolve(ctx context.Context, subject, email string, emailVerified bool) (Principal, error)
}

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewFromEnv() (Resolver, error) {
	baseURL := strings.TrimSpace(os.Getenv(envAPIURL))
	token := strings.TrimSpace(os.Getenv(envInternalToken))
	if baseURL == "" && token == "" {
		return nil, nil
	}
	if baseURL == "" || token == "" {
		return nil, fmt.Errorf("%s and %s must both be set for cloud account resolution", envAPIURL, envInternalToken)
	}

	timeout := 2 * time.Second
	if rawTimeout := strings.TrimSpace(os.Getenv(envTimeout)); rawTimeout != "" {
		parsed, err := time.ParseDuration(rawTimeout)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", envTimeout, err)
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("%s must be positive", envTimeout)
		}
		timeout = parsed
	}

	return NewClient(baseURL, token, &http.Client{Timeout: timeout})
}

func NewClient(baseURL, token string, httpClient *http.Client) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	token = strings.TrimSpace(token)
	if baseURL == "" || token == "" {
		return nil, errors.New("cloud account base URL and token are required")
	}
	if len(token) < 32 {
		return nil, errors.New("cloud account internal token must be at least 32 characters")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if parsed.Hostname() == "" {
		return nil, errors.New("cloud account URL host is required")
	}
	if parsed.Scheme != "https" {
		if parsed.Scheme != "http" || !isPrivateHTTPHost(parsed.Hostname()) {
			return nil, errors.New("cloud account URL must use https outside localhost, private addresses, or private service names")
		}
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 2 * time.Second}
	}
	return &Client{baseURL: baseURL, token: token, httpClient: httpClient}, nil
}

func isPrivateHTTPHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast()
	}
	return !strings.Contains(host, ".")
}

func (c *Client) Resolve(ctx context.Context, subject, email string, emailVerified bool) (Principal, error) {
	subject = strings.TrimSpace(subject)
	email = strings.TrimSpace(email)
	if subject == "" || email == "" {
		return Principal{}, errors.New("cloud account subject and email are required")
	}
	if !emailVerified {
		return Principal{}, errors.New("cloud account email must be verified")
	}

	payload := struct {
		Subject       string `json:"subject"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}{
		Subject:       subject,
		Email:         email,
		EmailVerified: emailVerified,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Principal{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/accounts/resolve", bytes.NewReader(body))
	if err != nil {
		return Principal{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-AnonBird-Cloud-Token", c.token)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return Principal{}, err
	}
	defer response.Body.Close()

	var principal Principal
	if err := json.NewDecoder(response.Body).Decode(&principal); err != nil {
		return Principal{}, err
	}
	if response.StatusCode != http.StatusOK {
		return Principal{}, fmt.Errorf("cloud account API returned HTTP %d", response.StatusCode)
	}
	if principal.AccountID == "" || principal.Domain == "" {
		return Principal{}, errors.New("cloud account API returned incomplete principal")
	}
	return principal, nil
}
