package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/config"
	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
	sovereignv1connect "knowledge-sovereign/gen/proto/services/sovereign/v1/sovereignv1connect"
)

// mockAuthRepo embeds mockRepo from sovereign_handler_test.go.
type mockAuthRepo struct {
	mockRepo
	events            []sovereign_db.KnowledgeEvent
	lenses            map[uuid.UUID]*sovereign_db.KnowledgeLens
	lensVersions      map[uuid.UUID]*sovereign_db.KnowledgeLensVersion
	currentSelections map[uuid.UUID]*sovereign_db.KnowledgeCurrentLens
}

func (m *mockAuthRepo) ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error) {
	return m.events, nil
}

func (m *mockAuthRepo) ListKnowledgeEventsSinceForUser(ctx context.Context, tenantID, userID uuid.UUID, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error) {
	return m.events, nil
}

func (m *mockAuthRepo) AppendKnowledgeEvent(ctx context.Context, event sovereign_db.KnowledgeEvent) (int64, error) {
	return 100, nil
}

func (m *mockAuthRepo) ListLenses(ctx context.Context, userID uuid.UUID) ([]sovereign_db.KnowledgeLens, error) {
	if m.lenses != nil {
		var res []sovereign_db.KnowledgeLens
		for _, l := range m.lenses {
			if l.UserID == userID {
				res = append(res, *l)
			}
		}
		return res, nil
	}
	return nil, nil
}

func (m *mockAuthRepo) GetLens(ctx context.Context, lensID uuid.UUID) (*sovereign_db.KnowledgeLens, error) {
	if m.lenses != nil {
		if l, ok := m.lenses[lensID]; ok {
			return l, nil
		}
	}
	return nil, nil
}

func (m *mockAuthRepo) GetLensVersion(ctx context.Context, versionID uuid.UUID) (*sovereign_db.KnowledgeLensVersion, error) {
	if m.lensVersions != nil {
		if v, ok := m.lensVersions[versionID]; ok {
			return v, nil
		}
	}
	return nil, nil
}

func (m *mockAuthRepo) GetCurrentLensSelection(ctx context.Context, userID uuid.UUID) (*sovereign_db.KnowledgeCurrentLens, error) {
	if m.currentSelections != nil {
		if s, ok := m.currentSelections[userID]; ok {
			return s, nil
		}
	}
	return nil, nil
}

func (m *mockAuthRepo) SelectCurrentLens(ctx context.Context, c sovereign_db.KnowledgeCurrentLens) error {
	if m.currentSelections == nil {
		m.currentSelections = make(map[uuid.UUID]*sovereign_db.KnowledgeCurrentLens)
	}
	m.currentSelections[c.UserID] = &c
	return nil
}

func (m *mockAuthRepo) ResolveLensFilter(ctx context.Context, userID uuid.UUID, lensID *uuid.UUID) (*sovereign_db.LensFilter, error) {
	var targetLensID uuid.UUID
	if lensID != nil {
		targetLensID = *lensID
	} else if m.currentSelections != nil {
		if sel, ok := m.currentSelections[userID]; ok {
			targetLensID = sel.LensID
		}
	}
	if targetLensID == uuid.Nil {
		return nil, nil
	}
	if m.lenses != nil {
		l := m.lenses[targetLensID]
		if l == nil || l.UserID != userID {
			return nil, nil
		}
		return &sovereign_db.LensFilter{QueryText: "resolved"}, nil
	}
	return nil, nil
}

