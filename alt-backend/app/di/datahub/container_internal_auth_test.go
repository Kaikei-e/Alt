package datahub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"alt/config"
)

// The bearer this root hands kratos_client is the one auth-hub's /internal
// group is keyed on, and that is INTERNAL_AUTH_SECRET — not the HS256 signing
// key. auth-hub refuses to start when the two are equal
// (auth-hub/config/config.go), so a root wired to BackendTokenSecret does not
// degrade: middleware/internal_auth.go answers 403 to every GetSystemUser call
// while alt-data-hub stays healthy.
//
// Asserted at the wire rather than on the struct because the field is
// unexported and the header is what auth-hub compares. GetFirstIdentityID puts
// the value in verbatim (kratos_client/client.go), so a stub auth-hub reading
// X-Internal-Auth sees exactly what the deployed one would.
func TestDataHubComponents_KratosClientPresentsInternalAuthSecret(t *testing.T) {
	const (
		backendTokenSecret = "hs256-signing-key-that-must-not-reach-a-plaintext-header"
		internalAuthSecret = "the-separate-internal-shared-bearer-value"
	)

	var presented string
	authHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented = r.Header.Get("X-Internal-Auth")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":"11111111-2222-3333-4444-555555555555"}`))
	}))
	defer authHub.Close()

	cfg := &config.Config{
		AppEnv:    "development",
		AuthHub:   config.AuthHubConfig{URL: authHub.URL},
		Auth:      config.AuthConfig{BackendTokenSecret: backendTokenSecret, InternalAuthSecret: internalAuthSecret},
		MQHub:     config.MQHubConfig{Enabled: true, ConnectURL: "http://mq-hub:9500"},
		Sovereign: config.SovereignConfig{URL: "http://knowledge-sovereign:9500"},
		Recap:     config.RecapConfig{DefaultPageSize: 500, MaxPageSize: 2000, MaxRangeDays: 8},
	}

	// A nil pool is enough: nothing on this path touches the database, and the
	// alternative would be a live Postgres for a header assertion.
	components := NewDataHubComponents(nil, cfg)

	if _, err := components.KratosClient.GetFirstIdentityID(context.Background()); err != nil {
		t.Fatalf("GetFirstIdentityID() error = %v", err)
	}

	if presented == backendTokenSecret {
		t.Fatalf("X-Internal-Auth carried BACKEND_TOKEN_SECRET; auth-hub keys /internal on " +
			"INTERNAL_AUTH_SECRET and answers 403 to every GetSystemUser call")
	}
	if presented != internalAuthSecret {
		t.Errorf("X-Internal-Auth = %q, want INTERNAL_AUTH_SECRET %q", presented, internalAuthSecret)
	}
}
