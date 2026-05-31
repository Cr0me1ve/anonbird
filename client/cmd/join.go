package cmd

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/netbirdio/netbird/client/internal/anonymous"
)

type joinToken struct {
	ManagementURL string
	SetupKey      string
	Transport     anonymous.TransportConfig
	Hostname      string
	Profile       string
	DNSLabels     []string
}

var joinCmd = &cobra.Command{
	Use:   "join <anonbird://join?...>",
	Short: "Join an AnonBird network with an anonymous invite link",
	Long:  "Parses an anonbird://join invite link, validates anonymous transport settings, and connects with anonymous mode enabled.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		token, err := parseJoinToken(args[0])
		if err != nil {
			return err
		}
		if err := applyJoinToken(token); err != nil {
			return err
		}

		upCmd.SetContext(cmd.Context())
		upCmd.SetOut(cmd.OutOrStdout())
		upCmd.SetErr(cmd.ErrOrStderr())
		previousForceReconnect := forceDaemonReconnect
		forceDaemonReconnect = true
		defer func() {
			forceDaemonReconnect = previousForceReconnect
		}()
		return upFunc(upCmd, nil)
	},
}

func parseJoinToken(raw string) (joinToken, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return joinToken{}, fmt.Errorf("join token is empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return joinToken{}, fmt.Errorf("parse join token: %w", err)
	}
	if parsed.Scheme != "anonbird" || parsed.Host != "join" {
		return joinToken{}, fmt.Errorf("join token must use anonbird://join")
	}

	query := parsed.Query()
	tunnelLength, err := parseJoinUint8(query, anonymous.DefaultI2PTunnelLength, "i2p_tunnel_length", "i2p-tunnel-length", "tunnel_length", "tunnel-length")
	if err != nil {
		return joinToken{}, err
	}
	tunnelQuantity, err := parseJoinUint8(query, anonymous.DefaultI2PTunnelQuantity, "i2p_tunnel_quantity", "i2p-tunnel-quantity", "tunnel_quantity", "tunnel-quantity")
	if err != nil {
		return joinToken{}, err
	}

	transport := anonymous.NormalizeTransport(anonymous.TransportConfig{
		Type:              firstNonEmpty(query.Get("transport"), anonymous.TransportTorRelayOnly),
		TorSOCKS5:         query.Get("tor_socks5"),
		I2PSAM:            query.Get("i2p_sam"),
		I2PTunnelLength:   tunnelLength,
		I2PTunnelQuantity: tunnelQuantity,
		I2PDaemonMode:     firstNonEmpty(query.Get("i2p_daemon_mode"), query.Get("i2p-daemon-mode")),
		I2PDaemonPath:     firstNonEmpty(query.Get("i2pd_path"), query.Get("i2pd-path")),
		I2PDataDir:        firstNonEmpty(query.Get("i2p_data_dir"), query.Get("i2p-data-dir")),
	})
	if err := anonymous.ValidateTransport(transport); err != nil {
		return joinToken{}, err
	}

	management := firstNonEmpty(query.Get("server"), query.Get("management_url"), query.Get("management-url"))
	if management == "" {
		return joinToken{}, fmt.Errorf("join token is missing management server")
	}
	managementURL, err := url.Parse(management)
	if err != nil || managementURL.Scheme == "" || managementURL.Host == "" {
		return joinToken{}, fmt.Errorf("join token management server must be an absolute http(s) URL")
	}
	switch managementURL.Scheme {
	case "http", "https":
	default:
		return joinToken{}, fmt.Errorf("join token management server must use http or https")
	}
	if err := anonymous.ValidateServiceURLForTransport("management", managementURL, transport); err != nil {
		return joinToken{}, err
	}

	setupKey := firstNonEmpty(query.Get("setup_key"), query.Get("setup-key"))
	if strings.TrimSpace(setupKey) == "" {
		return joinToken{}, fmt.Errorf("join token is missing setup key")
	}

	dnsLabels, err := parseJoinDNSLabels(query)
	if err != nil {
		return joinToken{}, err
	}

	return joinToken{
		ManagementURL: managementURL.String(),
		SetupKey:      setupKey,
		Transport:     transport,
		Hostname:      strings.TrimSpace(query.Get("hostname")),
		Profile:       strings.TrimSpace(query.Get("profile")),
		DNSLabels:     dnsLabels,
	}, nil
}

func parseJoinDNSLabels(query url.Values) ([]string, error) {
	var labels []string
	for _, key := range []string{"dns_label", "dns-label", "dns_labels", "dns-labels", "service", "services"} {
		for _, value := range query[key] {
			for _, label := range strings.Split(value, ",") {
				label = strings.TrimSpace(label)
				if label != "" {
					labels = append(labels, label)
				}
			}
		}
	}
	if len(labels) == 0 {
		return nil, nil
	}

	validated, err := validateDnsLabels(labels)
	if err != nil {
		return nil, err
	}
	return validated.ToPunycodeList(), nil
}

func applyJoinToken(token joinToken) error {
	flagSets := []struct {
		name  string
		value string
	}{
		{"management-url", token.ManagementURL},
		{"setup-key", token.SetupKey},
		{anonymousModeFlag, "true"},
		{anonymousTransportFlag, token.Transport.Type},
		{torSOCKS5Flag, token.Transport.TorSOCKS5},
		{i2pSAMFlag, token.Transport.I2PSAM},
		{i2pTunnelLengthFlag, fmt.Sprintf("%d", token.Transport.I2PTunnelLength)},
		{i2pTunnelQuantityFlag, fmt.Sprintf("%d", token.Transport.I2PTunnelQuantity)},
		{i2pDaemonModeFlag, token.Transport.I2PDaemonMode},
		{i2pDaemonPathFlag, token.Transport.I2PDaemonPath},
		{i2pDataDirFlag, token.Transport.I2PDataDir},
	}

	for _, set := range flagSets {
		if set.value == "" {
			continue
		}
		if err := rootCmd.PersistentFlags().Set(set.name, set.value); err != nil {
			return fmt.Errorf("set %s from join token: %w", set.name, err)
		}
	}
	if token.Hostname != "" {
		if err := rootCmd.PersistentFlags().Set("hostname", token.Hostname); err != nil {
			return fmt.Errorf("set hostname from join token: %w", err)
		}
	}
	if token.Profile != "" {
		if err := upCmd.PersistentFlags().Set(profileNameFlag, token.Profile); err != nil {
			return fmt.Errorf("set profile from join token: %w", err)
		}
	}
	if len(token.DNSLabels) > 0 {
		if err := upCmd.PersistentFlags().Set(dnsLabelsFlag, strings.Join(token.DNSLabels, ",")); err != nil {
			return fmt.Errorf("set DNS labels from join token: %w", err)
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseJoinUint8(query url.Values, fallback uint8, keys ...string) (uint8, error) {
	for _, key := range keys {
		value := strings.TrimSpace(query.Get(key))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseUint(value, 10, 8)
		if err != nil {
			return 0, fmt.Errorf("join token %s must be an integer between 0 and 255: %w", key, err)
		}
		return uint8(parsed), nil
	}
	return fallback, nil
}
