package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/custompacks"
	"github.com/nassiharel/clim/internal/envid"
	"github.com/nassiharel/clim/internal/favorites"
	"github.com/nassiharel/clim/internal/manifest"
	"github.com/nassiharel/clim/internal/registry"
)

// envCmd captures and reproduces a clim-managed environment.
//
// The same payload has two encodings:
//   - clim env (no args)  → compact base64 token for chat
//   - clim env --output yaml → human-readable YAML for git
//
// Receivers can decode with `clim env show <token-or-file>` and
// reproduce with `clim env apply <token-or-file>`.
var envCmd = &cobra.Command{
	Use:   "env [flags]",
	Short: "Generate and apply environment fingerprints",
	Long: `clim env captures the shape of your clim-managed environment
into a portable artifact you can share via chat or commit to git.

A profile contains:
  - installed tools (name, version, install source, category)
  - favorites
  - custom packs you've defined
  - which package managers are available on this host
  - clim version + commit
  - OS, architecture, and (best-effort) distro
  - observational audit/security counts

Privacy: the profile is deterministic from environment state alone
(plus a timestamp). It deliberately does NOT include hostname,
username, absolute paths, environment variables, or any file
contents.

Examples:
  # Print the token for the current environment (paste into chat).
  clim env

  # Write the rich YAML form to a file.
  clim env --output yaml > my-env.yaml

  # Decode someone else's token without applying it.
  clim env show 'clim:env:v1:H4sIAAAAAA...'

  # Diff a coworker's env against yours.
  clim env diff 'clim:env:v1:H4sIAAAAAA...'

  # Reproduce the env locally — installs missing tools, sets favorites,
  # registers custom packs. Cross-OS gaps are reported, never errors.
  clim env apply 'clim:env:v1:H4sIAAAAAA...'`,
	Args: cobra.NoArgs, // subcommands take args; bare 'clim env' must not silently swallow extras.
	RunE: runEnvIDPrint,
}

var (
	envOutputFmt func() (OutputFormat, error)
)

func init() {
	envOutputFmt = addOutputFlag(envCmd, OutputText, OutputJSON, OutputYAML)
	envCmd.AddCommand(envShowCmd)
	envCmd.AddCommand(envDiffCmd)
	envCmd.AddCommand(envApplyCmd)
}

// runEnvIDPrint generates a Profile from the live system and emits it
// in the requested format.
//
//   - text  → compact `clim:env:v1:...` token to stdout (so users can
//     pipe into pbcopy / xclip / wl-copy without ceremony).
//   - json  → JSON document to stdout.
//   - yaml  → YAML document to stdout.
//
// Human progress lines go to stderr per docs/cli-conventions.md.
func runEnvIDPrint(cmd *cobra.Command, _ []string) error {
	out, err := envOutputFmt()
	if err != nil {
		return err
	}
	cfg := cfgFrom(cmd)
	svc := svcFrom(cmd)

	p, err := envid.Build(cmd.Context(), svc, cfg, envid.BuildOptions{})
	if err != nil {
		return err
	}

	switch out {
	case OutputJSON:
		if err := printJSON(p); err != nil {
			return err
		}
	case OutputYAML:
		data, err := yaml.Marshal(p)
		if err != nil {
			return fmt.Errorf("marshalling yaml: %w", err)
		}
		if _, err := os.Stdout.Write(data); err != nil {
			return err
		}
	default:
		token, err := envid.Encode(p)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(os.Stdout, token); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "\n  hash: %s   tools: %d   favorites: %d   packs: %d\n",
		p.Hash, len(p.Tools), len(p.Favorites), len(p.Packs))
	return nil
}

// envShowCmd pretty-prints a profile for inspection.
var envShowCmd = &cobra.Command{
	Use:   "show <token-or-file>",
	Short: "Pretty-print an env token or file",
	Long: `Decode a clim:env:v1:... token (or read a YAML file) and print
its contents. No installs, no side effects — handy for previewing a
coworker's env before applying it.`,
	Args: cobra.ExactArgs(1),
	RunE: runEnvIDShow,
}

func runEnvIDShow(_ *cobra.Command, args []string) error {
	p, err := loadProfile(args[0])
	if err != nil {
		return err
	}
	renderProfileText(os.Stderr, p)
	return nil
}

