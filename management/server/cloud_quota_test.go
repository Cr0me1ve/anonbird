package server

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/management/server/cloudquota"
	nbpeer "github.com/netbirdio/netbird/management/server/peer"
	"github.com/netbirdio/netbird/management/server/store"
	"github.com/netbirdio/netbird/management/server/types"
	"github.com/netbirdio/netbird/shared/management/status"
)

type quotaCheckCall struct {
	accountID string
	resource  cloudquota.Resource
	current   int
	delta     int
}

type recordingQuotaChecker struct {
	calls    []quotaCheckCall
	decision cloudquota.Decision
	err      error
}

func (c *recordingQuotaChecker) Check(_ context.Context, accountID string, resource cloudquota.Resource, current, delta int) (cloudquota.Decision, error) {
	c.calls = append(c.calls, quotaCheckCall{
		accountID: accountID,
		resource:  resource,
		current:   current,
		delta:     delta,
	})
	if c.err != nil {
		return c.decision, c.err
	}
	if c.decision.Resource == "" {
		c.decision = cloudquota.Decision{Allowed: true, Resource: resource, PlanID: "free", Limit: current + delta, Current: current, Delta: delta}
	}
	return c.decision, nil
}

func TestCloudUserQuotaCountsPendingInvites(t *testing.T) {
	am, cleanup := setupInviteTestManagerWithEmbeddedIdP(t)
	defer cleanup()

	checker := &recordingQuotaChecker{}
	am.cloudQuota = checker
	ctx := context.Background()
	invite := &types.UserInviteRecord{
		ID:          "invite-1",
		AccountID:   testAccountID,
		Email:       "pending@example.com",
		Name:        "Pending User",
		Role:        string(types.UserRoleUser),
		HashedToken: "hash",
		ExpiresAt:   time.Now().Add(time.Hour),
		CreatedAt:   time.Now(),
		CreatedBy:   testAdminUserID,
	}
	require.NoError(t, am.Store.SaveUserInvite(ctx, invite))

	err := am.Store.ExecuteInTransaction(ctx, func(transaction store.Store) error {
		return am.enforceCloudUserQuota(ctx, transaction, testAccountID, 1, "")
	})
	require.NoError(t, err)
	require.Len(t, checker.calls, 1)
	assert.Equal(t, cloudquota.ResourceUsers, checker.calls[0].resource)
	assert.Equal(t, 3, checker.calls[0].current)
	assert.Equal(t, 1, checker.calls[0].delta)

	checker.calls = nil
	err = am.Store.ExecuteInTransaction(ctx, func(transaction store.Store) error {
		return am.enforceCloudUserQuota(ctx, transaction, testAccountID, 1, "invite-1")
	})
	require.NoError(t, err)
	require.Len(t, checker.calls, 1)
	assert.Equal(t, 2, checker.calls[0].current)
}

func TestCloudPeerQuotaExcludesEmbeddedProxyPeers(t *testing.T) {
	am, cleanup := setupInviteTestManagerWithEmbeddedIdP(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, am.Store.AddPeerToAccount(ctx, &nbpeer.Peer{
		ID:        "peer-1",
		AccountID: testAccountID,
		Key:       "peer-key-1",
		IP:        netip.MustParseAddr("100.64.0.10"),
		Name:      "peer-1",
		DNSLabel:  "peer-1",
		Status:    &nbpeer.PeerStatus{},
	}))
	require.NoError(t, am.Store.AddPeerToAccount(ctx, &nbpeer.Peer{
		ID:        "embedded-1",
		AccountID: testAccountID,
		Key:       "embedded-key-1",
		IP:        netip.MustParseAddr("100.64.0.11"),
		Name:      "embedded-1",
		DNSLabel:  "embedded-1",
		Status:    &nbpeer.PeerStatus{},
		ProxyMeta: nbpeer.ProxyMeta{Embedded: true, Cluster: "internal"},
	}))

	checker := &recordingQuotaChecker{}
	am.cloudQuota = checker

	err := am.Store.ExecuteInTransaction(ctx, func(transaction store.Store) error {
		return am.enforceCloudPeerQuota(ctx, transaction, testAccountID, 1)
	})
	require.NoError(t, err)
	require.Len(t, checker.calls, 1)
	assert.Equal(t, cloudquota.ResourcePeers, checker.calls[0].resource)
	assert.Equal(t, 1, checker.calls[0].current)
	assert.Equal(t, 1, checker.calls[0].delta)
}

func TestCreateUserInviteRejectsCloudQuotaLimit(t *testing.T) {
	am, cleanup := setupInviteTestManagerWithEmbeddedIdP(t)
	defer cleanup()

	decision := cloudquota.Decision{
		Allowed:  false,
		Resource: cloudquota.ResourceUsers,
		PlanID:   "free",
		Limit:    2,
		Current:  2,
		Delta:    1,
	}
	am.cloudQuota = &recordingQuotaChecker{
		decision: decision,
		err:      cloudquota.ErrLimitExceeded{Decision: decision},
	}

	_, err := am.CreateUserInvite(context.Background(), testAccountID, testAdminUserID, &types.UserInfo{
		Email:      "blocked@example.com",
		Name:       "Blocked User",
		Role:       string(types.UserRoleUser),
		AutoGroups: []string{},
	}, 0)
	require.Error(t, err)
	sErr, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, status.PreconditionFailed, sErr.Type())

	invites, err := am.Store.GetAccountUserInvites(context.Background(), store.LockingStrengthNone, testAccountID)
	require.NoError(t, err)
	assert.Empty(t, invites)
}
