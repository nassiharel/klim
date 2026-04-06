package registry

import "testing"

func TestMergeToolDefs_NewEmbeddedToolAdded(t *testing.T) {
	embedded := []toolDef{
		{Name: "git", DisplayName: "Git", Enabled: true, Packages: packageDef{Brew: "git"}},
		{Name: "rg", DisplayName: "ripgrep", Enabled: true, Packages: packageDef{Brew: "ripgrep"}},
	}
	user := []toolDef{
		{Name: "git", DisplayName: "Git", Enabled: true, Packages: packageDef{Brew: "git"}},
	}

	merged, changed := mergeToolDefs(embedded, user)

	if !changed {
		t.Fatal("expected changed=true when embedded has a new tool")
	}
	if len(merged) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(merged))
	}
	if merged[1].Name != "rg" {
		t.Errorf("expected new tool 'rg', got %q", merged[1].Name)
	}
}

func TestMergeToolDefs_UserCustomToolPreserved(t *testing.T) {
	embedded := []toolDef{
		{Name: "git", DisplayName: "Git", Enabled: true},
	}
	user := []toolDef{
		{Name: "git", DisplayName: "Git", Enabled: true},
		{Name: "my-tool", DisplayName: "My Tool", Enabled: true, Packages: packageDef{Brew: "my-tool"}},
	}

	merged, _ := mergeToolDefs(embedded, user)

	if len(merged) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(merged))
	}
	if merged[1].Name != "my-tool" {
		t.Errorf("expected user custom tool preserved, got %q", merged[1].Name)
	}
	if merged[1].Packages.Brew != "my-tool" {
		t.Errorf("expected user custom tool packages preserved, got %q", merged[1].Packages.Brew)
	}
}

func TestMergeToolDefs_UserEnabledPreserved(t *testing.T) {
	embedded := []toolDef{
		{Name: "git", DisplayName: "Git", Enabled: true},
	}
	user := []toolDef{
		{Name: "git", DisplayName: "Git", Enabled: false}, // user disabled it
	}

	merged, _ := mergeToolDefs(embedded, user)

	if merged[0].Enabled {
		t.Error("expected user's enabled=false to be preserved")
	}
}

func TestMergeToolDefs_EmbeddedFillsPackageGaps(t *testing.T) {
	embedded := []toolDef{
		{Name: "bat", Enabled: true, Packages: packageDef{
			Winget: "sharkdp.bat",
			Choco:  "bat",
			Brew:   "bat",
			Apt:    "bat",
		}},
	}
	user := []toolDef{
		{Name: "bat", Enabled: true, Packages: packageDef{
			Winget: "sharkdp.bat",
			// choco, brew, apt missing — should be filled from embedded
		}},
	}

	merged, changed := mergeToolDefs(embedded, user)

	if !changed {
		t.Fatal("expected changed=true when embedded fills package gaps")
	}
	if merged[0].Packages.Choco != "bat" {
		t.Errorf("expected choco filled from embedded, got %q", merged[0].Packages.Choco)
	}
	if merged[0].Packages.Brew != "bat" {
		t.Errorf("expected brew filled from embedded, got %q", merged[0].Packages.Brew)
	}
	if merged[0].Packages.Apt != "bat" {
		t.Errorf("expected apt filled from embedded, got %q", merged[0].Packages.Apt)
	}
}

func TestMergeToolDefs_UserPackageOverridesEmbedded(t *testing.T) {
	embedded := []toolDef{
		{Name: "git", Enabled: true, Packages: packageDef{Brew: "git"}},
	}
	user := []toolDef{
		{Name: "git", Enabled: true, Packages: packageDef{Brew: "git-custom"}},
	}

	merged, _ := mergeToolDefs(embedded, user)

	if merged[0].Packages.Brew != "git-custom" {
		t.Errorf("expected user's brew override preserved, got %q", merged[0].Packages.Brew)
	}
}

