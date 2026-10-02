package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/config"
	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
	sovereignv1connect "knowledge-sovereign/gen/proto/services/sovereign/v1/sovereignv1connect"
)

type capturedRepo struct {
	mockRepo
	lastMethod  string
	lastPayload json.RawMessage
}

func (c *capturedRepo) DismissKnowledgeHomeItem(_ context.Context, p json.RawMessage) error {
	c.lastMethod = "DismissKnowledgeHomeItem"
	c.lastPayload = p
	return nil
}

func (c *capturedRepo) ClearSupersedeState(_ context.Context, p json.RawMessage) error {
	c.lastMethod = "ClearSupersedeState"
	c.lastPayload = p
	return nil
}

func (c *capturedRepo) PatchKnowledgeHomeItemURL(_ context.Context, p json.RawMessage) error {
	c.lastMethod = "PatchKnowledgeHomeItemURL"
	c.lastPayload = p
	return nil
}

func (c *capturedRepo) SnoozeRecallCandidate(_ context.Context, p json.RawMessage) error {
	c.lastMethod = "SnoozeRecallCandidate"
	c.lastPayload = p
	return nil
}

func (c *capturedRepo) DismissRecallCandidate(_ context.Context, p json.RawMessage) error {
	c.lastMethod = "DismissRecallCandidate"
	c.lastPayload = p
	return nil
}

func setupTenantAuthTest(t *testing.T) (
	repo *capturedRepo,
	client sovereignv1connect.KnowledgeSovereignServiceClient,
	backendToken string,
	jwtSecret []byte,
	targetUserID string,
	targetTenantID string,
	otherTenantID string,
	cleanup func(),
) {
	t.Helper()
	backendToken = "auth-backend-test-token-long-enough-1234"
	jwtSecretStr := "auth-jwt-secret-key-long-enough-5678"
	jwtSecret = []byte(jwtSecretStr)

	targetUserID = uuid.New().String()
	targetTenantID = uuid.New().String()
	otherTenantID = uuid.New().String()

	pol := &config.AuthPolicy{
		Services: []config.ServicePolicy{
			{
				Name:  "alt-backend",
				Token: backendToken,
				AllowedMethods: map[string]bool{
					"/services.sovereign.v1.KnowledgeSovereignService/ApplyProjectionMutation": true,
					"/services.sovereign.v1.KnowledgeSovereignService/ApplyRecallMutation":     true,
					"/services.sovereign.v1.KnowledgeSovereignService/ApplyCurationMutation":   true,
				},
				RequireUserToken: true,
				AllowSystemScope: false,
			},
		},
	}

	verifier := &config.LocalUserJWTVerifier{
		Secret:   jwtSecretStr,
		Issuer:   "auth-hub",
		Audience: "alt-backend",
	}

	repo = &capturedRepo{}
	h := NewSovereignHandler(repo)
	interceptor := NewPolicyAuthInterceptor(pol, verifier, true)

	mux := http.NewServeMux()
	path, rpcHandler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(h, connect.WithInterceptors(interceptor))
	mux.Handle(path, rpcHandler)
	srv := httptest.NewServer(mux)

	client = sovereignv1connect.NewKnowledgeSovereignServiceClient(srv.Client(), srv.URL)
	cleanup = srv.Close
	return
}

func TestApplyProjectionMutation_Dismiss_ForeignTenant_Rejected(t *testing.T) {
	_, client, backendToken, jwtSecret, targetUserID, targetTenantID, otherTenantID, cleanup := setupTenantAuthTest(t)
	defer cleanup()

	validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

	req := connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
		MutationType:   MutationDismissHomeItem,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","tenant_id":"` + otherTenantID + `","item_key":"article:test","projection_version":1,"dismissed_at":"2026-10-02T12:00:00Z"}`),
		IdempotencyKey: uuid.New().String(),
	})
	req.Header().Set("Authorization", "Bearer "+backendToken)
	req.Header().Set("X-Alt-Backend-Token", validJWT)

	_, err := client.ApplyProjectionMutation(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "foreign tenant in DismissHomeItem payload MUST be rejected with PermissionDenied")
}

