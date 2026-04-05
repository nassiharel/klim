package pkgmgr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

// cmdTimeout is the max time for any package manager command.
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

// resolveOne populates versions for a single tool.
func resolveOne(tool *registry.Tool) {
	// Detect installed version for each instance.
	for j := range tool.Instances {
		inst := &tool.Instances[j]
		inst.Version = installedVersion(inst.Source, inst.Path, tool.Packages)
	}

	// Get latest version from the primary instance's source.
	if primary := tool.PrimaryInstance(); primary != nil {
		latest, from := latestVersion(primary.Source, tool.Packages)
		tool.Latest = latest
		tool.LatestFrom = from
	}
}

// installedVersion queries the package manager for the installed version.
func installedVersion(source, path string, pkgs registry.PackageIDs) string {
	switch source {
	case "winget":
		if pkgs.Winget != "" {
			return wingetInstalledVersion(pkgs.Winget)
		}
	case "choco":
		if pkgs.Choco != "" {
			return chocoInstalledVersion(pkgs.Choco)
		}
	case "brew":
		if pkgs.Brew != "" {
			return brewInstalledVersion(pkgs.Brew)
		}
	case "apt":
		if pkgs.Apt != "" {
			return dpkgInstalledVersion(pkgs.Apt)
		}
	case "snap":
		if pkgs.Snap != "" {
			return snapInstalledVersion(pkgs.Snap)
		}
	case "npm":
		if pkgs.NPM != "" {
			return npmInstalledVersion(pkgs.NPM)
		}
	case "go":
		return goBuildInfoVersion(path)
	}

	// Fallback: try PE metadata (Windows) or Go buildinfo.
	return fallbackVersion(path)
}

// latestVersion queries the package manager for the latest available version.
func latestVersion(source string, pkgs registry.PackageIDs) (version, from string) {
	switch source {
	case "winget":
		if pkgs.Winget != "" {
			if v := wingetLatestVersion(pkgs.Winget); v != "" {
				return v, "winget"
			}
		}
	case "choco":
		if pkgs.Choco != "" {
			if v := chocoLatestVersion(pkgs.Choco); v != "" {
				return v, "choco"
			}
		}
	case "brew":
		if pkgs.Brew != "" {
			if v := brewLatestVersion(pkgs.Brew); v != "" {
				return v, "brew"
			}
		}
	case "apt":
		if pkgs.Apt != "" {
			if v := aptLatestVersion(pkgs.Apt); v != "" {
				return v, "apt"
			}
		}
	case "snap":
		if pkgs.Snap != "" {
			if v := snapLatestVersion(pkgs.Snap); v != "" {
				return v, "snap"
			}
		}
	case "npm":
		if pkgs.NPM != "" {
			if v := npmLatestVersion(pkgs.NPM); v != "" {
				return v, "npm"
			}
		}
	}
	return "", ""
}

// --- winget ---

var versionLineRe = regexp.MustCompile(`(?i)^Version:\s+(.+)$`)

func wingetInstalledVersion(id string) string {
	// winget show --id X shows the installed version if installed.
	out := runCmd("winget", "show", "--id", id, "--accept-source-agreements")
	return parseKeyValue(out, "Version")
}

func wingetLatestVersion(id string) string {
	out := runCmd("winget", "show", "--id", id, "--accept-source-agreements")
	return parseKeyValue(out, "Version")
}

// --- choco ---

func chocoInstalledVersion(pkg string) string {
	out := runCmd("choco", "list", "--limit-output", "--exact", pkg)
	// Output format: "name|version"
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], pkg) {
			return parts[1]
		}
	}
	return ""
}

func chocoLatestVersion(pkg string) string {
	out := runCmd("choco", "search", "--exact", "--limit-output", pkg)
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], pkg) {
			return parts[1]
		}
	}
	return ""
}

// --- brew ---

func brewInstalledVersion(formula string) string {
	out := runCmd("brew", "list", "--versions", formula)
	// Output: "git 2.47.0" or "git 2.47.0 2.46.0" (multiple versions)
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) >= 2 {
		return parts[1] // most recent is first
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
	versions := getCachedDpkgVersions()
	if v, ok := versions[pkg]; ok {
		return v
	}
	return ""
}

func aptLatestVersion(pkg string) string {
	out := runCmd("apt-cache", "policy", pkg)
	// Parse "Candidate: 1:2.43.0-1ubuntu7.3"
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
	// Header: Name  Version  Rev  Tracking  Publisher  Notes
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
	// Parse "latest/stable: 1.33.3 2024-01-15 (123) 50MB classic"
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

// --- npm ---

func npmInstalledVersion(pkg string) string {
	out := runCmd("npm", "list", "-g", "--depth=0", "--json")
	if out == "" {
		return ""
	}
	var result struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(out), &result); err == nil {
		if dep, ok := result.Dependencies[pkg]; ok {
			return dep.Version
		}
		// Handle scoped packages: "@anthropic-ai/claude-code" → key in dependencies
		for name, dep := range result.Dependencies {
			if name == pkg || strings.HasSuffix(name, "/"+pkg) {
				return dep.Version
			}
		}
	}
	return ""
}

func npmLatestVersion(pkg string) string {
	out := runCmd("npm", "view", pkg, "version")
	return strings.TrimSpace(out)
}

// --- Go buildinfo ---

func goBuildInfoVersion(path string) string {
	// Reuse the existing detector's buildinfo logic.
	// Imported at runtime to avoid circular deps.
	return "" // Will be filled by detector fallback
}

// --- Fallback (PE / buildinfo) ---

func fallbackVersion(path string) string {
	return "" // Handled by detector package
}

// --- dpkg cache ---

var (
	dpkgCache  map[string]string
	dpkgOnce   sync.Once
)

func getCachedDpkgVersions() map[string]string {
	dpkgOnce.Do(func() {
		dpkgCache = parseDpkgStatus()
	})
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
	cmd.Stderr = nil // discard stderr

	if err := cmd.Run(); err != nil {
		return ""
	}
	return stdout.String()
}

func parseKeyValue(output, key string) string {
	prefix := fmt.Sprintf("%s:", key)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
