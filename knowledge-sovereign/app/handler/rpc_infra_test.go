package handler

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func TestValidateAndBuildProjectionVersion(t *testing.T) {
	refTime := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	activated := time.Date(2026, 9, 25, 14, 0, 0, 0, time.UTC)
	pbActivated := timestamppb.New(activated)
	explicitCreated := time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)
	pbCreated := timestamppb.New(explicitCreated)

	tests := []struct {
		name        string
		in          *sovereignv1.ProjectionVersion
		refTime     time.Time
		wantErr     bool
		wantVersion int
		wantDesc    string
		wantStatus  string
		wantCreated time.Time
		hasActive   bool
	}{
		{
			name:    "nil version returns error",
			in:      nil,
			refTime: refTime,
			wantErr: true,
		},
		{
			name: "uses refTime when created_at is nil",
			in: &sovereignv1.ProjectionVersion{
				Version:     1,
				Description: "v1 baseline",
				Status:      "active",
			},
			refTime:     refTime,
			wantErr:     false,
			wantVersion: 1,
			wantDesc:    "v1 baseline",
			wantStatus:  "active",
			wantCreated: refTime,
			hasActive:   false,
		},
		{
			name: "preserves explicit created_at and activated_at",
			in: &sovereignv1.ProjectionVersion{
				Version:     2,
				Description: "v2 shadow",
				Status:      "shadow",
				CreatedAt:   pbCreated,
				ActivatedAt: pbActivated,
			},
			refTime:     refTime,
			wantErr:     false,
			wantVersion: 2,
			wantDesc:    "v2 shadow",
			wantStatus:  "shadow",
			wantCreated: explicitCreated,
			hasActive:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateAndBuildProjectionVersion(tt.in, tt.refTime)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantVersion, got.Version)
				assert.Equal(t, tt.wantDesc, got.Description)
				assert.Equal(t, tt.wantStatus, got.Status)
				assert.True(t, got.CreatedAt.Equal(tt.wantCreated))
				if tt.hasActive {
					require.NotNil(t, got.ActivatedAt)
					assert.True(t, got.ActivatedAt.Equal(activated))
				} else {
					assert.Nil(t, got.ActivatedAt)
				}
			}
		})
	}
}

func TestProjectionVersionToProto(t *testing.T) {
	created := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	activated := time.Date(2026, 9, 25, 11, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		in        sovereign_db.ProjectionVersion
		hasActive bool
	}{
		{
			name: "without activated_at",
			in: sovereign_db.ProjectionVersion{
				Version:     1,
				Description: "initial",
				Status:      "draft",
				CreatedAt:   created,
			},
			hasActive: false,
		},
		{
			name: "with activated_at",
			in: sovereign_db.ProjectionVersion{
				Version:     2,
				Description: "promoted",
				Status:      "active",
				CreatedAt:   created,
				ActivatedAt: &activated,
			},
			hasActive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pb := projectionVersionToProto(tt.in)
			require.NotNil(t, pb)
			assert.Equal(t, int32(tt.in.Version), pb.Version)
			assert.Equal(t, tt.in.Description, pb.Description)
			assert.Equal(t, tt.in.Status, pb.Status)
			assert.True(t, pb.CreatedAt.AsTime().Equal(created))
			if tt.hasActive {
				require.NotNil(t, pb.ActivatedAt)
				assert.True(t, pb.ActivatedAt.AsTime().Equal(activated))
			} else {
				assert.Nil(t, pb.ActivatedAt)
			}
		})
	}
}

func TestGetProjectionCheckpoint_ReturnsZero(t *testing.T) {
	repo := &mockRepo{}
	client, cleanup := setupTestServer(repo)
	defer cleanup()

	resp, err := client.GetProjectionCheckpoint(context.Background(),
		connect.NewRequest(&sovereignv1.GetProjectionCheckpointRequest{
			ProjectorName: "test-projector",
		}))

	require.NoError(t, err)
	assert.Equal(t, int64(0), resp.Msg.LastEventSeq)
}

func TestUpdateProjectionCheckpoint(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)

	resp, err := h.UpdateProjectionCheckpoint(context.Background(),
		connect.NewRequest(&sovereignv1.UpdateProjectionCheckpointRequest{
			ProjectorName: "test-projector",
			LastEventSeq:  50,
		}))

	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestGetProjectionFreshness_NotFound(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)

	resp, err := h.GetProjectionFreshness(context.Background(),
		connect.NewRequest(&sovereignv1.GetProjectionFreshnessRequest{
			ProjectorName: "test-projector",
		}))

	require.NoError(t, err)
	assert.False(t, resp.Msg.Found)
}

func TestGetProjectionLag(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)

	resp, err := h.GetProjectionLag(context.Background(),
		connect.NewRequest(&sovereignv1.GetProjectionLagRequest{}))

	require.NoError(t, err)
	assert.Equal(t, float64(0), resp.Msg.LagSeconds)
	assert.Equal(t, float64(0), resp.Msg.AgeSeconds)
}

func TestProjectionVersionsRPCs(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)

	// GetActiveProjectionVersion
	activeResp, err := h.GetActiveProjectionVersion(context.Background(),
		connect.NewRequest(&sovereignv1.GetActiveProjectionVersionRequest{}))
	require.NoError(t, err)
	assert.Nil(t, activeResp.Msg.Version)

	// ListProjectionVersions
	listResp, err := h.ListProjectionVersions(context.Background(),
		connect.NewRequest(&sovereignv1.ListProjectionVersionsRequest{}))
	require.NoError(t, err)
	assert.Empty(t, listResp.Msg.Versions)

	// ActivateProjectionVersion
	actResp, err := h.ActivateProjectionVersion(context.Background(),
		connect.NewRequest(&sovereignv1.ActivateProjectionVersionRequest{Version: 1}))
	require.NoError(t, err)
	require.NotNil(t, actResp)
}
