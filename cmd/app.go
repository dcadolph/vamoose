package cmd

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// appUI holds the embedded local dashboard and its logo.
//
//go:embed appui/index.html appui/mark.png
var appUI embed.FS

// runApp serves a local web dashboard for vamoose and opens it in the browser. It binds to
// loopback, so the UI is reachable only from this machine, and guards every route against
// a non-loopback Host to blunt DNS rebinding.
func runApp(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("app", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8787", "Address to serve the local UI on")
	noOpen := fs.Bool("no-open", false, "Do not open the browser")
	if err := fs.Parse(args); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	index, err := appUI.ReadFile("appui/index.html")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if !localOnly(w, r) {
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if !localOnly(w, r) {
			return
		}
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		if !localOnly(w, r) {
			return
		}
		_, _ = io.WriteString(w, versionString())
	})
	mux.HandleFunc("GET /mark.png", func(w http.ResponseWriter, r *http.Request) {
		if !localOnly(w, r) {
			return
		}
		b, rerr := appUI.ReadFile("appui/mark.png")
		if rerr != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "max-age=86400")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("GET /api/workflows", appJSON(appWorkflows))
	mux.HandleFunc("GET /api/doctor", appJSON(appDoctor))
	mux.HandleFunc("GET /api/watches", appJSON(func() (any, error) { return loadWatches() }))
	mux.HandleFunc("GET /api/history", appJSON(func() (any, error) {
		store, serr := auditStore()
		if serr != nil {
			return nil, serr
		}
		return store.Events()
	}))
	mux.HandleFunc("POST /api/run", appRun(exe))
	mux.HandleFunc("POST /api/action", appAction(exe))
	mux.HandleFunc("GET /api/workflow", appWorkflowGet)
	mux.HandleFunc("POST /api/workflows/save", appWorkflowSave)
	mux.HandleFunc("POST /api/workflows/delete", appWorkflowDelete)

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sc, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(sc)
	}()

	url := "http://" + *addr
	fmt.Fprintf(os.Stdout, "vamoose app: open %s (Ctrl+C to stop)\n", url)
	if !*noOpen {
		openBrowser(url)
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("app server: %w", err)
	}
	return nil
}

// appWorkflow is a workflow summary with browser-friendly JSON keys.
type appWorkflow struct {
	// Name is the workflow name.
	Name string `json:"name"`
	// Description is the one-line summary.
	Description string `json:"description"`
	// Source is where the definition came from.
	Source string `json:"source"`
}

// appWorkflows lists the available workflows for the UI.
func appWorkflows() (any, error) {
	infos, err := workflowLoader().List()
	if err != nil {
		return nil, err
	}
	out := make([]appWorkflow, 0, len(infos))
	for _, in := range infos {
		out = append(out, appWorkflow{Name: in.Name, Description: in.Description, Source: string(in.Source)})
	}
	return out, nil
}

// appCheck is one setup check with browser-friendly JSON keys.
type appCheck struct {
	// Label describes what was checked.
	Label string `json:"label"`
	// OK reports whether the check passed.
	OK bool `json:"ok"`
	// Hint is a remedy shown when the check is missing.
	Hint string `json:"hint,omitempty"`
	// Optional marks an informational check.
	Optional bool `json:"optional"`
}

// appDoctor reports the configuration checks for the Setup page.
func appDoctor() (any, error) {
	checks := doctorChecks(os.Getenv)
	out := make([]appCheck, 0, len(checks))
	for _, c := range checks {
		out = append(out, appCheck(c))
	}
	return out, nil
}

// localOnly rejects a request that is not from this machine: its Host must be loopback,
// which blunts DNS rebinding, and any Origin header must be a loopback origin, which stops
// a cross-origin page in the user's browser from driving the server.
func localOnly(w http.ResponseWriter, r *http.Request) bool {
	if !isLoopbackHost(hostname(r.Host)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		if u, err := url.Parse(origin); err != nil || !isLoopbackHost(u.Hostname()) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return false
		}
	}
	return true
}

