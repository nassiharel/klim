package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/audit"
	"github.com/nassiharel/klim/internal/compliance"
	"github.com/nassiharel/klim/internal/doctor"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/score"
)

// computeScoreReport runs doctor.Diagnose, compliance (if configured),
// and audit.Analyze for the supplied tools, then assembles a
// score.Result. Returns the result plus the audit warning / info
// counts callers may want to surface separately (e.g. the audit
// badge in klim badge).
//
// Extracted so klim score and klim badge cannot diverge on what
// counts as a "canonical" score for a given scan — they share this
// single assembly path. Compliance policy load errors are written
// to stderr exactly once, here.
func computeScoreReport(cmd *cobra.Command, tools []registry.Tool) (score.Result, int, int) {
	doctorIssues := doctor.Diagnose(tools, doctor.ScanMeta{})

	var compResult *compliance.Result
	var compErrStr string
	if policyPath := findPolicyPath(cfgFrom(cmd)); policyPath != "" {
		policy, loadErr := compliance.LoadPolicy(policyPath)
		if loadErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "  ⚠ Compliance policy error: %v\n", loadErr)
			compErrStr = loadErr.Error()
		} else {
			r := compliance.Check(policy, tools, loadVulnSeveritiesForCompliance())
			compResult = &r
		}
	}

	findings, _ := audit.Analyze(tools)
	auditWarns, auditInfos := audit.CountBySeverity(findings)

	result := score.Compute(score.Input{
		Tools:         tools,
		DoctorIssues:  doctorIssues,
		AuditWarnings: auditWarns,
		AuditInfos:    auditInfos,
		CompResult:    compResult,
		ComplianceErr: compErrStr,
	})

	return result, auditWarns, auditInfos
}
