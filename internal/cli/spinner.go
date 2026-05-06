package cli

import "github.com/nassiharel/klim/internal/progress"

// spinnerFor returns a progress spinner suited to the given output
// format. For OutputText callers get the regular animated/staticspinner
// on stderr; for OutputJSON / OutputYAML it returns a silent no-op
// spinner so progress chatter never interleaves with the structured
// data the caller is likely piping to jq, yq, or another tool.
func spinnerFor(format OutputFormat, msg string) *progress.Spinner {
	if format == OutputText {
		return progress.New(msg)
	}
	return progress.NewSilent()
}
