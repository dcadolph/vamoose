// Package comms defines the outbound messaging adapters a workflow uses to announce
// an outcome on a channel, such as posting to Slack. It is the comms axis of the
// adapter model, separate from the calendar and directory axes.
package comms

import "context"

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
