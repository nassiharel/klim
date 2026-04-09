package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// installPlan represents a single tool to install with its resolved command.
type installPlan struct {
	name    string
	display string
	cmdArgs []string
	source  string
}

// planSummary holds the classified results of analyzing a tool list.
type planSummary struct {
	toInstall        []installPlan
	alreadyInstalled []string
	noPackage        []string
	noPkgMgr         []string
	unknown          []string
}

// printPlanSummary prints a human-readable plan summary to stderr.
func printPlanSummary(title string, ps planSummary) {
	fmt.Fprintf(os.Stderr, "\n──── %s ────\n\n", title)

	if len(ps.alreadyInstalled) > 0 {
		fmt.Fprintf(os.Stderr, "  ✓ Already installed (%d):\n", len(ps.alreadyInstalled))
		for _, name := range ps.alreadyInstalled {
			fmt.Fprintf(os.Stderr, "    · %s\n", name)
		}
		fmt.Fprintln(os.Stderr)
	}
	if len(ps.unknown) > 0 {
		fmt.Fprintf(os.Stderr, "  ⚠ Not in catalog (%d):\n", len(ps.unknown))
		for _, name := range ps.unknown {
			fmt.Fprintf(os.Stderr, "    · %s\n", name)
		}
		fmt.Fprintln(os.Stderr)
	}
	if len(ps.noPackage) > 0 {
		fmt.Fprintf(os.Stderr, "  ⚠ No package for %s (%d):\n", runtime.GOOS, len(ps.noPackage))
		for _, name := range ps.noPackage {
			fmt.Fprintf(os.Stderr, "    · %s\n", name)
		}
		fmt.Fprintln(os.Stderr)
	}
	if len(ps.noPkgMgr) > 0 {
		fmt.Fprintf(os.Stderr, "  ⚠ No supported package manager (%d):\n", len(ps.noPkgMgr))
		for _, name := range ps.noPkgMgr {
			fmt.Fprintf(os.Stderr, "    · %s\n", name)
		}
		fmt.Fprintln(os.Stderr)
	}

	if len(ps.toInstall) > 0 {
		fmt.Fprintf(os.Stderr, "  To install (%d):\n", len(ps.toInstall))
		for _, p := range ps.toInstall {
			fmt.Fprintf(os.Stderr, "    · %-20s  via %s\n", p.display, p.source)
		}
		fmt.Fprintln(os.Stderr)
	}
}

// confirmInstall prompts the user for confirmation unless autoYes is true.
// Returns true if the user confirmed or autoYes is set.
func confirmInstall(autoYes bool) bool {
	if autoYes {
		return true
	}
	fmt.Fprint(os.Stderr, "  Proceed? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

// executeInstalls runs each install plan sequentially with live terminal output.
// Returns the number of successes and failures.
func executeInstalls(plans []installPlan) (succeeded, failed int) {
	for _, p := range plans {
		fmt.Fprintf(os.Stderr, "\n──── Installing %s via %s ────\n", p.display, p.source)

		c := exec.Command(p.cmdArgs[0], p.cmdArgs[1:]...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		if err := c.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s failed: %s\n", p.display, err)
			failed++
		} else {
			fmt.Fprintf(os.Stderr, "  ✓ %s installed\n", p.display)
			succeeded++
		}
	}
	return
}
