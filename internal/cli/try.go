package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/registry"
)

var tryCmd = &cobra.Command{
	Use:   "try <tool> [-- args...]",
	Short: "Install a tool temporarily and run it",
	Long: `Install a tool, run it with optional arguments, then offer to remove it.

This lets you try tools without committing to a permanent install.
After the tool exits, clim asks whether to keep or remove it.

Examples:
  clim try bat                       # install bat, open a shell, then offer cleanup
  clim try bat -- README.md          # install bat, run 'bat README.md', then offer cleanup
  clim try ripgrep -- -i "TODO" .    # install ripgrep, search, then offer cleanup`,
	Args: cobra.MinimumNArgs(1),
	RunE: runTry,
}

var tryKeepFlag bool

func init() {
	tryCmd.Flags().BoolVar(&tryKeepFlag, "keep", false, "Keep the tool after trying (skip removal prompt)")
	rootCmd.AddCommand(tryCmd)
}

func runTry(cmd *cobra.Command, args []string) error {
	toolName := args[0]
	var toolArgs []string

	// Parse args after "--". Cobra does not include the literal "--" in args,
	// so prefer its recorded split point and fall back to forwarding any
	// remaining args after the tool name.
	if dashAt := cmd.ArgsLenAtDash(); dashAt > 0 && dashAt < len(args) {
		toolArgs = args[dashAt:]
	} else if len(args) > 1 {
		toolArgs = args[1:]
	}

	// Load catalog.
	sp := progress.New("Loading catalog...")
	tools, _, _, err := svc.LoadAndResolveCached(cmd.Context(), false)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Ready")

	toolMap := registry.ToolMap(tools)
	t, ok := toolMap[toolName]
	if !ok {
		return fmt.Errorf("%s not found in catalog", toolName)
	}

	// Check if already installed.
	alreadyInstalled := t.IsInstalled()
	if alreadyInstalled {
		fmt.Fprintf(os.Stderr, "%s is already installed.\n", t.DisplayName)
	}

	// Install if needed.
	var installSource registry.InstallSource
	if !alreadyInstalled {
		sources := registry.SourcesForOS()
		var installArgs []string
		for _, src := range sources {
			if ia := t.Packages.InstallArgs(src); ia != nil {
				installArgs = ia
				installSource = src
				break
			}
		}
		if installArgs == nil {
			return fmt.Errorf("no package manager available to install %s on %s", toolName, runtime.GOOS)
		}

		fmt.Fprintf(os.Stderr, "Installing %s via %s...\n", t.DisplayName, installSource)
		c := exec.Command(installArgs[0], installArgs[1:]...)
		c.Stdout = os.Stderr
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		if err := c.Run(); err != nil {
			return fmt.Errorf("installation failed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "✓ %s installed\n\n", t.DisplayName)
	}

	// Run the tool.
	if len(toolArgs) > 0 {
		fmt.Fprintf(os.Stderr, "Running: %s %s\n\n", toolName, strings.Join(toolArgs, " "))
		c := exec.Command(toolName, toolArgs...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin

		// Handle Ctrl+C gracefully.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		go func() {
			<-sigCh
			// Let the child process handle it.
		}()

		_ = c.Run()
		signal.Stop(sigCh)
		fmt.Fprintln(os.Stderr)
	} else {
		fmt.Fprintf(os.Stderr, "%s is ready. Run it manually, then come back here.\n", t.DisplayName)
		fmt.Fprintf(os.Stderr, "Press Enter when done...")
		fmt.Fscanln(os.Stdin)
	}

	// Cleanup prompt (unless --keep or was already installed).
	if alreadyInstalled || tryKeepFlag {
		return nil
	}

	fmt.Fprint(os.Stderr, "Keep "+t.DisplayName+"? [Y/n]: ")
	var answer string
	fmt.Fscan(os.Stdin, &answer)
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "" || answer == "y" || answer == "yes" {
		fmt.Fprintf(os.Stderr, "✓ Keeping %s.\n", t.DisplayName)
		return nil
	}

	// Remove.
	src := installSource
	if src == "" && t.PrimaryInstance() != nil {
		src = t.PrimaryInstance().Source
	}
	removeArgs := t.Packages.RemoveArgs(src)
	if removeArgs == nil {
		fmt.Fprintf(os.Stderr, "⚠ Cannot auto-remove %s — remove manually.\n", t.DisplayName)
		return nil
	}

	fmt.Fprintf(os.Stderr, "Removing %s...\n", t.DisplayName)
	c := exec.Command(removeArgs[0], removeArgs[1:]...)
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Removal failed: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "✓ %s removed.\n", t.DisplayName)
	}

	_ = svc.InvalidateScanCache()
	return nil
}
