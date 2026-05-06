package envid

import (
	"bufio"
	"os"
	"runtime"
	"strings"
)

// detectDistro returns a best-effort human-readable distro identifier
// for the host, or "" when we can't tell. Used purely for the Env ID
// "what kind of machine is this?" hint — never gates anything.
//
// Linux: parses /etc/os-release PRETTY_NAME.
// macOS: returns "macOS".
// Windows: returns "Windows".
// Other: empty.
func detectDistro() string {
	switch runtime.GOOS {
	case "linux":
		return readLinuxDistro()
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	}
	return ""
}

func readLinuxDistro() string {
	for _, path := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		if v := readPrettyName(path); v != "" {
			return v
		}
	}
	return ""
}

func readPrettyName(path string) string {
	f, err := os.Open(path) // #nosec G304 -- well-known system path
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "PRETTY_NAME=") {
			continue
		}
		v := strings.TrimPrefix(line, "PRETTY_NAME=")
		v = strings.Trim(v, `"`)
		v = strings.TrimSpace(v)
		return v
	}
	return ""
}
