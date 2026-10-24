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

func TestPolicyAuth_InfraReprojectBackfillAndProjectionState(t *testing.T) {
	const (
		backendToken  = "backend-secret-token-long-enough-1234"
		operatorToken = "operator-secret-token-long-enough-5678"
		jwtSecretKey  = "super-secret-jwt-verification-key-32"
		jwtIssuer     = "auth-hub"
		jwtAudience   = "alt-backend"
	)

	allInfraMethods := map[string]bool{
		"/services.sovereign.v1.KnowledgeSovereignService/GetActiveProjectionVersion": true,
		"/services.sovereign.v1.KnowledgeSovereignService/GetProjectionFreshness":     true,
		"/services.sovereign.v1.KnowledgeSovereignService/ListProjectionVersions":     true,
		"/services.sovereign.v1.KnowledgeSovereignService/CreateProjectionVersion":    true,
		"/services.sovereign.v1.KnowledgeSovereignService/ActivateProjectionVersion":  true,
		"/services.sovereign.v1.KnowledgeSovereignService/GetProjectionCheckpoint":    true,
		"/services.sovereign.v1.KnowledgeSovereignService/UpdateProjectionCheckpoint": true,
		"/services.sovereign.v1.KnowledgeSovereignService/GetProjectionLag":           true,
		"/services.sovereign.v1.KnowledgeSovereignService/GetReprojectRun":            true,
		"/services.sovereign.v1.KnowledgeSovereignService/ListReprojectRuns":          true,
		"/services.sovereign.v1.KnowledgeSovereignService/CreateReprojectRun":         true,
		"/services.sovereign.v1.KnowledgeSovereignService/UpdateReprojectRun":         true,
		"/services.sovereign.v1.KnowledgeSovereignService/CompareProjections":         true,
		"/services.sovereign.v1.KnowledgeSovereignService/ListProjectionAudits":       true,
		"/services.sovereign.v1.KnowledgeSovereignService/CreateProjectionAudit":      true,
		"/services.sovereign.v1.KnowledgeSovereignService/GetBackfillJob":             true,
		"/services.sovereign.v1.KnowledgeSovereignService/ListBackfillJobs":           true,
		"/services.sovereign.v1.KnowledgeSovereignService/CreateBackfillJob":          true,
		"/services.sovereign.v1.KnowledgeSovereignService/UpdateBackfillJob":          true,
	}

	pol := &config.AuthPolicy{
		Services: []config.ServicePolicy{
			{
				Name:             "alt-backend",
				Token:            backendToken,
				AllowedMethods:   allInfraMethods,
				RequireUserToken: true,
				AllowSystemScope: false,
			},
			{
				Name:             "operator",
				Token:            operatorToken,
				AllowedMethods:   allInfraMethods,
				RequireUserToken: false,
				AllowSystemScope: false,
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

	userID := uuid.New().String()
	tenantID := uuid.New().String()
	userJWT := createTestBackendToken(t, []byte(jwtSecretKey), userID, tenantID, jwtIssuer, jwtAudience, time.Hour)

	runID := uuid.New().String()
	auditID := uuid.New().String()
	jobID := uuid.New().String()

	callAll := func(t *testing.T, svcToken, delegationJWT string, expectSuccess bool, expectedErrCode connect.Code) {
		t.Helper()
		setHeaders := func(h http.Header) {
			h.Set("Authorization", "Bearer "+svcToken)
			if delegationJWT != "" {
				h.Set("X-Alt-Backend-Token", delegationJWT)
			}
		}
		check := func(name string, err error) {
			t.Helper()
			if expectSuccess {
				require.NoError(t, err, "expected %s to succeed", name)
			} else {
				require.Error(t, err, "expected %s to fail", name)
				assert.Equal(t, expectedErrCode, connect.CodeOf(err), "unexpected code for %s", name)
			}
		}

		req1 := connect.NewRequest(&sovereignv1.GetActiveProjectionVersionRequest{})
		setHeaders(req1.Header())
		_, err := client.GetActiveProjectionVersion(context.Background(), req1)
		check("GetActiveProjectionVersion", err)

		req2 := connect.NewRequest(&sovereignv1.GetProjectionFreshnessRequest{ProjectorName: "home"})
		setHeaders(req2.Header())
		_, err = client.GetProjectionFreshness(context.Background(), req2)
		check("GetProjectionFreshness", err)

		req3 := connect.NewRequest(&sovereignv1.ListProjectionVersionsRequest{})
		setHeaders(req3.Header())
		_, err = client.ListProjectionVersions(context.Background(), req3)
		check("ListProjectionVersions", err)

		req4 := connect.NewRequest(&sovereignv1.CreateProjectionVersionRequest{
			Version: &sovereignv1.ProjectionVersion{Version: 2, Description: "v2", Status: "pending"},
		})
		setHeaders(req4.Header())
		_, err = client.CreateProjectionVersion(context.Background(), req4)
		check("CreateProjectionVersion", err)

		req5 := connect.NewRequest(&sovereignv1.ActivateProjectionVersionRequest{Version: 2})
		setHeaders(req5.Header())
		_, err = client.ActivateProjectionVersion(context.Background(), req5)
		check("ActivateProjectionVersion", err)

		req6 := connect.NewRequest(&sovereignv1.GetProjectionCheckpointRequest{ProjectorName: "home"})
		setHeaders(req6.Header())
		_, err = client.GetProjectionCheckpoint(context.Background(), req6)
		check("GetProjectionCheckpoint", err)

		req7 := connect.NewRequest(&sovereignv1.UpdateProjectionCheckpointRequest{ProjectorName: "home", LastEventSeq: 42})
		setHeaders(req7.Header())
		_, err = client.UpdateProjectionCheckpoint(context.Background(), req7)
		check("UpdateProjectionCheckpoint", err)

		req8 := connect.NewRequest(&sovereignv1.GetProjectionLagRequest{})
		setHeaders(req8.Header())
		_, err = client.GetProjectionLag(context.Background(), req8)
		check("GetProjectionLag", err)

		req9 := connect.NewRequest(&sovereignv1.GetReprojectRunRequest{RunId: runID})
		setHeaders(req9.Header())
		_, err = client.GetReprojectRun(context.Background(), req9)
		check("GetReprojectRun", err)

		req10 := connect.NewRequest(&sovereignv1.ListReprojectRunsRequest{Limit: 10})
		setHeaders(req10.Header())
		_, err = client.ListReprojectRuns(context.Background(), req10)
		check("ListReprojectRuns", err)

		req11 := connect.NewRequest(&sovereignv1.CreateReprojectRunRequest{
			Run: &sovereignv1.ReprojectRun{ReprojectRunId: runID, ProjectionName: "home"},
		})
		setHeaders(req11.Header())
		_, err = client.CreateReprojectRun(context.Background(), req11)
		check("CreateReprojectRun", err)

		req12 := connect.NewRequest(&sovereignv1.UpdateReprojectRunRequest{
			Run: &sovereignv1.ReprojectRun{ReprojectRunId: runID, ProjectionName: "home"},
		})
		setHeaders(req12.Header())
		_, err = client.UpdateReprojectRun(context.Background(), req12)
		check("UpdateReprojectRun", err)

		req13 := connect.NewRequest(&sovereignv1.CompareProjectionsRequest{FromVersion: "1", ToVersion: "2"})
		setHeaders(req13.Header())
		_, err = client.CompareProjections(context.Background(), req13)
		check("CompareProjections", err)

		req14 := connect.NewRequest(&sovereignv1.ListProjectionAuditsRequest{ProjectionName: "home", Limit: 10})
		setHeaders(req14.Header())
		_, err = client.ListProjectionAudits(context.Background(), req14)
		check("ListProjectionAudits", err)

		req15 := connect.NewRequest(&sovereignv1.CreateProjectionAuditRequest{
			Audit: &sovereignv1.ProjectionAudit{AuditId: auditID, ProjectionName: "home"},
		})
		setHeaders(req15.Header())
		_, err = client.CreateProjectionAudit(context.Background(), req15)
		check("CreateProjectionAudit", err)

		req16 := connect.NewRequest(&sovereignv1.GetBackfillJobRequest{JobId: jobID})
		setHeaders(req16.Header())
		_, err = client.GetBackfillJob(context.Background(), req16)
		check("GetBackfillJob", err)

		req17 := connect.NewRequest(&sovereignv1.ListBackfillJobsRequest{})
		setHeaders(req17.Header())
		_, err = client.ListBackfillJobs(context.Background(), req17)
		check("ListBackfillJobs", err)

		req18 := connect.NewRequest(&sovereignv1.CreateBackfillJobRequest{
			Job: &sovereignv1.BackfillJob{JobId: jobID, Status: "pending"},
		})
		setHeaders(req18.Header())
		_, err = client.CreateBackfillJob(context.Background(), req18)
		check("CreateBackfillJob", err)

		req19 := connect.NewRequest(&sovereignv1.UpdateBackfillJobRequest{
			Job: &sovereignv1.BackfillJob{JobId: jobID, Status: "running"},
		})
		setHeaders(req19.Header())
		_, err = client.UpdateBackfillJob(context.Background(), req19)
		check("UpdateBackfillJob", err)
	}

	t.Run("alt-backend with valid user token can call all allowed infra/reproject/backfill methods", func(t *testing.T) {
		callAll(t, backendToken, userJWT, true, 0)
	})

	t.Run("alt-backend without user token is rejected with Unauthenticated", func(t *testing.T) {
		callAll(t, backendToken, "", false, connect.CodeUnauthenticated)
	})

	t.Run("operator without user token can call all allowed infra/reproject/backfill methods", func(t *testing.T) {
		callAll(t, operatorToken, "", true, 0)
	})

	t.Run("operator cannot call methods outside AllowedMethods", func(t *testing.T) {
		req := connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{UserId: userID})
		req.Header().Set("Authorization", "Bearer "+operatorToken)
		_, err := client.GetTrailFootprints(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	})

	t.Run("RequireUserToken with empty Subject or TenantID in claims returns CodePermissionDenied", func(t *testing.T) {
		p := &policyAuthInterceptor{policy: pol, enabled: true}
		principal := &Principal{
			Name:             "alt-backend",
			AllowedMethods:   allInfraMethods,
			RequireUserToken: true,
		}
		ctxEmptySub := ContextWithBackendClaims(context.Background(), &config.BackendClaims{
			Subject:  "",
			TenantID: tenantID,
		})
		err := p.authorize(ctxEmptySub, principal, "/services.sovereign.v1.KnowledgeSovereignService/GetActiveProjectionVersion", http.Header{}, &sovereignv1.GetActiveProjectionVersionRequest{})
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

		ctxEmptyTenant := ContextWithBackendClaims(context.Background(), &config.BackendClaims{
			Subject:  userID,
			TenantID: "",
		})
		err = p.authorize(ctxEmptyTenant, principal, "/services.sovereign.v1.KnowledgeSovereignService/GetProjectionFreshness", http.Header{}, &sovereignv1.GetProjectionFreshnessRequest{})
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	})
}

func TestPolicyAuth_WrapStreamingHandler_WatchProjectorEvents(t *testing.T) {
	const (
		systemToken    = "system-watcher-secret-token-long-1234"
		nonSystemToken = "non-system-watcher-token-long-567890"
	)

	pol := &config.AuthPolicy{
		Services: []config.ServicePolicy{
			{
				Name:  "system-watcher",
				Token: systemToken,
				AllowedMethods: map[string]bool{
					"/services.sovereign.v1.KnowledgeSovereignService/WatchProjectorEvents": true,
				},
				RequireUserToken: false,
				AllowSystemScope: true,
			},
			{
				Name:  "non-system-watcher",
				Token: nonSystemToken,
				AllowedMethods: map[string]bool{
					"/services.sovereign.v1.KnowledgeSovereignService/WatchProjectorEvents": true,
				},
				RequireUserToken: false,
				AllowSystemScope: false,
			},
		},
	}

	watcher := &mockProjectorWatcher{
		notifications: []mockNotification{
			{payload: "200", isTimeout: false},
		},
	}

	repo := &mockAuthRepo{}
	h := NewSovereignHandler(
		repo,
		WithDatabaseURL("postgres://mock"),
		WithProjectorWatcherOpener(func(ctx context.Context, databaseURL string) (ProjectorWatcher, error) {
			return watcher, nil
		}),
	)

	mux := http.NewServeMux()
	interceptor := NewPolicyAuthInterceptor(pol, nil, true)
	path, rpcHandler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(h, connect.WithInterceptors(interceptor))
	mux.Handle(path, rpcHandler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := sovereignv1connect.NewKnowledgeSovereignServiceClient(srv.Client(), srv.URL)

	t.Run("missing token returns CodeUnauthenticated", func(t *testing.T) {
		stream, err := client.WatchProjectorEvents(context.Background(), connect.NewRequest(&sovereignv1.WatchProjectorEventsRequest{
			ProjectorName: "test_projector",
		}))
		if err != nil {
			assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
			return
		}
		defer func() { _ = stream.Close() }()
		assert.False(t, stream.Receive())
		require.Error(t, stream.Err())
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(stream.Err()))
	})

	t.Run("AllowSystemScope=false returns CodePermissionDenied", func(t *testing.T) {
		req := connect.NewRequest(&sovereignv1.WatchProjectorEventsRequest{
			ProjectorName: "test_projector",
		})
		req.Header().Set("Authorization", "Bearer "+nonSystemToken)
		stream, err := client.WatchProjectorEvents(context.Background(), req)
		if err != nil {
			assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
			return
		}
		defer func() { _ = stream.Close() }()
		assert.False(t, stream.Receive())
		require.Error(t, stream.Err())
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(stream.Err()))
	})

	t.Run("AllowSystemScope=true streams events", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		req := connect.NewRequest(&sovereignv1.WatchProjectorEventsRequest{
			ProjectorName: "test_projector",
		})
		req.Header().Set("Authorization", "Bearer "+systemToken)
		stream, err := client.WatchProjectorEvents(ctx, req)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }()

		require.True(t, stream.Receive())
		assert.Equal(t, int64(200), stream.Msg().LatestEventSeq)
		cancel()
	})
}