func createTestBackendToken(t *testing.T, secret []byte, userID, tenantID, issuer, audience string, expiry time.Duration) string {
	t.Helper()
	now := time.Now()
	claims := jwt.MapClaims{
		"email":     "test@example.com",
		"role":      "user",
		"sid":       "sess-123",
		"tenant_id": tenantID,
		"sub":       userID,
		"iss":       issuer,
		"aud":       jwt.ClaimStrings{audience},
		"iat":       now.Unix(),
		"exp":       now.Add(expiry).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(secret)
	require.NoError(t, err)
	return tokenStr
}

func setupPolicyTestServer(t *testing.T, pol *config.AuthPolicy, verifier *config.LocalUserJWTVerifier) (sovereignv1connect.KnowledgeSovereignServiceClient, func()) {
	repo := &mockAuthRepo{}
	h := NewSovereignHandler(repo)

	mux := http.NewServeMux()
	interceptor := NewPolicyAuthInterceptor(pol, verifier, true)
	path, rpcHandler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(h, connect.WithInterceptors(interceptor))
	mux.Handle(path, rpcHandler)

	srv := httptest.NewServer(mux)
	client := sovereignv1connect.NewKnowledgeSovereignServiceClient(srv.Client(), srv.URL)
	return client, srv.Close
}

func TestPolicyAuth_Enforcement(t *testing.T) {
	const (
		backendToken     = "backend-secret-token-long-enough-1234"
		recapToken       = "recap-secret-token-long-enough-5678"
		ragToken         = "rag-secret-token-long-enough-9012"
		replicationToken = "replication-secret-token-long-3456"
		jwtSecretKey     = "super-secret-jwt-verification-key-32"
		jwtIssuer        = "auth-hub"
		jwtAudience      = "alt-backend"
	)

	pol := &config.AuthPolicy{
		Services: []config.ServicePolicy{
			{
				Name:  "alt-backend",
				Token: backendToken,
				AllowedMethods: map[string]bool{
					"/services.sovereign.v1.KnowledgeSovereignService/GetTrailFootprints":   true,
					"/services.sovereign.v1.KnowledgeSovereignService/ListKnowledgeEvents":  true,
					"/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent": true,
				},
				AllowedEvents: map[string]bool{
					"ArticleCreated":           true,
					"trail.branch_proposed.v1": true,
				},
				RequireUserToken: true,
				AllowSystemScope: false,
			},
			{
				Name:  "recap-worker",
				Token: recapToken,
				AllowedMethods: map[string]bool{
					"/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent": true,
				},
				AllowedEvents: map[string]bool{
					"recap.topic_snapshotted.v1": true,
				},
				RequireUserToken: false,
				AllowSystemScope: false,
			},
			{
				Name:  "rag-orchestrator",
				Token: ragToken,
				AllowedMethods: map[string]bool{
					"/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent": true,
				},
				AllowedEvents: map[string]bool{
					"augur.conversation_linked.v1": true,
				},
				RequireUserToken: false,
				AllowSystemScope: false,
			},
			{
				Name:  "replication-service",
				Token: replicationToken,
				AllowedMethods: map[string]bool{
					"/services.sovereign.v1.KnowledgeSovereignService/ListKnowledgeEvents": true,
				},
				AllowedEvents:    map[string]bool{},
				RequireUserToken: false,
				AllowSystemScope: true, // Intentionally privileged system replication scope
			},
		},
	}

	verifier := &config.LocalUserJWTVerifier{
		Secret:   jwtSecretKey,
		Issuer:   jwtIssuer,
		Audience: jwtAudience,
	}

	client, cleanup := setupPolicyTestServer(t, pol, verifier)
	defer cleanup()

	targetUserID := uuid.New().String()
	targetTenantID := uuid.New().String()
	otherUserID := uuid.New().String()
	otherTenantID := uuid.New().String()

	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		req := connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
			UserId: targetUserID,
		})
		_, err := client.GetTrailFootprints(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("wrong service token returns 401", func(t *testing.T) {
		req := connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
			UserId: targetUserID,
		})
		req.Header().Set("Authorization", "Bearer invalid-unknown-token-value-xyz")
		_, err := client.GetTrailFootprints(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("wrong method for service returns 403 permission denied", func(t *testing.T) {
		// recap-worker tries to call GetTrailFootprints
		req := connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
			UserId: targetUserID,
		})
		req.Header().Set("Authorization", "Bearer "+recapToken)
		_, err := client.GetTrailFootprints(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "not authorized to call procedure")
	})

	t.Run("wrong event type for service returns 403 permission denied", func(t *testing.T) {
		// recap-worker tries to emit ArticleCreated
		req := connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
			Event: &sovereignv1.KnowledgeEvent{
				EventId:    uuid.New().String(),
				TenantId:   targetTenantID,
				UserId:     targetUserID,
				EventType:  "ArticleCreated",
				OccurredAt: timestamppb.New(time.Now()),
			},
		})
		req.Header().Set("Authorization", "Bearer "+recapToken)
		_, err := client.AppendKnowledgeEvent(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "not authorized to append event type")
	})

	t.Run("global scan denied for service lacking system scope", func(t *testing.T) {
		// alt-backend tries to scan all events with UserId == ""
		req := connect.NewRequest(&sovereignv1.ListKnowledgeEventsRequest{
			UserId:   "",
			TenantId: "",
		})
		req.Header().Set("Authorization", "Bearer "+backendToken)
		_, err := client.ListKnowledgeEvents(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "global event log scan requires system replication scope")
	})

	t.Run("global scan denied for recap-worker", func(t *testing.T) {
		req := connect.NewRequest(&sovereignv1.ListKnowledgeEventsRequest{
			UserId:   "",
			TenantId: "",
		})
		req.Header().Set("Authorization", "Bearer "+recapToken)
		_, err := client.ListKnowledgeEvents(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	})

	t.Run("global scan allowed for replication-service with explicit system scope", func(t *testing.T) {
		req := connect.NewRequest(&sovereignv1.ListKnowledgeEventsRequest{
			UserId:   "",
			TenantId: "",
		})
		req.Header().Set("Authorization", "Bearer "+replicationToken)
		resp, err := client.ListKnowledgeEvents(context.Background(), req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("user-scoped operation without X-Alt-Backend-Token returns 401", func(t *testing.T) {
		req := connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
			UserId: targetUserID,
		})
		req.Header().Set("Authorization", "Bearer "+backendToken)
		// Missing X-Alt-Backend-Token
		_, err := client.GetTrailFootprints(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "X-Alt-Backend-Token")
	})

	t.Run("raw X-Alt-User-Id is never trusted without valid JWT", func(t *testing.T) {
		req := connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
			UserId: targetUserID,
		})
		req.Header().Set("Authorization", "Bearer "+backendToken)
		req.Header().Set("X-Alt-User-Id", targetUserID)
		_, err := client.GetTrailFootprints(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("user-scoped operation with mismatched user ID in JWT returns 403", func(t *testing.T) {
		userJWT := createTestBackendToken(t, []byte(jwtSecretKey), otherUserID, targetTenantID, jwtIssuer, jwtAudience, time.Hour)
		req := connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
			UserId: targetUserID, // requesting victim user
		})
		req.Header().Set("Authorization", "Bearer "+backendToken)
		req.Header().Set("X-Alt-Backend-Token", userJWT) // token has otherUserID

		_, err := client.GetTrailFootprints(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "delegation token subject/tenant does not match")
	})

	t.Run("user-scoped operation with mismatched tenant ID in JWT returns 403", func(t *testing.T) {
		userJWT := createTestBackendToken(t, []byte(jwtSecretKey), targetUserID, otherTenantID, jwtIssuer, jwtAudience, time.Hour)
		req := connect.NewRequest(&sovereignv1.ListKnowledgeEventsRequest{
			UserId:   targetUserID,
			TenantId: targetTenantID,
		})
		req.Header().Set("Authorization", "Bearer "+backendToken)
		req.Header().Set("X-Alt-Backend-Token", userJWT) // JWT has otherTenantID

		_, err := client.ListKnowledgeEvents(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "delegation token subject/tenant does not match")
	})

	t.Run("valid user-scoped call with matching JWT succeeds", func(t *testing.T) {
		userJWT := createTestBackendToken(t, []byte(jwtSecretKey), targetUserID, targetTenantID, jwtIssuer, jwtAudience, time.Hour)
		req := connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
			UserId: targetUserID,
		})
		req.Header().Set("Authorization", "Bearer "+backendToken)
		req.Header().Set("X-Alt-Backend-Token", userJWT)

		resp, err := client.GetTrailFootprints(context.Background(), req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("valid recap-worker append event succeeds without user JWT", func(t *testing.T) {
		req := connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
			Event: &sovereignv1.KnowledgeEvent{
				EventId:    uuid.New().String(),
				TenantId:   targetTenantID,
				UserId:     targetUserID,
				EventType:  "recap.topic_snapshotted.v1",
				OccurredAt: timestamppb.New(time.Now()),
			},
		})
		req.Header().Set("Authorization", "Bearer "+recapToken)

		resp, err := client.AppendKnowledgeEvent(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, int64(100), resp.Msg.EventSeq)
	})

	t.Run("valid RAG append event succeeds without user JWT", func(t *testing.T) {
		req := connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
			Event: &sovereignv1.KnowledgeEvent{
				EventId:    uuid.New().String(),
				TenantId:   targetTenantID,
				UserId:     targetUserID,
				EventType:  "augur.conversation_linked.v1",
				OccurredAt: timestamppb.New(time.Now()),
			},
		})
		req.Header().Set("Authorization", "Bearer "+ragToken)

		resp, err := client.AppendKnowledgeEvent(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, int64(100), resp.Msg.EventSeq)
	})
}
