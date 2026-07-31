package augur

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"alt/domain"
	augurv2 "alt/gen/proto/alt/augur/v2"
	"alt/mocks"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// timeFar returns a far-future timestamp for UserContext.ExpiresAt so
// IsValid() never trips during tests.
func timeFar() time.Time { return time.Now().Add(24 * time.Hour) }

// sanitizeMetaEvent must preserve conversation_id end-to-end so the
// frontend can keep sending the same id on subsequent turns. Stripping it
// (the original behaviour) caused every turn to spawn a brand-new
// conversation row, fragmenting the user's history.
func TestSanitizeMetaEvent_PreservesConversationID(t *testing.T) {
	h := NewHandler(nil, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	in := &augurv2.StreamChatResponse{
		Kind: "meta",
		Payload: &augurv2.StreamChatResponse_Meta{
			Meta: &augurv2.MetaPayload{
				ConversationId: "11111111-2222-3333-4444-555555555555",
				Citations: []*augurv2.Citation{
					{Url: "https://example.com", Title: "Example"},
				},
			},
		},
	}

	out := h.sanitizeMetaEvent(in)
	meta := out.GetMeta()
	if assert.NotNil(t, meta, "sanitized event must still carry MetaPayload") {
		assert.Equal(t, "11111111-2222-3333-4444-555555555555", meta.ConversationId,
			"conversation_id must round-trip through sanitization")
		assert.Len(t, meta.Citations, 1, "citations array must be preserved")
		assert.Equal(t, "https://example.com", meta.Citations[0].Url)
	}
}

// sanitizeMetaEvent must carry Citation.kind / Citation.ref_id through to the
// client. ADR-000926 introduced CitationKind precisely so ARTICLE / SUMMARY
// citations route via `/articles/<ref_id>` instead of gambling on `url`, and
// the FE (citation-href.ts) renders an UNSPECIFIED citation as a dead,
// unlinked span. Dropping the discriminator here therefore reproduces the very
// "Ask Augur で元記事が参照されない" bug ADR-000927 declared eliminated — the
// meta citations are what the FE falls back to whenever the stream ends
// without a done event, so the dead links are permanent, not transient.
func TestSanitizeMetaEvent_PreservesCitationKindAndRefID(t *testing.T) {
	h := NewHandler(nil, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	in := &augurv2.StreamChatResponse{
		Kind: "meta",
		Payload: &augurv2.StreamChatResponse_Meta{
			Meta: &augurv2.MetaPayload{
				ConversationId: "11111111-2222-3333-4444-555555555555",
				Citations: []*augurv2.Citation{
					{
						Url:         "https://example.com",
						Title:       "Example",
						PublishedAt: "2026-05-27T00:00:00Z",
						Kind:        augurv2.CitationKind_CITATION_KIND_ARTICLE,
						RefId:       "6b5bbd85-ed4a-4812-95f1-22f2d181437a",
					},
				},
			},
		},
	}

	out := h.sanitizeMetaEvent(in)
	meta := out.GetMeta()
	require.NotNil(t, meta, "sanitized event must still carry MetaPayload")
	require.Len(t, meta.Citations, 1, "citations array must be preserved")
	assert.Equal(t, augurv2.CitationKind_CITATION_KIND_ARTICLE, meta.Citations[0].Kind,
		"citation kind must survive sanitization; UNSPECIFIED renders as a dead link")
	assert.Equal(t, "6b5bbd85-ed4a-4812-95f1-22f2d181437a", meta.Citations[0].RefId,
		"citation ref_id must survive sanitization; it is the /articles/<id> target")
}

// TestSanitizeMetaEvent_CarriesEveryField is the rot guard. The 3-field
// allowlist that dropped kind / ref_id was not wrong when it was written — the
// Citation message had exactly those three fields at the time — it rotted the
// day ADR-000926 added two more. Rather than trusting the next author to
// remember this function, walk the descriptor: populate every field with a
// non-default value and require the sanitized copy to compare equal. Adding a
// proto field without carrying it fails here immediately.
//
// The walk covers the whole StreamChatResponse, not just MetaPayload: the
// envelope was rebuilt by hand too (kind + payload), which is the same
// two-field allowlist one level up. A future `trace_id` on StreamChatResponse
// would otherwise vanish on meta events while surviving on delta / done /
// error — the same asymmetric, hard-to-diagnose shape as the citation bug.
func TestSanitizeMetaEvent_CarriesEveryField(t *testing.T) {
	h := NewHandler(nil, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	meta := &augurv2.MetaPayload{}
	populateEveryField(t, meta.ProtoReflect())
	require.NotEmpty(t, meta.Citations, "guard must populate the repeated citations field")

	// The payload oneof is set here rather than by the walk: its members are
	// mutually exclusive, so populating each in turn would leave only the last
	// one set and there would be no meta payload left to sanitize.
	in := &augurv2.StreamChatResponse{
		Payload: &augurv2.StreamChatResponse_Meta{Meta: meta},
	}
	populateEveryField(t, in.ProtoReflect())
	require.NotEmpty(t, in.Kind, "guard must populate the envelope's non-oneof fields")

	want := proto.Clone(in).(*augurv2.StreamChatResponse)
	out := h.sanitizeMetaEvent(in)

	assert.Truef(t, proto.Equal(want, out),
		"sanitizeMetaEvent dropped at least one StreamChatResponse/MetaPayload/Citation field — copy the message wholesale instead of rebuilding it field by field\nwant: %v\ngot:  %v",
		want, out)
}

// TestSanitizeMetaEvent_ReturnsDeepCopy pins the one behaviour that survives
// from the old rebuild and is the only remaining reason this function exists:
// the forwarded event must not alias the stream's receive buffer, which
// connect reuses across Receive() calls. Once the body is a wholesale copy,
// "why not just return event?" is a tempting simplification — this test is
// what says no.
func TestSanitizeMetaEvent_ReturnsDeepCopy(t *testing.T) {
	h := NewHandler(nil, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	in := &augurv2.StreamChatResponse{
		Kind: "meta",
		Payload: &augurv2.StreamChatResponse_Meta{
			Meta: &augurv2.MetaPayload{
				ConversationId: "11111111-2222-3333-4444-555555555555",
				Citations: []*augurv2.Citation{
					{Url: "https://example.com", Kind: augurv2.CitationKind_CITATION_KIND_WEB},
				},
			},
		},
	}
	want := proto.Clone(in).(*augurv2.StreamChatResponse)

	out := h.sanitizeMetaEvent(in)

	// Simulate connect reusing the buffer for the next event.
	in.Kind = "delta"
	in.GetMeta().ConversationId = "overwritten"
	in.GetMeta().Citations[0].Url = "https://overwritten.example"

	assert.Truef(t, proto.Equal(want, out),
		"sanitizeMetaEvent must return a deep copy; mutating the received event changed the forwarded one\nwant: %v\ngot:  %v",
		want, out)
}

// populateEveryField sets every field of m to a non-default value, recursing
// into message fields and appending exactly one element to repeated fields.
// It fails the test on any construct it does not know how to fill, so an
// unhandled proto shape surfaces as a loud failure instead of a silent gap in
// the guard above.
//
// Members of a real oneof are left to the caller: they are mutually exclusive,
// so setting each in turn would leave only the last one populated. Synthetic
// oneofs (proto3 `optional`) are ordinary fields and are still filled.
func populateEveryField(t *testing.T, m protoreflect.Message) {
	t.Helper()

	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if oneof := fd.ContainingOneof(); oneof != nil && !oneof.IsSynthetic() {
			continue
		}
		switch {
		case fd.IsMap():
			t.Fatalf("map field %s is not covered by this guard; extend populateEveryField", fd.FullName())
		case fd.IsList():
			list := m.Mutable(fd).List()
			if fd.Message() != nil {
				elem := list.NewElement()
				populateEveryField(t, elem.Message())
				list.Append(elem)
				continue
			}
			list.Append(nonDefaultScalar(t, fd))
		case fd.Message() != nil:
			populateEveryField(t, m.Mutable(fd).Message())
		default:
			m.Set(fd, nonDefaultScalar(t, fd))
		}
	}
}

// nonDefaultScalar returns a value distinguishable from the proto3 zero value,
// so a dropped field shows up as a diff rather than as an accidental match.
func nonDefaultScalar(t *testing.T, fd protoreflect.FieldDescriptor) protoreflect.Value {
	t.Helper()

	switch fd.Kind() {
	case protoreflect.StringKind:
		return protoreflect.ValueOfString("guard-" + string(fd.Name()))
	case protoreflect.BytesKind:
		return protoreflect.ValueOfBytes([]byte("guard"))
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(true)
	case protoreflect.EnumKind:
		values := fd.Enum().Values()
		if values.Len() < 2 {
			t.Fatalf("enum %s has no non-default value to assert on", fd.Enum().FullName())
		}
		return protoreflect.ValueOfEnum(values.Get(1).Number())
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(1)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(1)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(1)
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(1)
	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(1)
	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(1)
	default:
		t.Fatalf("field %s has kind %s; extend nonDefaultScalar", fd.FullName(), fd.Kind())
		return protoreflect.Value{}
	}
}

// authedCtx returns a context with a valid UserContext for handler tests.
func authedCtx() context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		ExpiresAt: timeFar(),
	})
}