// envDiffCmd compares a remote profile against the local one.
var envDiffCmd = &cobra.Command{
	Use:   "diff <token-or-file>",
	Short: "Compare another env against the current environment",
	Long: `Decode an env and report what would change if you applied
it: tools they have that you don't, tools you have that they don't,
version drift, and per-section deltas (favorites, custom packs).`,
	Args: cobra.ExactArgs(1),
	RunE: runEnvIDDiff,
}

func runEnvIDDiff(cmd *cobra.Command, args []string) error {
	remote, err := loadProfile(args[0])
	if err != nil {
		return err
	}
	cfg := cfgFrom(cmd)
	svc := svcFrom(cmd)
	local, err := envid.Build(cmd.Context(), svc, cfg, envid.BuildOptions{})
	if err != nil {
		return err
	}
	renderProfileDiff(os.Stderr, local, remote)
	return nil
}

// envApplyCmd reproduces a remote profile locally.
var envApplyCmd = &cobra.Command{
	Use:   "apply <token-or-file>",
	Short: "Reproduce another env locally",
	Long: `Install missing tools, set favorites, and register custom
packs from a remote env. Cross-OS / cross-PM gaps (e.g. apt-only
tools on Windows) surface in the report as 'skipped' — they're never
errors. Use --yes to skip the install confirmation prompt.`,
	Args: cobra.ExactArgs(1),
	RunE: runEnvIDApply,
}

func init() {
	envApplyCmd.Flags().BoolVar(&yesFlag, "yes", false, "Skip confirmation prompts")
}

func runEnvIDApply(cmd *cobra.Command, args []string) error {
	p, err := loadProfile(args[0])
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\n──── Applying env %s ────\n\n", p.Hash)
	fmt.Fprintf(os.Stderr, "  Source clim:    %s\n", p.Clim.Version)
	fmt.Fprintf(os.Stderr, "  Source OS:      %s/%s\n", p.OS.GOOS, p.OS.Arch)
	fmt.Fprintf(os.Stderr, "  Tools:          %d\n", len(p.Tools))
	fmt.Fprintf(os.Stderr, "  Favorites:      %d\n", len(p.Favorites))
	fmt.Fprintf(os.Stderr, "  Custom packs:   %d\n\n", len(p.Packs))

	// 1. Tools — delegate to the existing import plan / install flow.
	failed := 0
	if len(p.Tools) > 0 {
		f, err := applyTools(cmd, p)
		if err != nil {
			return err
		}
		failed = f
	}

	// 2. Favorites — additive merge.
	if err := applyFavorites(p.Favorites); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ favorites: %v\n", err)
	}

	// 3. Custom packs — additive: existing packs with the same name
	// are preserved (the user must explicitly delete them first to
	// replace). Documented in env.md so this isn't surprising.
	if err := applyPacks(p.Packs); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ packs: %v\n", err)
	}

	if failed > 0 {
		return &PartialFailureError{Op: "env apply", Succeeded: len(p.Tools) - failed, Failed: failed}
	}
	return nil
}

// loadProfile reads either a token (clim:env:v1:...) or a file path.
// Distinguishing is unambiguous because tokens always start with the
// fixed prefix.
//
// Decode errors that are clearly user-caused (malformed token, empty
// token, schema-version mismatch) are wrapped in *UsageError so they
// exit with ExitUsage (2) per the documented contract; other errors
// (file I/O, unmarshal of a corrupted YAML on disk) propagate as
// runtime errors (ExitRuntime 1).
func loadProfile(arg string) (*envid.Profile, error) {
	if strings.HasPrefix(strings.TrimSpace(arg), "clim:env:") {
		p, err := envid.Decode(arg)
		if err != nil && isUserCausedDecodeError(err) {
			return nil, usageErrorf("invalid env token: %v", err)
		}
		return p, err
	}
	return envid.ReadFile(arg)
}

