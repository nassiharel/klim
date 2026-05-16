package promote

import (
	"bytes"
	"os"

	"gopkg.in/yaml.v3"
)

// convertSkillFrontmatter rewrites a SKILL.md frontmatter so its
// fields match the target provider's conventions. Claude Code uses
// hyphen-prefixed keys (`when_to_use`, `allowed-tools`, …); Copilot
// CLI accepts the same names plus a couple of provider-specific
// aliases. The body is left untouched.
//
// Returns the new SKILL.md bytes. When parsing fails we fall back to
// re-emitting a minimal frontmatter from the SkillRef fields so the
// destination is always valid.
func convertSkillFrontmatter(src SkillRef, targetProvider string) []byte {
	body := readSkillBody(src.Path)
	// Build a fresh frontmatter from the SkillRef. The source's
	// existing frontmatter on disk may differ but the SkillRef
	// already captured the canonical fields we care about.
	fm := map[string]interface{}{
		"name": src.Name,
	}
	if src.Description != "" {
		fm["description"] = src.Description
	}
	if src.WhenToUse != "" {
		fm["when_to_use"] = src.WhenToUse
	}
	if src.AllowedTools != "" {
		// Both providers accept allowed-tools (hyphen); keep that form.
		fm["allowed-tools"] = src.AllowedTools
	}
	if src.Model != "" {
		fm["model"] = src.Model
	}
	// Mark the promotion so users tracing edits can see where this
	// file came from. Hidden under a comment line, not a key, so it
	// never breaks the frontmatter parser.
	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		yamlBytes = []byte("name: " + src.Name + "\n")
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n")
	// Provenance footer (kept *after* the frontmatter so it shows up
	// when the user reads the file but doesn't confuse parsers).
	buf.WriteString("<!-- klim: promoted from " + src.Provider + " to " + targetProvider + " -->\n\n")
	buf.Write(body)
	return buf.Bytes()
}

// readSkillBody returns the markdown body (everything after the
// second `---`). Returns "" on any error — the converter is best-
// effort.
func readSkillBody(path string) []byte {
	if path == "" {
		return nil
	}
	data, err := skillFileRead(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	const fence = "---"
	rest := bytes.TrimSpace(data)
	if !bytes.HasPrefix(rest, []byte(fence)) {
		return data
	}
	rest = rest[len(fence):]
	// Skip optional newline after opening fence.
	for len(rest) > 0 && (rest[0] == '\r' || rest[0] == '\n') {
		rest = rest[1:]
	}
	if idx := bytes.Index(rest, []byte("\n"+fence)); idx >= 0 {
		body := rest[idx+len(fence)+1:]
		// Strip leading newline.
		for len(body) > 0 && (body[0] == '\r' || body[0] == '\n') {
			body = body[1:]
		}
		return body
	}
	return data
}

// skillFileRead reads a SKILL.md file. PR #77 review: collapsed
// three stacked test seams (skillFileRead -> osReadFile ->
// readFileBridge) into one. Tests override skillFileRead directly.
var skillFileRead = os.ReadFile