func TestApplyProjectionMutation_Dismiss_MissingEmptyNullTenant_NormalizedWithClaims(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "missing tenant_id",
			payload: `{"user_id":"%s","item_key":"article:test","projection_version":1,"dismissed_at":"2026-10-02T12:00:00Z"}`,
		},
		{
			name:    "empty tenant_id",
			payload: `{"user_id":"%s","tenant_id":"","item_key":"article:test","projection_version":1,"dismissed_at":"2026-10-02T12:00:00Z"}`,
		},
		{
			name:    "null tenant_id",
			payload: `{"user_id":"%s","tenant_id":null,"item_key":"article:test","projection_version":1,"dismissed_at":"2026-10-02T12:00:00Z"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, client, backendToken, jwtSecret, targetUserID, targetTenantID, _, cleanup := setupTenantAuthTest(t)
			defer cleanup()

			validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

			body := []byte(strings.ReplaceAll(tc.payload, "%s", targetUserID))
			req := connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
				MutationType:   MutationDismissHomeItem,
				EntityId:       uuid.New().String(),
				Payload:        body,
				IdempotencyKey: uuid.New().String(),
			})
			req.Header().Set("Authorization", "Bearer "+backendToken)
			req.Header().Set("X-Alt-Backend-Token", validJWT)

			resp, err := client.ApplyProjectionMutation(context.Background(), req)
			require.NoError(t, err)
			assert.True(t, resp.Msg.Success)

			var captured map[string]any
			require.NoError(t, json.Unmarshal(repo.lastPayload, &captured))
			assert.Equal(t, targetTenantID, captured["tenant_id"], "repo payload MUST be normalized with claims tenant_id")
			assert.Equal(t, targetUserID, captured["user_id"], "repo payload MUST have claims user_id")
		})
	}
}

func TestApplyProjectionMutation_ClearSupersede_TenantAuth(t *testing.T) {
	repo, client, backendToken, jwtSecret, targetUserID, targetTenantID, otherTenantID, cleanup := setupTenantAuthTest(t)
	defer cleanup()

	validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

	// 1. Foreign tenant -> rejected
	reqForeign := connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
		MutationType:   MutationClearSupersede,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","tenant_id":"` + otherTenantID + `","item_key":"k","projection_version":1}`),
		IdempotencyKey: uuid.New().String(),
	})
	reqForeign.Header().Set("Authorization", "Bearer "+backendToken)
	reqForeign.Header().Set("X-Alt-Backend-Token", validJWT)
	_, err := client.ApplyProjectionMutation(context.Background(), reqForeign)
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	// 2. Missing tenant -> normalized
	reqMissing := connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
		MutationType:   MutationClearSupersede,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","item_key":"k","projection_version":1}`),
		IdempotencyKey: uuid.New().String(),
	})
	reqMissing.Header().Set("Authorization", "Bearer "+backendToken)
	reqMissing.Header().Set("X-Alt-Backend-Token", validJWT)
	resp, err := client.ApplyProjectionMutation(context.Background(), reqMissing)
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)

	var captured map[string]any
	require.NoError(t, json.Unmarshal(repo.lastPayload, &captured))
	assert.Equal(t, targetTenantID, captured["tenant_id"])
}

