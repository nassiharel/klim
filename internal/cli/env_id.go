package cli

import (
	"errors"
	"fmt"
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

// envIDCmd captures and reproduces a clim-managed environment.
//
// The same payload has two encodings:
//   - clim env-id (no args)  → compact base64 token for chat
//   - clim env-id --output yaml → human-readable YAML for git
//
// Receivers can decode with `clim env-id show <token-or-file>` and
// reproduce with `clim env-id apply <token-or-file>`.
var envIDCmd = &cobra.Command{
	Use:   "env-id [flags]",
	Short: "Generate and apply environment fingerprints",
	Long: `clim env-id captures the shape of your clim-managed environment
(installed tools, favorites, custom packs, available package managers,
clim version, OS, audit/security counts) into a portable artifact you
can share via chat or commit to git.

Privacy: the token contains only what 'clim list' already shows. No
hostname, username, paths, or environment variables are captured.

Examples:
  # Print the token for the current environment (paste into chat).
  clim env-id

  # Write the rich YAML form to a file.
  clim env-id --output yaml > my-env.yaml

  # Decode someone else's token without applying it.
  clim env-id show 'clim:env:v1:H4sIAAAAAA...'

  # Diff a coworker's env against yours.
  clim env-id diff 'clim:env:v1:H4sIAAAAAA...'

  # Reproduce the env locally — installs missing tools, sets favorites,
  # registers custom packs. Cross-OS gaps are reported, never errors.
  clim env-id apply 'clim:env:v1:H4sIAAAAAA...'`,
	RunE: runEnvIDPrint,
}

var (
	envIDOutputFmt func() (OutputFormat, error)
)

func init() {
	envIDOutputFmt = addOutputFlag(envIDCmd, OutputText, OutputJSON, OutputYAML)
	envIDCmd.AddCommand(envIDShowCmd)
	envIDCmd.AddCommand(envIDDiffCmd)
	envIDCmd.AddCommand(envIDApplyCmd)
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
	out, err := envIDOutputFmt()
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

// envIDShowCmd pretty-prints a profile for inspection.
var envIDShowCmd = &cobra.Command{
	Use:   "show <token-or-file>",
	Short: "Pretty-print an env-id token or file",
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

// envIDDiffCmd compares a remote profile against the local one.
var envIDDiffCmd = &cobra.Command{
	Use:   "diff <token-or-file>",
	Short: "Compare another env-id against the current environment",
	Long: `Decode an env-id and report what would change if you applied
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

// envIDApplyCmd reproduces a remote profile locally.
var envIDApplyCmd = &cobra.Command{
	Use:   "apply <token-or-file>",
	Short: "Reproduce another env-id locally",
	Long: `Install missing tools, set favorites, and register custom
packs from a remote env-id. Cross-OS / cross-PM gaps (e.g. apt-only
tools on Windows) surface in the report as 'skipped' — they're never
errors. Use --yes to skip the install confirmation prompt.`,
	Args: cobra.ExactArgs(1),
	RunE: runEnvIDApply,
}

func init() {
	envIDApplyCmd.Flags().BoolVar(&yesFlag, "yes", false, "Skip confirmation prompts")
}

func runEnvIDApply(cmd *cobra.Command, args []string) error {
	p, err := loadProfile(args[0])
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\n──── Applying env-id %s ────\n\n", p.Hash)
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
	// replace). Documented in env-id.md so this isn't surprising.
	if err := applyPacks(p.Packs); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ packs: %v\n", err)
	}

	if failed > 0 {
		return &PartialFailureError{Op: "env-id apply", Succeeded: len(p.Tools) - failed, Failed: failed}
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
			return nil, usageErrorf("invalid env-id token: %v", err)
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

func renderProfileText(w *os.File, p *envid.Profile) {
	_, _ = fmt.Fprintf(w, "\n──── Env ID %s ────\n\n", p.Hash)
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

func renderProfileDiff(w *os.File, local, remote *envid.Profile) {
	_, _ = fmt.Fprintf(w, "\n──── env-id diff ────\n")
	_, _ = fmt.Fprintf(w, "  local:  %s   tools=%d favorites=%d packs=%d\n",
		local.Hash, len(local.Tools), len(local.Favorites), len(local.Packs))
	_, _ = fmt.Fprintf(w, "  remote: %s   tools=%d favorites=%d packs=%d\n",
		remote.Hash, len(remote.Tools), len(remote.Favorites), len(remote.Packs))

	if local.Hash == remote.Hash {
		_, _ = fmt.Fprintln(w, "\n  Same hash — environments match.")
		return
	}

	localTools := envIDToolMap(local.Tools)
	remoteTools := envIDToolMap(remote.Tools)
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

func envIDToolMap(tools []envid.Tool) map[string]envid.Tool {
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
// don't duplicate the install logic. We convert the env-id Profile's
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

// applyFavorites merges the env-id's favorites into the local list.
// Names are trimmed of whitespace and empties dropped — applying a
// hand-edited file/token with " jq " or "" entries shouldn't persist
// invalid favorite names. The merge is additive: existing favorites
// are preserved.
func applyFavorites(names []string) error {
	if len(names) == 0 {
		return nil
	}
	existing, err := favorites.Set()
	if err != nil {
		return err
	}
	added := 0
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := existing[n]; ok {
			continue
		}
		if err := favorites.Add(n); err != nil {
			return err
		}
		added++
	}
	fmt.Fprintf(os.Stderr, "  Favorites: %d added (of %d in env-id)\n", added, len(names))
	return nil
}

// applyPacks adds env-id custom packs that don't already exist
// locally. Existing packs with the same name are preserved (the user
// must explicitly delete them first to replace).
//
// The "exists" check loads the on-disk pack list ONCE up front and
// builds an in-memory set; iterating with custompacks.Exists per
// pack would re-parse the YAML on every loop iteration.
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
	added := 0
	for _, p := range packs {
		if _, ok := existingSet[strings.ToLower(p.Name)]; ok {
			continue
		}
		if err := custompacks.Add(registry.Pack{
			Name:        p.Name,
			DisplayName: p.DisplayName,
			ToolNames:   p.Tools,
		}); err != nil {
			return err
		}
		existingSet[strings.ToLower(p.Name)] = struct{}{}
		added++
	}
	fmt.Fprintf(os.Stderr, "  Custom packs: %d added (of %d in env-id)\n", added, len(packs))
	return nil
}
