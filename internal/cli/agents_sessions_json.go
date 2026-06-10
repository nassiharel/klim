package cli

import "encoding/json"

// jsonStdUnmarshal is a thin alias used by agents_sessions.go. It
// exists in its own file so the main runner stays free of an
// encoding/json import (the runner relies on jsonUnmarshalFn to
// dispatch here).
func jsonStdUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}