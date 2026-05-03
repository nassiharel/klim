package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/web"
)

var (
	browserPort         int
	browserBind         string
	browserNoOpen       bool
	browserInsecureBind bool
)

var browserCmd = &cobra.Command{
	Use:   "browser",
	Short: "Launch the local web UI",
	Long: `clim browser starts a local HTTP server and opens it in your default browser.

The server binds to 127.0.0.1 by default and refuses non-loopback bind
addresses unless --insecure-bind is set. The web UI mirrors the TUI's
read-only views (Installed, Tool detail, Dashboard, Trail) and exposes a
JSON API at /api/* with the same shape as the corresponding CLI
subcommands' --output json payloads.

Examples:
  clim browser                       # auto-pick a free port, open browser
  clim browser --port 7777           # bind a specific port
  clim browser --no-open             # don't auto-launch the browser
  clim browser --bind 0.0.0.0 --insecure-bind   # share on LAN (no auth!)`,
	RunE: runBrowser,
}

func init() {
	browserCmd.Flags().IntVar(&browserPort, "port", 0, "listen port (0 = pick a free one)")
	browserCmd.Flags().StringVar(&browserBind, "bind", "127.0.0.1", "bind address")
	browserCmd.Flags().BoolVar(&browserNoOpen, "no-open", false, "do not open the browser automatically")
	browserCmd.Flags().BoolVar(&browserInsecureBind, "insecure-bind", false, "allow non-loopback bind addresses (no auth — use with caution)")
}

func runBrowser(cmd *cobra.Command, _ []string) error {
	srv, err := web.New(web.Options{
		Bind:         browserBind,
		Port:         browserPort,
		InsecureBind: browserInsecureBind,
		Service:      svcFrom(cmd),
	})
	if err != nil {
		return err
	}
	url := srv.URL()
	fmt.Fprintf(os.Stderr, "clim browser listening on %s\n", url)
	if browserInsecureBind && browserBind != "127.0.0.1" && browserBind != "localhost" {
		fmt.Fprintln(os.Stderr, "  ⚠ bound to a non-loopback address; the server has NO authentication")
	}
	if !browserNoOpen {
		if err := web.OpenBrowser(url); err != nil {
			fmt.Fprintf(os.Stderr, "  (could not auto-open browser: %v — visit %s manually)\n", err, url)
		}
	}
	fmt.Fprintln(os.Stderr, "  press Ctrl-C to stop")

	// Bridge the OS signal channel into the cobra context so Serve's
	// graceful shutdown trips on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return srv.Serve(ctx)
}
