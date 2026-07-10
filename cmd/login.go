package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
)

// runLogin signs the user in to the selected calendar provider, running the interactive
// consent flow when no valid token is cached, then prints who they are. For Google it
// uses the built-in OAuth client, so no Cloud project setup is needed: the first call
// opens the browser for consent and caches the token for later commands.
func runLogin(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	var (
		provider = fs.String("provider", "", "Calendar provider; overrides VAMOOSE_PROVIDER (default graph)")
		tzFlag   = fs.String("tz", "", "IANA time zone")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	name := resolveProvider(*provider)
	prov, err := newProvider(name, resolveTimeZone(*tzFlag))
	if err != nil {
		return err
	}
	switch name {
	case providerGoogle:
		fmt.Fprintln(os.Stderr, "Signing in to Google. Your browser will open to grant access.")
	case defaultProvider:
		fmt.Fprintln(os.Stderr, "Signing in to Microsoft. Follow the device-code prompt below.")
	}
	me, err := prov.Me(ctx)
	if err != nil {
		return fmt.Errorf("sign in: %w", err)
	}
	// Google and Graph resolve the account over the network, so success here proves the
	// credentials work. iCloud and CalDAV report the configured account without a round
	// trip, so a real command is what exercises the password.
	switch name {
	case providerGoogle, defaultProvider:
		fmt.Fprintf(os.Stdout, "Signed in as %s\n", personLabel(me))
	default:
		fmt.Fprintf(os.Stdout, "Configured %s as %s. Credentials are verified on first calendar access.\n", name, personLabel(me))
	}
	return nil
}
