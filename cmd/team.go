package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// teamSource labels where a resolved team came from.
type teamSource string

const (
	// sourceConfig means the team came from team.json.
	sourceConfig teamSource = "config"
	// sourceDirectory means the team came from the manager's direct reports.
	sourceDirectory teamSource = "directory"
)

// teamPath returns the team config file location under the user config directory.
func teamPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vamoose", "team.json"), nil
}

// loadTeamConfig reads the configured team emails, or nil when unset.
func loadTeamConfig() ([]string, error) {
	path, err := teamPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var emails []string
	if err := json.Unmarshal(b, &emails); err != nil {
		return nil, fmt.Errorf("parse team config: %w", err)
	}
	return emails, nil
}

// saveTeamConfig writes the team emails to the config file.
func saveTeamConfig(emails []string) error {
	path, err := teamPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(emails, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// clearTeamConfig removes the team config file, reverting to directory lookup.
func clearTeamConfig() error {
	path, err := teamPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// peopleFromEmails builds people from a list of email addresses, dropping blanks.
func peopleFromEmails(emails []string) []calendar.Person {
	out := make([]calendar.Person, 0, len(emails))
	for _, e := range emails {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, calendar.Person{Email: e})
		}
	}
	return out
}

// mergeTeam returns the configured team when set, otherwise the directory team.
// The directory lookup is a function so it runs only on the fallback path.
func mergeTeam(configEmails []string, directory func() ([]calendar.Person, error)) ([]calendar.Person, teamSource, error) {
	if people := peopleFromEmails(configEmails); len(people) > 0 {
		return people, sourceConfig, nil
	}
	dir, err := directory()
	if err != nil {
		return nil, "", err
	}
	return dir, sourceDirectory, nil
}

// resolveTeam returns the effective team and where it came from.
func resolveTeam(ctx context.Context, prov calendar.Provider) ([]calendar.Person, teamSource, error) {
	emails, err := loadTeamConfig()
	if err != nil {
		return nil, "", err
	}
	return mergeTeam(emails, func() ([]calendar.Person, error) { return prov.Team(ctx) })
}

// runTeam handles the team subcommand: list, set, or clear the default team.
func runTeam(ctx context.Context, args []string) error {
	action := "list"
	var rest []string
	if len(args) > 0 {
		action, rest = args[0], args[1:]
	}
	switch action {
	case "list":
		return runTeamList(ctx)
	case "set":
		return runTeamSet(rest)
	case "clear":
		if err := clearTeamConfig(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "Team config cleared; promote will use the directory.")
		return nil
	default:
		return fmt.Errorf("team: unknown action %q (use list, set, or clear)", action)
	}
}

// runTeamSet writes the given emails as the default team.
func runTeamSet(emails []string) error {
	if len(emails) == 0 {
		return fmt.Errorf("team set: provide at least one email")
	}
	if err := saveTeamConfig(emails); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Saved %d team member(s) to your default team.\n", len(emails))
	return nil
}

// runTeamList prints the configured team, or the directory team when unset.
func runTeamList(ctx context.Context) error {
	emails, err := loadTeamConfig()
	if err != nil {
		return err
	}
	if people := peopleFromEmails(emails); len(people) > 0 {
		fmt.Fprintf(os.Stdout, "Team (config, %d):\n", len(people))
		for _, p := range people {
			fmt.Fprintf(os.Stdout, "  %s\n", personLabel(p))
		}
		return nil
	}
	prov, err := newProvider(resolveProvider(""), resolveTimeZone(""))
	if err != nil {
		return err
	}
	people, source, err := resolveTeam(ctx, prov)
	if err != nil {
		return fmt.Errorf("resolve team: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Team (%s, %d):\n", source, len(people))
	for _, p := range people {
		fmt.Fprintf(os.Stdout, "  %s\n", personLabel(p))
	}
	return nil
}
