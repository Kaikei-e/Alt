package handler

import (
	"encoding/json"
	"net/http"
)

// HealthHandler returns the service health status.
func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "knowledge-sovereign"})
}
