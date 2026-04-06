package pkgmgr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

const cmdTimeout = 10 * time.Second

// ResolveVersions populates Version on each instance and Latest on each tool,
// using the appropriate package manager for each install source.
func ResolveVersions(tools []registry.Tool, concurrency int) {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

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
			resolveOne(t)
		}(&tools[i])
	}
	wg.Wait()
}

// ResolveOne populates versions for a single tool.
func ResolveOne(tool *registry.Tool) {
	resolveOne(tool)
}

func resolveOne(tool *registry.Tool) {
	for j := range tool.Instances {
		inst := &tool.Instances[j]
		inst.Version = installedVersion(inst.Source, tool.Packages)
	}

	if primary := tool.PrimaryInstance(); primary != nil {
		latest, from := latestVersion(primary.Source, tool.Packages)
		tool.Latest = latest
		tool.LatestFrom = from
	}
}

func installedVersion(source registry.InstallSource, pkgs registry.PackageIDs) string {
	switch source {
	case registry.SourceWinget:
		if pkgs.Winget != "" {
			return wingetVersion(pkgs.Winget)
		}
	case registry.SourceChoco:
		if pkgs.Choco != "" {
			return chocoInstalledVersion(pkgs.Choco)
		}
	case registry.SourceBrew:
		if pkgs.Brew != "" {
			return brewInstalledVersion(pkgs.Brew)
		}
	case registry.SourceApt:
		if pkgs.Apt != "" {
			return dpkgInstalledVersion(pkgs.Apt)
		}
	case registry.SourceSnap:
		if pkgs.Snap != "" {
			return snapInstalledVersion(pkgs.Snap)
		}
	case registry.SourceNPM:
		if pkgs.NPM != "" {
			return npmInstalledVersion(pkgs.NPM)
		}
	}
	// Sources like "go", "cargo", "pip", "manual" — handled by detector fallback.
	return ""
}

func latestVersion(source registry.InstallSource, pkgs registry.PackageIDs) (version string, from string) {
	switch source {
	case registry.SourceWinget:
		if pkgs.Winget != "" {
			if v := wingetVersion(pkgs.Winget); v != "" {
				return v, "winget"
			}
		}
	case registry.SourceChoco:
		if pkgs.Choco != "" {
			if v := chocoLatestVersion(pkgs.Choco); v != "" {
				return v, "choco"
			}
		}
	case registry.SourceBrew:
		if pkgs.Brew != "" {
			if v := brewLatestVersion(pkgs.Brew); v != "" {
				return v, "brew"
			}
		}
	case registry.SourceApt:
		if pkgs.Apt != "" {
			if v := aptLatestVersion(pkgs.Apt); v != "" {
				return v, "apt"
			}
		}
	case registry.SourceSnap:
		if pkgs.Snap != "" {
			if v := snapLatestVersion(pkgs.Snap); v != "" {
				return v, "snap"
			}
		}
	case registry.SourceNPM:
		if pkgs.NPM != "" {
			if v := npmLatestVersion(pkgs.NPM); v != "" {
				return v, "npm"
			}
		}
	}
	return "", ""
}

// --- winget ---
// winget show returns the available version; used for both installed and latest
// since winget show --id X reports the catalog version for the matched install.
func wingetVersion(id string) string {
	out := runCmd("winget", "show", "--id", id, "--accept-source-agreements")
	return parseKeyValue(out, "Version")
}

// --- choco ---

func chocoInstalledVersion(pkg string) string {
	out := runCmd("choco", "list", "--limit-output", "--exact", pkg)
	return parsePipeSeparated(out, pkg)
}

func chocoLatestVersion(pkg string) string {
	out := runCmd("choco", "search", "--exact", "--limit-output", pkg)
	return parsePipeSeparated(out, pkg)
}

// --- brew ---

