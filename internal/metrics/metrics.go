// Package metrics provides lightweight counters and gauges the hosted server exposes in
// Prometheus text format, using only the standard library. It is safe for concurrent use
// and orders its output by registration so scrapes are stable.
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
)

// Counter is a monotonically increasing metric.
type Counter struct {
	// v is the current value.
	v atomic.Int64
	// name is the metric name.
	name string
	// help is the one-line description.
	help string
}

// Inc adds one to the counter.
func (c *Counter) Inc() { c.v.Add(1) }

// Add adds n to the counter; a negative n is ignored, since counters only rise.
func (c *Counter) Add(n int64) {
	if n > 0 {
		c.v.Add(n)
	}
}

// Value returns the current count.
func (c *Counter) Value() int64 { return c.v.Load() }

// Gauge is a metric that can rise and fall.
type Gauge struct {
	// v is the current value.
	v atomic.Int64
	// name is the metric name.
	name string
	// help is the one-line description.
	help string
}

// Set sets the gauge value.
func (g *Gauge) Set(n int64) { g.v.Store(n) }

// Inc adds one to the gauge.
func (g *Gauge) Inc() { g.v.Add(1) }

// Dec subtracts one from the gauge.
func (g *Gauge) Dec() { g.v.Add(-1) }

// Value returns the current value.
func (g *Gauge) Value() int64 { return g.v.Load() }

// Registry holds the metrics a process exposes.
type Registry struct {
	// mu guards the maps and order.
	mu sync.Mutex
	// counters holds registered counters by name.
	counters map[string]*Counter
	// gauges holds registered gauges by name.
	gauges map[string]*Gauge
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{counters: map[string]*Counter{}, gauges: map[string]*Gauge{}}
}

// Counter returns the named counter, creating it with help on first use.
func (r *Registry) Counter(name, help string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.counters[name]; ok {
		return c
	}
	c := &Counter{name: name, help: help}
	r.counters[name] = c
	return c
}

// Gauge returns the named gauge, creating it with help on first use.
func (r *Registry) Gauge(name, help string) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.gauges[name]; ok {
		return g
	}
	g := &Gauge{name: name, help: help}
	r.gauges[name] = g
	return g
}

// WriteText writes the metrics in Prometheus exposition format, sorted by name so the
// output is stable across scrapes.
func (r *Registry) WriteText(w io.Writer) {
	r.mu.Lock()
	counters := make([]*Counter, 0, len(r.counters))
	for _, c := range r.counters {
		counters = append(counters, c)
	}
	gauges := make([]*Gauge, 0, len(r.gauges))
	for _, g := range r.gauges {
		gauges = append(gauges, g)
	}
	r.mu.Unlock()

	sort.Slice(counters, func(i, j int) bool { return counters[i].name < counters[j].name })
	sort.Slice(gauges, func(i, j int) bool { return gauges[i].name < gauges[j].name })

	for _, c := range counters {
		writeMetric(w, c.name, c.help, "counter", c.v.Load())
	}
	for _, g := range gauges {
		writeMetric(w, g.name, g.help, "gauge", g.v.Load())
	}
}

// writeMetric writes one metric's HELP, TYPE, and value lines.
func writeMetric(w io.Writer, name, help, typ string, value int64) {
	if help != "" {
		fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	}
	fmt.Fprintf(w, "# TYPE %s %s\n", name, typ)
	fmt.Fprintf(w, "%s %d\n", name, value)
}

// Handler returns an HTTP handler that serves the metrics in Prometheus text format.
func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		r.WriteText(w)
	}
}
