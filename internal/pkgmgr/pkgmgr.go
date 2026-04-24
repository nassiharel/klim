package pkgmgr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/registry"
)

const defaultCmdTimeout = 30 * time.Second

// VersionResolver abstracts version querying for tools.
type VersionResolver interface {
	ResolveVersions(ctx context.Context, tools []registry.Tool, concurrency int)
	ResolveOne(ctx context.Context, tool *registry.Tool)
}

// PackageManagerResolver is the default VersionResolver that queries
// native package managers (winget, brew, apt, choco, snap, npm) and
// falls back to binary detection (Go buildinfo, PE version resources).
type PackageManagerResolver struct {
	Timeout time.Duration // timeout per subprocess call; 0 = defaultCmdTimeout
}

// cmdTimeout returns the effective per-command timeout.
func (r *PackageManagerResolver) cmdTimeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return defaultCmdTimeout
}

// NewResolver returns the default package-manager-based version resolver.
func NewResolver() VersionResolver {
	return &PackageManagerResolver{}
}

// NewResolverWithTimeout returns a resolver with a custom command timeout.
func NewResolverWithTimeout(timeout time.Duration) VersionResolver {
	return &PackageManagerResolver{Timeout: timeout}
}

var defaultResolver VersionResolver = &PackageManagerResolver{}

// ResolveVersions is a convenience wrapper around the default resolver.
func ResolveVersions(ctx context.Context, tools []registry.Tool, concurrency int) {
	defaultResolver.ResolveVersions(ctx, tools, concurrency)
}

// ResolveVersions implements VersionResolver.
func (r *PackageManagerResolver) ResolveVersions(ctx context.Context, tools []registry.Tool, concurrency int) {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	timeout := r.cmdTimeout()
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := range tools {
		if !tools[i].IsInstalled() {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(t *registry.Tool) {
			defer wg.Done()
			defer func() { <-sem }()
			toolCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			resolveOne(toolCtx, t)
			detector.EnrichOne(t)
		}(&tools[i])
	}
	wg.Wait()
}

// ResolveOne is a convenience wrapper around the default resolver.
func ResolveOne(ctx context.Context, tool *registry.Tool) {
	defaultResolver.ResolveOne(ctx, tool)
}

// ResolveOne implements VersionResolver.
func (r *PackageManagerResolver) ResolveOne(ctx context.Context, tool *registry.Tool) {
	ctx, cancel := context.WithTimeout(ctx, r.cmdTimeout())
	defer cancel()
	resolveOne(ctx, tool)
	detector.EnrichOne(tool)
}

func resolveOne(ctx context.Context, tool *registry.Tool) {
	for j := range tool.Instances {
		inst := &tool.Instances[j]
		inst.Version = installedVersion(ctx, inst.Source, tool.Packages)
	}

	// Use a fresh timeout for latest version — installed version checks
	// may have consumed most of the parent context's deadline.
	latestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if primary := tool.PrimaryInstance(); primary != nil {
		latest, from := latestVersion(latestCtx, primary.Source, tool.Packages)
		tool.Latest = latest
		tool.LatestFrom = from
	}
}

func installedVersion(ctx context.Context, source registry.InstallSource, pkgs registry.PackageIDs) string {
	switch source {
	case registry.SourceWinget:
		if pkgs.Winget != "" {
			return wingetInstalledVersion(ctx, pkgs.Winget)
		}
	case registry.SourceChoco:
		if pkgs.Choco != "" {
			return chocoInstalledVersion(ctx, pkgs.Choco)
		}
	case registry.SourceScoop:
		if pkgs.Scoop != "" {
			return scoopInstalledVersion(ctx, pkgs.Scoop)
		}
	case registry.SourceBrew:
		if pkgs.Brew != "" {
			return brewInstalledVersion(ctx, pkgs.Brew)
		}
	case registry.SourceApt:
		if pkgs.Apt != "" {
			return dpkgInstalledVersion(pkgs.Apt)
		}
	case registry.SourceSnap:
		if pkgs.Snap != "" {
			return snapInstalledVersion(ctx, pkgs.Snap)
		}
	case registry.SourceNPM:
		if pkgs.NPM != "" {
			return npmInstalledVersion(ctx, pkgs.NPM)
		}
	}
	// Sources like "go", "cargo", "pip", "manual" — handled by detector fallback.
	return ""
}

func latestVersion(ctx context.Context, source registry.InstallSource, pkgs registry.PackageIDs) (version string, from string) {
	switch source {
	case registry.SourceWinget:
		if pkgs.Winget != "" {
			if v := wingetVersion(ctx, pkgs.Winget); v != "" {
				return v, "winget"
			}
		}
	case registry.SourceChoco:
		if pkgs.Choco != "" {
			if v := chocoLatestVersion(ctx, pkgs.Choco); v != "" {
				return v, "choco"
			}
		}
	case registry.SourceScoop:
		if pkgs.Scoop != "" {
			if v := scoopLatestVersion(ctx, pkgs.Scoop); v != "" {
				return v, "scoop"
			}
		}
	case registry.SourceBrew:
		if pkgs.Brew != "" {
			if v := brewLatestVersion(ctx, pkgs.Brew); v != "" {
				return v, "brew"
			}
		}
	case registry.SourceApt:
		if pkgs.Apt != "" {
			if v := aptLatestVersion(ctx, pkgs.Apt); v != "" {
				return v, "apt"
			}
		}
	case registry.SourceSnap:
		if pkgs.Snap != "" {
			if v := snapLatestVersion(ctx, pkgs.Snap); v != "" {
				return v, "snap"
			}
		}
	case registry.SourceNPM:
		if pkgs.NPM != "" {
			if v := npmLatestVersion(ctx, pkgs.NPM); v != "" {
				return v, "npm"
			}
		}
	}
	return "", ""
}

// --- winget ---

func wingetVersion(ctx context.Context, id string) string {
	out := cleanWingetOutput(runCmd(ctx, "winget", "show", "--id", id, "--accept-source-agreements"))
	return parseKeyValue(out, "Version")
}

func wingetInstalledVersion(ctx context.Context, id string) string {
	out := cleanWingetOutput(runCmd(ctx, "winget", "list", "--id", id, "--exact", "--accept-source-agreements"))
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i, f := range fields {
			if strings.EqualFold(f, id) && i+1 < len(fields) {
				ver := fields[i+1]
				if strings.HasPrefix(ver, "-") {
					continue
				}
				return ver
			}
		}
	}
	return ""
}

