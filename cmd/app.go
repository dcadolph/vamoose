package cmd

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// appUI holds the embedded local dashboard.
//
//go:embed appui/index.html
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
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		if !localOnly(w, r) {
			return
		}
		_, _ = io.WriteString(w, versionString())
	})
	mux.HandleFunc("GET /api/workflows", appJSON(appWorkflows))
	mux.HandleFunc("GET /api/watches", appJSON(func() (any, error) { return loadWatches() }))
	mux.HandleFunc("GET /api/history", appJSON(func() (any, error) {
		store, serr := auditStore()
		if serr != nil {
			return nil, serr
		}
		return store.Events()
	}))
	mux.HandleFunc("POST /api/run", appRun(exe))

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

// localOnly rejects a request whose Host is not loopback, so a page in the user's browser
// cannot reach this local server by rebinding a hostname to 127.0.0.1.
func localOnly(w http.ResponseWriter, r *http.Request) bool {
	host := r.Host
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	switch host {
	case "127.0.0.1", "localhost", "::1", "[::1]":
		return true
	}
	http.Error(w, "forbidden", http.StatusForbidden)
	return false
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
