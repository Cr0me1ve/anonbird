package rest

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/netbirdio/netbird/shared/management/http/api"
)

// UsersAPI APIs for users, do not use directly
type UsersAPI struct {
	c *Client
}

// List list all users, only returns one user always
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) List(ctx context.Context) ([]api.User, error) {
	resp, err := a.c.NewRequest(ctx, "GET", "/api/users", nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[[]api.User](resp)
	return ret, err
}

// Create create user
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) Create(ctx context.Context, request api.PostApiUsersJSONRequestBody) (*api.User, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := a.c.NewRequest(ctx, "POST", "/api/users", bytes.NewReader(requestBytes), nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.User](resp)
	return &ret, err
}

// Update update user settings
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) Update(ctx context.Context, userID string, request api.PutApiUsersUserIdJSONRequestBody) (*api.User, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := a.c.NewRequest(ctx, "PUT", "/api/users/"+userID, bytes.NewReader(requestBytes), nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.User](resp)
	return &ret, err
}

// Delete delete user
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) Delete(ctx context.Context, userID string) error {
	resp, err := a.c.NewRequest(ctx, "DELETE", "/api/users/"+userID, nil, nil)
	if err != nil {
		return err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	return nil
}

// ResendInvitation resend user invitation
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) ResendInvitation(ctx context.Context, userID string) error {
	resp, err := a.c.NewRequest(ctx, "POST", "/api/users/"+userID+"/invite", nil, nil)
	if err != nil {
		return err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	return nil
}

// Current gets the current user info
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) Current(ctx context.Context) (*api.User, error) {
	resp, err := a.c.NewRequest(ctx, "GET", "/api/users/current", nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	ret, err := parseResponse[api.User](resp)
	return &ret, err
}

// ListInvites list all user invites
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) ListInvites(ctx context.Context) ([]api.UserInvite, error) {
	resp, err := a.c.NewRequest(ctx, "GET", "/api/users/invites", nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[[]api.UserInvite](resp)
	return ret, err
}

// CreateInvite create a user invite
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) CreateInvite(ctx context.Context, request api.PostApiUsersInvitesJSONRequestBody) (*api.UserInvite, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := a.c.NewRequest(ctx, "POST", "/api/users/invites", bytes.NewReader(requestBytes), nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.UserInvite](resp)
	return &ret, err
}

// DeleteInvite delete a user invite
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) DeleteInvite(ctx context.Context, inviteID string) error {
	resp, err := a.c.NewRequest(ctx, "DELETE", "/api/users/invites/"+inviteID, nil, nil)
	if err != nil {
		return err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	return nil
}

// RegenerateInvite regenerate a user invite token
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) RegenerateInvite(ctx context.Context, inviteID string, request api.PostApiUsersInvitesInviteIdRegenerateJSONRequestBody) (*api.UserInviteRegenerateResponse, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := a.c.NewRequest(ctx, "POST", "/api/users/invites/"+inviteID+"/regenerate", bytes.NewReader(requestBytes), nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.UserInviteRegenerateResponse](resp)
	return &ret, err
}

// GetInviteByToken get a user invite by token
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) GetInviteByToken(ctx context.Context, token string) (*api.UserInviteInfo, error) {
	resp, err := a.c.NewRequest(ctx, "GET", "/api/users/invites/"+token, nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.UserInviteInfo](resp)
	return &ret, err
}

// AcceptInvite accept a user invite
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) AcceptInvite(ctx context.Context, token string, request api.PostApiUsersInvitesTokenAcceptJSONRequestBody) (*api.UserInviteAcceptResponse, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := a.c.NewRequest(ctx, "POST", "/api/users/invites/"+token+"/accept", bytes.NewReader(requestBytes), nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.UserInviteAcceptResponse](resp)
	return &ret, err
}

// Approve approve a pending user
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) Approve(ctx context.Context, userID string) (*api.User, error) {
	resp, err := a.c.NewRequest(ctx, "POST", "/api/users/"+userID+"/approve", nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	ret, err := parseResponse[api.User](resp)
	return &ret, err
}

// ChangePassword change a user's password
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) ChangePassword(ctx context.Context, userID string, request api.PutApiUsersUserIdPasswordJSONRequestBody) error {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}
	resp, err := a.c.NewRequest(ctx, "PUT", "/api/users/"+userID+"/password", bytes.NewReader(requestBytes), nil)
	if err != nil {
		return err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	return nil
}

// Reject reject a pending user
// See more: https://github.com/Cr0me1ve/anonbird/tree/main/docs
func (a *UsersAPI) Reject(ctx context.Context, userID string) error {
	resp, err := a.c.NewRequest(ctx, "DELETE", "/api/users/"+userID+"/reject", nil, nil)
	if err != nil {
		return err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	return nil
}
