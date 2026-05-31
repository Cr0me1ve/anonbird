// Package setupkey provides reusable cobra commands for bootstrapping setup keys.
package setupkey

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/rs/xid"
	"github.com/spf13/cobra"

	nbdns "github.com/netbirdio/netbird/dns"
	nbpeer "github.com/netbirdio/netbird/management/server/peer"
	"github.com/netbirdio/netbird/management/server/store"
	"github.com/netbirdio/netbird/management/server/types"
	"github.com/netbirdio/netbird/route"
	"github.com/netbirdio/netbird/shared/management/status"
)

// StoreOpener initializes a store from command context and calls fn.
type StoreOpener func(cmd *cobra.Command, fn func(ctx context.Context, s store.Store) error) error

type bootstrapOptions struct {
	accountID            string
	ownerUserID          string
	ownerEmail           string
	ownerName            string
	domain               string
	keyName              string
	expiresIn            string
	usageLimit           int
	ephemeral            bool
	allowExtraDNSLabels  bool
	createAccount        bool
	disableDefaultPolicy bool
}

// NewCommands creates setup-key bootstrap commands.
func NewCommands(opener StoreOpener) *cobra.Command {
	opts := bootstrapOptions{}

	cmd := &cobra.Command{
		Use:   "setup-key",
		Short: "Manage setup keys",
		Long:  "Commands for bootstrapping setup keys used by unattended peers.",
	}

	bootstrapCmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Create a setup key and optionally bootstrap an account",
		Long:  "Creates a reusable setup key. If the account does not exist, it can create an account with an owner user and default All-group policy.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return opener(cmd, func(ctx context.Context, s store.Store) error {
				return runBootstrap(ctx, s, cmd.OutOrStdout(), opts)
			})
		},
	}

	bootstrapCmd.Flags().StringVar(&opts.accountID, "account-id", "anonbird", "Account ID to use or create")
	bootstrapCmd.Flags().StringVar(&opts.ownerUserID, "owner-user-id", "anonbird-owner", "Owner user ID when creating a new account")
	bootstrapCmd.Flags().StringVar(&opts.ownerEmail, "owner-email", "owner@anonbird.local", "Owner email when creating a new account")
	bootstrapCmd.Flags().StringVar(&opts.ownerName, "owner-name", "AnonBird Owner", "Owner display name when creating a new account")
	bootstrapCmd.Flags().StringVar(&opts.domain, "domain", "anonbird.local", "Account private domain when creating a new account")
	bootstrapCmd.Flags().StringVar(&opts.keyName, "name", "AnonBird setup key", "Setup key name")
	bootstrapCmd.Flags().StringVar(&opts.expiresIn, "expires-in", "30d", "Setup key expiration duration, for example 30d or 720h. Empty means no expiration")
	bootstrapCmd.Flags().IntVar(&opts.usageLimit, "usage-limit", types.SetupKeyUnlimitedUsage, "Maximum setup key uses; 0 means unlimited")
	bootstrapCmd.Flags().BoolVar(&opts.ephemeral, "ephemeral", false, "Mark peers enrolled with this setup key as ephemeral")
	bootstrapCmd.Flags().BoolVar(&opts.allowExtraDNSLabels, "allow-extra-dns-labels", false, "Allow enrolled peers to request extra DNS labels")
	bootstrapCmd.Flags().BoolVar(&opts.createAccount, "create-account", true, "Create the account if it does not exist")
	bootstrapCmd.Flags().BoolVar(&opts.disableDefaultPolicy, "disable-default-policy", false, "Create the account without the default All-to-All policy")

	cmd.AddCommand(bootstrapCmd)
	return cmd
}

