package cli

import "fmt"

// Exit codes returned by Run. They follow the convention documented in
// docs/cli-conventions.md:
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
// `klim plan --detailed-exitcode` — there's no failure, but the diff
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