// isUserCausedDecodeError returns true when err matches one of the
// envid sentinels that signal a malformed input (as opposed to an
// internal/I/O failure). Used to map to ExitUsage instead of
// ExitRuntime.
//
// ErrCorruptToken covers tampered base64 / gzip / yaml — all of
// which are user-caused (someone edited or pasted a bad token), so
// they belong in the usage-error bucket.
func isUserCausedDecodeError(err error) bool {
	for _, sentinel := range []error{
		envid.ErrInvalidToken, envid.ErrEmptyToken,
		envid.ErrUnknownVersion, envid.ErrTokenTooLarge,
		envid.ErrPayloadTooLarge, envid.ErrSchemaMismatch,
		envid.ErrCorruptToken,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}

func renderProfileText(w io.Writer, p *envid.Profile) {
	_, _ = fmt.Fprintf(w, "\n──── env %s ────\n\n", p.Hash)
	_, _ = fmt.Fprintf(w, "  clim version:    %s\n", p.Clim.Version)
	if p.Clim.Commit != "" {
		_, _ = fmt.Fprintf(w, "  clim commit:     %s\n", p.Clim.Commit)
	}
	_, _ = fmt.Fprintf(w, "  generated:       %s\n", p.GeneratedAt.Format("2006-01-02 15:04 MST"))
	_, _ = fmt.Fprintf(w, "  OS:              %s/%s", p.OS.GOOS, p.OS.Arch)
	if p.OS.Distro != "" {
		_, _ = fmt.Fprintf(w, "  (%s)", p.OS.Distro)
	}
	_, _ = fmt.Fprintln(w)

	if len(p.PackageManagers) > 0 {
		keys := sortedMapKeys(p.PackageManagers)
		var avail, miss []string
		for _, k := range keys {
			if p.PackageManagers[k] {
				avail = append(avail, k)
			} else {
				miss = append(miss, k)
			}
		}
		_, _ = fmt.Fprintf(w, "  package mgrs:    available=%s   missing=%s\n",
			joinOrDash(avail), joinOrDash(miss))
	}

	if len(p.Tools) > 0 {
		_, _ = fmt.Fprintf(w, "\n  Tools (%d):\n", len(p.Tools))
		for _, t := range p.Tools {
			ver := t.Version
			if ver == "" {
				ver = "?"
			}
			src := t.Source
			if src == "" {
				src = "?"
			}
			_, _ = fmt.Fprintf(w, "    · %-25s %-12s via %s\n", t.Name, ver, src)
		}
	}
	if len(p.Favorites) > 0 {
		_, _ = fmt.Fprintf(w, "\n  Favorites (%d): %s\n", len(p.Favorites), strings.Join(p.Favorites, ", "))
	}
	if len(p.Packs) > 0 {
		_, _ = fmt.Fprintf(w, "\n  Custom packs (%d):\n", len(p.Packs))
		for _, pk := range p.Packs {
			_, _ = fmt.Fprintf(w, "    · %-20s [%s]\n", pk.Name, strings.Join(pk.Tools, ", "))
		}
	}
	_, _ = fmt.Fprintf(w, "\n  Audit:           %d warnings, %d infos\n", p.Security.AuditWarnings, p.Security.AuditInfos)
	_, _ = fmt.Fprintf(w, "  Verdicts:        clean=%d watch=%d risk=%d unknown=%d\n",
		p.Security.Verdicts.Clean, p.Security.Verdicts.Watch,
		p.Security.Verdicts.Risk, p.Security.Verdicts.Unknown)
	_, _ = fmt.Fprintln(w)
}

// renderProfileDiffTo is the io.Writer-flavored variant used by
// tests; production callers go through renderProfileDiff which
// passes os.Stderr.
func renderProfileDiffTo(w io.Writer, local, remote *envid.Profile) {
	renderProfileDiff(w, local, remote)
}

func renderProfileDiff(w io.Writer, local, remote *envid.Profile) {
	// Recompute both hashes from the decoded content so user-
	// editable inputs (a hand-tweaked YAML, a forged token) can't
	// claim "match" by lying in the hash field. The local profile
	// was just built by Build (already trusted), but recomputing
	// is cheap and keeps the comparison symmetric.
	localHash := envid.ComputeHash(local)
	remoteHash := envid.ComputeHash(remote)

	_, _ = fmt.Fprintf(w, "\n──── env diff ────\n")
	_, _ = fmt.Fprintf(w, "  local:  %s   tools=%d favorites=%d packs=%d\n",
		localHash, len(local.Tools), len(local.Favorites), len(local.Packs))
	_, _ = fmt.Fprintf(w, "  remote: %s   tools=%d favorites=%d packs=%d\n",
		remoteHash, len(remote.Tools), len(remote.Favorites), len(remote.Packs))
	if remote.Hash != "" && remote.Hash != remoteHash {
		_, _ = fmt.Fprintf(w, "  ⚠ remote claimed hash %s; recomputed %s (token may have been edited)\n",
			remote.Hash, remoteHash)
	}

	if localHash == remoteHash {
		_, _ = fmt.Fprintln(w, "\n  Same hash — environments match.")
		return
	}

	localTools := envToolMap(local.Tools)
	remoteTools := envToolMap(remote.Tools)
	var onlyLocal, onlyRemote, drifted []string
	for n, lt := range localTools {
		rt, ok := remoteTools[n]
		if !ok {
			onlyLocal = append(onlyLocal, n)
			continue
		}
		if lt.Version != rt.Version && lt.Version != "" && rt.Version != "" {
			drifted = append(drifted, fmt.Sprintf("%s (local=%s, remote=%s)", n, lt.Version, rt.Version))
		}
	}
	for n := range remoteTools {
		if _, ok := localTools[n]; !ok {
			onlyRemote = append(onlyRemote, n)
		}
	}
	sort.Strings(onlyLocal)
	sort.Strings(onlyRemote)
	sort.Strings(drifted)

	if len(onlyRemote) > 0 {
		_, _ = fmt.Fprintf(w, "\n  Tools to install (%d):\n", len(onlyRemote))
		for _, n := range onlyRemote {
			_, _ = fmt.Fprintf(w, "    + %s\n", n)
		}
	}
	if len(onlyLocal) > 0 {
		_, _ = fmt.Fprintf(w, "\n  Tools you have that remote doesn't (%d):\n", len(onlyLocal))
		for _, n := range onlyLocal {
			_, _ = fmt.Fprintf(w, "    - %s\n", n)
		}
	}
	if len(drifted) > 0 {
		_, _ = fmt.Fprintf(w, "\n  Version drift (%d):\n", len(drifted))
		for _, d := range drifted {
			_, _ = fmt.Fprintf(w, "    ~ %s\n", d)
		}
	}
	if added := setDiff(remote.Favorites, local.Favorites); len(added) > 0 {
		_, _ = fmt.Fprintf(w, "\n  Favorites to add (%d): %s\n", len(added), strings.Join(added, ", "))
	}
	if added := packsDiff(remote.Packs, local.Packs); len(added) > 0 {
		_, _ = fmt.Fprintf(w, "\n  Custom packs to add (%d): %s\n", len(added), strings.Join(added, ", "))
	}
	_, _ = fmt.Fprintln(w)
}

func envToolMap(tools []envid.Tool) map[string]envid.Tool {
	out := make(map[string]envid.Tool, len(tools))
	for _, t := range tools {
		out[t.Name] = t
	}
	return out
}

func setDiff(want, have []string) []string {
	hv := make(map[string]struct{}, len(have))
	for _, h := range have {
		hv[h] = struct{}{}
	}
	var out []string
	for _, w := range want {
		if _, ok := hv[w]; !ok {
			out = append(out, w)
		}
	}
	sort.Strings(out)
	return out
}

func packsDiff(want, have []envid.Pack) []string {
	hv := make(map[string]struct{}, len(have))
	for _, h := range have {
		hv[h.Name] = struct{}{}
	}
	var out []string
	for _, w := range want {
		if _, ok := hv[w.Name]; !ok {
			out = append(out, w.Name)
		}
	}
	sort.Strings(out)
	return out
}

func sortedMapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func joinOrDash(s []string) string {
	if len(s) == 0 {
		return "—"
	}
	return strings.Join(s, ",")
}

// applyTools delegates to the existing import install plan so we
// don't duplicate the install logic. We convert the env Profile's
// tool list into a manifest.Manifest and reuse buildImportPlan.
//
// Returns the count of failed installs (so the caller can surface
// PartialFailureError); a nil error means the apply pass completed
// (with possible per-tool failures captured in the count).
//
// The cancellation context comes from cmd.Context(); we don't take
// a separate ctx parameter to avoid the confusion of two contexts
// on the call site.
func applyTools(cmd *cobra.Command, p *envid.Profile) (int, error) {
	mtools := make([]manifest.Tool, 0, len(p.Tools))
	for _, t := range p.Tools {
		mtools = append(mtools, manifest.Tool{
			Name:     t.Name,
			Version:  t.Version,
			Source:   t.Source,
			Category: t.Category,
		})
	}

	regTools, _, err := svcFrom(cmd).ScanOnly(cmd.Context())
	if err != nil {
		return 0, fmt.Errorf("scanning installed tools: %w", err)
	}
	regMap := registry.ToolMap(regTools)
	ps := buildImportPlan(mtools, regMap)
	printPlanSummary("Apply Plan — Tools", ps)

	if len(ps.toInstall) == 0 {
		fmt.Fprintln(os.Stderr, "  Nothing new to install.")
		return 0, nil
	}
	if !confirmInstall(yesFlag) {
		fmt.Fprintln(os.Stderr, "  Cancelled.")
		return 0, nil
	}
	succeeded, failed := executeInstalls(ps.toInstall)
	if err := svcFrom(cmd).InvalidateScanCache(); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ Failed to invalidate scan cache: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "\n  Tools: %d installed, %d failed, %d already present\n",
		succeeded, failed, len(ps.alreadyInstalled))
	return failed, nil
}

