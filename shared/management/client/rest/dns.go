package rest

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/netbirdio/netbird/shared/management/http/api"
)

// DNSAPI APIs for DNS Management, do not use directly
type DNSAPI struct {
	c *Client
}

// ListNameserverGroups list all nameserver groups
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *DNSAPI) ListNameserverGroups(ctx context.Context) ([]api.NameserverGroup, error) {
	resp, err := a.c.NewRequest(ctx, "GET", "/api/dns/nameservers", nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[[]api.NameserverGroup](resp)
	return ret, err
}

// GetNameserverGroup get nameserver group info
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *DNSAPI) GetNameserverGroup(ctx context.Context, nameserverGroupID string) (*api.NameserverGroup, error) {
	resp, err := a.c.NewRequest(ctx, "GET", "/api/dns/nameservers/"+nameserverGroupID, nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.NameserverGroup](resp)
	return &ret, err
}

// CreateNameserverGroup create new nameserver group
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *DNSAPI) CreateNameserverGroup(ctx context.Context, request api.PostApiDnsNameserversJSONRequestBody) (*api.NameserverGroup, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := a.c.NewRequest(ctx, "POST", "/api/dns/nameservers", bytes.NewReader(requestBytes), nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.NameserverGroup](resp)
	return &ret, err
}

// UpdateNameserverGroup update nameserver group info
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *DNSAPI) UpdateNameserverGroup(ctx context.Context, nameserverGroupID string, request api.PutApiDnsNameserversNsgroupIdJSONRequestBody) (*api.NameserverGroup, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := a.c.NewRequest(ctx, "PUT", "/api/dns/nameservers/"+nameserverGroupID, bytes.NewReader(requestBytes), nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.NameserverGroup](resp)
	return &ret, err
}

// DeleteNameserverGroup delete nameserver group
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *DNSAPI) DeleteNameserverGroup(ctx context.Context, nameserverGroupID string) error {
	resp, err := a.c.NewRequest(ctx, "DELETE", "/api/dns/nameservers/"+nameserverGroupID, nil, nil)
	if err != nil {
		return err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	return nil
}

// GetSettings get DNS settings
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *DNSAPI) GetSettings(ctx context.Context) (*api.DNSSettings, error) {
	resp, err := a.c.NewRequest(ctx, "GET", "/api/dns/settings", nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.DNSSettings](resp)
	return &ret, err
}

// UpdateSettings update DNS settings
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *DNSAPI) UpdateSettings(ctx context.Context, request api.PutApiDnsSettingsJSONRequestBody) (*api.DNSSettings, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := a.c.NewRequest(ctx, "PUT", "/api/dns/settings", bytes.NewReader(requestBytes), nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.DNSSettings](resp)
	return &ret, err
}
