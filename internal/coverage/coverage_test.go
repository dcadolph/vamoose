package coverage

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// day builds a UTC date for the tables.
func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// newLedger returns a ledger over a temp file with a fixed clock.
func newLedger(t *testing.T, now time.Time) *Ledger {
	t.Helper()
	l := NewLedger(filepath.Join(t.TempDir(), "coverage.json"))
	l.now = func() time.Time { return now }
	return l
}

// TestOverlapping covers the half-open overlap test and owner exclusion.
func TestOverlapping(t *testing.T) {
	t.Parallel()
	now := day(2026, 8, 1)
	l := newLedger(t, now)
	// Amy off Mon-Wed, Bob off Wed-Fri, Cara off the following week.
	mon := day(2026, 8, 3)
	must(t, l.Record(Entry{Owner: "amy@x.com", Start: mon, End: mon.AddDate(0, 0, 2)}))         // Mon..Wed(excl)
	must(t, l.Record(Entry{Owner: "bob@x.com", Start: mon.AddDate(0, 0, 2), End: mon.AddDate(0, 0, 4)})) // Wed..Fri(excl)
	must(t, l.Record(Entry{Owner: "cara@x.com", Start: mon.AddDate(0, 0, 7), End: mon.AddDate(0, 0, 9)}))

	tests := []struct {
		Name      string
		Start     time.Time
		End       time.Time
		Exclude   string
		WantCount int
	}{{ // Test 0: Monday only overlaps Amy.
		Name: "monday", Start: mon, End: mon.AddDate(0, 0, 1), WantCount: 1,
	}, { // Test 1: Wednesday touches Amy's exclusive end and overlaps Bob's start.
		Name: "wednesday", Start: mon.AddDate(0, 0, 2), End: mon.AddDate(0, 0, 3), WantCount: 1,
	}, { // Test 2: The whole week overlaps Amy and Bob.
		Name: "whole week", Start: mon, End: mon.AddDate(0, 0, 5), WantCount: 2,
	}, { // Test 3: Excluding Bob drops him from the week.
		Name: "exclude bob", Start: mon, End: mon.AddDate(0, 0, 5), Exclude: "bob@x.com", WantCount: 1,
	}, { // Test 4: A gap week overlaps no one booked.
		Name: "gap", Start: mon.AddDate(0, 0, 5), End: mon.AddDate(0, 0, 7), WantCount: 0,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			got, err := l.Overlapping(test.Start, test.End, test.Exclude)
			if err != nil {
				t.Fatalf("%s: %v", test.Name, err)
			}
			if len(got) != test.WantCount {
				t.Errorf("%s: overlapping = %d, want %d (%+v)", test.Name, len(got), test.WantCount, got)
			}
		})
	}
}

// TestCountOffDistinct confirms two holds by one person count once.
func TestCountOffDistinct(t *testing.T) {
	t.Parallel()
	now := day(2026, 8, 1)
	l := newLedger(t, now)
	mon := day(2026, 8, 3)
	must(t, l.Record(Entry{Owner: "amy@x.com", Start: mon, End: mon.AddDate(0, 0, 1)}))
	must(t, l.Record(Entry{Owner: "amy@x.com", Start: mon.AddDate(0, 0, 1), End: mon.AddDate(0, 0, 2)}))
	must(t, l.Record(Entry{Owner: "bob@x.com", Start: mon, End: mon.AddDate(0, 0, 3)}))

	n, err := l.CountOff(mon, mon.AddDate(0, 0, 3), "")
	if err != nil {
		t.Fatalf("CountOff: %v", err)
	}
	if n != 2 {
		t.Errorf("distinct off = %d, want 2", n)
	}
	if n, _ := l.CountOff(mon, mon.AddDate(0, 0, 3), "amy@x.com"); n != 1 {
		t.Errorf("excluding amy = %d, want 1 (bob)", n)
	}
}

// TestRecordPrunesAndValidates confirms past entries are dropped on record and an
// inverted window is rejected.
func TestRecordPrunesAndValidates(t *testing.T) {
	t.Parallel()
	now := day(2026, 8, 10)
	l := newLedger(t, now)
	// A past entry (ended before now) and a future one.
	must(t, l.Record(Entry{Owner: "old@x.com", Start: day(2026, 8, 1), End: day(2026, 8, 3)}))
	must(t, l.Record(Entry{Owner: "new@x.com", Start: day(2026, 8, 12), End: day(2026, 8, 14)}))

	all, err := l.Overlapping(day(2026, 7, 1), day(2026, 9, 1), "")
	if err != nil {
		t.Fatalf("Overlapping: %v", err)
	}
	if len(all) != 1 || all[0].Owner != "new@x.com" {
		t.Errorf("after prune = %+v, want only new@x.com", all)
	}
	if err := l.Record(Entry{Owner: "x@x.com", Start: day(2026, 8, 5), End: day(2026, 8, 5)}); err == nil {
		t.Error("want an error for a zero-length window")
	}
}

// TestRemove confirms a canceled hold's entry is dropped and removing an unknown id is
// a no-op.
func TestRemove(t *testing.T) {
	t.Parallel()
	now := day(2026, 8, 1)
	l := newLedger(t, now)
	mon := day(2026, 8, 3)
	must(t, l.Record(Entry{Owner: "amy@x.com", HoldID: "h1", Start: mon, End: mon.AddDate(0, 0, 2)}))
	must(t, l.Record(Entry{Owner: "bob@x.com", HoldID: "h2", Start: mon, End: mon.AddDate(0, 0, 2)}))

	if err := l.Remove("h1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	got, _ := l.Overlapping(mon, mon.AddDate(0, 0, 2), "")
	if len(got) != 1 || got[0].Owner != "bob@x.com" {
		t.Errorf("after remove = %+v, want only bob", got)
	}
	if err := l.Remove("unknown"); err != nil {
		t.Errorf("removing an unknown id should be a no-op, got %v", err)
	}
	if err := l.Remove(""); err != nil {
		t.Errorf("removing an empty id should be a no-op, got %v", err)
	}
}

// must fails the test on error.
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("record: %v", err)
	}
}
