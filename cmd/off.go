package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
)

// runOff creates a time-off hold from a natural date phrase or explicit dates,
// invites the manager, and optionally watches for approval. It is the friendly
// front to request: "vamoose off next week".
func runOff(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("off", flag.ContinueOnError)
	var (
		start    = fs.String("start", "", "Explicit start date/time; overrides the phrase")
		end      = fs.String("end", "", "Explicit end date/time; overrides the phrase")
		subject  = fs.String("subject", "Out of office", "Event subject")
		body     = fs.String("body", "", "Event description")
		manager  = fs.String("manager", "", "Manager email; resolved from the directory when empty")
		provider = fs.String("provider", "", "Calendar provider; overrides VAMOOSE_PROVIDER (default graph)")
		tzFlag   = fs.String("tz", "", "IANA time zone for event times")
		watch    = fs.Bool("watch", false, "Add the hold to the daemon watch list for auto-promote on approval")
		dryRun   = fs.Bool("dry-run", false, "Print what would be sent without calling the calendar")
	)
	phraseWords, flagArgs := splitPhrase(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	phrase := strings.Join(phraseWords, " ")
	if phrase == "" && fs.NArg() > 0 {
		phrase = strings.Join(fs.Args(), " ")
	}

	startAt, endAt, allDay, err := resolveWindow(*start, *end, phrase)
	if err != nil {
		return fmt.Errorf("off: %w", err)
	}
	if *start == "" && *end == "" {
		fmt.Fprintf(os.Stdout, "Reading %q as %s through %s.\n",
			phrase, startAt.Format("Mon 2006-01-02"), endAt.AddDate(0, 0, -1).Format("Mon 2006-01-02"))
	}

	return createHold(ctx, holdRequest{
		Provider: *provider,
		TZ:       *tzFlag,
		Subject:  *subject,
		Body:     *body,
		Manager:  *manager,
		Start:    startAt,
		End:      endAt,
		AllDay:   allDay,
		DryRun:   *dryRun,
		Watch:    *watch,
	})
}

// splitPhrase divides args into the leading positional words (the date phrase)
// and the remaining flag arguments, so flags may follow the phrase.
func splitPhrase(args []string) (words, flags []string) {
	for i, a := range args {
		if strings.HasPrefix(a, "-") {
			return args[:i], args[i:]
		}
	}
	return args, nil
}
