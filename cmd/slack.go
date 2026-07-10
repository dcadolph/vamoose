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
	"strings"
	"syscall"
	"time"

	"github.com/dcadolph/vamoose/internal/secret"
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
	runner := func(ctx context.Context, args, env []string) (string, error) {
		cmd := exec.CommandContext(ctx, exe, args...)
		cmd.Env = mergeEnv(os.Environ(), env)
		out, err := cmd.CombinedOutput()
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
	perUserOn := false
	if perUser := os.Getenv("VAMOOSE_SLACK_PER_USER"); perUser != "" && perUser != "0" {
		pub := os.Getenv("VAMOOSE_SLACK_PUBLIC_URL")
		if pub == "" {
			return fmt.Errorf("per-user mode requires VAMOOSE_SLACK_PUBLIC_URL for the OAuth callback")
		}
		links, lerr := slackUserLinkStore()
		if lerr != nil {
			return fmt.Errorf("slack user link store: %w", lerr)
		}
		var linkers []slack.Linker
		if id, sec := os.Getenv("VAMOOSE_GOOGLE_CLIENT_ID"), os.Getenv("VAMOOSE_GOOGLE_CLIENT_SECRET"); id != "" && sec != "" {
			linkers = append(linkers, newGoogleLinker(id, sec))
		}
		if id, sec := os.Getenv("VAMOOSE_CLIENT_ID"), os.Getenv("VAMOOSE_GRAPH_CLIENT_SECRET"); id != "" && sec != "" {
			tenant := os.Getenv("VAMOOSE_TENANT")
			if tenant == "" {
				tenant = "organizations"
			}
			linkers = append(linkers, newGraphLinker(tenant, id, sec))
		}
		// iCloud needs no server credentials: users submit an app-specific password
		// through a modal, which requires the workspace bot token from an install.
		linkers = append(linkers, icloudLinker{})
		if len(linkers) == 0 {
			return fmt.Errorf("per-user mode needs a provider configured, for example VAMOOSE_GOOGLE_CLIENT_ID and VAMOOSE_GOOGLE_CLIENT_SECRET")
		}
		opts = append(opts, slack.WithPublicURL(pub), slack.WithLinkers(links, linkers...), slack.WithPerUserEnv(slackUserWatchEnv))
		perUserOn = true
		logger.Printf("per-user mode: %d provider(s); each user runs /vamoose link <provider>", len(linkers))
	}
	srv := slack.NewServer(secret, runner, opts...)

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	if perUserOn {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					srv.PollUsers(ctx)
				}
			}
		}()
		logger.Printf("per-user auto-advance: polling watched holds every %s", time.Minute)
	}

	logger.Printf("listening on %s (slash: /slack/commands, interactivity: /slack/interactivity)", *addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("slack server: %w", err)
	}
	logger.Println("stopped")
	return nil
}

// slackTokenStore returns a file-backed store for per-workspace bot tokens under the
// user config directory, encrypted at rest when VAMOOSE_SECRET_KEY is set for a hosted
// server, otherwise a plaintext file.
func slackTokenStore() (*slack.FileStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	name := "slack-tokens.json"
	if os.Getenv(secret.KeyEnv) != "" {
		name = "slack-tokens.enc"
	}
	return slack.NewTokenStore(filepath.Join(dir, "vamoose", name))
}

// slackUserLinkStore returns a store for per-user calendar links, encrypted at rest
// when VAMOOSE_SECRET_KEY is set (for a hosted server), otherwise a plaintext file.
func slackUserLinkStore() (*slack.UserLinkFileStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	name := "slack-user-links.json"
	if os.Getenv(secret.KeyEnv) != "" {
		name = "slack-user-links.enc"
	}
	return slack.NewUserLinkStore(filepath.Join(dir, "vamoose", name))
}

// slackUserWatchEnv returns the VAMOOSE_WATCH_FILE environment for a linked user, so
// their watched holds live in their own file that the per-user poll loop advances
// with their credentials.
func slackUserWatchEnv(team, user string) []string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil
	}
	return []string{"VAMOOSE_WATCH_FILE=" + filepath.Join(dir, "vamoose", "watches", team+"-"+user+".json")}
}

// perUserEnvKeys are the environment variables the Slack server injects per user.
// They are stripped from the inherited environment so the injected values win, since
// duplicate keys are resolved inconsistently across platforms.
var perUserEnvKeys = []string{
	"VAMOOSE_PROVIDER",
	"VAMOOSE_GOOGLE_ACCESS_TOKEN",
	"VAMOOSE_GRAPH_ACCESS_TOKEN",
	"VAMOOSE_ICLOUD_USERNAME",
	"VAMOOSE_ICLOUD_APP_PASSWORD",
	"VAMOOSE_ICLOUD_CALENDAR",
}

// mergeEnv returns base with any per-user keys removed, followed by inject, so the
// injected per-user credentials take effect regardless of the server's environment.
func mergeEnv(base, inject []string) []string {
	if len(inject) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(inject))
	for _, kv := range base {
		drop := false
		for _, key := range perUserEnvKeys {
			if strings.HasPrefix(kv, key+"=") {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return append(out, inject...)
}
