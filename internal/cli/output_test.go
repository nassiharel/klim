package cli

import (
	"strings"
	"testing"
)

type structuredCase struct {
	OptIn    string `json:"opt_in" yaml:"opt_in"`
	OmitMe   string `json:"omit_me,omitempty" yaml:"omit_me,omitempty"`
	MaxTotal int    `json:"max_total" yaml:"max_total"`
}

// TestPrintStructured_JSON_PreservesTagsAndOmitempty pins the JSON
// shape used by printJSON. Field names track the json: tag, and
// `omitempty` drops zero-value strings.
func TestPrintStructured_JSON_PreservesTagsAndOmitempty(t *testing.T) {
	captured := captureStdout(t, func() {
		if err := printStructured(OutputJSON, structuredCase{OptIn: "hi", MaxTotal: 100}); err != nil {
			t.Fatalf("printStructured: %v", err)
		}
	})
	out := string(captured)
	if !strings.Contains(out, `"opt_in": "hi"`) {
		t.Errorf("JSON missing opt_in key:\n%s", out)
	}
	if !strings.Contains(out, `"max_total": 100`) {
		t.Errorf("JSON missing max_total key:\n%s", out)
	}
	if strings.Contains(out, `"omit_me"`) {
		t.Errorf("JSON should have omitted omit_me:\n%s", out)
	}
}

// TestPrintStructured_YAML_HonoursJSONTags is the critical regression
// guard for the JSON-roundtrip routing in printYAML. yaml.v3 doesn't
// honour json: tags natively, so without the routing this would emit
// 'OptIn' / 'MaxTotal' Go field names.
func TestPrintStructured_YAML_HonoursJSONTags(t *testing.T) {
	captured := captureStdout(t, func() {
		if err := printStructured(OutputYAML, structuredCase{OptIn: "hi", MaxTotal: 100}); err != nil {
			t.Fatalf("printStructured: %v", err)
		}
	})
	out := string(captured)
	if !strings.Contains(out, "opt_in:") || !strings.Contains(out, "max_total:") {
		t.Errorf("YAML missing snake_case keys:\n%s", out)
	}
	if strings.Contains(out, "OptIn") || strings.Contains(out, "MaxTotal") {
		t.Errorf("YAML leaked Go field names — JSON-roundtrip broken:\n%s", out)
	}
	if strings.Contains(out, "omit_me") {
		t.Errorf("YAML did not honour omitempty:\n%s", out)
	}
}

// TestPrintStructured_RejectsText catches accidental routing of
// OutputText through this helper. Callers must gate explicitly.
func TestPrintStructured_RejectsText(t *testing.T) {
	err := printStructured(OutputText, structuredCase{})
	if err == nil {
		t.Error("printStructured(OutputText) returned nil; want error")
	}
}
