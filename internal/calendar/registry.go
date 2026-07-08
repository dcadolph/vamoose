package calendar

import (
	"fmt"
	"slices"
)

// Settings carries the provider-neutral configuration a Factory needs to build
// a Provider.
type Settings struct {
	// TimeZone is the IANA zone used when sending event times.
	TimeZone string
}

// Factory builds a Provider from settings. Each calendar backend registers one.
type Factory func(s Settings) (Provider, error)

// Registry maps provider names to the factories that build them.
type Registry struct {
	// factories holds the registered provider factories keyed by name.
	factories map[string]Factory
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

// Register adds a factory under name. It panics on an empty name, a nil factory,
// or a duplicate name, all of which signal developer error.
func (r *Registry) Register(name string, f Factory) {
	if name == "" {
		panic("calendar: Register with empty provider name")
	}
	if f == nil {
		panic("calendar: Register with nil factory for " + name)
	}
	if _, dup := r.factories[name]; dup {
		panic("calendar: Register called twice for " + name)
	}
	r.factories[name] = f
}

// Build constructs the provider registered under name, returning
// ErrUnknownProvider when none matches.
func (r *Registry) Build(name string, s Settings) (Provider, error) {
	f, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, name)
	}
	return f(s)
}

// Names returns the registered provider names in sorted order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