// TestGetConversation_PassesThroughCodeNotFound pins the fix for the
// "Error ID: <hash>" red banner the UI showed every time a user asked a
// just-created conversation but the consumer polled before rag-orchestrator
// had finished the row insert. The provider returns `CodeNotFound`; the
// previous handler wrapped that into `CodeInternal` with an "internal server
// error (caused by: not_found)" message, so the FE treated it as an outage.
// The handler MUST transparently re-emit CodeNotFound so the FE can render a
// graceful "conversation not yet available" state instead.
func TestGetConversation_PassesThroughCodeNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockRagStreamPort(ctrl)
	mockPort.EXPECT().
		GetConversation(gomock.Any(), gomock.Any()).
		Return(nil, connect.NewError(connect.CodeNotFound, nil))

	h := NewHandler(nil, mockPort, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	resp, err := h.GetConversation(authedCtx(), connect.NewRequest(&augurv2.GetConversationRequest{
		Id: uuid.NewString(),
	}))

	require.Error(t, err)
	require.Nil(t, resp)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err),
		"GetConversation must transparently re-emit CodeNotFound, not wrap as Internal")
}

// TestListConversations_PassesThroughCodeNotFound — same pattern: a user
// whose history has been purged should see an empty / "no history" UI, not
// a red error banner.
func TestListConversations_PassesThroughCodeNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockRagStreamPort(ctrl)
	mockPort.EXPECT().
		ListConversations(gomock.Any(), gomock.Any()).
		Return(nil, connect.NewError(connect.CodeNotFound, nil))

	h := NewHandler(nil, mockPort, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	resp, err := h.ListConversations(authedCtx(), connect.NewRequest(&augurv2.ListConversationsRequest{}))

	require.Error(t, err)
	require.Nil(t, resp)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err),
		"ListConversations must transparently re-emit CodeNotFound")
}

