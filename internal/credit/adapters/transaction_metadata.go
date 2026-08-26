package adapters

import (
	"encoding/json"
	"log/slog"
)

// txMetadata builds the jsonb payload for a transaction row.
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