func TestMergeToolDefs_NoChangesReturnsFalse(t *testing.T) {
	defs := []toolDef{
		{Name: "git", DisplayName: "Git", Enabled: true, Packages: packageDef{Brew: "git"}},
	}

	_, changed := mergeToolDefs(defs, defs)

	if changed {
		t.Error("expected changed=false when embedded and user are identical")
	}
}

func TestMergeToolDefs_EmbeddedMetadataWins(t *testing.T) {
	embedded := []toolDef{
		{Name: "git", DisplayName: "Git (Updated)", Category: "VCS", BinaryNames: []string{"git"}, Enabled: true},
	}
	user := []toolDef{
		{Name: "git", DisplayName: "Git (Old)", Category: "Old Category", BinaryNames: []string{"old-git"}, Enabled: true},
	}

	merged, _ := mergeToolDefs(embedded, user)

	if merged[0].DisplayName != "Git (Updated)" {
		t.Errorf("expected embedded display_name, got %q", merged[0].DisplayName)
	}
	if merged[0].Category != "VCS" {
		t.Errorf("expected embedded category, got %q", merged[0].Category)
	}
	if len(merged[0].BinaryNames) != 1 || merged[0].BinaryNames[0] != "git" {
		t.Errorf("expected embedded binary_names, got %v", merged[0].BinaryNames)
	}
}

func TestMergeToolDefs_OrderEmbeddedFirstThenUserCustom(t *testing.T) {
	embedded := []toolDef{
		{Name: "b-tool", Enabled: true},
		{Name: "a-tool", Enabled: true},
	}
	user := []toolDef{
		{Name: "a-tool", Enabled: true},
		{Name: "z-custom", Enabled: true},
		{Name: "b-tool", Enabled: true},
	}

	merged, _ := mergeToolDefs(embedded, user)

	if len(merged) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(merged))
	}
	// Embedded order preserved, then user-only appended.
	if merged[0].Name != "b-tool" {
		t.Errorf("expected embedded order: b-tool first, got %q", merged[0].Name)
	}
	if merged[1].Name != "a-tool" {
		t.Errorf("expected embedded order: a-tool second, got %q", merged[1].Name)
	}
	if merged[2].Name != "z-custom" {
		t.Errorf("expected user-only tool last, got %q", merged[2].Name)
	}
}

func TestMergePackages(t *testing.T) {
	embedded := packageDef{
		Winget: "X.Y",
		Choco:  "x",
		Brew:   "x",
		Apt:    "x",
		Snap:   "x",
		NPM:    "x",
	}
	user := packageDef{
		Winget: "Custom.ID", // user override — keep
		Choco:  "",          // gap — fill from embedded
		Brew:   "x-custom",  // user override — keep
		Apt:    "",          // gap — fill from embedded
		Snap:   "",          // gap — fill from embedded
		NPM:    "",          // gap — fill from embedded
	}

	got := mergePackages(embedded, user)

	if got.Winget != "Custom.ID" {
		t.Errorf("Winget: expected user override 'Custom.ID', got %q", got.Winget)
	}
	if got.Choco != "x" {
		t.Errorf("Choco: expected embedded fill 'x', got %q", got.Choco)
	}
	if got.Brew != "x-custom" {
		t.Errorf("Brew: expected user override 'x-custom', got %q", got.Brew)
	}
	if got.Apt != "x" {
		t.Errorf("Apt: expected embedded fill 'x', got %q", got.Apt)
	}
	if got.Snap != "x" {
		t.Errorf("Snap: expected embedded fill 'x', got %q", got.Snap)
	}
	if got.NPM != "x" {
		t.Errorf("NPM: expected embedded fill 'x', got %q", got.NPM)
	}
}

func TestPickNonEmpty(t *testing.T) {
	if got := pickNonEmpty("a", "b"); got != "a" {
		t.Errorf("pickNonEmpty(a, b) = %q, want a", got)
	}
	if got := pickNonEmpty("", "b"); got != "b" {
		t.Errorf("pickNonEmpty('', b) = %q, want b", got)
	}
	if got := pickNonEmpty("", ""); got != "" {
		t.Errorf("pickNonEmpty('', '') = %q, want ''", got)
	}
}
