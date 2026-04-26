package augur_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	augurv2 "alt/gen/proto/alt/augur/v2"

	"rag-orchestrator/internal/adapter/connect/augur"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// validUUIDv7 is a UUIDv7 literal suitable for the idempotency key regex test.
const validUUIDv7 = "01938e82-7c00-7a7b-9b10-0123456789ab"

func newLoopHandler(t *testing.T, conv *MockAugurConversationUsecase) *augur.Handler {
	t.Helper()
	mockAnswer := new(MockAnswerWithRAGUsecase)
	mockRetrieve := new(MockRetrieveContextUsecase)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return augur.NewHandler(mockAnswer, mockRetrieve, conv, nil, logger)
}

// defaultTestTenantID is a fixed non-zero tenant uuid used by every loop
// request unless the test explicitly overrides it. Wave 4-A made
// X-Alt-Tenant-Id required on this path; every legitimate caller sends it.
var defaultTestTenantID = uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")

func newLoopRequest(userID uuid.UUID, body *augurv2.CreateAugurSessionFromLoopEntryRequest) *connect.Request[augurv2.CreateAugurSessionFromLoopEntryRequest] {
	return newLoopRequestWithTenant(userID, defaultTestTenantID, body)
}

// newLoopRequestWithTenant lets a single test override the tenant header
// (e.g. to simulate alt-backend dropping it). userID == uuid.Nil omits the
// user header; tenantID == uuid.Nil omits the tenant header.
func newLoopRequestWithTenant(userID, tenantID uuid.UUID, body *augurv2.CreateAugurSessionFromLoopEntryRequest) *connect.Request[augurv2.CreateAugurSessionFromLoopEntryRequest] {
	req := connect.NewRequest(body)
	if userID != uuid.Nil {
		req.Header().Set("X-Alt-User-Id", userID.String())
	}
	if tenantID != uuid.Nil {
		req.Header().Set("X-Alt-Tenant-Id", tenantID.String())
	}
	return req
}

func TestCreateAugurSessionFromLoopEntry_MissingUserHeader_401(t *testing.T) {
	handler := newLoopHandler(t, new(MockAugurConversationUsecase))
	req := newLoopRequest(uuid.Nil, &augurv2.CreateAugurSessionFromLoopEntryRequest{
		ClientHandshakeId: validUUIDv7,
		EntryKey:          "entry-1",
		WhyText:           "a why",
	})

	_, err := handler.CreateAugurSessionFromLoopEntry(context.Background(), req)
	require.Error(t, err)
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	assert.Equal(t, connect.CodeUnauthenticated, cerr.Code())
}

// TestCreateAugurSessionFromLoopEntry_MissingTenantHeader_401 pins the
// Wave 4-A strictness contract: a caller that supplies X-Alt-User-Id but
// not X-Alt-Tenant-Id is rejected as Unauthenticated. Without this guard
// the projector would persist knowledge_events.tenant_id = ” and
// Surface Planner v2's tenant-scoped resolver could leak across tenants.
func TestCreateAugurSessionFromLoopEntry_MissingTenantHeader_401(t *testing.T) {
	handler := newLoopHandler(t, new(MockAugurConversationUsecase))
	req := newLoopRequestWithTenant(uuid.New(), uuid.Nil, &augurv2.CreateAugurSessionFromLoopEntryRequest{
		ClientHandshakeId: validUUIDv7,
		EntryKey:          "entry-1",
		WhyText:           "a why",
	})

	_, err := handler.CreateAugurSessionFromLoopEntry(context.Background(), req)
	require.Error(t, err)
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	assert.Equal(t, connect.CodeUnauthenticated, cerr.Code())
}

func TestCreateAugurSessionFromLoopEntry_NotUUIDv7_400(t *testing.T) {
	handler := newLoopHandler(t, new(MockAugurConversationUsecase))
	req := newLoopRequest(uuid.New(), &augurv2.CreateAugurSessionFromLoopEntryRequest{
		ClientHandshakeId: "not-a-uuid",
		EntryKey:          "entry-1",
		WhyText:           "a why",
	})

	_, err := handler.CreateAugurSessionFromLoopEntry(context.Background(), req)
	require.Error(t, err)
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	assert.Equal(t, connect.CodeInvalidArgument, cerr.Code())
}