func brewInstalledVersion(formula string) string {
	out := runCmd("brew", "list", "--versions", formula)
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func brewLatestVersion(formula string) string {
	out := runCmd("brew", "info", "--json=v2", formula)
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

func aptLatestVersion(pkg string) string {
	out := runCmd("apt-cache", "policy", pkg)
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

func snapInstalledVersion(pkg string) string {
	out := runCmd("snap", "list", pkg)
	lines := strings.Split(out, "\n")
	if len(lines) >= 2 {
		fields := strings.Fields(lines[1])
		if len(fields) >= 2 {
			return fields[1]
		}
	}
	return ""
}

func snapLatestVersion(pkg string) string {
	out := runCmd("snap", "info", pkg)
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

func loadNpmGlobals() map[string]string {
	out := runCmd("npm", "list", "-g", "--depth=0", "--json")
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

func npmInstalledVersion(pkg string) string {
	npmOnce.Do(func() { npmGlobalCache = loadNpmGlobals() })
	if npmGlobalCache == nil {
		return ""
	}
	if v, ok := npmGlobalCache[pkg]; ok {
		return v
	}
	// Handle scoped packages by suffix match.
	for name, ver := range npmGlobalCache {
		if strings.HasSuffix(name, "/"+pkg) {
			return ver
		}
	}
	return ""
}

func npmLatestVersion(pkg string) string {
	out := runCmd("npm", "view", pkg, "version")
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
	defer f.Close()

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
		if strings.HasPrefix(line, "Package: ") {
			pkg = strings.TrimPrefix(line, "Package: ")
		} else if strings.HasPrefix(line, "Version: ") {
			version = strings.TrimPrefix(line, "Version: ")
		} else if strings.HasPrefix(line, "Status: ") {
			installed = strings.Contains(line, "installed")
		}
	}
	if pkg != "" && version != "" && installed {
		versions[pkg] = cleanDebianVersion(version)
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

func runCmd(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = nil

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return ""
	}
	return stdout.String()
}

func parseKeyValue(output, key string) string {
	prefix := key + ":"
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
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

// --- Tool info ---

// FetchToolInfo retrieves rich metadata from the package manager for display.
// Uses the primary instance's source to determine which PM to query.
// Falls back to any available source if the primary has no package ID.
func FetchToolInfo(tool *registry.Tool) {
	if tool.Info != nil {
		return // already fetched
	}

	source, pkgID := bestInfoSource(tool)
	if pkgID == "" {
		return
	}

	switch source {
	case registry.SourceWinget:
		tool.Info = fetchWingetInfo(pkgID)
	case registry.SourceBrew:
		tool.Info = fetchBrewInfo(pkgID)
	case registry.SourceApt:
		tool.Info = fetchAptInfo(pkgID)
	case registry.SourceSnap:
		tool.Info = fetchSnapInfo(pkgID)
	case registry.SourceNPM:
		tool.Info = fetchNpmInfo(pkgID)
	}
}

// bestInfoSource picks the best source+pkgID for fetching tool info.
// Prefers the primary instance's source, falls back through all available sources
// in priority order (winget is richest, then brew, apt, snap, npm).
func bestInfoSource(tool *registry.Tool) (registry.InstallSource, string) {
	// Try the primary instance's source first.
	if primary := tool.PrimaryInstance(); primary != nil {
		if id := pkgIDForSource(tool, primary.Source); id != "" {
			return primary.Source, id
		}
	}

	// Fallback: try sources in priority order (richest info first).
	fallback := []struct {
		src registry.InstallSource
		id  string
	}{
		{registry.SourceWinget, tool.Packages.Winget},
		{registry.SourceBrew, tool.Packages.Brew},
		{registry.SourceApt, tool.Packages.Apt},
		{registry.SourceSnap, tool.Packages.Snap},
		{registry.SourceNPM, tool.Packages.NPM},
	}
	for _, f := range fallback {
		if f.id != "" {
			return f.src, f.id
		}
	}
	return "", ""
}

func pkgIDForSource(tool *registry.Tool, source registry.InstallSource) string {
	switch source {
	case registry.SourceWinget:
		return tool.Packages.Winget
	case registry.SourceChoco:
		return tool.Packages.Choco
	case registry.SourceBrew:
		return tool.Packages.Brew
	case registry.SourceApt:
		return tool.Packages.Apt
	case registry.SourceSnap:
		return tool.Packages.Snap
	case registry.SourceNPM:
		return tool.Packages.NPM
	}
	return ""
}

func fetchWingetInfo(id string) *registry.ToolInfo {
	out := runCmd("winget", "show", "--id", id, "--accept-source-agreements")
	if out == "" {
		return nil
	}
	info := &registry.ToolInfo{
		Publisher:   parseKeyValue(out, "Publisher"),
		Homepage:    parseKeyValue(out, "Homepage"),
		License:     parseKeyValue(out, "License"),
		ReleaseDate: parseKeyValue(out, "Release Date"),
	}
	info.Description = parseWingetDescription(out)
	if info.Description == "" && info.Publisher == "" && info.Homepage == "" {
		return nil
	}
	return info
}

// parseWingetDescription extracts the Description field which may span multiple
// indented lines in winget show output.
func parseWingetDescription(output string) string {
	lines := strings.Split(output, "\n")
	var desc []string
	inDesc := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Description:") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "Description:"))
			if rest != "" {
				desc = append(desc, rest)
			}
			inDesc = true
			continue
		}
		if inDesc {
			if strings.HasPrefix(line, "  ") && trimmed != "" {
				desc = append(desc, trimmed)
			} else {
				break
			}
		}
	}
	return strings.Join(desc, " ")
}

func fetchBrewInfo(formula string) *registry.ToolInfo {
	out := runCmd("brew", "info", "--json=v2", formula)
	if out == "" {
		return nil
	}
	var result struct {
		Formulae []struct {
			Desc     string `json:"desc"`
			Homepage string `json:"homepage"`
			License  string `json:"license"`
		} `json:"formulae"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || len(result.Formulae) == 0 {
		return nil
	}
	f := result.Formulae[0]
	if f.Desc == "" && f.Homepage == "" {
		return nil
	}
	return &registry.ToolInfo{
		Description: f.Desc,
		Homepage:    f.Homepage,
		License:     f.License,
	}
}

func fetchAptInfo(pkg string) *registry.ToolInfo {
	out := runCmd("apt-cache", "show", pkg)
	if out == "" {
		return nil
	}
	desc := parseKeyValue(out, "Description")
	homepage := parseKeyValue(out, "Homepage")
	if desc == "" && homepage == "" {
		return nil
	}
	return &registry.ToolInfo{
		Description: desc,
		Homepage:    homepage,
	}
}

func fetchSnapInfo(pkg string) *registry.ToolInfo {
	out := runCmd("snap", "info", pkg)
	if out == "" {
		return nil
	}
	desc := parseKeyValue(out, "summary")
	publisher := parseKeyValue(out, "publisher")
	if desc == "" {
		return nil
	}
	return &registry.ToolInfo{
		Description: desc,
		Publisher:   publisher,
	}
}

func fetchNpmInfo(pkg string) *registry.ToolInfo {
	desc := strings.TrimSpace(runCmd("npm", "view", pkg, "description"))
	homepage := strings.TrimSpace(runCmd("npm", "view", pkg, "homepage"))
	license := strings.TrimSpace(runCmd("npm", "view", pkg, "license"))
	if desc == "" && homepage == "" {
		return nil
	}
	return &registry.ToolInfo{
		Description: desc,
		Homepage:    homepage,
		License:     license,
	}
}