// applyFavorites merges the env's favorites into the local list.
// Names are trimmed of whitespace and empties dropped — applying a
// hand-edited file/token with " jq " or "" entries shouldn't
// persist invalid favorite names.
//
// The merge is additive (existing favorites preserved) and writes
// the favorites file ONCE at the end. The earlier per-name
// favorites.Add loop reloaded and rewrote the YAML for every
// addition; with N new names that's N reads + N writes. Now it's
// 1 read + 1 write regardless of N.
func applyFavorites(names []string) error {
	if len(names) == 0 {
		return nil
	}
	existing, err := favorites.Load()
	if err != nil {
		return err
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, n := range existing {
		existingSet[n] = struct{}{}
	}
	merged := append([]string(nil), existing...)
	added := 0
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := existingSet[n]; ok {
			continue
		}
		existingSet[n] = struct{}{}
		merged = append(merged, n)
		added++
	}
	if added == 0 {
		fmt.Fprintf(os.Stderr, "  Favorites: 0 added (of %d in env)\n", len(names))
		return nil
	}
	if err := favorites.Save(merged); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "  Favorites: %d added (of %d in env)\n", added, len(names))
	return nil
}

// applyPacks adds env custom packs that don't already exist
// locally. Existing packs with the same name are preserved (users
// must delete first to replace).
//
// Same I/O-batching pattern as applyFavorites: load the existing
// pack list once, merge in the new ones, write once. Avoids
// rewriting the custom-packs YAML for every additional pack.
func applyPacks(packs []envid.Pack) error {
	if len(packs) == 0 {
		return nil
	}
	existing, err := custompacks.Load()
	if err != nil {
		return err
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, p := range existing {
		existingSet[strings.ToLower(p.Name)] = struct{}{}
	}
	merged := append([]registry.Pack(nil), existing...)
	added := 0
	for _, p := range packs {
		// Hygiene: hand-edited or malicious profiles may contain
		// empty/whitespace pack names or empty tool lists.
		// Mirror applyFavorites — trim, skip empties, and drop
		// invalid tool entries — so we never persist garbage.
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		toolNames := make([]string, 0, len(p.Tools))
		for _, tn := range p.Tools {
			tn = strings.TrimSpace(tn)
			if tn != "" {
				toolNames = append(toolNames, tn)
			}
		}
		if len(toolNames) == 0 {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := existingSet[key]; ok {
			continue
		}
		existingSet[key] = struct{}{}
		merged = append(merged, registry.Pack{
			Name:        name,
			DisplayName: strings.TrimSpace(p.DisplayName),
			ToolNames:   toolNames,
		})
		added++
	}
	if added == 0 {
		fmt.Fprintf(os.Stderr, "  Custom packs: 0 added (of %d in env)\n", len(packs))
		return nil
	}
	if err := custompacks.Save(merged); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "  Custom packs: %d added (of %d in env)\n", added, len(packs))
	return nil
}
