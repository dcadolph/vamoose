package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/dcadolph/vamoose/internal/hris"
)

// runBalance prints the remaining time off read from the configured HR system, so a
// request can be weighed against what is actually left.
func runBalance(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("balance", flag.ContinueOnError)
	asOf := fs.String("as-of", "", "Date to check the balance as of, YYYY-MM-DD (default today)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	reader, employeeID := resolveBalanceReader(os.Getenv)
	if reader == nil {
		return fmt.Errorf("no HR system configured: set VAMOOSE_BAMBOOHR_* or VAMOOSE_BALANCE_WEBHOOK_URL")
	}
	if employeeID == "" {
		return fmt.Errorf("set VAMOOSE_BAMBOOHR_EMPLOYEE_ID or VAMOOSE_HRIS_EMPLOYEE_ID to whose balance to read")
	}
	var when time.Time
	if *asOf != "" {
		t, err := time.Parse("2006-01-02", *asOf)
		if err != nil {
			return fmt.Errorf("balance: bad --as-of %q: want YYYY-MM-DD", *asOf)
		}
		when = t
	}
	balances, err := reader.Balance(ctx, employeeID, when)
	if err != nil {
		return fmt.Errorf("balance: %w", err)
	}
	if len(balances) == 0 {
		fmt.Fprintln(os.Stdout, "No balances reported.")
		return nil
	}
	for _, b := range balances {
		name := b.Name
		if name == "" {
			name = "type " + b.TypeID
		}
		unit := b.Unit
		if unit == "" {
			unit = "days"
		}
		fmt.Fprintf(os.Stdout, "%-24s %g %s\n", name, b.Available, unit)
	}
	return nil
}

// resolveBalanceReader returns the configured balance reader and the employee id to
// query, or a nil reader when none is set. BambooHR is preferred; otherwise a generic
// balance webhook is used, so any HR system can report a balance.
func resolveBalanceReader(getenv func(string) string) (hris.BalanceReader, string) {
	if sub, key := getenv("VAMOOSE_BAMBOOHR_SUBDOMAIN"), getenv("VAMOOSE_BAMBOOHR_API_KEY"); sub != "" && key != "" {
		return hris.NewBambooHRBalanceReader(sub, key), resolveEmployeeID(getenv)
	}
	if url := getenv("VAMOOSE_BALANCE_WEBHOOK_URL"); url != "" {
		return hris.NewWebhookBalanceReader(url, getenv("VAMOOSE_BALANCE_WEBHOOK_AUTH")), resolveEmployeeID(getenv)
	}
	return nil, ""
}

// resolveEmployeeID returns the HR employee id from the generic or BambooHR variable.
func resolveEmployeeID(getenv func(string) string) string {
	if id := getenv("VAMOOSE_HRIS_EMPLOYEE_ID"); id != "" {
		return id
	}
	return getenv("VAMOOSE_BAMBOOHR_EMPLOYEE_ID")
}
