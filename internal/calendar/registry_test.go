package calendar

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// stubProvider is a no-op Provider used to verify registry wiring.
type stubProvider struct {
	// name records which factory produced the provider.
	name string
}

func (stubProvider) Me(context.Context) (Person, error)             { return Person{}, nil }
func (stubProvider) Manager(context.Context) (Person, error)        { return Person{}, nil }
func (stubProvider) Team(context.Context) ([]Person, error)         { return nil, nil }
func (stubProvider) CreateHold(context.Context, Hold) (Hold, error) { return Hold{}, nil }
func (stubProvider) GetHold(context.Context, string) (Hold, error)  { return Hold{}, nil }
func (stubProvider) UpdateHold(context.Context, Hold) (Hold, error) { return Hold{}, nil }
func (stubProvider) DeleteHold(context.Context, string) error       { return nil }

// stubFactory returns a Factory that builds a named stubProvider.
func stubFactory(name string) Factory {
	return func(Settings) (Provider, error) { return stubProvider{name: name}, nil }
}

func TestRegistryBuild(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name string
		Want error
	}{{ // Test 0: A registered provider builds.
		Name: "graph", Want: nil,
	}, { // Test 1: A second registered provider builds.
		Name: "google", Want: nil,
	}, { // Test 2: An unknown provider errors.
		Name: "outlook", Want: ErrUnknownProvider,
	}, { // Test 3: An empty name errors.
		Name: "", Want: ErrUnknownProvider,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			r := NewRegistry()
			r.Register("graph", stubFactory("graph"))
			r.Register("google", stubFactory("google"))
			prov, err := r.Build(test.Name, Settings{})
			if !errors.Is(err, test.Want) {
				t.Fatalf("Build(%q) err = %v, want %v", test.Name, err, test.Want)
			}
			if test.Want == nil && prov == nil {
				t.Errorf("Build(%q) returned a nil provider", test.Name)
			}
		})
	}
}

func TestRegistryNames(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register("graph", stubFactory("graph"))
	r.Register("google", stubFactory("google"))
	got := r.Names()
	want := []string{"google", "graph"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names = %v, want %v", got, want)
	}
}

func TestRegistryRegisterPanics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name    string
		Factory Factory
		Dup     bool
	}{{ // Test 0: An empty name panics.
		Name: "", Factory: stubFactory("x"),
	}, { // Test 1: A nil factory panics.
		Name: "graph", Factory: nil,
	}, { // Test 2: A duplicate name panics.
		Name: "graph", Factory: stubFactory("x"), Dup: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			r := NewRegistry()
			if test.Dup {
				r.Register("graph", stubFactory("x"))
			}
			defer func() {
				if recover() == nil {
					t.Errorf("Register(%q) did not panic", test.Name)
				}
			}()
			r.Register(test.Name, test.Factory)
		})
	}
}
