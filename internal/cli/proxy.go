package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/fileutil"
	"github.com/nassiharel/clim/internal/paths"
	"github.com/nassiharel/clim/internal/registry"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Manage auto-install shims for CLI tools",
	Long: `Create lightweight shims that auto-install tools on first use.

When you run a shimmed tool that isn't installed, clim automatically
installs it via the best available package manager, then runs it.

Subcommands:
  setup    Create the shims directory and show PATH instructions
  add      Create a shim for a tool
  remove   Remove a shim
  list     List active shims
  run      (internal) Find-or-install-then-exec a tool`,
}

var proxySetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Create the shims directory and show PATH instructions",
	RunE:  runProxySetup,
}

var proxyAddCmd = &cobra.Command{
	Use:   "add <tool> [tool...]",
	Short: "Create a shim for one or more tools",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runProxyAdd,
}

var proxyRemoveCmd = &cobra.Command{
	Use:   "remove <tool> [tool...]",
	Short: "Remove a shim for one or more tools",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runProxyRemove,
}

var proxyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active shims",
	RunE:  runProxyList,
}

var proxyRunCmd = &cobra.Command{
	Use:    "run <tool> [-- args...]",
	Short:  "Find or install a tool, then execute it",
	Hidden: true, // internal command used by shims
	Args:   cobra.MinimumNArgs(1),
	RunE:   runProxyRun,
}

func init() {
	proxyCmd.AddCommand(proxySetupCmd)
	proxyCmd.AddCommand(proxyAddCmd)
	proxyCmd.AddCommand(proxyRemoveCmd)
	proxyCmd.AddCommand(proxyListCmd)
	proxyCmd.AddCommand(proxyRunCmd)
	rootCmd.AddCommand(proxyCmd)
}

// shimsDir returns the path to the shims directory.
func shimsDir() (string, error) {
	return paths.ShimsDir()
}

func runProxySetup(cmd *cobra.Command, args []string) error {
	dir, err := shimsDir()
	if err != nil {
		return err
	}
	if err := fileutil.EnsureDir(filepath.Join(dir, "placeholder")); err != nil {
		return fmt.Errorf("creating shims directory: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✓ Shims directory created: %s\n\n", dir)
	fmt.Fprintf(os.Stderr, "Add it to your PATH (before other tool directories):\n\n")

	switch runtime.GOOS {
	case "windows":
		fmt.Fprintf(os.Stderr, "  PowerShell (current session):\n")
		fmt.Fprintf(os.Stderr, "    $env:PATH = \"%s;\" + $env:PATH\n\n", dir)
		fmt.Fprintf(os.Stderr, "  PowerShell (permanent):\n")
		fmt.Fprintf(os.Stderr, "    [Environment]::SetEnvironmentVariable('PATH', '%s;' + [Environment]::GetEnvironmentVariable('PATH', 'User'), 'User')\n\n", dir)
	default:
		fmt.Fprintf(os.Stderr, "  bash/zsh:\n")
		fmt.Fprintf(os.Stderr, "    export PATH=\"%s:$PATH\"\n", dir)
		fmt.Fprintf(os.Stderr, "    # Add to ~/.bashrc or ~/.zshrc for persistence\n\n")
		fmt.Fprintf(os.Stderr, "  fish:\n")
		fmt.Fprintf(os.Stderr, "    fish_add_path --prepend %s\n\n", dir)
	}

	fmt.Fprintf(os.Stderr, "Then create shims with:\n")
	fmt.Fprintf(os.Stderr, "  clim proxy add kubectl terraform helm\n")
	return nil
}

func runProxyAdd(cmd *cobra.Command, args []string) error {
	dir, err := shimsDir()
	if err != nil {
		return err
	}
	if err := fileutil.EnsureDir(filepath.Join(dir, "placeholder")); err != nil {
		return fmt.Errorf("creating shims directory: %w", err)
	}

	// Load catalog metadata to validate tool names (no PATH scan needed).
	tools, _, err := svc.Catalog.LoadTools(cmd.Context())
	if err != nil {
		return fmt.Errorf("loading catalog: %w", err)
	}
	toolMap := registry.ToolMap(tools)

	var created, skipped int
	for _, name := range args {
		t, ok := toolMap[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "⚠ %s: not found in catalog, skipping\n", name)
			skipped++
			continue
		}

		// Determine binary names.
		binNames := t.BinaryNames
		if len(binNames) == 0 {
			binNames = []string{name}
		}

		for _, bin := range binNames {
			if !isValidShimName(bin) {
				fmt.Fprintf(os.Stderr, "⚠ %s: invalid binary name, skipping\n", bin)
				continue
			}
			shimPath := shimFilePath(dir, bin)
			if _, err := os.Stat(shimPath); err == nil {
				fmt.Fprintf(os.Stderr, "  %s: shim already exists\n", bin)
				continue
			}
			content := generateShim(bin, name)
			if err := os.WriteFile(shimPath, []byte(content), 0755); err != nil {
				return fmt.Errorf("writing shim for %s: %w", bin, err)
			}
			fmt.Fprintf(os.Stderr, "✓ %s → shim created\n", bin)
			created++
		}
	}

	fmt.Fprintf(os.Stderr, "\n%d shim(s) created", created)
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, ", %d skipped", skipped)
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

func runProxyRemove(cmd *cobra.Command, args []string) error {
	dir, err := shimsDir()
	if err != nil {
		return err
	}

	var removed int
	for _, name := range args {
		shimPath := shimFilePath(dir, name)
		if err := os.Remove(shimPath); err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "  %s: no shim found\n", name)
			} else {
				fmt.Fprintf(os.Stderr, "⚠ %s: %v\n", name, err)
			}
			continue
		}
		fmt.Fprintf(os.Stderr, "✓ %s shim removed\n", name)
		removed++
	}
	fmt.Fprintf(os.Stderr, "%d shim(s) removed\n", removed)
	return nil
}

