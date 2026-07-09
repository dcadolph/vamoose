package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/dcadolph/vamoose/internal/caldav"
)

// runCalendars lists or creates calendars. It works only on the iCloud/CalDAV
// provider, where making a dedicated calendar avoids the Calendar app.
func runCalendars(ctx context.Context, args []string) error {
	action := "list"
	var rest []string
	if len(args) > 0 {
		action, rest = args[0], args[1:]
	}
	prov, err := newProvider(resolveProvider(""), resolveTimeZone(""))
	if err != nil {
		return err
	}
	cp, ok := prov.(*caldav.Provider)
	if !ok {
		return fmt.Errorf("calendars is supported only on the icloud provider; set VAMOOSE_PROVIDER=icloud")
	}
	switch action {
	case "list":
		cals, err := cp.ListCalendars(ctx)
		if err != nil {
			return err
		}
		for _, c := range cals {
			fmt.Fprintln(os.Stdout, c.Name)
		}
		return nil
	case "create":
		if len(rest) == 0 {
			return fmt.Errorf("calendars create: give a calendar name")
		}
		name := rest[0]
		if _, err := cp.CreateCalendar(ctx, name); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Created calendar %q.\nUse it with: export VAMOOSE_ICLOUD_CALENDAR=%q\n", name, name)
		return nil
	default:
		return fmt.Errorf("calendars: unknown action %q (use list or create)", action)
	}
}
