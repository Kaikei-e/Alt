package sovereign_db

import (
	"encoding/json"
	"log/slog"
)

// unmarshalJSONWarn unmarshals data into dest. On failure it logs a warning and
// leaves dest unchanged rather than silently pretending the field was empty.
func unmarshalJSONWarn(data []byte, dest any, field string) {
	if len(data) == 0 {
		return
	}
	if err := json.Unmarshal(data, dest); err != nil {
		slog.Warn("json unmarshal failed", "field", field, "error", err)
	}
}
