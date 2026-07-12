// Package comms defines the outbound messaging adapters a workflow uses to announce
// an outcome on a channel, such as posting to Slack. It is the comms axis of the
// adapter model, separate from the calendar and directory axes.
package comms

import (
	"context"
	"fmt"
	"strings"
)

// Notifier sends a short message to a channel on a comms backend.
type Notifier interface {
	// Notify posts text to the given channel.
	Notify(ctx context.Context, channel, text string) error
}

// NotifierFunc adapts a plain function to a Notifier.
type NotifierFunc func(ctx context.Context, channel, text string) error

// Notify calls f.
func (f NotifierFunc) Notify(ctx context.Context, channel, text string) error {
	return f(ctx, channel, text)
}

// Route returns a Notifier that dispatches by the channel: an http or https URL is
// posted to as an incoming webhook, an address containing "@" goes to email, anything
// else to Slack. A message for a backend that is not configured returns a clear error
// naming the setting to set.
func Route(slack, email, webhook Notifier) Notifier {
	return NotifierFunc(func(ctx context.Context, channel, text string) error {
		switch {
		case strings.HasPrefix(channel, "https://") || strings.HasPrefix(channel, "http://"):
			if webhook == nil {
				return fmt.Errorf("no webhook backend for %q", channel)
			}
			return webhook.Notify(ctx, channel, text)
		case strings.Contains(channel, "@"):
			if email == nil {
				return fmt.Errorf("no email backend for %q: set VAMOOSE_SMTP_HOST", channel)
			}
			return email.Notify(ctx, channel, text)
		default:
			if slack == nil {
				return fmt.Errorf("no slack backend for %q: set VAMOOSE_SLACK_BOT_TOKEN", channel)
			}
			return slack.Notify(ctx, channel, text)
		}
	})
}