func TestApplyProjectionMutation_PatchURL_TenantAuth(t *testing.T) {
	repo, client, backendToken, jwtSecret, targetUserID, targetTenantID, otherTenantID, cleanup := setupTenantAuthTest(t)
	defer cleanup()

	validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

	// 1. Foreign tenant -> rejected
	reqForeign := connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
		MutationType:   MutationPatchHomeItemURL,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","tenant_id":"` + otherTenantID + `","item_key":"k","projection_version":1,"url":"https://example.com"}`),
		IdempotencyKey: uuid.New().String(),
	})
	reqForeign.Header().Set("Authorization", "Bearer "+backendToken)
	reqForeign.Header().Set("X-Alt-Backend-Token", validJWT)
	_, err := client.ApplyProjectionMutation(context.Background(), reqForeign)
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	// 2. Missing tenant -> normalized
	reqMissing := connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
		MutationType:   MutationPatchHomeItemURL,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","item_key":"k","projection_version":1,"url":"https://example.com"}`),
		IdempotencyKey: uuid.New().String(),
	})
	reqMissing.Header().Set("Authorization", "Bearer "+backendToken)
	reqMissing.Header().Set("X-Alt-Backend-Token", validJWT)
	resp, err := client.ApplyProjectionMutation(context.Background(), reqMissing)
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)

	var captured map[string]any
	require.NoError(t, json.Unmarshal(repo.lastPayload, &captured))
	assert.Equal(t, targetTenantID, captured["tenant_id"])
}

func TestApplyRecallMutation_SnoozeAndDismiss_TenantAuth(t *testing.T) {
	repo, client, backendToken, jwtSecret, targetUserID, targetTenantID, otherTenantID, cleanup := setupTenantAuthTest(t)
	defer cleanup()

	validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

	// 1. Snooze foreign tenant -> rejected
	reqSnoozeForeign := connect.NewRequest(&sovereignv1.ApplyRecallMutationRequest{
		MutationType:   MutationSnoozeCandidate,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","tenant_id":"` + otherTenantID + `","item_key":"k","until":"2026-10-03T12:00:00Z","occurred_at":"2026-10-02T12:00:00Z"}`),
		IdempotencyKey: uuid.New().String(),
	})
	reqSnoozeForeign.Header().Set("Authorization", "Bearer "+backendToken)
	reqSnoozeForeign.Header().Set("X-Alt-Backend-Token", validJWT)
	_, err := client.ApplyRecallMutation(context.Background(), reqSnoozeForeign)
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	// 2. Snooze missing tenant -> normalized
	reqSnoozeMissing := connect.NewRequest(&sovereignv1.ApplyRecallMutationRequest{
		MutationType:   MutationSnoozeCandidate,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","item_key":"k","until":"2026-10-03T12:00:00Z","occurred_at":"2026-10-02T12:00:00Z"}`),
		IdempotencyKey: uuid.New().String(),
	})
	reqSnoozeMissing.Header().Set("Authorization", "Bearer "+backendToken)
	reqSnoozeMissing.Header().Set("X-Alt-Backend-Token", validJWT)
	resp, err := client.ApplyRecallMutation(context.Background(), reqSnoozeMissing)
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)

	var captured map[string]any
	require.NoError(t, json.Unmarshal(repo.lastPayload, &captured))
	assert.Equal(t, targetTenantID, captured["tenant_id"])

	// 3. Dismiss foreign tenant -> rejected
	reqDismissForeign := connect.NewRequest(&sovereignv1.ApplyRecallMutationRequest{
		MutationType:   MutationDismissCandidate,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","tenant_id":"` + otherTenantID + `","item_key":"k","occurred_at":"2026-10-02T12:00:00Z"}`),
		IdempotencyKey: uuid.New().String(),
	})
	reqDismissForeign.Header().Set("Authorization", "Bearer "+backendToken)
	reqDismissForeign.Header().Set("X-Alt-Backend-Token", validJWT)
	_, err = client.ApplyRecallMutation(context.Background(), reqDismissForeign)
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	// 4. Dismiss empty tenant -> normalized
	reqDismissEmpty := connect.NewRequest(&sovereignv1.ApplyRecallMutationRequest{
		MutationType:   MutationDismissCandidate,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","tenant_id":"","item_key":"k","occurred_at":"2026-10-02T12:00:00Z"}`),
		IdempotencyKey: uuid.New().String(),
	})
	reqDismissEmpty.Header().Set("Authorization", "Bearer "+backendToken)
	reqDismissEmpty.Header().Set("X-Alt-Backend-Token", validJWT)
	resp, err = client.ApplyRecallMutation(context.Background(), reqDismissEmpty)
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)

	require.NoError(t, json.Unmarshal(repo.lastPayload, &captured))
	assert.Equal(t, targetTenantID, captured["tenant_id"])
}