func runProxyList(cmd *cobra.Command, args []string) error {
	dir, err := shimsDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "No shims directory. Run 'clim proxy setup' first.")
			return nil
		}
		return err
	}

	var shims []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Strip extension.
		name = strings.TrimSuffix(name, ".cmd")
		name = strings.TrimSuffix(name, ".bat")
		shims = append(shims, name)
	}

	if len(shims) == 0 {
		fmt.Fprintln(os.Stderr, "No active shims. Create one with 'clim proxy add <tool>'.")
		return nil
	}

	sort.Strings(shims)
	fmt.Fprintf(os.Stderr, "%d active shim(s):\n", len(shims))
	for _, s := range shims {
		fmt.Fprintf(os.Stderr, "  %s\n", s)
	}
	return nil
}

func runProxyRun(cmd *cobra.Command, args []string) error {
	toolName := args[0]
	var toolArgs []string

	// Parse args after "--".
	for i, a := range args {
		if a == "--" && i+1 < len(args) {
			toolArgs = args[i+1:]
			break
		}
	}
	// If no "--" found, use remaining args.
	if toolArgs == nil && len(args) > 1 {
		toolArgs = args[1:]
	}

	dir, err := shimsDir()
	if err != nil {
		return fmt.Errorf("resolving shims directory: %w", err)
	}

	// Look for the real binary in PATH, excluding the shims directory.
	realPath := findRealBinary(toolName, dir)
	if realPath != "" {
		return execBinary(realPath, toolArgs)
	}

	// Not installed — load catalog definitions and install.
	fmt.Fprintf(os.Stderr, "[clim] %s is not installed. Installing...\n", toolName)

	tools, _, catErr := svc.Catalog.LoadTools(cmd.Context())
	if catErr != nil {
		return fmt.Errorf("loading catalog: %w", catErr)
	}
	toolMap := registry.ToolMap(tools)

	t, ok := toolMap[toolName]
	if !ok {
		return fmt.Errorf("[clim] %s not found in catalog", toolName)
	}

	// Find best available PM with a package ID.
	sources := registry.SourcesForOS()
	var installArgs []string
	var installSource registry.InstallSource
	for _, src := range sources {
		if ia := t.Packages.InstallArgs(src); ia != nil {
			installArgs = ia
			installSource = src
			break
		}
	}

	if installArgs == nil {
		return fmt.Errorf("[clim] no package manager available to install %s on %s", toolName, runtime.GOOS)
	}

	fmt.Fprintf(os.Stderr, "[clim] Installing via %s: %s\n", installSource, strings.Join(installArgs, " "))

	installCmd := exec.Command(installArgs[0], installArgs[1:]...)
	installCmd.Stdout = os.Stderr
	installCmd.Stderr = os.Stderr
	installCmd.Stdin = os.Stdin
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("[clim] installation failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "[clim] ✓ %s installed successfully\n\n", toolName)

	// Invalidate scan cache after install.
	_ = svc.InvalidateScanCache()

	// Find the real binary now.
	realPath = findRealBinary(toolName, dir)
	if realPath == "" {
		// Try the binary names from the catalog.
		for _, bin := range t.BinaryNames {
			realPath = findRealBinary(bin, dir)
			if realPath != "" {
				break
			}
		}
	}
	if realPath == "" {
		return fmt.Errorf("[clim] %s was installed but binary not found in PATH", toolName)
	}

	return execBinary(realPath, toolArgs)
}

// findRealBinary looks for a binary in PATH, excluding the shims directory.
func findRealBinary(name, excludeDir string) string {
	pathEnv := os.Getenv("PATH")
	dirs := filepath.SplitList(pathEnv)

	excludeNorm := normalizeDirPath(excludeDir)

	for _, dir := range dirs {
		dirNorm := normalizeDirPath(dir)
		if excludeNorm != "" && dirNorm == excludeNorm {
			continue
		}

		candidates := candidatePaths(dir, name)
		for _, cand := range candidates {
			if _, err := os.Stat(cand); err == nil {
				return cand
			}
		}
	}
	return ""
}

// normalizeDirPath cleans a directory path for comparison.
// On Windows, paths are lowercased for case-insensitive matching.
func normalizeDirPath(p string) string {
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

// candidatePaths returns possible executable paths for a binary name in a directory.
func candidatePaths(dir, name string) []string {
	if runtime.GOOS == "windows" {
		// On Windows, check with common extensions.
		exts := []string{".exe", ".cmd", ".bat", ".com", ""}
		pathExt := os.Getenv("PATHEXT")
		if pathExt != "" {
			exts = strings.Split(strings.ToLower(pathExt), ";")
		}
		var paths []string
		for _, ext := range exts {
			paths = append(paths, filepath.Join(dir, name+ext))
		}
		return paths
	}
	return []string{filepath.Join(dir, name)}
}

// execBinary runs a binary with the given args as a subprocess and
// propagates its exit code when available.
func execBinary(path string, args []string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

// shimFilePath returns the full path for a shim file.
func shimFilePath(dir, name string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, name+".cmd")
	}
	return filepath.Join(dir, name)
}

// generateShim creates the shim content for a given binary/tool.
// Tool names are validated before reaching here via isValidShimName.
func generateShim(binaryName, toolName string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("@echo off\r\nclim proxy run %q -- %%*\r\n", toolName)
	}
	return fmt.Sprintf("#!/bin/sh\nexec clim proxy run %q -- \"$@\"\n", toolName)
}

// isValidShimName checks that a name is a plain base name without path
// separators or traversal sequences.
func isValidShimName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	if filepath.Base(name) != name {
		return false
	}
	return true
}
