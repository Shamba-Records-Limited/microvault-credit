package adapters

import (
	"encoding/json"
	"log/slog"
)

// txMetadata builds the jsonb payload for a transaction row.
//
// Metadata is diagnostic: it holds what a provider reported that has no
// first-class column, so a stuck loan can be reconciled against what the anchor
// actually said rather than what we inferred. Nothing in the application should
// branch on a key in here — a key worth a predicate has earned a column.
//
// Keys with zero values are omitted rather than written as null, keeping the
// payload to what was actually observed. Returns nil when nothing was, which
// leaves the column NULL rather than storing an empty object.
func txMetadata(fields map[string]any) *string {
	present := make(map[string]any, len(fields))
	for k, v := range fields {
		switch t := v.(type) {
		case nil:
			continue
		case string:
			if t == "" {
				continue
			}
		case *string:
			if t == nil || *t == "" {
				continue
			}
			v = *t
		case int64:
			if t == 0 {
				continue
			}
		}
		present[k] = v
	}
	if len(present) == 0 {
		return nil
	}

	encoded, err := json.Marshal(present)
	if err != nil {
		// Never fatal: metadata is diagnostic, so a row without it is still a
		// correct row. Losing the transaction over it would not be.
		slog.Warn("could not encode transaction metadata", "error", err)
		return nil
	}
	out := string(encoded)
	return &out
}