func TestApplyCurationMutation_Dismiss_ForeignTenant_Rejected(t *testing.T) {
	_, client, backendToken, jwtSecret, targetUserID, targetTenantID, otherTenantID, cleanup := setupTenantAuthTest(t)
	defer cleanup()

	validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

	req := connect.NewRequest(&sovereignv1.ApplyCurationMutationRequest{
		MutationType:   MutationDismissCuration,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","tenant_id":"` + otherTenantID + `","item_key":"article:curation-1","dismissed_at":"2026-10-02T12:00:00Z"}`),
		IdempotencyKey: uuid.New().String(),
	})
	req.Header().Set("Authorization", "Bearer "+backendToken)
	req.Header().Set("X-Alt-Backend-Token", validJWT)

	_, err := client.ApplyCurationMutation(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "foreign tenant in DismissCuration payload MUST be rejected with PermissionDenied")
}

func TestApplyCurationMutation_Dismiss_MissingEmptyNullTenant_NormalizedWithClaims(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "missing tenant_id",
			payload: `{"user_id":"%s","item_key":"article:c1","dismissed_at":"2026-10-02T12:00:00Z"}`,
		},
		{
			name:    "empty tenant_id",
			payload: `{"user_id":"%s","tenant_id":"","item_key":"article:c1","dismissed_at":"2026-10-02T12:00:00Z"}`,
		},
		{
			name:    "null tenant_id",
			payload: `{"user_id":"%s","tenant_id":null,"item_key":"article:c1","dismissed_at":"2026-10-02T12:00:00Z"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, client, backendToken, jwtSecret, targetUserID, targetTenantID, _, cleanup := setupTenantAuthTest(t)
			defer cleanup()

			validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

			body := []byte(strings.ReplaceAll(tc.payload, "%s", targetUserID))
			req := connect.NewRequest(&sovereignv1.ApplyCurationMutationRequest{
				MutationType:   MutationDismissCuration,
				EntityId:       uuid.New().String(),
				Payload:        body,
				IdempotencyKey: uuid.New().String(),
			})
			req.Header().Set("Authorization", "Bearer "+backendToken)
			req.Header().Set("X-Alt-Backend-Token", validJWT)

			resp, err := client.ApplyCurationMutation(context.Background(), req)
			require.NoError(t, err)
			assert.True(t, resp.Msg.Success)

			var captured map[string]any
			require.NoError(t, json.Unmarshal(repo.lastPayload, &captured))
			assert.Equal(t, targetTenantID, captured["tenant_id"], "repo payload MUST be normalized with claims tenant_id")
			assert.Equal(t, targetUserID, captured["user_id"], "repo payload MUST have claims user_id")
		})
	}
}

func TestApplyMutation_CaseVariantBypassPrevention(t *testing.T) {
	repo, client, backendToken, jwtSecret, targetUserID, targetTenantID, otherTenantID, cleanup := setupTenantAuthTest(t)
	defer cleanup()

	validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

	// 1. Foreign tenant with case variant Tenant_ID -> MUST BE REJECTED
	reqVariantForeign := connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
		MutationType:   MutationDismissHomeItem,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","Tenant_ID":"` + otherTenantID + `","item_key":"k","projection_version":1,"dismissed_at":"2026-10-02T12:00:00Z"}`),
		IdempotencyKey: uuid.New().String(),
	})
	reqVariantForeign.Header().Set("Authorization", "Bearer "+backendToken)
	reqVariantForeign.Header().Set("X-Alt-Backend-Token", validJWT)

	_, err := client.ApplyProjectionMutation(context.Background(), reqVariantForeign)
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "case variant foreign tenant MUST be rejected")

	// 2. Matching case variant Tenant_ID -> normalized to canonical tenant_id
	reqVariantOwn := connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
		MutationType:   MutationDismissHomeItem,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","Tenant_ID":"` + targetTenantID + `","item_key":"k","projection_version":1,"dismissed_at":"2026-10-02T12:00:00Z"}`),
		IdempotencyKey: uuid.New().String(),
	})
	reqVariantOwn.Header().Set("Authorization", "Bearer "+backendToken)
	reqVariantOwn.Header().Set("X-Alt-Backend-Token", validJWT)

	resp, err := client.ApplyProjectionMutation(context.Background(), reqVariantOwn)
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)

	var captured map[string]any
	require.NoError(t, json.Unmarshal(repo.lastPayload, &captured))
	assert.Equal(t, targetTenantID, captured["tenant_id"])
	_, hasOldVariant := captured["Tenant_ID"]
	assert.False(t, hasOldVariant, "non-canonical case variant MUST be stripped from normalized payload")
}

