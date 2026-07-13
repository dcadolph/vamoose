package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/dcadolph/vamoose/internal/audit"
)

// trayClient reads tray state from the local dashboard server and posts hold actions
// through it, so the tray sees the same truth as the web UI regardless of which process
// owns the daemon.
type trayClient struct {
	// base is the dashboard root, such as http://127.0.0.1:8787.
	base string
	// http performs the requests, injectable for tests.
	http *http.Client
}

// Per-call deadlines: probes and reads answer from memory and disk, while an action
// execs a vamoose command that may talk to a calendar provider, so it gets the same
// order of patience as the server's own 60-second exec timeout.
const (
	trayHealthTimeout = time.Second
	trayReadTimeout   = 3 * time.Second
	trayActionTimeout = 90 * time.Second
)

// newTrayClient returns a client for the dashboard server at addr (host:port).
// Deadlines are set per call, so a slow action does not inherit a read timeout.
func newTrayClient(addr string) *trayClient {
	return &trayClient{base: "http://" + addr, http: &http.Client{}}
}

// Health reports whether the dashboard server answers on loopback.
func (c *trayClient) Health(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, trayHealthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// Watches returns the holds the daemon is watching.
func (c *trayClient) Watches(ctx context.Context) ([]watchItem, error) {
	var out []watchItem
	err := c.getJSON(ctx, "/api/watches", &out)
	return out, err
}

// History returns the most recent run events, newest first, capped at n.
func (c *trayClient) History(ctx context.Context, n int) ([]audit.Event, error) {
	var all []audit.Event
	if err := c.getJSON(ctx, "/api/history", &all); err != nil {
		return nil, err
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all, nil
}

// Version returns the server's version string, empty on any error.
func (c *trayClient) Version(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, trayReadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/version", nil)
	if err != nil {
		return ""
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Action posts a check, promote, or cancel for a watched hold, returning an error
// when the server refuses or fails the action.
func (c *trayClient) Action(ctx context.Context, action, holdID, provider string) error {
	ctx, cancel := context.WithTimeout(ctx, trayActionTimeout)
	defer cancel()
	body, err := json.Marshal(map[string]string{
		"action": action, "holdID": holdID, "provider": provider,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/action", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("action %s: %s: %s", action, resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

// getJSON decodes a dashboard JSON response into out. The APIs return JSON null for an
// empty store, which decodes to the zero value, so callers see an empty slice.
func (c *trayClient) getJSON(ctx context.Context, path string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, trayReadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// titleCase capitalizes the first letter of an ASCII action name for a menu label.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}

// trayWatchLine formats one watched hold for the dropdown: the subject or hold id,
// then the workflow and the approver the gate waits on.
func trayWatchLine(w watchItem) string {
	name := w.Subject
	if name == "" {
		name = w.HoldID
	}
	line := fmt.Sprintf("%s  (%s", name, w.Workflow)
	if w.Approver != "" {
		line += " · awaiting " + w.Approver
	}
	return line + ")"
}

// trayEventLine formats one history event for the dropdown: the action, workflow,
// actor when present, and the minute the event happened.
func trayEventLine(e audit.Event) string {
	line := string(e.Action)
	if e.Workflow != "" {
		line += "  " + e.Workflow
	}
	if e.Actor != "" {
		line += " · " + e.Actor
	}
	if !e.Time.IsZero() {
		line += "  " + e.Time.Local().Format("2006-01-02 15:04")
	}
	return line
}

// trayChild is one spawned process and the signal that it has exited. The reaper
// goroutine owns cmd.Wait, so alive is answered from the channel rather than from
// ProcessState, which Wait writes concurrently.
type trayChild struct {
	// cmd is the running process.
	cmd *exec.Cmd
	// done is closed once the process has exited and been reaped.
	done chan struct{}
}

// alive reports whether the child is still running.
func (c *trayChild) alive() bool {
	if c == nil {
		return false
	}
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// stop kills the child and waits for the reaper to confirm the exit.
func (c *trayChild) stop() {
	if !c.alive() || c.cmd == nil {
		return
	}
	_ = c.cmd.Process.Kill()
	<-c.done
}

// trayServices owns the dashboard server and daemon the tray spawns on a cold start.
// When the server already answers, it attaches instead of fighting for the port; the
// daemon's watch-file lock guarantees a spawned duplicate exits cleanly on its own.
type trayServices struct {
	// addr is the dashboard address passed to a spawned server.
	addr string
	// server and daemon are the children this tray owns, nil when attached to
	// processes started elsewhere.
	server, daemon *trayChild
}

// Ensure starts the dashboard server and daemon when the server is not answering.
// It reports whether a spawn was attempted, so the caller can delay its next refresh
// to give the server time to bind.
func (s *trayServices) Ensure(ctx context.Context, c *trayClient) (bool, error) {
	if c.Health(ctx) {
		return false, nil
	}
	if s.server.alive() {
		return false, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("locate vamoose binary: %w", err)
	}
	s.server = traySpawn(exe, "app", "--no-open", "--addr", s.addr)
	if s.server == nil {
		return false, fmt.Errorf("start vamoose app: spawn failed")
	}
	if !s.daemon.alive() {
		s.daemon = traySpawn(exe, "daemon")
	}
	return true, nil
}

// Terminate stops the children this tray started, leaving attached processes alone.
func (s *trayServices) Terminate() {
	s.server.stop()
	s.daemon.stop()
}

// traySpawn starts the vamoose binary with args as an owned child, returning nil when
// the start fails. Output is discarded; the child is hidden from the desktop on
// platforms where a console window would otherwise appear.
func traySpawn(exe string, args ...string) *trayChild {
	cmd := exec.Command(exe, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = trayChildAttr()
	if err := cmd.Start(); err != nil {
		return nil
	}
	child := &trayChild{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(child.done)
	}()
	return child
}
