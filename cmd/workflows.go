package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dcadolph/vamoose/internal/util"
	"github.com/dcadolph/vamoose/internal/workflow"
)

// runWorkflows dispatches the workflows subcommands: list (the default), add, and
// remove. add and remove manage user workflows under the config directory.
func runWorkflows(_ context.Context, args []string) error {
	if len(args) == 0 {
		return workflowList()
	}
	switch args[0] {
	case "list":
		return workflowList()
	case "add":
		return workflowAdd(args[1:])
	case "remove", "rm":
		return workflowRemove(args[1:])
	default:
		return fmt.Errorf("workflows: unknown subcommand %q; use list, add, or remove", args[0])
	}
}

// workflowList prints the workflows available to run, built-ins and user definitions,
// with the user directory overriding built-ins of the same name.
func workflowList() error {
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

// workflowAdd saves a user workflow from a JSON definition read from a file or stdin.
// The definition is validated, and its own name becomes the file name, so an invalid
// or unsafely named workflow is rejected rather than written.
func workflowAdd(args []string) error {
	fs := flag.NewFlagSet("workflows add", flag.ContinueOnError)
	file := fs.String("file", "", "Read the JSON definition from this file instead of stdin")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var (
		data []byte
		err  error
	)
	if *file != "" {
		data, err = os.ReadFile(*file)
	} else {
		data, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		return fmt.Errorf("workflows add: read definition: %w", err)
	}
	wf, err := saveUserWorkflow(data)
	if err != nil {
		return fmt.Errorf("workflows add: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Saved workflow %q\n", wf.Name)
	return nil
}

// saveUserWorkflow validates a JSON definition and writes it to the user workflow
// directory under its own name, so an invalid or unsafely named workflow is rejected
// rather than written. The CLI and the dashboard both save through here.
func saveUserWorkflow(data []byte) (workflow.Workflow, error) {
	wf, err := workflow.Parse(data)
	if err != nil {
		return workflow.Workflow{}, err
	}
	if !safeWorkflowName(wf.Name) {
		return workflow.Workflow{}, fmt.Errorf("invalid workflow name %q", wf.Name)
	}
	dir, err := workflowsDir()
	if err != nil {
		return workflow.Workflow{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return workflow.Workflow{}, err
	}
	if err := util.WriteFileAtomic(filepath.Join(dir, wf.Name+".json"), data, 0o600); err != nil {
		return workflow.Workflow{}, err
	}
	return wf, nil
}

// workflowRemove deletes a user workflow by name.
func workflowRemove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("workflows remove: name a workflow")
	}
	if err := removeUserWorkflow(args[0]); err != nil {
		return fmt.Errorf("workflows remove: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Removed workflow %q\n", args[0])
	return nil
}

// removeUserWorkflow deletes a user workflow by name. It never touches a built-in,
// since those are not files in the user directory.
func removeUserWorkflow(name string) error {
	if !safeWorkflowName(name) {
		return fmt.Errorf("invalid name %q", name)
	}
	dir, err := workflowsDir()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, name+".json")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no user workflow named %q", name)
		}
		return err
	}
	return nil
}

// safeWorkflowName reports whether name is safe to use as a file name: a short run of
// letters, digits, hyphens, and underscores. That excludes path separators, dots, NUL
// and other control bytes, and unicode separator lookalikes, so a name can neither
// escape the workflow directory nor create a surprising file.
func safeWorkflowName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}