type fakePgx struct {
	execFunc func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (f *fakePgx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if f.execFunc != nil {
		return f.execFunc(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (f *fakePgx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

func (f *fakePgx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

func (f *fakePgx) Begin(ctx context.Context) (pgx.Tx, error) {
	return nil, nil
}

func TestApplyMutation_EndToEnd_CapturedDriverSQLIsolation(t *testing.T) {
	backendToken := "auth-backend-test-token-long-enough-1234"
	jwtSecretStr := "auth-jwt-secret-key-long-enough-5678"
	jwtSecret := []byte(jwtSecretStr)

	targetUserID := uuid.New().String()
	targetTenantID := uuid.New().String()

	pol := &config.AuthPolicy{
		Services: []config.ServicePolicy{
			{
				Name:  "alt-backend",
				Token: backendToken,
				AllowedMethods: map[string]bool{
					"/services.sovereign.v1.KnowledgeSovereignService/ApplyProjectionMutation": true,
					"/services.sovereign.v1.KnowledgeSovereignService/ApplyRecallMutation":     true,
					"/services.sovereign.v1.KnowledgeSovereignService/ApplyCurationMutation":   true,
				},
				RequireUserToken: true,
				AllowSystemScope: false,
			},
		},
	}

	verifier := &config.LocalUserJWTVerifier{
		Secret:   jwtSecretStr,
		Issuer:   "auth-hub",
		Audience: "alt-backend",
	}

	execCalls := []struct {
		sql  string
		args []any
	}{}
	mock := &fakePgx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			execCalls = append(execCalls, struct {
				sql  string
				args []any
			}{sql: sql, args: args})
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	// Create real Repository with mock driver pool
	realRepo := sovereign_db.NewRepository(mock)
	h := NewSovereignHandler(realRepo)
	interceptor := NewPolicyAuthInterceptor(pol, verifier, true)

	mux := http.NewServeMux()
	path, rpcHandler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(h, connect.WithInterceptors(interceptor))
	mux.Handle(path, rpcHandler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := sovereignv1connect.NewKnowledgeSovereignServiceClient(srv.Client(), srv.URL)
	validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

	// Call ApplyProjectionMutation (DismissHomeItem) with NO tenant in payload
	req := connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
		MutationType:   MutationDismissHomeItem,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","item_key":"article:e2e-item","projection_version":1,"dismissed_at":"2026-10-02T12:00:00Z"}`),
		IdempotencyKey: uuid.New().String(),
	})
	req.Header().Set("Authorization", "Bearer "+backendToken)
	req.Header().Set("X-Alt-Backend-Token", validJWT)

	resp, err := client.ApplyProjectionMutation(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)

	require.Len(t, execCalls, 1)
	assert.Contains(t, execCalls[0].sql, "AND tenant_id =", "SQL emitted to driver pool MUST enforce tenant_id")
	targetTenantUUID := uuid.MustParse(targetTenantID)
	assert.Contains(t, execCalls[0].args, targetTenantUUID, "SQL args MUST contain the verified claims tenant_id UUID")
}

func TestApplyProjectionMutation_UpsertTodayDigest_DeniedBeforeDriver(t *testing.T) {
	repo, client, backendToken, jwtSecret, targetUserID, targetTenantID, _, cleanup := setupTenantAuthTest(t)
	defer cleanup()

	validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

	req := connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
		MutationType:   MutationUpsertTodayDigest,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","digest_date":"2026-10-02","last_event_seq":99999999}`),
		IdempotencyKey: uuid.New().String(),
	})
	req.Header().Set("Authorization", "Bearer "+backendToken)
	req.Header().Set("X-Alt-Backend-Token", validJWT)

	_, err := client.ApplyProjectionMutation(context.Background(), req)
	require.Error(t, err, "remote upsert_today_digest MUST be denied before driver")
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	assert.Empty(t, repo.lastMethod, "driver must NOT be called on denied remote mutation subtype")
}

func TestApplyRecallMutation_UpsertCandidate_HomeAssociationAuth(t *testing.T) {
	repo, client, backendToken, jwtSecret, targetUserID, targetTenantID, otherTenantID, cleanup := setupTenantAuthTest(t)
	defer cleanup()

	validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

	// 1. Foreign tenant in payload -> rejected by interceptor/handler
	reqForeign := connect.NewRequest(&sovereignv1.ApplyRecallMutationRequest{
		MutationType:   MutationUpsertCandidate,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","tenant_id":"` + otherTenantID + `","item_key":"k","recall_score":0.5}`),
		IdempotencyKey: uuid.New().String(),
	})
	reqForeign.Header().Set("Authorization", "Bearer "+backendToken)
	reqForeign.Header().Set("X-Alt-Backend-Token", validJWT)
	_, err := client.ApplyRecallMutation(context.Background(), reqForeign)
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	// 2. Missing tenant -> normalized with claims, repo returns ErrRecallHomeAssociationDenied -> permission denied
	repo.returnErr = sovereign_db.ErrRecallHomeAssociationDenied
	reqMissingHome := connect.NewRequest(&sovereignv1.ApplyRecallMutationRequest{
		MutationType:   MutationUpsertCandidate,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","item_key":"k","recall_score":0.5}`),
		IdempotencyKey: uuid.New().String(),
	})
	reqMissingHome.Header().Set("Authorization", "Bearer "+backendToken)
	reqMissingHome.Header().Set("X-Alt-Backend-Token", validJWT)
	_, err = client.ApplyRecallMutation(context.Background(), reqMissingHome)
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "missing/foreign home association MUST fail closed with PermissionDenied")

	// 3. Own positive home association -> succeeds
	repo.returnErr = nil

	reqMissingHomePositive := connect.NewRequest(&sovereignv1.ApplyRecallMutationRequest{
		MutationType:   MutationUpsertCandidate,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","item_key":"k","recall_score":0.5}`),
		IdempotencyKey: uuid.New().String(),
	})
	reqMissingHomePositive.Header().Set("Authorization", "Bearer "+backendToken)
	reqMissingHomePositive.Header().Set("X-Alt-Backend-Token", validJWT)

	resp, err := client.ApplyRecallMutation(context.Background(), reqMissingHomePositive)
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)

	var captured map[string]any
	require.NoError(t, json.Unmarshal(repo.lastPayload, &captured))
	assert.Equal(t, targetTenantID, captured["tenant_id"])
}

func TestApplyProjectionMutation_UpsertHomeItem_ExistingForeignRow_Rejected(t *testing.T) {
	repo, client, backendToken, jwtSecret, targetUserID, targetTenantID, _, cleanup := setupTenantAuthTest(t)
	defer cleanup()

	validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

	// Repo simulates tenant mismatch on existing row (tag.RowsAffected() == 0)
	repo.returnErr = sovereign_db.ErrHomeItemTenantMismatch

	req := connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
		MutationType:   MutationUpsertHomeItem,
		EntityId:       uuid.New().String(),
		Payload:        []byte(`{"user_id":"` + targetUserID + `","tenant_id":"` + targetTenantID + `","item_key":"k","score_op":"set"}`),
		IdempotencyKey: uuid.New().String(),
	})
	req.Header().Set("Authorization", "Bearer "+backendToken)
	req.Header().Set("X-Alt-Backend-Token", validJWT)

	_, err := client.ApplyProjectionMutation(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "overwriting existing foreign home item MUST be rejected with PermissionDenied")
}

