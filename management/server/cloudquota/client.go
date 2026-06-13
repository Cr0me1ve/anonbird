package cloudquota

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
	"strconv"
	"strings"
	"time"
)

const (
	envAPIURL        = "ANONBIRD_CLOUD_API_URL"
	envInternalToken = "ANONBIRD_CLOUD_INTERNAL_TOKEN"
	envTimeout       = "ANONBIRD_CLOUD_QUOTA_TIMEOUT"
)

type Resource string

const (
	ResourceUsers Resource = "users"
	ResourcePeers Resource = "peers"
)

type Decision struct {
	Allowed  bool     `json:"allowed"`
	Resource Resource `json:"resource"`
	PlanID   string   `json:"plan_id"`
	Limit    int      `json:"limit"`
	Current  int      `json:"current"`
	Delta    int      `json:"delta"`
}

type Checker interface {
	Check(ctx context.Context, accountID string, resource Resource, current, delta int) (Decision, error)
}

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewFromEnv() (Checker, error) {
	baseURL := strings.TrimSpace(os.Getenv(envAPIURL))
	token := strings.TrimSpace(os.Getenv(envInternalToken))
	if baseURL == "" && token == "" {
		return nil, nil
	}
	if baseURL == "" || token == "" {
		return nil, fmt.Errorf("%s and %s must both be set for cloud quota enforcement", envAPIURL, envInternalToken)
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
		return nil, errors.New("cloud quota base URL and token are required")
	}
	if len(token) < 32 {
		return nil, errors.New("cloud quota internal token must be at least 32 characters")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "https" {
		if parsed.Scheme != "http" || !isPrivateHTTPHost(parsed.Hostname()) {
			return nil, errors.New("cloud quota URL must use https outside localhost, private addresses, or private service names")
		}
	}
	if parsed.Hostname() == "" {
		return nil, errors.New("cloud quota URL host is required")
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

func (c *Client) Check(ctx context.Context, accountID string, resource Resource, current, delta int) (Decision, error) {
	payload := map[string]any{
		"account_id": accountID,
		"resource":   resource,
		"current":    current,
		"delta":      delta,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Decision{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/quota/check", bytes.NewReader(body))
	if err != nil {
		return Decision{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-AnonBird-Cloud-Token", c.token)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return Decision{}, err
	}
	defer response.Body.Close()

	var decision Decision
	if err := json.NewDecoder(response.Body).Decode(&decision); err != nil {
		return Decision{}, err
	}

	switch response.StatusCode {
	case http.StatusOK:
		if !decision.Allowed {
			return decision, errors.New("cloud quota API returned allow=false with HTTP 200")
		}
		return decision, nil
	case http.StatusPaymentRequired:
		return decision, ErrLimitExceeded{Decision: decision}
	default:
		return decision, fmt.Errorf("cloud quota API returned HTTP %s", strconv.Itoa(response.StatusCode))
	}
}

type ErrLimitExceeded struct {
	Decision Decision
}

func (e ErrLimitExceeded) Error() string {
	return fmt.Sprintf("%s limit exceeded for plan %s: current=%d delta=%d limit=%d",
		e.Decision.Resource, e.Decision.PlanID, e.Decision.Current, e.Decision.Delta, e.Decision.Limit)
}
