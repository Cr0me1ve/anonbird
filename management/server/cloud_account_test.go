package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/management/server/cloudaccount"
	"github.com/netbirdio/netbird/management/server/cloudquota"
	"github.com/netbirdio/netbird/management/server/store"
	"github.com/netbirdio/netbird/shared/auth"
	"github.com/netbirdio/netbird/shared/management/status"
)

type cloudResolveCall struct {
	subject       string
	email         string
	emailVerified bool
}

type recordingCloudAccountResolver struct {
	calls     []cloudResolveCall
	principal cloudaccount.Principal
	err       error
}

func (r *recordingCloudAccountResolver) Resolve(_ context.Context, subject, email string, emailVerified bool) (cloudaccount.Principal, error) {
	r.calls = append(r.calls, cloudResolveCall{subject: subject, email: email, emailVerified: emailVerified})
	if r.err != nil {
		return cloudaccount.Principal{}, r.err
	}
	return r.principal, nil
}

func TestCloudAccountResolverCreatesManagementAccountWithCloudID(t *testing.T) {
	manager, _, err := createManager(t)
	require.NoError(t, err)

	resolver := &recordingCloudAccountResolver{
		principal: cloudaccount.Principal{
			AccountID: "acc_cloud",
			UserID:    "usr_cloud",
			Email:     "owner@example.com",
			Role:      "owner",
			Domain:    "acc-cloud.accounts.anonbird.cloud",
		},
	}
	manager.cloudAccount = resolver

	accountID, userID, err := manager.GetAccountIDFromUserAuth(context.Background(), auth.UserAuth{
		AccountId:      "forged-account-claim",
		UserId:         "oidc-subject",
		Email:          "owner@example.com",
		EmailVerified:  true,
		Name:           "Owner User",
		Domain:         "evil.example.com",
		DomainCategory: "private",
	})
	require.NoError(t, err)
	assert.Equal(t, "acc_cloud", accountID)
	assert.Equal(t, "oidc-subject", userID)
	require.Len(t, resolver.calls, 1)
	assert.Equal(t, "oidc-subject", resolver.calls[0].subject)
	assert.Equal(t, "owner@example.com", resolver.calls[0].email)
	assert.True(t, resolver.calls[0].emailVerified)

	account, err := manager.Store.GetAccount(context.Background(), "acc_cloud")
	require.NoError(t, err)
	assert.Equal(t, "acc_cloud", account.Id)
	assert.Equal(t, "acc-cloud.accounts.anonbird.cloud", account.Domain)
	assert.Equal(t, "private", account.DomainCategory)
	assert.True(t, account.IsDomainPrimaryAccount)
	require.Contains(t, account.Users, "oidc-subject")
	assert.Equal(t, "owner@example.com", account.Users["oidc-subject"].Email)
}

func TestCloudAccountResolverFailsClosed(t *testing.T) {
	manager, _, err := createManager(t)
	require.NoError(t, err)
	manager.cloudAccount = &recordingCloudAccountResolver{err: errors.New("cloud api unavailable")}

	_, _, err = manager.GetAccountIDFromUserAuth(context.Background(), auth.UserAuth{
		UserId:        "oidc-subject",
		Email:         "owner@example.com",
		EmailVerified: true,
	})
	require.Error(t, err)
	sErr, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, status.Unauthorized, sErr.Type())
	assert.Empty(t, manager.Store.GetAllAccounts(context.Background()))
}

func TestCloudAccountResolverRequiresVerifiedEmail(t *testing.T) {
	manager, _, err := createManager(t)
	require.NoError(t, err)
	resolver := &recordingCloudAccountResolver{
		principal: cloudaccount.Principal{
			AccountID: "acc_cloud",
			UserID:    "usr_cloud",
			Email:     "owner@example.com",
			Role:      "owner",
			Domain:    "acc-cloud.accounts.anonbird.cloud",
		},
	}
	manager.cloudAccount = resolver

	_, _, err = manager.GetAccountIDFromUserAuth(context.Background(), auth.UserAuth{
		UserId:        "oidc-subject",
		Email:         "owner@example.com",
		EmailVerified: false,
	})
	require.Error(t, err)
	require.Empty(t, resolver.calls)
	assert.Empty(t, manager.Store.GetAllAccounts(context.Background()))
}

func TestCloudAccountExistingTenantEnforcesUserQuota(t *testing.T) {
	manager, _, err := createManager(t)
	require.NoError(t, err)
	manager.cloudAccount = &recordingCloudAccountResolver{
		principal: cloudaccount.Principal{
			AccountID: "acc_cloud",
			UserID:    "usr_cloud",
			Email:     "owner@example.com",
			Role:      "owner",
			Domain:    "acc-cloud.accounts.anonbird.cloud",
		},
	}

	_, _, err = manager.GetAccountIDFromUserAuth(context.Background(), auth.UserAuth{
		UserId:        "owner-subject",
		Email:         "owner@example.com",
		EmailVerified: true,
	})
	require.NoError(t, err)

	decision := cloudquota.Decision{
		Allowed:  false,
		Resource: cloudquota.ResourceUsers,
		PlanID:   "free",
		Limit:    1,
		Current:  1,
		Delta:    1,
	}
	manager.cloudQuota = &recordingQuotaChecker{
		decision: decision,
		err:      cloudquota.ErrLimitExceeded{Decision: decision},
	}

	_, _, err = manager.GetAccountIDFromUserAuth(context.Background(), auth.UserAuth{
		UserId:        "second-subject",
		Email:         "second@example.com",
		EmailVerified: true,
	})
	require.Error(t, err)
	sErr, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, status.PreconditionFailed, sErr.Type())

	_, err = manager.Store.GetUserByUserID(context.Background(), store.LockingStrengthNone, "second-subject")
	require.Error(t, err)
	sErr, ok = status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, status.NotFound, sErr.Type())
}
