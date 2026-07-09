package cmd

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dcadolph/vamoose/internal/slack"
)

// runSlack starts the vamoose Slack server, which drives vamoose subcommands from
// slash commands. It needs VAMOOSE_SLACK_SIGNING_SECRET to verify Slack requests.
func runSlack(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("slack", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "Address to listen on")
	if err := fs.Parse(args); err != nil {
		return err
	}
	secret := os.Getenv("VAMOOSE_SLACK_SIGNING_SECRET")
	if secret == "" {
		return fmt.Errorf("VAMOOSE_SLACK_SIGNING_SECRET not set: copy it from your Slack app's Basic Information")
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	runner := func(ctx context.Context, args []string) (string, error) {
		out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput()
		return string(out), err
	}
	logger := log.New(os.Stderr, "vamoose slack: ", log.LstdFlags)

	var opts []slack.Option
	if id, sec, pub := os.Getenv("VAMOOSE_SLACK_CLIENT_ID"), os.Getenv("VAMOOSE_SLACK_CLIENT_SECRET"), os.Getenv("VAMOOSE_SLACK_PUBLIC_URL"); id != "" && sec != "" && pub != "" {
		store, serr := slackTokenStore()
		if serr != nil {
			return fmt.Errorf("slack token store: %w", serr)
		}
		opts = append(opts, slack.WithOAuth(id, sec, pub, store))
		logger.Printf("install flow enabled: %s/slack/install", pub)
	}
	srv := slack.NewServer(secret, runner, opts...)

	httpSrv := &http.Server{Addr: *addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	logger.Printf("listening on %s (slash: /slack/commands, interactivity: /slack/interactivity)", *addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("slack server: %w", err)
	}
	logger.Println("stopped")
	return nil
}

// slackTokenStore returns a file-backed store for per-workspace bot tokens under the
// user config directory.
func slackTokenStore() (*slack.FileStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return slack.NewFileStore(filepath.Join(dir, "vamoose", "slack-tokens.json")), nil
}
