package cli

import "github.com/spf13/cobra"

// This file defines the noun-first command groups that organize klim's CLI.
// Each parent below is a non-runnable umbrella command: invoking it bare
// prints its help (Cobra's default for a command with subcommands and no
// RunE). The actual leaf commands live in their own files and are wired
// into these parents — and the parents into rootCmd — from root.go, which
// is the single source of truth for the command tree.

// toolCmd groups commands that discover, inspect, install, and manage
// developer tools.
var toolCmd = &cobra.Command{
	Use:   "tool",
	Short: "Discover, install, and manage developer tools",
	Long: `Discover, install, inspect, and manage the developer tools klim knows about.

Subcommands:
  search    Search the tool marketplace
  install   Install tools or packs
  upgrade   Upgrade installed tools
  remove    Remove installed tools
  info      Show everything klim knows about a tool
  why       Explain why a tool is needed and where it's referenced
  graph     Visualize installed tools as a force-directed graph
  list      List installed tools with versions and update status
  try       Install a tool temporarily and run it
  onboard   Install a recommended set of tools for a role
  watch     Check for available tool updates
  catalog   Inspect and manage the local tool catalog cache`,
}

// projectCmd groups commands that operate on a project's .klim.yaml contract.
var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Work with a project's .klim.yaml contract",
	Long: `Create, check, and build on the .klim.yaml tool contract for a project.

Subcommands:
  init      Generate a .klim.yaml from the current project
  check     Check installed tools against .klim.yaml requirements
  generate  Generate CI / container configs from .klim.yaml`,
}

// planCmd groups the declarative "preview → apply → roll back" workflow.
// Bare `klim plan [tool...]` runs the preview (same as `klim plan show`);
// its flags are mirrored from planShowCmd in plan.go.
var planCmd = &cobra.Command{
	Use:   "plan [tool...]",
	Short: "Preview, apply, and roll back toolchain changes",
	Long: `Declarative workflow for changing your toolchain: preview what would
change, apply it with a safety net, and roll back to a checkpoint.

Run bare (` + "`klim plan`" + `) to preview pending changes — identical to
` + "`klim plan show`" + `.

Subcommands:
  show        Preview what klim would change (upgrades, installs, removes)
  apply       Apply pending changes with a checkpoint + postcheck safety net
  diff        Compare installed tools against a manifest or share token
  rollback    Plan a rollback to a saved checkpoint
  checkpoint  Save, list, show, and delete toolchain snapshots`,
	Args: cobra.ArbitraryArgs,
	RunE: runPlan,
}

// shareCmd groups commands that move a toolchain between machines and people.
// Bare `klim share` generates a share token (same as `klim share link`);
// its flags are mirrored from shareLinkCmd in share.go.
var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Export, import, and share your toolchain",
	Long: `Move your toolchain between machines and teammates.

Run bare (` + "`klim share`" + `) to generate a share token — identical to
` + "`klim share link`" + `.

Subcommands:
  export    Export installed tools to stdout, snapshots, or profiles
  import    Install tools from an exported manifest
  link      Share your toolchain as a compact token (and install from one)
  badge     Generate README badges for your toolchain`,
	Args: cobra.NoArgs,
	RunE: runShare,
}