func TestCreateAugurSessionFromLoopEntry_EmptyEntryKey_400(t *testing.T) {
	handler := newLoopHandler(t, new(MockAugurConversationUsecase))
	req := newLoopRequest(uuid.New(), &augurv2.CreateAugurSessionFromLoopEntryRequest{
		ClientHandshakeId: validUUIDv7,
		WhyText:           "a why",
	})

	_, err := handler.CreateAugurSessionFromLoopEntry(context.Background(), req)
	require.Error(t, err)
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	assert.Equal(t, connect.CodeInvalidArgument, cerr.Code())
}

func TestCreateAugurSessionFromLoopEntry_EmptyWhyText_400(t *testing.T) {
	handler := newLoopHandler(t, new(MockAugurConversationUsecase))
	req := newLoopRequest(uuid.New(), &augurv2.CreateAugurSessionFromLoopEntryRequest{
		ClientHandshakeId: validUUIDv7,
		EntryKey:          "entry-1",
	})

	_, err := handler.CreateAugurSessionFromLoopEntry(context.Background(), req)
	require.Error(t, err)
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	assert.Equal(t, connect.CodeInvalidArgument, cerr.Code())
}

func TestCreateAugurSessionFromLoopEntry_WhyTextTooLong_400(t *testing.T) {
	long := make([]byte, 513)
	for i := range long {
		long[i] = 'a'
	}
	handler := newLoopHandler(t, new(MockAugurConversationUsecase))
	req := newLoopRequest(uuid.New(), &augurv2.CreateAugurSessionFromLoopEntryRequest{
		ClientHandshakeId: validUUIDv7,
		EntryKey:          "entry-1",
		WhyText:           string(long),
	})

	_, err := handler.CreateAugurSessionFromLoopEntry(context.Background(), req)
	require.Error(t, err)
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	assert.Equal(t, connect.CodeInvalidArgument, cerr.Code())
}

func TestCreateAugurSessionFromLoopEntry_TooManyEvidenceRefs_400(t *testing.T) {
	refs := make([]*augurv2.LoopEvidenceRef, 9)
	for i := range refs {
		refs[i] = &augurv2.LoopEvidenceRef{RefId: "r", Label: "l"}
	}
	handler := newLoopHandler(t, new(MockAugurConversationUsecase))
	req := newLoopRequest(uuid.New(), &augurv2.CreateAugurSessionFromLoopEntryRequest{
		ClientHandshakeId: validUUIDv7,
		EntryKey:          "entry-1",
		WhyText:           "a why",
		EvidenceRefs:      refs,
	})

	_, err := handler.CreateAugurSessionFromLoopEntry(context.Background(), req)
	require.Error(t, err)
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	assert.Equal(t, connect.CodeInvalidArgument, cerr.Code())
}

func TestCreateAugurSessionFromLoopEntry_Success_ReturnsConversationID(t *testing.T) {
	mockConv := new(MockAugurConversationUsecase)
	userID := uuid.New()
	createdID := uuid.New()

	mockConv.On("CreateSessionFromLoopEntry", mock.Anything, mock.MatchedBy(func(input usecase.CreateSessionFromLoopEntryInput) bool {
		return input.UserID == userID &&
			input.EntryKey == "entry-1" &&
			input.LensModeID == "default" &&
			input.WhyText == "A fresh why." &&
			len(input.EvidenceRefs) == 2 &&
			input.EvidenceRefs[0].URL == "https://example.com/a" &&
			input.EvidenceRefs[0].Title == "a"
	})).Return(&domain.AugurConversation{
		ID:     createdID,
		UserID: userID,
		Title:  "A fresh why.",
	}, nil)

	handler := newLoopHandler(t, mockConv)
	req := newLoopRequest(userID, &augurv2.CreateAugurSessionFromLoopEntryRequest{
		ClientHandshakeId: validUUIDv7,
		EntryKey:          "entry-1",
		LensModeId:        "default",
		WhyText:           "A fresh why.",
		EvidenceRefs: []*augurv2.LoopEvidenceRef{
			{RefId: "https://example.com/a", Label: "a"},
			{RefId: "https://example.com/b", Label: "b"},
		},
	})

	resp, err := handler.CreateAugurSessionFromLoopEntry(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, createdID.String(), resp.Msg.ConversationId)

	mockConv.AssertExpectations(t)
}
