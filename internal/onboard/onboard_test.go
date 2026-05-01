package onboard

import (
	"testing"

	"github.com/nassiharel/clim/internal/registry"
)

func TestFindRole(t *testing.T) {
	tests := []struct {
		name  string
		found bool
	}{
		{"web", true},
		{"WEB", true},
		{"devops", true},
		{"DevOps", true},
		{"nonexistent", false},
		{"", false},
	}
	for _, tt := range tests {
		r := FindRole(tt.name)
		if tt.found && r == nil {
			t.Errorf("FindRole(%q) = nil, want non-nil", tt.name)
		}
		if !tt.found && r != nil {
			t.Errorf("FindRole(%q) = %v, want nil", tt.name, r)
		}
	}
}

func allPkgs() registry.PackageIDs {
	return registry.PackageIDs{Brew: "x", Winget: "x", Apt: "x"}
}

func TestRecommend_FiltersInstalled(t *testing.T) {
	role := &Roles[0] // web
	tools := []registry.Tool{
		{
			Name:     "node",
			Category: "JavaScript",
			Tags:     []string{"javascript", "node"},
			Packages: allPkgs(),
			Instances: []registry.Instance{
				{Path: "/usr/bin/node", Source: "brew"},
			},
		},
		{
			Name:     "deno",
			Category: "JavaScript",
			Tags:     []string{"javascript", "typescript"},
			Packages: allPkgs(),
		},
	}
	result := Recommend(role, tools, 0)
	// node is installed, should be filtered out
	for _, r := range result {
		if r.Tool.Name == "node" {
			t.Error("installed tool 'node' should be filtered out")
		}
	}
	// deno should be included
	found := false
	for _, r := range result {
		if r.Tool.Name == "deno" {
			found = true
		}
	}
	if !found {
		t.Error("uninstalled tool 'deno' should be recommended")
	}
}

func TestRecommend_MaxResults(t *testing.T) {
	role := &Roles[0] // web
	var tools []registry.Tool
	for i := 0; i < 30; i++ {
		tools = append(tools, registry.Tool{
			Name:     "tool" + string(rune('a'+i)),
			Category: "JavaScript",
			Tags:     []string{"web"},
			Packages: allPkgs(),
		})
	}
	result := Recommend(role, tools, 5)
	if len(result) != 5 {
		t.Errorf("got %d results, want 5", len(result))
	}
}

func TestRecommend_DeterministicOrder(t *testing.T) {
	role := &Roles[0] // web
	tools := []registry.Tool{
		{Name: "zzz", Category: "JavaScript", Packages: allPkgs()},
		{Name: "aaa", Category: "JavaScript", Packages: allPkgs()},
		{Name: "mmm", Category: "JavaScript", Packages: allPkgs()},
	}
	r1 := Recommend(role, tools, 0)
	r2 := Recommend(role, tools, 0)
	if len(r1) != len(r2) {
		t.Fatal("different lengths")
	}
	for i := range r1 {
		if r1[i].Tool.Name != r2[i].Tool.Name {
			t.Errorf("order differs at %d: %s vs %s", i, r1[i].Tool.Name, r2[i].Tool.Name)
		}
	}
	// Should be sorted by name since all have same score
	if r1[0].Tool.Name != "aaa" {
		t.Errorf("expected aaa first, got %s", r1[0].Tool.Name)
	}
}
