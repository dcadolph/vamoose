package slack

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

// TestUserLinkFileStore covers save, get, per-user isolation, missing, and delete.
func TestUserLinkFileStore(t *testing.T) {
	t.Parallel()
	google := UserLink{Provider: "google", RefreshToken: "rt-123"}
	icloud := UserLink{Provider: "icloud", ICloudUser: "me@icloud.com", ICloudAppPassword: "abcd-efgh"}

	// Test 0: A saved google link round-trips.
	t.Run("test 0", func(t *testing.T) {
		t.Parallel()
		s := NewUserLinkFileStore(filepath.Join(t.TempDir(), "links.json"))
		if err := s.SaveLink("T1", "U1", google); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, err := s.GetLink("T1", "U1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !reflect.DeepEqual(got, google) {
			t.Errorf("got %#v, want %#v", got, google)
		}
	})

	// Test 1: A saved icloud link round-trips with its credentials.
	t.Run("test 1", func(t *testing.T) {
		t.Parallel()
		s := NewUserLinkFileStore(filepath.Join(t.TempDir(), "links.json"))
		if err := s.SaveLink("T1", "U1", icloud); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, err := s.GetLink("T1", "U1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !reflect.DeepEqual(got, icloud) {
			t.Errorf("got %#v, want %#v", got, icloud)
		}
	})

	// Test 2: The same user id in different workspaces links independently.
	t.Run("test 2", func(t *testing.T) {
		t.Parallel()
		s := NewUserLinkFileStore(filepath.Join(t.TempDir(), "links.json"))
		if err := s.SaveLink("T1", "U1", google); err != nil {
			t.Fatalf("save T1: %v", err)
		}
		if err := s.SaveLink("T2", "U1", icloud); err != nil {
			t.Fatalf("save T2: %v", err)
		}
		got1, _ := s.GetLink("T1", "U1")
		got2, _ := s.GetLink("T2", "U1")
		if got1.Provider != "google" || got2.Provider != "icloud" {
			t.Errorf("isolation failed: T1=%s T2=%s", got1.Provider, got2.Provider)
		}
	})

	// Test 3: An unlinked user reports ErrNotLinked.
	t.Run("test 3", func(t *testing.T) {
		t.Parallel()
		s := NewUserLinkFileStore(filepath.Join(t.TempDir(), "links.json"))
		if _, err := s.GetLink("T1", "nobody"); !errors.Is(err, ErrNotLinked) {
			t.Errorf("err = %v, want ErrNotLinked", err)
		}
	})

	// Test 4: Delete removes a link; a later get reports ErrNotLinked. Deleting an
	// absent link is not an error.
	t.Run("test 4", func(t *testing.T) {
		t.Parallel()
		s := NewUserLinkFileStore(filepath.Join(t.TempDir(), "links.json"))
		if err := s.SaveLink("T1", "U1", google); err != nil {
			t.Fatalf("save: %v", err)
		}
		if err := s.DeleteLink("T1", "U1"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := s.GetLink("T1", "U1"); !errors.Is(err, ErrNotLinked) {
			t.Errorf("after delete err = %v, want ErrNotLinked", err)
		}
		if err := s.DeleteLink("T1", "absent"); err != nil {
			t.Errorf("delete absent = %v, want nil", err)
		}
	})
}

// TestUserLinkFileStoreList confirms List returns every linked user.
func TestUserLinkFileStoreList(t *testing.T) {
	t.Parallel()
	s := NewUserLinkFileStore(filepath.Join(t.TempDir(), "l.json"))
	if err := s.SaveLink("T1", "U1", UserLink{Provider: "google"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := s.SaveLink("T2", "U2", UserLink{Provider: "icloud"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	ids, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := map[string]bool{}
	for _, id := range ids {
		found[id.Team+":"+id.User] = true
	}
	if len(ids) != 2 || !found["T1:U1"] || !found["T2:U2"] {
		t.Errorf("List = %+v, want T1:U1 and T2:U2", ids)
	}
}

// TestUserLinkRedacted confirms secrets are masked and non-secret fields kept.
func TestUserLinkRedacted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In   UserLink
		Want UserLink
	}{{ // Test 0: A google refresh token is masked.
		In:   UserLink{Provider: "google", RefreshToken: "rt-secret"},
		Want: UserLink{Provider: "google", RefreshToken: "<redacted>"},
	}, { // Test 1: An icloud password is masked but the user is kept.
		In:   UserLink{Provider: "icloud", ICloudUser: "me@icloud.com", ICloudAppPassword: "abcd"},
		Want: UserLink{Provider: "icloud", ICloudUser: "me@icloud.com", ICloudAppPassword: "<redacted>"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := test.In.Redacted(); !reflect.DeepEqual(got, test.Want) {
				t.Errorf("Redacted() = %#v, want %#v", got, test.Want)
			}
		})
	}
}
