package cmd

import (
	"fmt"
	"strings"
	"testing"
)

func TestSplitPhrase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         []string
		WantPhrase string
		WantFlags  string
	}{{ // Test 0: A phrase followed by flags.
		In: []string{"next", "week", "--watch"}, WantPhrase: "next week", WantFlags: "--watch",
	}, { // Test 1: Flags only, no phrase.
		In: []string{"--start", "x", "--end", "y"}, WantPhrase: "", WantFlags: "--start x --end y",
	}, { // Test 2: A phrase only.
		In: []string{"tomorrow"}, WantPhrase: "tomorrow", WantFlags: "",
	}, { // Test 3: Nothing.
		In: nil, WantPhrase: "", WantFlags: "",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			words, flags := splitPhrase(test.In)
			if got := strings.Join(words, " "); got != test.WantPhrase {
				t.Errorf("phrase = %q, want %q", got, test.WantPhrase)
			}
			if got := strings.Join(flags, " "); got != test.WantFlags {
				t.Errorf("flags = %q, want %q", got, test.WantFlags)
			}
		})
	}
}
