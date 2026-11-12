package v2

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/connect/v2/middleware"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"
)

func TestSetupConnectHandlers_WiresUserQuotaLimiterBeforeUsecase(t *testing.T) {
	container, cfg, logger := testDeps()
	limiter, err := middleware.NewUserQuotaLimiter(config.UserQuotaConfig{
		MaxTrackedUsers: 16,
		Summarize:       config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1},
		AugurChat:       config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1},
		MorningChat:     config.OperationQuotaConfig{Rate: 1, Burst: 1, Window: time.Minute, MaxInFlight: 1},
		MorningRegenerate: config.OperationQuotaConfig{
			Rate:        1,
			Burst:       1,
			Window:      time.Hour,
			MaxInFlight: 1,
		},
	})
	require.NoError(t, err)
	container.UserQuotaLimiter = limiter

	mux := http.NewServeMux()
	SetupConnectHandlers(mux, container, cfg, logger)
	token := issueTestBackendToken(t)

	// 1st call consumes the single token (reaching the nil MorningLetterUsecase and panicking).
	func() {
		defer func() { _ = recover() }()
		req1 := httptest.NewRequest(http.MethodPost, morningletterv2connect.MorningLetterReadServiceRegenerateLatestProcedure, strings.NewReader("{}"))
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("X-Alt-Backend-Token", token)
		mux.ServeHTTP(httptest.NewRecorder(), req1)
	}()

	// 2nd call for the same user must be rejected by the quota interceptor (HTTP 429 / resource_exhausted)
	// before reaching MorningLetterUsecase (no panic).
	req2 := httptest.NewRequest(http.MethodPost, morningletterv2connect.MorningLetterReadServiceRegenerateLatestProcedure, strings.NewReader("{}"))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Alt-Backend-Token", token)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}
