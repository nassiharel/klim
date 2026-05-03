package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/service"
	"github.com/nassiharel/clim/internal/trail"
)

// --- root: clim trail ---

var trailCmd = &cobra.Command{
	Use:   "trail",
	Short: "Inspect your environment history (git for your toolchain)",
	Long: `clim trail records every captured environment state as a
content-addressed snapshot, exposing git-style history inspection.

Two captures of an identical toolchain share storage. Each capture
appends an Entry to the linear trail history.

Subcommands:
  capture   Manually record the current toolchain
  log       Show captured entries (newest first)
  show      Display the toolchain at a specific entry
  diff      Compare two entries (or one entry vs HEAD)
  prune     Trim the trail and garbage-collect orphan objects

A <ref> can be: HEAD, HEAD~N, @<index>, a content hash (full or 7+ char
prefix), or an entry's --label.`,
}

// --- clim trail capture ---

// trailCaptureFresh controls whether `clim trail capture` does a fresh
// PATH scan (default true) or reuses the on-disk scan cache. Default is
// true so captures match the user's current toolchain; pass
// `--refresh=false` to opt into cached behavior for back-to-back clim
// commands.
var (
	trailCaptureLabel string
	trailCaptureOp    string
	trailCaptureFresh = true
)

var trailCaptureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Record the current toolchain as a new trail entry",
	Long: `Scan installed tools and append a new entry to the trail.

If the resulting environment is identical to a previous capture, no
new object is stored on disk — only the new history entry is appended.
Pass --label to tag the entry for later reference (e.g. "before-upgrade").

By default capture forces a fresh PATH scan so the recorded snapshot
matches the toolchain you have right now (not whatever the scan cache
last saw). Pass --refresh=false to use cached scan data — useful only
when you've just run another clim command that already populated the
cache and want to capture the exact same view.`,
	Args: cobra.NoArgs,
	RunE: runTrailCapture,
}

// --- clim trail log ---

var (
	trailLogLimit  int
	trailLogSince  string
	trailLogOutput func() (OutputFormat, error)
)

var trailLogCmd = &cobra.Command{
	Use:   "log",
	Short: "Show trail entries (newest first)",
	Long: `List trail entries newest first.

  --limit N       cap output at N entries
  --since DUR     only entries newer than DUR ago (e.g. 7d, 24h, 30m)
  --output text|json`,
	Args: cobra.NoArgs,
	RunE: runTrailLog,
}

// --- clim trail show ---

var trailShowOutput func() (OutputFormat, error)

var trailShowCmd = &cobra.Command{
	Use:   "show <ref>",
	Short: "Display the toolchain at a specific entry",
	Args:  requireArgs(1, "clim trail show <ref>"),
	RunE:  runTrailShow,
}

// --- clim trail diff ---

var trailDiffOutput func() (OutputFormat, error)

var trailDiffCmd = &cobra.Command{
	Use:   "diff <ref> [<ref>]",
	Short: "Compare two trail entries (defaults the second arg to HEAD)",
	Long: `Show the change set between two entries.

  clim trail diff HEAD~1            # HEAD~1 vs HEAD
  clim trail diff HEAD~3 HEAD       # explicit two-arg form
  clim trail diff before-upgrade    # vs HEAD (label)`,
	Args: trailDiffArgs,
	RunE: runTrailDiff,
}

// trailDiffArgs accepts 1 or 2 args, returning a UsageError on any other
// count so wrong invocations exit 2 (not 1) per docs/cli-conventions.md.
func trailDiffArgs(cmd *cobra.Command, args []string) error {
	if len(args) >= 1 && len(args) <= 2 {
		return nil
	}
	return &UsageError{Err: fmt.Errorf(
		"requires 1 or 2 arguments, got %d\n\nUsage:\n  clim trail diff <ref> [<ref>]\n\nRun '%s --help' for more information",
		len(args), cmd.CommandPath())}
}

// --- clim trail prune ---

