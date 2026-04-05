package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/scanner"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all executables found on PATH",
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Scanning PATH...")
	tools, err := scanner.ScanPATH(cfg)
	if err != nil {
		return fmt.Errorf("scanning PATH: %w", err)
	}

	if len(tools) == 0 {
		fmt.Fprintln(os.Stderr, "No executables found on PATH.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tPATH")
	fmt.Fprintln(w, "────\t────")

	for _, tool := range tools {
		path := tool.Path
		if len(path) > 60 {
			path = "..." + path[len(path)-57:]
		}
		fmt.Fprintf(w, "%s\t%s\n", tool.Name, path)
	}

	w.Flush()
	fmt.Fprintf(os.Stderr, "\n%d executables found.\n", len(tools))

	return nil
}
