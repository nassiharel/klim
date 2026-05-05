// Package envid produces a portable, paste-friendly fingerprint of a
// clim-managed environment ("Env ID"). The same payload has two
// encodings:
//
//   - YAML / JSON file form for git, code review, and `clim` itself.
//   - Compact base64 token (`clim:env:v1:<gz+b64>`) for chat / quick
//     share.
//
// Privacy: tokens contain only what `clim list` already shows
// (catalog tool names, installed versions, install sources, GOOS/
// GOARCH, available PM binaries). No hostname, username, paths, or
// environment variables are captured.
package envid

import "time"

// SchemaVersion is the on-the-wire version stamped on every Profile.
// Bump only on incompatible field changes; additive changes keep
// SchemaVersion=1.
const SchemaVersion = 1

// Profile is the canonical representation of an Env ID.
type Profile struct {
	SchemaVersion   int             `yaml:"schema_version"     json:"schema_version"`
	Clim            ClimInfo        `yaml:"clim"               json:"clim"`
	GeneratedAt     time.Time       `yaml:"generated_at"       json:"generated_at"`
	Hash            string          `yaml:"hash"               json:"hash"`
	OS              OSInfo          `yaml:"os"                 json:"os"`
	PackageManagers map[string]bool `yaml:"package_managers"   json:"package_managers"`
	Tools           []Tool          `yaml:"tools,omitempty"    json:"tools,omitempty"`
	Favorites       []string        `yaml:"favorites,omitempty" json:"favorites,omitempty"`
	Packs           []Pack          `yaml:"packs,omitempty"    json:"packs,omitempty"`
	Security        Security        `yaml:"security"           json:"security"`
}

// ClimInfo identifies the clim build that produced the profile.
type ClimInfo struct {
	Version string `yaml:"version"          json:"version"`
	Commit  string `yaml:"commit,omitempty" json:"commit,omitempty"`
}

// OSInfo describes the host OS and architecture. Distro is best-
// effort and may be empty.
type OSInfo struct {
	GOOS   string `yaml:"goos"             json:"goos"`
	Arch   string `yaml:"arch"             json:"arch"`
	Distro string `yaml:"distro,omitempty" json:"distro,omitempty"`
}

// Tool is a compact view of a single installed tool.
type Tool struct {
	Name     string `yaml:"name"               json:"name"`
	Version  string `yaml:"version,omitempty"  json:"version,omitempty"`
	Source   string `yaml:"source,omitempty"   json:"source,omitempty"`
	Category string `yaml:"category,omitempty" json:"category,omitempty"`
}

// Pack mirrors a user-defined custom pack. Marketplace packs are not
// captured (the catalog supplies them independently).
type Pack struct {
	Name        string   `yaml:"name"                   json:"name"`
	DisplayName string   `yaml:"display_name,omitempty" json:"display_name,omitempty"`
	Tools       []string `yaml:"tools"                  json:"tools"`
}

// Security carries observational counts from audit + the security
// verdict index. None of these gate apply; they're recorded so the
// receiver can compare their environment's safety to the source's.
type Security struct {
	AuditWarnings int            `yaml:"audit_warnings" json:"audit_warnings"`
	AuditInfos    int            `yaml:"audit_infos"    json:"audit_infos"`
	Verdicts      VerdictsCounts `yaml:"verdicts"       json:"verdicts"`
}

// VerdictsCounts mirrors security.Index.Counts() — one bucket per
// 4-state status.
type VerdictsCounts struct {
	Clean   int `yaml:"clean"   json:"clean"`
	Watch   int `yaml:"watch"   json:"watch"`
	Risk    int `yaml:"risk"    json:"risk"`
	Unknown int `yaml:"unknown" json:"unknown"`
}