var (
	trailPruneKeep      int
	trailPruneOlderThan string
)

var trailPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Trim the trail and garbage-collect orphan objects",
	Long: `Drop trail entries beyond a retention window. Pass at least one of:

  --keep N          retain only the N newest entries
  --older-than DUR  drop entries older than DUR (e.g. 30d, 12h)

Both filters apply (AND). After log pruning, any object no entry
references is deleted from disk.`,
	Args: cobra.NoArgs,
	RunE: runTrailPrune,
}

func init() {
	// capture
	trailCaptureCmd.Flags().StringVar(&trailCaptureLabel, "label", "", "Optional label for the new entry (must be unique)")
	trailCaptureCmd.Flags().StringVar(&trailCaptureOp, "op", trail.OpCapture, "Operation kind (capture, install, upgrade, remove, import)")
	trailCaptureCmd.Flags().BoolVar(&trailCaptureFresh, "refresh", true, "Force a fresh PATH scan before capturing (default true; pass --refresh=false to reuse the scan cache)")

	// log
	trailLogCmd.Flags().IntVarP(&trailLogLimit, "limit", "n", 0, "Maximum number of entries to print (0 = no limit)")
	trailLogCmd.Flags().StringVar(&trailLogSince, "since", "", "Only entries newer than this duration ago (e.g. 7d, 24h)")
	trailLogOutput = addOutputFlag(trailLogCmd, OutputText, OutputJSON)

	// show
	trailShowOutput = addOutputFlag(trailShowCmd, OutputText, OutputJSON)

	// diff
	trailDiffOutput = addOutputFlag(trailDiffCmd, OutputText, OutputJSON)

	// prune
	trailPruneCmd.Flags().IntVar(&trailPruneKeep, "keep", 0, "Maximum number of newest entries to retain")
	trailPruneCmd.Flags().StringVar(&trailPruneOlderThan, "older-than", "", "Drop entries older than this duration (e.g. 30d)")

	trailCmd.AddCommand(trailCaptureCmd, trailLogCmd, trailShowCmd, trailDiffCmd, trailPruneCmd)
	// trailCmd is registered in root.go with its command group.
}

// --- runners ---

