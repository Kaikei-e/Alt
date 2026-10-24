package handler

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/config"
	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func authClaimsForTest(userID, tenantID uuid.UUID) *config.BackendClaims {
	return &config.BackendClaims{
		Subject:  userID.String(),
		TenantID: tenantID.String(),
	}
}

func timestamppbNowForTest() *timestamppb.Timestamp {
	return timestamppb.Now()
}

func TestValidateAndBuildLensFilter(t *testing.T) {
	tests := []struct {
		name string
		in   *sovereignv1.LensFilter
		want *sovereign_db.LensFilter
	}{
		{
			name: "nil input returns nil",
			in:   nil,
			want: nil,
		},
		{
			name: "populated filter maps correctly",
			in: &sovereignv1.LensFilter{
				TagIds:       []string{"go", "solid"},
				SourceIds:    []string{"source-1", "source-2"},
				TimeWindow:   "30d",
				QueryText:    "clean architecture",
				IncludeRecap: true,
				IncludePulse: false,
				SortMode:     "date_desc",
			},
			want: &sovereign_db.LensFilter{
				QueryText:    "clean architecture",
				TagNames:     []string{"go", "solid"},
				SourceIDs:    []string{"source-1", "source-2"},
				TimeWindow:   "30d",
				IncludeRecap: true,
				IncludePulse: false,
				SortMode:     "date_desc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateAndBuildLensFilter(tt.in)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestLensMappers(t *testing.T) {
	lensID := uuid.New()
	userID := uuid.New()
	tenantID := uuid.New()
	versionID := uuid.New()
	created := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	archived := time.Date(2026, 9, 25, 11, 0, 0, 0, time.UTC)

	version := sovereign_db.KnowledgeLensVersion{
		LensVersionID: versionID,
		LensID:        lensID,
		CreatedAt:     created,
		QueryText:     "search query",
		TagIDs:        []string{"tag1"},
		SourceIDs:     []string{"source1"},
		TimeWindow:    "7d",
		IncludeRecap:  true,
		IncludePulse:  true,
		SortMode:      "relevance",
	}

	lens := sovereign_db.KnowledgeLens{
		LensID:         lensID,
		UserID:         userID,
		TenantID:       tenantID,
		Name:           "Tech Lens",
		Description:    "Tech articles",
		CreatedAt:      created,
		UpdatedAt:      created,
		ArchivedAt:     &archived,
		CurrentVersion: &version,
	}

	pbLens := lensToProto(lens)
	require.NotNil(t, pbLens)
	assert.Equal(t, lensID.String(), pbLens.LensId)
	assert.Equal(t, "Tech Lens", pbLens.Name)
	assert.True(t, pbLens.ArchivedAt.AsTime().Equal(archived))
	require.NotNil(t, pbLens.CurrentVersion)
	assert.Equal(t, versionID.String(), pbLens.CurrentVersion.LensVersionId)
	assert.Equal(t, "search query", pbLens.CurrentVersion.QueryText)
}

func TestGetLens_PopulatesCurrentVersion(t *testing.T) {
	lensID := uuid.New()
	versionID := uuid.New()

	tests := []struct {
		name        string
		repo        *mockRepo
		wantVersion bool
		wantNilLens bool
	}{
		{
			name: "lens with current version",
			repo: &mockRepo{
				returnLens: &sovereign_db.KnowledgeLens{
					LensID: lensID,
					UserID: uuid.New(),
					Name:   "Test Lens",
				},
				returnVersion: &sovereign_db.KnowledgeLensVersion{
					LensVersionID: versionID,
					LensID:        lensID,
					TagIDs:        []string{"llm", "ai"},
					TimeWindow:    "90d",
					SortMode:      "relevance",
				},
			},
			wantVersion: true,
		},
		{
			name: "lens without version",
			repo: &mockRepo{
				returnLens: &sovereign_db.KnowledgeLens{
					LensID: lensID,
					UserID: uuid.New(),
					Name:   "Orphan Lens",
				},
				returnVersion: nil,
			},
			wantVersion: false,
		},
		{
			name:        "lens not found",
			repo:        &mockRepo{},
			wantNilLens: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, cleanup := setupTestServer(tt.repo)
			defer cleanup()

			resp, err := client.GetLens(context.Background(),
				connect.NewRequest(&sovereignv1.GetLensRequest{
					LensId: lensID.String(),
				}))

			require.NoError(t, err)

			if tt.wantNilLens {
				assert.Nil(t, resp.Msg.Lens)
				return
			}

			require.NotNil(t, resp.Msg.Lens)

			if tt.wantVersion {
				require.NotNil(t, resp.Msg.Lens.CurrentVersion, "CurrentVersion should be populated")
				assert.Equal(t, versionID.String(), resp.Msg.Lens.CurrentVersion.LensVersionId)
				assert.Equal(t, []string{"llm", "ai"}, resp.Msg.Lens.CurrentVersion.TagIds)
				assert.Equal(t, "90d", resp.Msg.Lens.CurrentVersion.TimeWindow)
			} else {
				assert.Nil(t, resp.Msg.Lens.CurrentVersion)
			}
		})
	}
}

func TestAreArticlesVisibleInLens_ValidatesRequiredFields(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)

	// Missing tenant_id & user_id
	_, err := h.AreArticlesVisibleInLens(context.Background(),
		connect.NewRequest(&sovereignv1.AreArticlesVisibleInLensRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	// Invalid article_id
	_, err = h.AreArticlesVisibleInLens(context.Background(),
		connect.NewRequest(&sovereignv1.AreArticlesVisibleInLensRequest{
			TenantId:   uuid.New().String(),
			UserId:     uuid.New().String(),
			ArticleIds: []string{"invalid-uuid"},
		}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	// Invalid tenant_id
	_, err = h.AreArticlesVisibleInLens(context.Background(),
		connect.NewRequest(&sovereignv1.AreArticlesVisibleInLensRequest{
			TenantId: "bad-tenant",
			UserId:   uuid.New().String(),
		}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	// Invalid user_id
	_, err = h.AreArticlesVisibleInLens(context.Background(),
		connect.NewRequest(&sovereignv1.AreArticlesVisibleInLensRequest{
			TenantId: uuid.New().String(),
			UserId:   "bad-user",
		}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

type lensTestRepo struct {
	mockRepo
	lenses       map[uuid.UUID]*sovereign_db.KnowledgeLens
	versions     map[uuid.UUID]*sovereign_db.KnowledgeLensVersion
	selections   map[uuid.UUID]*sovereign_db.KnowledgeCurrentLens
	archived     map[uuid.UUID]bool
	visibility   map[uuid.UUID]bool
	gotFilter    *sovereign_db.LensFilter
	savedVersion *sovereign_db.KnowledgeLensVersion
}

func newLensTestRepo() *lensTestRepo {
	return &lensTestRepo{
		lenses:     make(map[uuid.UUID]*sovereign_db.KnowledgeLens),
		versions:   make(map[uuid.UUID]*sovereign_db.KnowledgeLensVersion),
		selections: make(map[uuid.UUID]*sovereign_db.KnowledgeCurrentLens),
		archived:   make(map[uuid.UUID]bool),
		visibility: make(map[uuid.UUID]bool),
	}
}

func (r *lensTestRepo) GetLens(_ context.Context, lensID uuid.UUID) (*sovereign_db.KnowledgeLens, error) {
	if r.returnErr != nil {
		return nil, r.returnErr
	}
	return r.lenses[lensID], nil
}

func (r *lensTestRepo) GetLensVersion(_ context.Context, versionID uuid.UUID) (*sovereign_db.KnowledgeLensVersion, error) {
	if r.returnErr != nil {
		return nil, r.returnErr
	}
	return r.versions[versionID], nil
}

func (r *lensTestRepo) CreateLensVersion(_ context.Context, v sovereign_db.KnowledgeLensVersion) error {
	if r.returnErr != nil {
		return r.returnErr
	}
	r.versions[v.LensVersionID] = &v
	r.savedVersion = &v
	return nil
}

func (r *lensTestRepo) SelectCurrentLens(_ context.Context, c sovereign_db.KnowledgeCurrentLens) error {
	if r.returnErr != nil {
		return r.returnErr
	}
	r.selections[c.UserID] = &c
	return nil
}

func (r *lensTestRepo) ClearCurrentLens(_ context.Context, userID uuid.UUID) error {
	if r.returnErr != nil {
		return r.returnErr
	}
	delete(r.selections, userID)
	return nil
}

func (r *lensTestRepo) ArchiveLens(_ context.Context, lensID uuid.UUID) error {
	if r.returnErr != nil {
		return r.returnErr
	}
	r.archived[lensID] = true
	return nil
}

func (r *lensTestRepo) AreArticlesVisibleInLens(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID, filter *sovereign_db.LensFilter) (map[uuid.UUID]bool, error) {
	if r.returnErr != nil {
		return nil, r.returnErr
	}
	r.gotFilter = filter
	return r.visibility, nil
}

func TestArchiveLens(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	otherUserID := uuid.New()
	otherTenantID := uuid.New()
	lensID := uuid.New()

	t.Run("invalid lens_id returns CodeInvalidArgument", func(t *testing.T) {
		repo := newLensTestRepo()
		h := NewSovereignHandler(repo)
		_, err := h.ArchiveLens(context.Background(), connect.NewRequest(&sovereignv1.ArchiveLensRequest{
			LensId: "not-a-uuid",
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("lens not found returns CodeNotFound", func(t *testing.T) {
		repo := newLensTestRepo()
		h := NewSovereignHandler(repo)
		_, err := h.ArchiveLens(context.Background(), connect.NewRequest(&sovereignv1.ArchiveLensRequest{
			LensId: lensID.String(),
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
		assert.False(t, repo.archived[lensID])
	})

	t.Run("cross-user ownership mismatch returns CodePermissionDenied", func(t *testing.T) {
		repo := newLensTestRepo()
		repo.lenses[lensID] = &sovereign_db.KnowledgeLens{
			LensID:   lensID,
			UserID:   userID,
			TenantID: tenantID,
		}
		h := NewSovereignHandler(repo)
		ctx := ContextWithBackendClaims(context.Background(), authClaimsForTest(otherUserID, tenantID))
		_, err := h.ArchiveLens(ctx, connect.NewRequest(&sovereignv1.ArchiveLensRequest{
			LensId: lensID.String(),
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.False(t, repo.archived[lensID])
	})

	t.Run("cross-tenant ownership mismatch returns CodePermissionDenied", func(t *testing.T) {
		repo := newLensTestRepo()
		repo.lenses[lensID] = &sovereign_db.KnowledgeLens{
			LensID:   lensID,
			UserID:   userID,
			TenantID: tenantID,
		}
		h := NewSovereignHandler(repo)
		ctx := ContextWithBackendClaims(context.Background(), authClaimsForTest(userID, otherTenantID))
		_, err := h.ArchiveLens(ctx, connect.NewRequest(&sovereignv1.ArchiveLensRequest{
			LensId: lensID.String(),
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.False(t, repo.archived[lensID])
	})

	t.Run("happy path archives lens", func(t *testing.T) {
		repo := newLensTestRepo()
		repo.lenses[lensID] = &sovereign_db.KnowledgeLens{
			LensID:   lensID,
			UserID:   userID,
			TenantID: tenantID,
		}
		h := NewSovereignHandler(repo)
		ctx := ContextWithBackendClaims(context.Background(), authClaimsForTest(userID, tenantID))
		resp, err := h.ArchiveLens(ctx, connect.NewRequest(&sovereignv1.ArchiveLensRequest{
			LensId: lensID.String(),
		}))
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, repo.archived[lensID])
	})
}

func TestGetLens_ValidationAndOwnership(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	otherUserID := uuid.New()
	otherTenantID := uuid.New()
	lensID := uuid.New()

	t.Run("invalid lens_id returns CodeInvalidArgument", func(t *testing.T) {
		repo := newLensTestRepo()
		h := NewSovereignHandler(repo)
		_, err := h.GetLens(context.Background(), connect.NewRequest(&sovereignv1.GetLensRequest{
			LensId: "bad-uuid",
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("cross-user ownership mismatch returns CodePermissionDenied", func(t *testing.T) {
		repo := newLensTestRepo()
		repo.lenses[lensID] = &sovereign_db.KnowledgeLens{
			LensID:   lensID,
			UserID:   userID,
			TenantID: tenantID,
		}
		h := NewSovereignHandler(repo)
		ctx := ContextWithBackendClaims(context.Background(), authClaimsForTest(otherUserID, tenantID))
		_, err := h.GetLens(ctx, connect.NewRequest(&sovereignv1.GetLensRequest{
			LensId: lensID.String(),
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	})

	t.Run("cross-tenant ownership mismatch returns CodePermissionDenied", func(t *testing.T) {
		repo := newLensTestRepo()
		repo.lenses[lensID] = &sovereign_db.KnowledgeLens{
			LensID:   lensID,
			UserID:   userID,
			TenantID: tenantID,
		}
		h := NewSovereignHandler(repo)
		ctx := ContextWithBackendClaims(context.Background(), authClaimsForTest(userID, otherTenantID))
		_, err := h.GetLens(ctx, connect.NewRequest(&sovereignv1.GetLensRequest{
			LensId: lensID.String(),
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	})
}

func TestCreateLensVersion(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	otherUserID := uuid.New()
	otherTenantID := uuid.New()
	lensID := uuid.New()
	versionID := uuid.New()

	t.Run("validation errors return CodeInvalidArgument", func(t *testing.T) {
		repo := newLensTestRepo()
		h := NewSovereignHandler(repo)

		// nil version
		_, err := h.CreateLensVersion(context.Background(), connect.NewRequest(&sovereignv1.CreateLensVersionRequest{}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		// invalid lens_version_id
		_, err = h.CreateLensVersion(context.Background(), connect.NewRequest(&sovereignv1.CreateLensVersionRequest{
			Version: &sovereignv1.LensVersion{LensVersionId: "bad"},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		// invalid lens_id
		_, err = h.CreateLensVersion(context.Background(), connect.NewRequest(&sovereignv1.CreateLensVersionRequest{
			Version: &sovereignv1.LensVersion{LensVersionId: versionID.String(), LensId: "bad"},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		// nil created_at
		_, err = h.CreateLensVersion(context.Background(), connect.NewRequest(&sovereignv1.CreateLensVersionRequest{
			Version: &sovereignv1.LensVersion{LensVersionId: versionID.String(), LensId: lensID.String()},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("lens not found returns CodeNotFound", func(t *testing.T) {
		repo := newLensTestRepo()
		h := NewSovereignHandler(repo)
		_, err := h.CreateLensVersion(context.Background(), connect.NewRequest(&sovereignv1.CreateLensVersionRequest{
			Version: &sovereignv1.LensVersion{
				LensVersionId: versionID.String(),
				LensId:        lensID.String(),
				CreatedAt:     timestamppbNowForTest(),
			},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	})

	t.Run("ownership mismatch returns CodePermissionDenied", func(t *testing.T) {
		repo := newLensTestRepo()
		repo.lenses[lensID] = &sovereign_db.KnowledgeLens{
			LensID:   lensID,
			UserID:   userID,
			TenantID: tenantID,
		}
		h := NewSovereignHandler(repo)

		ctxOtherUser := ContextWithBackendClaims(context.Background(), authClaimsForTest(otherUserID, tenantID))
		_, err := h.CreateLensVersion(ctxOtherUser, connect.NewRequest(&sovereignv1.CreateLensVersionRequest{
			Version: &sovereignv1.LensVersion{
				LensVersionId: versionID.String(),
				LensId:        lensID.String(),
				CreatedAt:     timestamppbNowForTest(),
			},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

		ctxOtherTenant := ContextWithBackendClaims(context.Background(), authClaimsForTest(userID, otherTenantID))
		_, err = h.CreateLensVersion(ctxOtherTenant, connect.NewRequest(&sovereignv1.CreateLensVersionRequest{
			Version: &sovereignv1.LensVersion{
				LensVersionId: versionID.String(),
				LensId:        lensID.String(),
				CreatedAt:     timestamppbNowForTest(),
			},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	})

	t.Run("happy path creates lens version", func(t *testing.T) {
		repo := newLensTestRepo()
		repo.lenses[lensID] = &sovereign_db.KnowledgeLens{
			LensID:   lensID,
			UserID:   userID,
			TenantID: tenantID,
		}
		h := NewSovereignHandler(repo)
		ctx := ContextWithBackendClaims(context.Background(), authClaimsForTest(userID, tenantID))
		resp, err := h.CreateLensVersion(ctx, connect.NewRequest(&sovereignv1.CreateLensVersionRequest{
			Version: &sovereignv1.LensVersion{
				LensVersionId: versionID.String(),
				LensId:        lensID.String(),
				QueryText:     "golang",
				TagIds:        []string{"go"},
				CreatedAt:     timestamppbNowForTest(),
			},
		}))
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, repo.savedVersion)
		assert.Equal(t, versionID, repo.savedVersion.LensVersionID)
		assert.Equal(t, "golang", repo.savedVersion.QueryText)
	})
}

func TestSelectCurrentLens(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	lensID := uuid.New()
	versionID := uuid.New()

	t.Run("validation errors return CodeInvalidArgument", func(t *testing.T) {
		repo := newLensTestRepo()
		h := NewSovereignHandler(repo)

		// nil selection
		_, err := h.SelectCurrentLens(context.Background(), connect.NewRequest(&sovereignv1.SelectCurrentLensRequest{}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		// invalid user_id
		_, err = h.SelectCurrentLens(context.Background(), connect.NewRequest(&sovereignv1.SelectCurrentLensRequest{
			Selection: &sovereignv1.CurrentLensSelection{UserId: "bad"},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		// nil selected_at
		_, err = h.SelectCurrentLens(context.Background(), connect.NewRequest(&sovereignv1.SelectCurrentLensRequest{
			Selection: &sovereignv1.CurrentLensSelection{
				UserId:        userID.String(),
				LensId:        lensID.String(),
				LensVersionId: versionID.String(),
			},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("not found and happy path", func(t *testing.T) {
		repo := newLensTestRepo()
		h := NewSovereignHandler(repo)

		// lens not found
		_, err := h.SelectCurrentLens(context.Background(), connect.NewRequest(&sovereignv1.SelectCurrentLensRequest{
			Selection: &sovereignv1.CurrentLensSelection{
				UserId:        userID.String(),
				LensId:        lensID.String(),
				LensVersionId: versionID.String(),
				SelectedAt:    timestamppbNowForTest(),
			},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

		// lens exists, version not found
		repo.lenses[lensID] = &sovereign_db.KnowledgeLens{
			LensID:   lensID,
			UserID:   userID,
			TenantID: tenantID,
		}
		_, err = h.SelectCurrentLens(context.Background(), connect.NewRequest(&sovereignv1.SelectCurrentLensRequest{
			Selection: &sovereignv1.CurrentLensSelection{
				UserId:        userID.String(),
				LensId:        lensID.String(),
				LensVersionId: versionID.String(),
				SelectedAt:    timestamppbNowForTest(),
			},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

		// version exists -> happy path
		repo.versions[versionID] = &sovereign_db.KnowledgeLensVersion{
			LensVersionID: versionID,
			LensID:        lensID,
		}
		ctx := ContextWithBackendClaims(context.Background(), authClaimsForTest(userID, tenantID))
		resp, err := h.SelectCurrentLens(ctx, connect.NewRequest(&sovereignv1.SelectCurrentLensRequest{
			Selection: &sovereignv1.CurrentLensSelection{
				UserId:        userID.String(),
				LensId:        lensID.String(),
				LensVersionId: versionID.String(),
				SelectedAt:    timestamppbNowForTest(),
			},
		}))
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, repo.selections[userID])
		assert.Equal(t, lensID, repo.selections[userID].LensID)
	})
}

func TestClearCurrentLens(t *testing.T) {
	userID := uuid.New()
	otherUserID := uuid.New()
	tenantID := uuid.New()

	t.Run("invalid user_id returns CodeInvalidArgument", func(t *testing.T) {
		repo := newLensTestRepo()
		h := NewSovereignHandler(repo)
		_, err := h.ClearCurrentLens(context.Background(), connect.NewRequest(&sovereignv1.ClearCurrentLensRequest{
			UserId: "bad-uuid",
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("cross-user mismatch returns CodePermissionDenied", func(t *testing.T) {
		repo := newLensTestRepo()
		h := NewSovereignHandler(repo)
		ctx := ContextWithBackendClaims(context.Background(), authClaimsForTest(otherUserID, tenantID))
		_, err := h.ClearCurrentLens(ctx, connect.NewRequest(&sovereignv1.ClearCurrentLensRequest{
			UserId: userID.String(),
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	})

	t.Run("happy path clears current lens", func(t *testing.T) {
		repo := newLensTestRepo()
		repo.selections[userID] = &sovereign_db.KnowledgeCurrentLens{UserID: userID}
		h := NewSovereignHandler(repo)
		ctx := ContextWithBackendClaims(context.Background(), authClaimsForTest(userID, tenantID))
		resp, err := h.ClearCurrentLens(ctx, connect.NewRequest(&sovereignv1.ClearCurrentLensRequest{
			UserId: userID.String(),
		}))
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Nil(t, repo.selections[userID])
	})
}

func TestAreArticlesVisibleInLens_HappyPath(t *testing.T) {
	repo := newLensTestRepo()
	art1 := uuid.New()
	art2 := uuid.New()
	repo.visibility[art1] = true
	repo.visibility[art2] = false

	h := NewSovereignHandler(repo)
	resp, err := h.AreArticlesVisibleInLens(context.Background(), connect.NewRequest(&sovereignv1.AreArticlesVisibleInLensRequest{
		TenantId:   uuid.New().String(),
		UserId:     uuid.New().String(),
		ArticleIds: []string{art1.String(), art2.String()},
		Filter: &sovereignv1.LensFilter{
			QueryText: "rust",
			TagIds:    []string{"systems"},
		},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Msg.Visibility[art1.String()])
	assert.False(t, resp.Msg.Visibility[art2.String()])
	require.NotNil(t, repo.gotFilter)
	assert.Equal(t, "rust", repo.gotFilter.QueryText)
	assert.Equal(t, []string{"systems"}, repo.gotFilter.TagNames)
}
