package server

import (
	"context"
	"errors"

	"github.com/netbirdio/netbird/management/server/cloudquota"
	nbpeer "github.com/netbirdio/netbird/management/server/peer"
	"github.com/netbirdio/netbird/management/server/store"
	"github.com/netbirdio/netbird/shared/management/status"
)

func (am *DefaultAccountManager) enforceCloudUserQuota(ctx context.Context, transaction store.Store, accountID string, delta int, excludedInviteID string) error {
	if am.cloudQuota == nil || delta <= 0 {
		return nil
	}
	if _, err := transaction.GetAccountSettings(ctx, store.LockingStrengthUpdate, accountID); err != nil {
		return err
	}
	current, err := userQuotaUsage(ctx, transaction, accountID, excludedInviteID)
	if err != nil {
		return err
	}
	return am.checkCloudQuota(ctx, accountID, cloudquota.ResourceUsers, current, delta)
}

func (am *DefaultAccountManager) enforceCloudPeerQuota(ctx context.Context, transaction store.Store, accountID string, delta int) error {
	if am.cloudQuota == nil || delta <= 0 {
		return nil
	}
	if _, err := transaction.GetAccountSettings(ctx, store.LockingStrengthUpdate, accountID); err != nil {
		return err
	}
	peers, err := transaction.GetAccountPeers(ctx, store.LockingStrengthNone, accountID, "", "")
	if err != nil {
		return err
	}
	return am.checkCloudQuota(ctx, accountID, cloudquota.ResourcePeers, countBillablePeers(peers), delta)
}

func userQuotaUsage(ctx context.Context, transaction store.Store, accountID, excludedInviteID string) (int, error) {
	users, err := transaction.GetAccountUsers(ctx, store.LockingStrengthNone, accountID)
	if err != nil {
		return 0, err
	}
	invites, err := transaction.GetAccountUserInvites(ctx, store.LockingStrengthNone, accountID)
	if err != nil {
		return 0, err
	}

	current := len(users)
	for _, invite := range invites {
		if invite == nil || invite.ID == excludedInviteID || invite.IsExpired() {
			continue
		}
		current++
	}
	return current, nil
}

func countBillablePeers(peers []*nbpeer.Peer) int {
	current := 0
	for _, peer := range peers {
		if peer == nil || peer.ProxyMeta.Embedded {
			continue
		}
		current++
	}
	return current
}

func (am *DefaultAccountManager) checkCloudQuota(ctx context.Context, accountID string, resource cloudquota.Resource, current, delta int) error {
	decision, err := am.cloudQuota.Check(ctx, accountID, resource, current, delta)
	if err == nil && decision.Allowed {
		return nil
	}

	var limitErr cloudquota.ErrLimitExceeded
	if errors.As(err, &limitErr) {
		return cloudQuotaLimitError(limitErr.Decision)
	}
	if err == nil {
		return cloudQuotaLimitError(decision)
	}
	return status.Errorf(status.PreconditionFailed, "cloud quota check failed for %s: %v", resource, err)
}

func cloudQuotaLimitError(decision cloudquota.Decision) error {
	return status.Errorf(status.PreconditionFailed, "%s limit exceeded for plan %s: current=%d delta=%d limit=%d",
		decision.Resource, decision.PlanID, decision.Current, decision.Delta, decision.Limit)
}
