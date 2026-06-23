package doctor

import (
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

func TestActions_MultipleInstallationsDetected(t *testing.T) {
	tools := []registry.Tool{{
		Name:        "node",
		DisplayName: "Node.js",
		Instances: []registry.Instance{
			{Path: "/a/node", Version: "20", Source: "manual"},
			{Path: "/b/node", Version: "18", Source: "manual"},
		},
	}}
	issues := checkMultipleInstallations(tools)
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].Severity != SeverityError {
		t.Errorf("Severity = %q, want %q", issues[0].Severity, SeverityError)
	}
	if issues[0].Category != CategoryTools {
		t.Errorf("Category = %q, want %q", issues[0].Category, CategoryTools)
	}
}