func runTrailCapture(cmd *cobra.Command, _ []string) error {
	// Validate flags up-front so bad input exits with the documented
	// usage code (2) rather than getting wrapped into a runtime error
	// after a possibly-slow PATH scan. Op + label structure are
	// checked here; duplicate-label collisions still happen inside
	// trail.Capture under the trail lock (we'd otherwise have to
	// double-acquire the lock just for early validation).
	if err := trail.ValidateOp(trailCaptureOp); err != nil {
		return &UsageError{Err: err}
	}
	if err := trail.ValidateLabel(trailCaptureLabel); err != nil {
		return &UsageError{Err: err}
	}

	// Word the spinner so it matches what actually happens. With
	// --refresh=true we always run a fresh PATH scan; with
	// --refresh=false the service may either reuse the on-disk scan
	// cache or fall through to a fresh scan if the cache is missing —
	// only the returned ScanInfo can tell us which one happened.
	startMsg := "Loading toolchain..."
	if trailCaptureFresh {
		startMsg = "Scanning installed tools..."
	}
	sp := progress.New(startMsg)
	tools, _, scanInfo, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), trailCaptureFresh)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	switch {
	case scanInfo != nil && scanInfo.Source == service.ScanSourceCache:
		sp.Done(fmt.Sprintf("Reused scan cache (%s)", humanizeAge(scanInfo.CacheAt)))
	default:
		sp.Done("Tools scanned")
	}

	entry, err := trail.Capture(trailCaptureOp, trailCaptureLabel, tools)
	if err != nil {
		return fmt.Errorf("capturing trail entry: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✓ Captured %s (%s)", entry.Object.Short(), entry.Operation)
	if entry.Label != "" {
		fmt.Fprintf(os.Stderr, " — %s", entry.Label)
	}
	if entry.Summary != "" {
		fmt.Fprintf(os.Stderr, "  [%s]", entry.Summary)
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

// humanizeAge renders how long ago t was, for short status messages.
func humanizeAge(t time.Time) string {
	if t.IsZero() {
		return "fresh"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "moments ago"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func runTrailLog(cmd *cobra.Command, _ []string) error {
	out, err := trailLogOutput()
	if err != nil {
		return err
	}

	if trailLogLimit < 0 {
		return &UsageError{Err: fmt.Errorf("--limit must be >= 0, got %d", trailLogLimit)}
	}

	opts := trail.LogOptions{Limit: trailLogLimit}
	if trailLogSince != "" {
		dur, err := parseTrailDuration(trailLogSince)
		if err != nil {
			return &UsageError{Err: fmt.Errorf("--since: %w", err)}
		}
		if dur < 0 {
			return &UsageError{Err: fmt.Errorf("--since must be a positive duration, got %q", trailLogSince)}
		}
		opts.Since = time.Now().Add(-dur)
	}

	entries, err := trail.Log(opts)
	if err != nil {
		return err
	}

	if out == OutputJSON {
		return printJSON(map[string]any{
			"entries": entries,
			"count":   len(entries),
		})
	}

	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "No trail entries yet. Run `clim trail capture` to record one.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "INDEX\tREF\tWHEN\tOP\tLABEL\tSUMMARY")
	_, _ = fmt.Fprintln(w, "-----\t---\t----\t--\t-----\t-------")
	for _, e := range entries {
		_, _ = fmt.Fprintf(w, "@%d\t%s\t%s\t%s\t%s\t%s\n",
			e.Index,
			e.Object.Short(),
			humanAgo(e.Time),
			e.Operation,
			truncate(e.Label, 24),
			truncate(e.Summary, 36),
		)
	}
	_ = w.Flush()
	return nil
}

type trailShowOutputJSON struct {
	Entry    trail.Entry     `json:"entry"`
	Snapshot *trail.Snapshot `json:"snapshot"`
}

func runTrailShow(cmd *cobra.Command, args []string) error {
	out, err := trailShowOutput()
	if err != nil {
		return err
	}
	snap, entry, err := trail.Show(args[0])
	if err != nil {
		return err
	}

	if out == OutputJSON {
		return printJSON(trailShowOutputJSON{Entry: *entry, Snapshot: snap})
	}

	fmt.Fprintf(os.Stderr, "Entry %s  (index %d)\n", entry.Object.Short(), entry.Index)
	fmt.Fprintf(os.Stderr, "  Time:    %s\n", entry.Time.Local().Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "  Op:      %s\n", entry.Operation)
	if entry.Label != "" {
		fmt.Fprintf(os.Stderr, "  Label:   %s\n", entry.Label)
	}
	if entry.Summary != "" {
		fmt.Fprintf(os.Stderr, "  Summary: %s\n", entry.Summary)
	}
	fmt.Fprintf(os.Stderr, "  Tools:   %d  (os=%s arch=%s)\n\n", len(snap.Tools), snap.OS, snap.Arch)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "TOOL\tVERSION\tSOURCE")
	_, _ = fmt.Fprintln(w, "----\t-------\t------")
	for _, t := range snap.Tools {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", t.Name, dashIfEmpty(t.Version), dashIfEmpty(t.Source))
	}
	_ = w.Flush()
	return nil
}

func runTrailDiff(cmd *cobra.Command, args []string) error {
	out, err := trailDiffOutput()
	if err != nil {
		return err
	}
	a := args[0]
	b := "HEAD"
	if len(args) == 2 {
		b = args[1]
	}
	d, err := trail.Diff(a, b)
	if err != nil {
		return err
	}
	if out == OutputJSON {
		return printJSON(map[string]any{
			"from": a,
			"to":   b,
			"diff": d,
		})
	}

	totalChanges := len(d.Added) + len(d.Removed) + len(d.VersionChanged) + len(d.SourceChanged)
	if totalChanges == 0 {
		fmt.Fprintf(os.Stderr, "%s == %s (no changes)\n", a, b)
		return nil
	}
	fmt.Fprintf(os.Stderr, "Diff %s → %s\n\n", a, b)
	if len(d.Added) > 0 {
		_, _ = fmt.Fprintln(os.Stdout, "  Added:")
		for _, t := range d.Added {
			_, _ = fmt.Fprintf(os.Stdout, "    + %s %s (%s)\n", t.Name, dashIfEmpty(t.Version), dashIfEmpty(t.Source))
		}
	}
	if len(d.Removed) > 0 {
		_, _ = fmt.Fprintln(os.Stdout, "  Removed:")
		for _, t := range d.Removed {
			_, _ = fmt.Fprintf(os.Stdout, "    - %s %s (%s)\n", t.Name, dashIfEmpty(t.Version), dashIfEmpty(t.Source))
		}
	}
	if len(d.VersionChanged) > 0 {
		_, _ = fmt.Fprintln(os.Stdout, "  Version changed:")
		for _, c := range d.VersionChanged {
			_, _ = fmt.Fprintf(os.Stdout, "    ~ %s %s → %s (%s)\n", c.Name, c.From, c.To, dashIfEmpty(c.Source))
		}
	}
	if len(d.SourceChanged) > 0 {
		_, _ = fmt.Fprintln(os.Stdout, "  Source changed:")
		for _, c := range d.SourceChanged {
			fromLabel := c.From
			toLabel := c.To
			if c.FromVersion != "" {
				fromLabel = fmt.Sprintf("%s@%s", c.From, c.FromVersion)
			}
			if c.ToVersion != "" {
				toLabel = fmt.Sprintf("%s@%s", c.To, c.ToVersion)
			}
			_, _ = fmt.Fprintf(os.Stdout, "    ⇄ %s %s → %s\n", c.Name, fromLabel, toLabel)
		}
	}
	return nil
}

func runTrailPrune(_ *cobra.Command, _ []string) error {
	if trailPruneKeep < 0 {
		return &UsageError{Err: fmt.Errorf("--keep must be >= 0, got %d", trailPruneKeep)}
	}
	if trailPruneKeep == 0 && trailPruneOlderThan == "" {
		return &UsageError{Err: errors.New("specify at least one of --keep or --older-than")}
	}
	opts := trail.PruneOptions{Keep: trailPruneKeep}
	if trailPruneOlderThan != "" {
		dur, err := parseTrailDuration(trailPruneOlderThan)
		if err != nil {
			return &UsageError{Err: fmt.Errorf("--older-than: %w", err)}
		}
		if dur <= 0 {
			return &UsageError{Err: fmt.Errorf("--older-than must be a positive duration, got %q", trailPruneOlderThan)}
		}
		opts.OlderThan = dur
	}
	res, err := trail.Prune(opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "✓ Pruned trail: %d entries kept (%d removed), %d objects kept (%d removed)\n",
		res.EntriesKept, res.EntriesRemoved, res.ObjectsKept, res.ObjectsRemoved)
	return nil
}

// --- helpers ---

// parseTrailDuration extends Go's time.ParseDuration with day suffixes.
func parseTrailDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "d") {
		days, err := time.ParseDuration(strings.TrimSuffix(s, "d") + "h")
		if err != nil {
			return 0, fmt.Errorf("invalid day duration %q", s)
		}
		return days * 24, nil
	}
	return time.ParseDuration(s)
}

func humanAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Local().Format("2006-01-02")
	}
}

// truncate shortens s to fit in n display columns, replacing the dropped
// suffix with "…". Operates on runes (not bytes), so non-ASCII labels and
// summaries can never be cut mid-codepoint and emit malformed UTF-8.
func truncate(s string, n int) string {
	if n <= 1 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