func TestApplyProjectionMutation_NumberPreserveExactValue_Above2Pow53(t *testing.T) {
	repo, client, backendToken, jwtSecret, targetUserID, targetTenantID, _, cleanup := setupTenantAuthTest(t)
	defer cleanup()

	validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

	// 9007199254740993 is 2^53 + 1. If decoded into float64, it loses precision and rounds to 9007199254740992.
	// dec.UseNumber() preserves exact decimal string value on actual mutation forwarding.
	largeNumberSeq := "9007199254740993"
	payloadJSON := `{"user_id":"` + targetUserID + `","tenant_id":"` + targetTenantID + `","item_key":"k","score_op":"set","large_seq":` + largeNumberSeq + `}`

	req := connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
		MutationType:   MutationUpsertHomeItem,
		EntityId:       uuid.New().String(),
		Payload:        []byte(payloadJSON),
		IdempotencyKey: uuid.New().String(),
	})
	req.Header().Set("Authorization", "Bearer "+backendToken)
	req.Header().Set("X-Alt-Backend-Token", validJWT)

	resp, err := client.ApplyProjectionMutation(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)
	assert.Equal(t, "UpsertKnowledgeHomeItem", repo.lastMethod)

	forwardedStr := string(repo.lastPayload)
	assert.Contains(t, forwardedStr, `"large_seq":9007199254740993`, "forwarded payload MUST preserve exact digits for numbers > 2^53")
	assert.NotContains(t, forwardedStr, "9007199254740992", "float64 precision loss must not occur")
}

