package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/dcadolph/vamoose/internal/workflow"
)

// runWorkflows lists the workflows available to run, built-ins and user
// definitions, with the user directory overriding built-ins of the same name.
func runWorkflows(_ context.Context, _ []string) error {
	infos, err := workflowLoader().List()
	if err != nil {
		return fmt.Errorf("workflows: %w", err)
	}
	for _, in := range infos {
		desc := in.Description
		if desc == "" {
			desc = "-"
		}
		suffix := ""
		if in.Source == workflow.SourceUser {
			suffix = " (user)"
		}
		fmt.Fprintf(os.Stdout, "%-13s %s%s\n", in.Name, desc, suffix)
	}
	return nil
}
