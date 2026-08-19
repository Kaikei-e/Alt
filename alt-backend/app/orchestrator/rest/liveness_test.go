package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRestHandleLiveness_DoesNotClaimDatabase(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := restHandleLiveness(c); err != nil {
		t.Fatalf("restHandleLiveness: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "healthy" {
		t.Errorf("status = %q, want healthy", body["status"])
	}
	if _, ok := body["database"]; ok {
		t.Fatalf("liveness must not claim database connectivity, got %v", body)
	}
}