func TestApplyRecallMutation_NumberPreserveExactValue_Above2Pow53(t *testing.T) {
	repo, client, backendToken, jwtSecret, targetUserID, targetTenantID, _, cleanup := setupTenantAuthTest(t)
	defer cleanup()

	validJWT := makeJWT(t, jwtSecret, targetUserID, targetTenantID, "auth-hub", "alt-backend", time.Hour)

	largeNumberSeq := "9007199254740993"
	payloadJSON := `{"user_id":"` + targetUserID + `","tenant_id":"` + targetTenantID + `","item_key":"k","large_seq":` + largeNumberSeq + `}`

	req := connect.NewRequest(&sovereignv1.ApplyRecallMutationRequest{
		MutationType:   MutationUpsertCandidate,
		EntityId:       uuid.New().String(),
		Payload:        []byte(payloadJSON),
		IdempotencyKey: uuid.New().String(),
	})
	req.Header().Set("Authorization", "Bearer "+backendToken)
	req.Header().Set("X-Alt-Backend-Token", validJWT)

	resp, err := client.ApplyRecallMutation(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)
	assert.Equal(t, "UpsertRecallCandidate", repo.lastMethod)

	forwardedStr := string(repo.lastPayload)
	assert.Contains(t, forwardedStr, `"large_seq":9007199254740993`, "forwarded recall mutation payload MUST preserve exact digits for numbers > 2^53")
	assert.NotContains(t, forwardedStr, "9007199254740992", "float64 precision loss must not occur")
}

func (c *capturedRepo) UpsertRecallCandidate(_ context.Context, p json.RawMessage) error {
	c.lastMethod = "UpsertRecallCandidate"
	c.lastPayload = p
	return c.returnErr
}
func (c *capturedRepo) UpsertKnowledgeHomeItem(_ context.Context, p json.RawMessage) error {
	c.lastMethod = "UpsertKnowledgeHomeItem"
	c.lastPayload = p
	return c.returnErr
}
