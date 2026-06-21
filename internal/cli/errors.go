package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Exit codes returned by Run. They follow the convention documented in
// CLI-CONVENTIONS.md:
//
//	0 — success
//	1 — runtime error (default)
//	2 — usage error (bad flags / args)
//	3 — partial failure (some operations succeeded, some failed)
const (
	ExitOK             = 0
	ExitRuntime        = 1
	ExitUsage          = 2
	ExitPartialFailure = 3
)

// UsageError signals the user invoked the CLI incorrectly (missing/extra args,
// unknown flags, malformed input). Run translates this to ExitUsage.
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

func usageErrorf(format string, a ...any) error {
	return &UsageError{Err: fmt.Errorf(format, a...)}
}

// PartialFailureError signals a multi-item operation where some items
// succeeded and some failed. Run translates this to ExitPartialFailure.
type PartialFailureError struct {
	Succeeded int
	Failed    int
	Op        string
}

func (e *PartialFailureError) Error() string {
	return fmt.Sprintf("%s: %d succeeded, %d failed", e.Op, e.Succeeded, e.Failed)
}

// PendingChangesError signals a non-failure outcome that should still
// map to ExitPartialFailure (exit 3). The canonical use case is
// `klim plan show --detailed-exitcode` — there's no failure, but the diff
// is non-empty and the CI caller asked us to gate on that. Keeping
// this distinct from PartialFailureError avoids the misleading
// "0 succeeded, N failed" framing for an outcome that's really
// "N changes pending".
type PendingChangesError struct {
	Op      string
	Pending int
}

func (e *PendingChangesError) Error() string {
	if e.Pending == 1 {
		return e.Op + ": 1 change pending"
	}
	return fmt.Sprintf("%s: %d changes pending", e.Op, e.Pending)
}

// usageArgs wraps a Cobra args-validator so that any validation
// failure is returned as a *UsageError. Without this wrap, Cobra's
// built-in validators (cobra.ExactArgs, cobra.MaximumNArgs, …) return
// plain `errors.New(...)` instances whose message starts with words
// like "accepts" — those don't match isCobraUsageError's prefix list
// in root.go, so Run() exits with code 1 (runtime error) instead of
// code 2 (usage error) per CLI-CONVENTIONS.md. Use this whenever a
// Cobra command takes a constrained arg count and wants the correct
// exit-code mapping.
//
// Example:
//
//	&cobra.Command{
//	    Args: usageArgs(cobra.MaximumNArgs(1)),
//	    RunE: ...,
//	}
func usageArgs(fn cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := fn(cmd, args); err != nil {
			return &UsageError{Err: err}
		}
		return nil
	}
}
