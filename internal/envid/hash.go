package envid

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"gopkg.in/yaml.v3"
)

// ComputeHash returns a 12-character hex prefix of SHA256 over the
// canonical, deterministic encoding of p with Hash and GeneratedAt
// blanked out. Two profiles that differ only in capture time share a
// hash, which makes "are these the same env?" cheap and stable.
func ComputeHash(p *Profile) string {
	if p == nil {
		return ""
	}
	clone := *p
	clone.Hash = ""
	clone.GeneratedAt = time.Time{}

	data, err := yaml.Marshal(&clone)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}
