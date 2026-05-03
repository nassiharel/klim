package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/web"
)

var (
	browserPort         int
	browserBind         string
	browserNoOpen       bool
	browserInsecureBind bool
	browserKeepAlive    bool
)

var browserCmd = &cobra.Command{
	Use:   "browser",
	Short: "Launch the local web UI",
	Long: `clim browser starts a local HTTP server and opens it in your default browser.

The server binds to 127.0.0.1 by default and refuses non-loopback bind
addresses unless --insecure-bind is set. When --insecure-bind exposes
the server on the network, clim auto-generates a 32-byte bearer token
and prints a ?token=<token> URL — the token is required (cookie or
Authorization: Bearer header) for every request other than /healthz.

The web UI mirrors the TUI's tabs (Installed, Updates, Discover,
Packs, For You, Favorites, Projects, Backup, Dashboard, Trail, Config)
and exposes a JSON API at /api/* with the same shape as the
corresponding CLI subcommands' --output json payloads.

By default the server shuts itself down 10 seconds after the last
browser tab closes; pass --keep-alive to keep it running.

Examples:
  clim browser                                  # auto-pick a free port, open browser
  clim browser --port 7777                      # bind a specific port
  clim browser --no-open                        # don't auto-launch the browser
  clim browser --keep-alive                     # don't auto-shutdown when tabs close
  clim browser --bind 0.0.0.0 --insecure-bind   # expose to LAN (token-authenticated)`,
	RunE: runBrowser,
}

func init() {
	browserCmd.Flags().IntVar(&browserPort, "port", 0, "listen port (0 = pick a free one)")
	browserCmd.Flags().StringVar(&browserBind, "bind", "127.0.0.1", "bind address")
	browserCmd.Flags().BoolVar(&browserNoOpen, "no-open", false, "do not open the browser automatically")
	browserCmd.Flags().BoolVar(&browserInsecureBind, "insecure-bind", false, "allow non-loopback bind addresses (auto-generates a bearer token)")
	browserCmd.Flags().BoolVar(&browserKeepAlive, "keep-alive", false, "keep the server running after the last browser tab closes (default: shut down)")
}

func runBrowser(cmd *cobra.Command, _ []string) error {
	authToken := ""
	// Auto-generate a bearer token for non-loopback binds. Without
	// auth, any device on the LAN that can reach the listener can
	// install / upgrade / remove tools. Loopback stays unauthenticated
	// so the default `clim browser` invocation behaves the same as the
	// TUI.
	if browserInsecureBind && !isLoopbackAddr(browserBind) {
		var err error
		authToken, err = web.GenerateAuthToken()
		if err != nil {
			return fmt.Errorf("generating auth token: %w", err)
		}
	}

	srv, err := web.New(web.Options{
		Bind:               browserBind,
		Port:               browserPort,
		InsecureBind:       browserInsecureBind,
		AuthToken:          authToken,
		Service:            svcFrom(cmd),
		Config:             cfgFrom(cmd),
		AutoShutdownOnIdle: !browserKeepAlive,
		AutoShutdownGrace:  10 * time.Second,
	})
	if err != nil {
		return err
	}
	url := srv.URL()
	authed := srv.AuthedURL()
	fmt.Fprintf(os.Stderr, "clim browser listening on %s\n", url)
	if authToken != "" {
		fmt.Fprintf(os.Stderr, "  ⚠ bound to a non-loopback address; auth token required\n")
		fmt.Fprintf(os.Stderr, "  open: %s\n", authed)
	} else if browserInsecureBind && !isLoopbackAddr(browserBind) {
		// Defensive: reach this only if a future change generates
		// no token but still allows non-loopback bind. Reuse the
		// shared isLoopbackAddr so all loopback spellings (including
		// ::1) are treated identically to the auth-token decision
		// just above.
		fmt.Fprintln(os.Stderr, "  ⚠ bound to a non-loopback address with NO authentication")
	}
	if !browserNoOpen {
		if err := web.OpenBrowser(authed); err != nil {
			fmt.Fprintf(os.Stderr, "  (could not auto-open browser: %v — visit %s manually)\n", err, authed)
		}
	}
	if browserKeepAlive {
		fmt.Fprintln(os.Stderr, "  press Ctrl-C to stop")
	} else {
		fmt.Fprintln(os.Stderr, "  press Ctrl-C to stop, or close the last browser tab")
	}

	// Bridge the OS signal channel into the cobra context so Serve's
	// graceful shutdown trips on SIGINT/SIGTERM. EnableAutoShutdown
	// hooks the same stop func so closing the last tab runs the same
	// shutdown path.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv.EnableAutoShutdown(stop)
	return srv.Serve(ctx)
}

// isLoopbackAddr is the CLI-side mirror of internal/web.isLoopback,
// used only to decide whether to generate an auth token.
func isLoopbackAddr(host string) bool {
	switch host {
	case "", "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}