// hostname returns the host without its port, unwrapping an IPv6 address's brackets.
func hostname(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// isLoopbackHost reports whether host is a loopback name or address.
func isLoopbackHost(host string) bool {
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

// appJSON serves fn's result as JSON, guarded to loopback.
func appJSON(fn func() (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !localOnly(w, r) {
			return
		}
		v, err := fn()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
}

// appRun runs a workflow by shelling out to the vamoose binary and returns its output, so
// the UI reuses the exact CLI behavior.
func appRun(exe string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !localOnly(w, r) {
			return
		}
		var req struct {
			Workflow string `json:"workflow"`
			Phrase   string `json:"phrase"`
			Subject  string `json:"subject"`
			Manager  string `json:"manager"`
			Provider string `json:"provider"`
			DryRun   bool   `json:"dryRun"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Workflow == "" {
			http.Error(w, "a workflow is required", http.StatusBadRequest)
			return
		}
		args := []string{"run", req.Workflow}
		args = append(args, strings.Fields(req.Phrase)...)
		if req.Subject != "" {
			args = append(args, "--subject", req.Subject)
		}
		if req.Manager != "" {
			args = append(args, "--manager", req.Manager)
		}
		if req.Provider != "" {
			args = append(args, "--provider", req.Provider)
		}
		if req.DryRun {
			args = append(args, "--dry-run")
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput()
		resp := map[string]any{"output": string(out)}
		if err != nil {
			resp["error"] = err.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// appWorkflowGet returns a workflow's raw JSON definition and source, so the dashboard
// editor can show it as written.
func appWorkflowGet(w http.ResponseWriter, r *http.Request) {
	if !localOnly(w, r) {
		return
	}
	name := r.URL.Query().Get("name")
	data, source, err := workflowLoader().Raw(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"name": name, "source": string(source), "definition": string(data),
	})
}

// appWorkflowSave validates and saves a user workflow definition from the dashboard
// editor, through the same path as the workflows add command. A validation failure
// comes back as the response error text so the editor can show it.
func appWorkflowSave(w http.ResponseWriter, r *http.Request) {
	if !localOnly(w, r) {
		return
	}
	var req struct {
		Definition string `json:"definition"`
	}
	// A workflow definition is small; cap the body so a runaway request cannot balloon.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || strings.TrimSpace(req.Definition) == "" {
		http.Error(w, "a definition is required", http.StatusBadRequest)
		return
	}
	wf, err := saveUserWorkflow([]byte(req.Definition))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"name": wf.Name})
}

// appWorkflowDelete removes a user workflow from the dashboard. Built-ins are not
// files in the user directory, so they cannot be deleted, only shadowed.
func appWorkflowDelete(w http.ResponseWriter, r *http.Request) {
	if !localOnly(w, r) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "a name is required", http.StatusBadRequest)
		return
	}
	if err := removeUserWorkflow(req.Name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// appAction runs check, promote, or cancel on a watched hold by shelling out, so the
// dashboard can act on a hold. The action is allowlisted to those hold-scoped commands.
func appAction(exe string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !localOnly(w, r) {
			return
		}
		var req struct {
			Action   string `json:"action"`
			Provider string `json:"provider"`
			HoldID   string `json:"holdID"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.HoldID == "" {
			http.Error(w, "an action and hold id are required", http.StatusBadRequest)
			return
		}
		switch req.Action {
		case "check", "promote", "cancel":
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		args := []string{req.Action, "--id", req.HoldID}
		if req.Provider != "" {
			args = append(args, "--provider", req.Provider)
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput()
		resp := map[string]any{"output": string(out)}
		if err != nil {
			resp["error"] = err.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// openBrowser opens url in the default browser, best-effort per platform.
func openBrowser(url string) {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{url}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		name, args = "xdg-open", []string{url}
	}
	_ = exec.Command(name, args...).Start()
}