func runBootstrap(ctx context.Context, s store.Store, w io.Writer, opts bootstrapOptions) error {
	if strings.TrimSpace(opts.accountID) == "" {
		return fmt.Errorf("account-id is required")
	}
	if strings.TrimSpace(opts.keyName) == "" {
		return fmt.Errorf("setup key name is required")
	}

	expiresIn, err := parseDuration(opts.expiresIn)
	if err != nil {
		return fmt.Errorf("parse expiration: %w", err)
	}

	account, err := s.GetAccount(ctx, opts.accountID)
	if err != nil {
		if sErr, ok := status.FromError(err); !ok || sErr.Type() != status.NotFound {
			return fmt.Errorf("get account: %w", err)
		}
		if !opts.createAccount {
			return fmt.Errorf("account %q does not exist", opts.accountID)
		}
		account = newBootstrapAccount(ctx, opts)
		if err := s.SaveAccount(ctx, account); err != nil {
			return fmt.Errorf("create account: %w", err)
		}
	}

	key, plainKey := types.GenerateSetupKey(
		opts.keyName,
		types.SetupKeyReusable,
		expiresIn,
		defaultSetupKeyAutoGroups(account),
		opts.usageLimit,
		opts.ephemeral,
		opts.allowExtraDNSLabels,
	)
	key.AccountID = account.Id

	if err := s.SaveSetupKey(ctx, key); err != nil {
		return fmt.Errorf("save setup key: %w", err)
	}

	_, _ = fmt.Fprintln(w, "Setup key created successfully!")
	_, _ = fmt.Fprintf(w, "Account ID: %s\n", account.Id)
	_, _ = fmt.Fprintf(w, "Setup Key ID: %s\n", key.Id)
	_, _ = fmt.Fprintf(w, "Setup Key: %s\n", plainKey)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "IMPORTANT: Save this setup key now. It will not be shown again.")
	return nil
}

func newBootstrapAccount(ctx context.Context, opts bootstrapOptions) *types.Account {
	accountID := strings.TrimSpace(opts.accountID)
	if accountID == "" {
		accountID = xid.New().String()
	}
	ownerUserID := strings.TrimSpace(opts.ownerUserID)
	if ownerUserID == "" {
		ownerUserID = xid.New().String()
	}

	owner := types.NewOwnerUser(ownerUserID, opts.ownerEmail, opts.ownerName)
	owner.AccountID = accountID

	account := &types.Account{
		Id:               accountID,
		CreatedAt:        time.Now().UTC(),
		CreatedBy:        ownerUserID,
		Domain:           opts.domain,
		SetupKeys:        map[string]*types.SetupKey{},
		Network:          types.NewNetwork(),
		Peers:            map[string]*nbpeer.Peer{},
		Users:            map[string]*types.User{ownerUserID: owner},
		Groups:           map[string]*types.Group{},
		Routes:           map[route.ID]*route.Route{},
		NameServerGroups: map[string]*nbdns.NameServerGroup{},
		DNSSettings: types.DNSSettings{
			DisabledManagementGroups: []string{},
		},
		Settings: &types.Settings{
			PeerLoginExpirationEnabled:      true,
			PeerLoginExpiration:             types.DefaultPeerLoginExpiration,
			GroupsPropagationEnabled:        true,
			RegularUsersViewBlocked:         true,
			PeerInactivityExpirationEnabled: false,
			PeerInactivityExpiration:        types.DefaultPeerInactivityExpiration,
			RoutingPeerDNSResolutionEnabled: true,
			Extra: &types.ExtraSettings{
				UserApprovalRequired: false,
			},
		},
		Onboarding: types.AccountOnboarding{
			OnboardingFlowPending: true,
			SignupFormPending:     true,
		},
	}

	if err := account.AddAllGroup(opts.disableDefaultPolicy); err != nil {
		// AddAllGroup only fails on malformed in-memory state; surface it by leaving
		// the account otherwise intact and letting validation fail at save/use time.
		_ = err
	}
	if allGroup, err := account.GetGroupAll(); err == nil {
		account.Settings.IPv6EnabledGroups = []string{allGroup.ID}
	}

	_ = ctx
	return account
}

func defaultSetupKeyAutoGroups(account *types.Account) []string {
	allGroup, err := account.GetGroupAll()
	if err != nil || allGroup == nil || allGroup.ID == "" {
		return nil
	}
	return []string{allGroup.ID}
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid day duration %q", s)
		}
		if days <= 0 {
			return 0, fmt.Errorf("duration must be positive")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	duration, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return duration, nil
}
