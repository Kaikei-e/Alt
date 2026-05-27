package augur_test

import (
	"context"
	"testing"

	augurv2 "alt/gen/proto/alt/augur/v2"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestCreateAugurSessionFromLoopEntry_EvidenceKindDiscriminator pins how the
// handler must populate domain.AugurCitation based on the upstream
// LoopEvidenceRef.kind sent by the BFF. The screenshot bug — a bare UUID
// ending up in `URL` and being resolved by the browser as
// /augur/<uuid> — must not recur regardless of which kind value is sent.
//
// Rule of thumb:
//   - WEB     : RefId is an absolute URL → goes to AugurCitation.URL only
//   - ARTICLE : RefId is an alt-db UUID  → goes to AugurCitation.RefID only
//   - SUMMARY : RefId is an alt-db UUID  → goes to AugurCitation.RefID only
//   - UNSPECIFIED : legacy / rolling-deploy → preserve old behaviour
//     (URL=RefId) so the FE-side defensive renderer (citation-href.ts) can
//     still decide whether to render a link.
func TestCreateAugurSessionFromLoopEntry_EvidenceKindDiscriminator(t *testing.T) {
	const (
		webURL    = "https://example.test/posts/x"
		articleID = "11111111-1111-4111-8111-111111111111"
		summaryID = "22222222-2222-4222-8222-222222222222"
		legacyID  = "33333333-3333-4333-8333-333333333333"
	)

	cases := []struct {
		name string
		ref  *augurv2.LoopEvidenceRef
		want domain.AugurCitation
	}{
		{
			name: "WEB carries a URL, never a refId",
			ref: &augurv2.LoopEvidenceRef{
				RefId: webURL,
				Label: "Reference",
				Kind:  augurv2.CitationKind_CITATION_KIND_WEB,
			},
			want: domain.AugurCitation{
				URL:   webURL,
				Title: "Reference",
				Kind:  domain.CitationKindWeb,
			},
		},
		{
			name: "ARTICLE carries a refId, never a URL",
			ref: &augurv2.LoopEvidenceRef{
				RefId: articleID,
				Label: "article",
				Kind:  augurv2.CitationKind_CITATION_KIND_ARTICLE,
			},
			want: domain.AugurCitation{
				Title: "article",
				Kind:  domain.CitationKindArticle,
				RefID: articleID,
			},
		},
		{
			name: "SUMMARY carries a refId, never a URL",
			ref: &augurv2.LoopEvidenceRef{
				RefId: summaryID,
				Label: "summary",
				Kind:  augurv2.CitationKind_CITATION_KIND_SUMMARY,
			},
			want: domain.AugurCitation{
				Title: "summary",
				Kind:  domain.CitationKindSummary,
				RefID: summaryID,
			},
		},
		{
			name: "UNSPECIFIED preserves the old shape (URL=RefId, Kind empty) for rolling deploys",
			ref: &augurv2.LoopEvidenceRef{
				RefId: legacyID,
				Label: "legacy",
			},
			want: domain.AugurCitation{
				URL:   legacyID,
				Title: "legacy",
				Kind:  domain.CitationKindUnspecified,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockConv := new(MockAugurConversationUsecase)
			userID := uuid.New()
			createdID := uuid.New()

			mockConv.On(
				"CreateSessionFromLoopEntry",
				mock.Anything,
				mock.MatchedBy(func(input usecase.CreateSessionFromLoopEntryInput) bool {
					if len(input.EvidenceRefs) != 1 {
						return false
					}
					got := input.EvidenceRefs[0]
					return got == tc.want
				}),
			).Return(&domain.AugurConversation{
				ID:     createdID,
				UserID: userID,
				Title:  "Why",
			}, nil)

			handler := newLoopHandler(t, mockConv)
			req := newLoopRequest(userID, &augurv2.CreateAugurSessionFromLoopEntryRequest{
				ClientHandshakeId: validUUIDv7,
				EntryKey:          "entry-1",
				LensModeId:        "default",
				WhyText:           "Why",
				EvidenceRefs:      []*augurv2.LoopEvidenceRef{tc.ref},
			})

			_, err := handler.CreateAugurSessionFromLoopEntry(context.Background(), req)
			require.NoError(t, err)
			mockConv.AssertExpectations(t)

			// Independent assertion for clarity — the matcher above already
			// pins the shape but we restate it so a future failure is easy
			// to read in the test output.
			callArgs := mockConv.Calls[0].Arguments
			input := callArgs.Get(1).(usecase.CreateSessionFromLoopEntryInput)
			require.Len(t, input.EvidenceRefs, 1)
			assert.Equal(t, tc.want, input.EvidenceRefs[0])
		})
	}
}
