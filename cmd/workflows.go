package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	wf, err := workflow.Parse(data)
	if err != nil {
		return fmt.Errorf("workflows add: %w", err)
	}
	if !safeWorkflowName(wf.Name) {
		return fmt.Errorf("workflows add: invalid workflow name %q", wf.Name)
	}
	dir, err := workflowsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, wf.Name+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Saved workflow %q to %s\n", wf.Name, path)
	return nil
}

// workflowRemove deletes a user workflow by name. It never touches a built-in.
func workflowRemove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("workflows remove: name a workflow")
	}
	name := args[0]
	if !safeWorkflowName(name) {
		return fmt.Errorf("workflows remove: invalid name %q", name)
	}
	dir, err := workflowsDir()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, name+".json")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("workflows remove: no user workflow named %q", name)
		}
		return err
	}
	fmt.Fprintf(os.Stdout, "Removed workflow %q\n", name)
	return nil
}

// safeWorkflowName reports whether name is safe to use as a file name: non-empty and
// free of path separators or dots, so it cannot escape the workflow directory.
func safeWorkflowName(name string) bool {
	return name != "" && !strings.ContainsAny(name, `/\.`)
}
