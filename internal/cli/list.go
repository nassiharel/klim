package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/scanner"
)

var noVersion bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all executables found on PATH with their versions",
	RunE:  runList,
}

func init() {
	listCmd.Flags().BoolVarP(&noVersion, "no-version", "q", false, "skip version detection (faster, shows only names and paths)")
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Phase A: scan PATH for all executables.
	fmt.Fprintln(os.Stderr, "Scanning PATH...")
	tools, err := scanner.ScanPATH(cfg)
	if err != nil {
		return fmt.Errorf("scanning PATH: %w", err)
	}

	if len(tools) == 0 {
		fmt.Fprintln(os.Stderr, "No executables found on PATH.")
		return nil
	}

	// Phase B: detect versions (unless --no-version).
	if !noVersion {
		fmt.Fprintf(os.Stderr, "Detecting versions for %d binaries...\n", len(tools))
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		timeout := time.Duration(cfg.EffectiveTimeout()) * time.Second
		detector.DetectAll(ctx, tools, timeout, runtime.NumCPU()*2)
	}

	// Phase C: render table.
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tPATH")
	fmt.Fprintln(w, "────\t───────\t────")

	for _, tool := range tools {
		ver := tool.Version
		if ver == "" {
			if noVersion {
				ver = "—"
			} else {
				ver = "(unknown)"
			}
		}

		path := tool.Path
		if len(path) > 50 {
			path = "..." + path[len(path)-47:]
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n", tool.Name, ver, path)
	}

	w.Flush()
	fmt.Fprintf(os.Stderr, "\n%d executables found.\n", len(tools))

	return nil
}
