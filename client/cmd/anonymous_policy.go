package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const unsafeClearnetConfirmationPhrase = "I understand this may leak my real IP"

func effectiveAnonymousMode() bool {
	return anonymousMode && !noAnonymousMode
}

func anonymousModeExplicitlyDisabled() bool {
	return noAnonymousMode || (rootCmd.PersistentFlags().Changed(anonymousModeFlag) && !anonymousMode)
}

func validateAnonymousModePolicy(cmd *cobra.Command) error {
	if noAnonymousMode && rootCmd.PersistentFlags().Changed(anonymousModeFlag) && anonymousMode {
		return fmt.Errorf("--%s conflicts with --%s=true", noAnonymousModeFlag, anonymousModeFlag)
	}
	if !effectiveAnonymousMode() && anonymousTransportFlagsChanged() {
		return fmt.Errorf("anonymous transport flags require anonymous mode; remove --%s or --%s=false", noAnonymousModeFlag, anonymousModeFlag)
	}
	if !anonymousModeExplicitlyDisabled() {
		return nil
	}

	printUnsafeClearnetWarning(cmd)
	if allowUnsafeClearnet && unsafeClearnetAck {
		return nil
	}
	if commandInputIsTerminal(cmd) {
		fmt.Fprintf(cmd.ErrOrStderr(), "\nType %q to continue: ", unsafeClearnetConfirmationPhrase)
		reader := bufio.NewReader(cmd.InOrStdin())
		response, err := reader.ReadString('\n')
		if err == nil && strings.TrimSpace(response) == unsafeClearnetConfirmationPhrase {
			return nil
		}
		return fmt.Errorf("unsafe clearnet confirmation did not match")
	}

	return fmt.Errorf("non-anonymous mode requires --%s and --%s", allowUnsafeClearnetFlag, unsafeClearnetAckFlag)
}

func printUnsafeClearnetWarning(cmd *cobra.Command) {
	fmt.Fprintf(cmd.ErrOrStderr(), `%s
WARNING: You are trying to connect without AnonBird anonymous mode.
Your real IP address, NAT endpoint, and local network metadata may be visible
to the management server, relay, and/or other peers.
%s
`, strings.Repeat("=", 72), strings.Repeat("=", 72))
}

func commandInputIsTerminal(cmd *cobra.Command) bool {
	f, ok := cmd.InOrStdin().(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

func shouldApplyAnonymousMode() bool {
	return effectiveAnonymousMode() || rootCmd.PersistentFlags().Changed(anonymousModeFlag) || noAnonymousMode
}