// TestDeleteConversation_PassesThroughCodeNotFound — delete on a missing
// conversation is idempotent semantically; the FE treats NotFound as a
// no-op success. Wrapping as Internal turned the action into a hard error.
func TestDeleteConversation_PassesThroughCodeNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockRagStreamPort(ctrl)
	mockPort.EXPECT().
		DeleteConversation(gomock.Any(), gomock.Any()).
		Return(nil, connect.NewError(connect.CodeNotFound, nil))

	h := NewHandler(nil, mockPort, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	resp, err := h.DeleteConversation(authedCtx(), connect.NewRequest(&augurv2.DeleteConversationRequest{
		Id: uuid.NewString(),
	}))

	require.Error(t, err)
	require.Nil(t, resp)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err),
		"DeleteConversation must transparently re-emit CodeNotFound")
}

// TestGetConversation_NonNotFoundStillWrapsAsInternal — defense in depth.
// Provider-side network failure, mTLS denial, deadline exceeded etc. MUST
// still be sanitised to CodeInternal so internal details do not leak.
func TestGetConversation_NonNotFoundStillWrapsAsInternal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockRagStreamPort(ctrl)
	mockPort.EXPECT().
		GetConversation(gomock.Any(), gomock.Any()).
		Return(nil, connect.NewError(connect.CodeUnavailable, nil))

	h := NewHandler(nil, mockPort, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	resp, err := h.GetConversation(authedCtx(), connect.NewRequest(&augurv2.GetConversationRequest{
		Id: uuid.NewString(),
	}))

	require.Error(t, err)
	require.Nil(t, resp)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err),
		"non-NotFound upstream codes must still be sanitised to Internal")
}
