package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ANSI color codes used by the colorized root help template.
const (
	cReset = "\033[0m"
	cBold  = "\033[1m"
	cTeal  = "\033[38;5;37m"
	cGreen = "\033[38;5;78m"
	cWhite = "\033[38;5;15m"
	cGray  = "\033[38;5;244m"
)

// initColorHelp installs a colorized help template on rootCmd. Subcommands
// continue to use Cobra's default help so that piped/captured output (e.g.
// `clim list --help | less`) is unaffected by color codes.
func initColorHelp() {
	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd != rootCmd {
			defaultHelp(cmd, args)
			return
		}
		w := cmd.OutOrStdout()
		p := func(format string, a ...any) {
			_, _ = fmt.Fprintf(w, format, a...)
		}

		// Brand header.
		brand := cBold + cWhite + "\033[48;5;37m" + " clim " + cReset
		p("\n  %s  %s\n\n", brand, cmd.Short)

		// Description.
		if cmd.Long != "" {
			p("%s%s%s\n\n", cGray, cmd.Long, cReset)
		}

		// Command groups.
		for _, g := range cmd.Groups() {
			p("%s%s%s%s\n", cBold, cTeal, g.Title, cReset)
			for _, c := range cmd.Commands() {
				if c.GroupID == g.ID && c.IsAvailableCommand() {
					p("  %s%-16s%s%s%s%s\n",
						cGreen, c.Name(), cReset,
						cGray, c.Short, cReset)
				}
			}
			p("\n")
		}

		// Ungrouped commands.
		var ungrouped []*cobra.Command
		for _, c := range cmd.Commands() {
			if c.IsAvailableCommand() && c.GroupID == "" {
				ungrouped = append(ungrouped, c)
			}
		}
		if len(ungrouped) > 0 {
			p("%s%sAdditional Commands:%s\n", cBold, cTeal, cReset)
			for _, c := range ungrouped {
				p("  %s%-16s%s%s%s%s\n",
					cGreen, c.Name(), cReset,
					cGray, c.Short, cReset)
			}
			p("\n")
		}

		// Flags (includes persistent flags like --verbose).
		if cmd.HasAvailableFlags() {
			p("%s%sFlags:%s\n", cBold, cTeal, cReset)
			p("%s\n", cmd.Flags().FlagUsages())
		}

		// Usage hint.
		p("Use \"%sclim [command] --help%s\" for more information.\n\n",
			cGreen, cReset)
	})
}
