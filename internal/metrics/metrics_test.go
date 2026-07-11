package metrics

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestCounterAndGauge exercises the metric primitives.
func TestCounterAndGauge(t *testing.T) {
	t.Parallel()
	r := New()
	c := r.Counter("vamoose_things_total", "Things done.")
	c.Inc()
	c.Add(4)
	c.Add(-1) // ignored: counters only rise
	if c.Value() != 5 {
		t.Errorf("counter = %d, want 5", c.Value())
	}
	g := r.Gauge("vamoose_active", "Active items.")
	g.Set(3)
	g.Inc()
	g.Dec()
	g.Dec()
	if g.Value() != 2 {
		t.Errorf("gauge = %d, want 2", g.Value())
	}
	// The same name returns the same instrument.
	if r.Counter("vamoose_things_total", "") != c {
		t.Error("Counter should return the existing instrument for a known name")
	}
}

// TestWriteText confirms the Prometheus exposition format, sorted and typed.
func TestWriteText(t *testing.T) {
	t.Parallel()
	r := New()
	r.Counter("vamoose_b_total", "B help.").Add(2)
	r.Counter("vamoose_a_total", "A help.").Inc()
	r.Gauge("vamoose_active", "Active.").Set(7)

	var buf bytes.Buffer
	r.WriteText(&buf)
	out := buf.String()

	// Counters come first, sorted by name; a_total before b_total.
	if strings.Index(out, "vamoose_a_total") > strings.Index(out, "vamoose_b_total") {
		t.Errorf("metrics not sorted:\n%s", out)
	}
	for _, want := range []string{
		"# HELP vamoose_a_total A help.",
		"# TYPE vamoose_a_total counter",
		"vamoose_a_total 1",
		"# TYPE vamoose_active gauge",
		"vamoose_active 7",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// TestHandler confirms the metrics endpoint serves the text format.
func TestHandler(t *testing.T) {
	t.Parallel()
	r := New()
	r.Counter("vamoose_hits_total", "Hits.").Inc()
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body bytes.Buffer
	_, _ = body.ReadFrom(resp.Body)
	if !strings.Contains(body.String(), "vamoose_hits_total 1") {
		t.Errorf("handler body = %q, want the hits counter", body.String())
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("content type = %q, want text/plain", ct)
	}
}

// TestConcurrent confirms counters are safe under concurrent increments.
func TestConcurrent(t *testing.T) {
	t.Parallel()
	r := New()
	c := r.Counter("vamoose_race_total", "Race.")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()
	if c.Value() != 5000 {
		t.Errorf("concurrent counter = %d, want 5000", c.Value())
	}
}
