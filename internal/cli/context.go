package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/config"
	"github.com/nassiharel/klim/internal/service"
)

// cliCtx carries the runtime collaborators (config + service) that command
// runners need. It is bound to the command's context.Context in cli.Run, so
// every subcommand can retrieve it via cliCtxFrom(cmd.Context()).
//
// Tests can override the wired collaborators by constructing a cliCtx
// directly and calling withCLICtx(ctx, &cliCtx{...}) on a command's context.
type cliCtx struct {
	Svc            *service.ToolService
	Cfg            *config.Config
	ConfigWarnings []string
}

type cliCtxKey struct{}

// newCLICtx loads config (lenient on read failure — defaults are used and a
// warning is printed to stderr) and constructs the wired ToolService.
func newCLICtx() *cliCtx {
	cfg, warnings, err := config.LoadWithWarnings()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		cfg = config.Default()
		warnings = nil
	}
	return &cliCtx{
		Svc:            service.NewWithConfig(cfg),
		Cfg:            cfg,
		ConfigWarnings: warnings,
	}
}

// withCLICtx returns a new context carrying c.
func withCLICtx(ctx context.Context, c *cliCtx) context.Context {
	return context.WithValue(ctx, cliCtxKey{}, c)
}

// cliCtxFrom retrieves the cliCtx bound to ctx, or panics if none was set.
// The panic is intentional: every command should run inside a context that
// has been initialized by cli.Run, and a missing cliCtx indicates a wiring
// bug rather than a user-recoverable condition.
func cliCtxFrom(ctx context.Context) *cliCtx {
	c, ok := ctx.Value(cliCtxKey{}).(*cliCtx)
	if !ok {
		panic("cli: cliCtx not found in context — command was not invoked through cli.Run")
	}
	return c
}

// svcFrom is a short alias for the service in cmd's context.
func svcFrom(cmd *cobra.Command) *service.ToolService {
	return cliCtxFrom(cmd.Context()).Svc
}

// cfgFrom is a short alias for the config in cmd's context.
func cfgFrom(cmd *cobra.Command) *config.Config {
	return cliCtxFrom(cmd.Context()).Cfg
}