// cleanWingetOutput strips winget's VT100 progress spinner noise from captured
// stdout. Winget uses \r (carriage return without newline) to animate the
// spinner — when captured in a buffer these \r segments accumulate into one
// long line. We normalise \r to \n so downstream line-based parsers work.
func cleanWingetOutput(out string) string {
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\r", "\n")
	return out
}

// --- choco ---

func chocoInstalledVersion(ctx context.Context, pkg string) string {
	out := runCmd(ctx, "choco", "list", "--limit-output", "--exact", pkg)
	return parsePipeSeparated(out, pkg)
}

func chocoLatestVersion(ctx context.Context, pkg string) string {
	out := runCmd(ctx, "choco", "search", "--exact", "--limit-output", pkg)
	return parsePipeSeparated(out, pkg)
}

// --- scoop ---

// scoopInstalledVersion queries `scoop list <pkg>` and parses the version
// column. Scoop emits a table like:
//
//	Name   Version Source Updated           Info
//	----   ------- ------ -------           ----
//	bat    0.24.0  main   2024-01-15 ...
//
// so we find the line whose first field matches the package (case-insensitive)
// and return the second field.
func scoopInstalledVersion(ctx context.Context, pkg string) string {
	return parseScoopList(runCmd(ctx, "scoop", "list", pkg), pkg)
}

// ansiRe matches ANSI escape sequences (color codes, cursor moves, etc.).
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes ANSI escape sequences from s.
func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// parseScoopList extracts the version column for pkg from `scoop list` output.
// Split out from scoopInstalledVersion so it can be covered by unit tests
// without shelling out.
func parseScoopList(out, pkg string) string {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && strings.EqualFold(fields[0], pkg) {
			// Skip header/separator lines like "Name Version ..." or "---- -------".
			if strings.EqualFold(fields[0], "Name") || strings.HasPrefix(fields[0], "---") {
				continue
			}
			return fields[1]
		}
	}
	return ""
}

// scoopLatestVersion queries `scoop info <pkg>` and extracts the Version:
// line. `scoop info` prints key/value pairs, e.g.:
//
//	Name: bat
//	Version: 0.24.0
//	...
func scoopLatestVersion(ctx context.Context, pkg string) string {
	out := runCmd(ctx, "scoop", "info", pkg)
	return parseKeyValue(out, "Version")
}

// --- brew ---

func brewInstalledVersion(ctx context.Context, formula string) string {
	out := runCmd(ctx, "brew", "list", "--versions", formula)
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func brewLatestVersion(ctx context.Context, formula string) string {
	out := runCmd(ctx, "brew", "info", "--json=v2", formula)
	if out == "" {
		return ""
	}
	var result struct {
		Formulae []struct {
			Versions struct {
				Stable string `json:"stable"`
			} `json:"versions"`
		} `json:"formulae"`
	}
	if err := json.Unmarshal([]byte(out), &result); err == nil && len(result.Formulae) > 0 {
		return result.Formulae[0].Versions.Stable
	}
	return ""
}

// --- apt/dpkg ---

func dpkgInstalledVersion(pkg string) string {
	if v, ok := getCachedDpkgVersions()[pkg]; ok {
		return v
	}
	return ""
}

func aptLatestVersion(ctx context.Context, pkg string) string {
	out := runCmd(ctx, "apt-cache", "policy", pkg)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Candidate:") {
			ver := strings.TrimSpace(strings.TrimPrefix(line, "Candidate:"))
			if ver != "(none)" {
				return cleanDebianVersion(ver)
			}
		}
	}
	return ""
}

