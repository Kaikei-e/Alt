package datahub_capability_gateway

import (
	"context"
	"testing"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSummaryVersionDriver struct {
	gotSupersedeArticleID uuid.UUID
	gotSupersedeNewID     uuid.UUID
	gotSupersedeUserID    uuid.UUID
	gotByIDVersionID      uuid.UUID
	gotByIDUserID         uuid.UUID
	gotLatestArticleID    uuid.UUID
	gotLatestUserID       *uuid.UUID
	prev                  *domain.SummaryVersion
	byID                  domain.SummaryVersion
	latest                domain.SummaryVersion
}

func (m *mockSummaryVersionDriver) CreateSummaryVersion(_ context.Context, _ domain.SummaryVersion) error {
	return nil
}

func (m *mockSummaryVersionDriver) MarkSummaryVersionSuperseded(_ context.Context, articleID, newVersionID, userID uuid.UUID) (*domain.SummaryVersion, error) {
	m.gotSupersedeArticleID = articleID
	m.gotSupersedeNewID = newVersionID
	m.gotSupersedeUserID = userID
	return m.prev, nil
}

func (m *mockSummaryVersionDriver) GetSummaryVersionByID(_ context.Context, summaryVersionID, userID uuid.UUID) (domain.SummaryVersion, error) {
	m.gotByIDVersionID = summaryVersionID
	m.gotByIDUserID = userID
	return m.byID, nil
}

func (m *mockSummaryVersionDriver) GetLatestSummaryVersionForUser(_ context.Context, articleID uuid.UUID, userID *uuid.UUID) (domain.SummaryVersion, error) {
	m.gotLatestArticleID = articleID
	m.gotLatestUserID = userID
	return m.latest, nil
}

func TestSummaryVersionGateway_MarkSummaryVersionSuperseded_PassesUserScope(t *testing.T) {
	articleID := uuid.New()
	newVersionID := uuid.New()
	userID := uuid.New()
	prevID := uuid.New()

	drv := &mockSummaryVersionDriver{
		prev: &domain.SummaryVersion{SummaryVersionID: prevID, ArticleID: articleID, UserID: userID},
	}
	gw := &SummaryVersionGateway{db: drv}

	got, err := gw.MarkSummaryVersionSuperseded(context.Background(), articleID, newVersionID, userID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, prevID, got.SummaryVersionID)
	assert.Equal(t, articleID, drv.gotSupersedeArticleID)
	assert.Equal(t, newVersionID, drv.gotSupersedeNewID)
	assert.Equal(t, userID, drv.gotSupersedeUserID)
}

func TestSummaryVersionGateway_GetSummaryVersionByID_PassesUserScope(t *testing.T) {
	versionID := uuid.New()
	articleID := uuid.New()
	userID := uuid.New()

	drv := &mockSummaryVersionDriver{
		byID: domain.SummaryVersion{SummaryVersionID: versionID, ArticleID: articleID, UserID: userID},
	}
	gw := &SummaryVersionGateway{db: drv}

	got, err := gw.GetSummaryVersionByID(context.Background(), versionID, userID)
	require.NoError(t, err)
	assert.Equal(t, versionID, got.SummaryVersionID)
	assert.Equal(t, versionID, drv.gotByIDVersionID)
	assert.Equal(t, userID, drv.gotByIDUserID)
}

func TestSummaryVersionGateway_GetLatestSummaryVersion_PassesUserScope(t *testing.T) {
	articleID := uuid.New()
	userID := uuid.New()
	versionID := uuid.New()

	drv := &mockSummaryVersionDriver{
		latest: domain.SummaryVersion{SummaryVersionID: versionID, ArticleID: articleID, UserID: userID},
	}
	gw := &SummaryVersionGateway{db: drv}

	got, err := gw.GetLatestSummaryVersion(context.Background(), articleID, userID)
	require.NoError(t, err)
	assert.Equal(t, versionID, got.SummaryVersionID)
	assert.Equal(t, articleID, drv.gotLatestArticleID)
	require.NotNil(t, drv.gotLatestUserID)
	assert.Equal(t, userID, *drv.gotLatestUserID)
}
