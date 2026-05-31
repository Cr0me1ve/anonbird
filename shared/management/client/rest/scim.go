package rest

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/netbirdio/netbird/shared/management/http/api"
)

// SCIMAPI APIs for SCIM IDP integrations
type SCIMAPI struct {
	c *Client
}

// List retrieves all SCIM IDP integrations
// See more: https://github.com/Cr0me1ve/netbird/tree/main/docs
func (a *SCIMAPI) List(ctx context.Context) ([]api.ScimIntegration, error) {
	resp, err := a.c.NewRequest(ctx, "GET", "/api/integrations/scim-idp", nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[[]api.ScimIntegration](resp)
	return ret, err
}

// Get retrieves a specific SCIM IDP integration by ID
// See more: https://github.com/Cr0me1ve/netbird/tree/main/docs
func (a *SCIMAPI) Get(ctx context.Context, integrationID string) (*api.ScimIntegration, error) {
	resp, err := a.c.NewRequest(ctx, "GET", "/api/integrations/scim-idp/"+integrationID, nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.ScimIntegration](resp)
	return &ret, err
}

// Create creates a new SCIM IDP integration
// See more: https://github.com/Cr0me1ve/netbird/tree/main/docs
func (a *SCIMAPI) Create(ctx context.Context, request api.CreateScimIntegrationRequest) (*api.ScimIntegration, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := a.c.NewRequest(ctx, "POST", "/api/integrations/scim-idp", bytes.NewReader(requestBytes), nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.ScimIntegration](resp)
	return &ret, err
}

// Update updates an existing SCIM IDP integration
// See more: https://github.com/Cr0me1ve/netbird/tree/main/docs
func (a *SCIMAPI) Update(ctx context.Context, integrationID string, request api.UpdateScimIntegrationRequest) (*api.ScimIntegration, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := a.c.NewRequest(ctx, "PUT", "/api/integrations/scim-idp/"+integrationID, bytes.NewReader(requestBytes), nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.ScimIntegration](resp)
	return &ret, err
}

// Delete deletes a SCIM IDP integration
// See more: https://github.com/Cr0me1ve/netbird/tree/main/docs
func (a *SCIMAPI) Delete(ctx context.Context, integrationID string) error {
	resp, err := a.c.NewRequest(ctx, "DELETE", "/api/integrations/scim-idp/"+integrationID, nil, nil)
	if err != nil {
		return err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	return nil
}

// RegenerateToken regenerates the SCIM API token for an integration
// See more: https://github.com/Cr0me1ve/netbird/tree/main/docs
func (a *SCIMAPI) RegenerateToken(ctx context.Context, integrationID string) (*api.ScimTokenResponse, error) {
	resp, err := a.c.NewRequest(ctx, "POST", "/api/integrations/scim-idp/"+integrationID+"/token", nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.ScimTokenResponse](resp)
	return &ret, err
}

// GetLogs retrieves synchronization logs for an SCIM IDP integration
// See more: https://github.com/Cr0me1ve/netbird/tree/main/docs
func (a *SCIMAPI) GetLogs(ctx context.Context, integrationID string) ([]api.IdpIntegrationSyncLog, error) {
	resp, err := a.c.NewRequest(ctx, "GET", "/api/integrations/scim-idp/"+integrationID+"/logs", nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[[]api.IdpIntegrationSyncLog](resp)
	return ret, err
}