// --- snap ---

func snapInstalledVersion(ctx context.Context, pkg string) string {
	out := runCmd(ctx, "snap", "list", pkg)
	lines := strings.Split(out, "\n")
	if len(lines) >= 2 {
		fields := strings.Fields(lines[1])
		if len(fields) >= 2 {
			return fields[1]
		}
	}
	return ""
}

func snapLatestVersion(ctx context.Context, pkg string) string {
	out := runCmd(ctx, "snap", "info", pkg)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "latest/stable:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return ""
}

// --- npm (cached) ---

var (
	npmGlobalCache map[string]string
	npmOnce        sync.Once
)

func loadNpmGlobals(ctx context.Context) map[string]string {
	out := runCmd(ctx, "npm", "list", "-g", "--depth=0", "--json")
	if out == "" {
		return nil
	}
	var result struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil
	}
	m := make(map[string]string, len(result.Dependencies))
	for name, dep := range result.Dependencies {
		m[name] = dep.Version
	}
	return m
}

func npmInstalledVersion(ctx context.Context, pkg string) string {
	// Use a dedicated timeout for cache population instead of the caller's
	// context — sync.Once runs exactly once, so a cancelled/expired context
	// from the first caller would permanently poison the cache.
	npmOnce.Do(func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), defaultCmdTimeout)
		defer cancel()
		npmGlobalCache = loadNpmGlobals(cacheCtx)
	})
	cache := npmGlobalCache

	if cache == nil {
		return ""
	}
	if v, ok := cache[pkg]; ok {
		return v
	}
	// Handle scoped packages by suffix match.
	for name, ver := range cache {
		if strings.HasSuffix(name, "/"+pkg) {
			return ver
		}
	}
	return ""
}

func npmLatestVersion(ctx context.Context, pkg string) string {
	out := runCmd(ctx, "npm", "view", pkg, "version")
	return strings.TrimSpace(out)
}

// --- dpkg cache ---

var (
	dpkgCache map[string]string
	dpkgOnce  sync.Once
)

func getCachedDpkgVersions() map[string]string {
	dpkgOnce.Do(func() { dpkgCache = parseDpkgStatus() })
	return dpkgCache
}

func parseDpkgStatus() map[string]string {
	f, err := os.Open("/var/lib/dpkg/status")
	if err != nil {
		return make(map[string]string)
	}
	defer f.Close() //nolint:errcheck // best-effort close on read-only file

	versions := make(map[string]string)
	var pkg, version string
	installed := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if pkg != "" && version != "" && installed {
				versions[pkg] = cleanDebianVersion(version)
			}
			pkg, version = "", ""
			installed = false
			continue
		}
		switch {
		case strings.HasPrefix(line, "Package: "):
			pkg = strings.TrimPrefix(line, "Package: ")
		case strings.HasPrefix(line, "Version: "):
			version = strings.TrimPrefix(line, "Version: ")
		case strings.HasPrefix(line, "Status: "):
			installed = strings.Contains(line, "installed")
		}
	}
	if pkg != "" && version != "" && installed {
		versions[pkg] = cleanDebianVersion(version)
	}
	if err := scanner.Err(); err != nil {
		slog.Warn("dpkg status scan incomplete", "error", err)
	}
	return versions
}

func cleanDebianVersion(v string) string {
	if idx := strings.Index(v, ":"); idx >= 0 {
		v = v[idx+1:]
	}
	if idx := strings.Index(v, "-"); idx >= 0 {
		v = v[:idx]
	}
	return v
}

// --- Helpers ---

func runCmd(ctx context.Context, name string, args ...string) string {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = nil

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		slog.Warn("subprocess failed", "cmd", name, "args", args, "error", err)
		return ""
	}
	slog.Debug("subprocess ok", "cmd", name, "args", args, "bytes", stdout.Len())
	// Strip ANSI escape sequences — some tools (scoop, winget) emit color codes
	// that break field-based parsing.
	return stripANSI(stdout.String())
}

func parseKeyValue(output, key string) string {
	lowerKey := strings.ToLower(key)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		// Match "Key : value" or "Key: value" (key may have trailing spaces before colon).
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		lineKey := strings.TrimSpace(line[:idx])
		if strings.EqualFold(lineKey, lowerKey) {
			return strings.TrimSpace(line[idx+1:])
		}
	}
	return ""
}

// parsePipeSeparated parses "name|version" lines (choco --limit-output format).
func parsePipeSeparated(output, pkg string) string {
	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], pkg) {
			return parts[1]
		}
	}
	return ""
}
