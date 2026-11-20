package preprocessor_test

import (
	"context"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"pre-processor/connect/v2/preprocessor"
	"pre-processor/domain"
	preprocessorv2 "pre-processor/gen/proto/services/preprocessor/v2"
	"pre-processor/test/mocks"
)

func TestHandler_QueueSummarize_ValidationAndOriginMapping(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	const canonical = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

	t.Run("canonicalizes upper-case UUID and maps user origin", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), canonical).Return(false, nil)
		mockJobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), canonical, gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasInFlightJob(gomock.Any(), canonical, gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasDeadLetterJob(gomock.Any(), canonical).Return(false, nil)
		mockJobRepo.EXPECT().HasRecentFailedJob(gomock.Any(), canonical, gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().CountInvalidatedCompletedJobs(gomock.Any(), canonical).Return(0, nil)
		mockJobRepo.EXPECT().CreateJob(gomock.Any(), canonical, domain.SummarizeOriginUser).Return("job-connect-1", nil)

		h := preprocessor.NewHandler(mockAPIRepo, mockSummaryRepo, mockArticleRepo, mockJobRepo, logger)

		resp, err := h.QueueSummarize(context.Background(), connect.NewRequest(&preprocessorv2.QueueSummarizeRequest{
			ArticleId: "AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE",
			Origin:    preprocessorv2.SummarizeOrigin_SUMMARIZE_ORIGIN_USER,
		}))

		require.NoError(t, err)
		assert.Equal(t, "job-connect-1", resp.Msg.GetJobId())
		assert.Equal(t, "pending", resp.Msg.GetStatus())
	})

	t.Run("maps system origin", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), canonical).Return(false, nil)
		mockJobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), canonical, gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasInFlightJob(gomock.Any(), canonical, gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasDeadLetterJob(gomock.Any(), canonical).Return(false, nil)
		mockJobRepo.EXPECT().HasRecentFailedJob(gomock.Any(), canonical, gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().CountInvalidatedCompletedJobs(gomock.Any(), canonical).Return(0, nil)
		mockJobRepo.EXPECT().CreateJob(gomock.Any(), canonical, domain.SummarizeOriginSystem).Return("job-connect-sys", nil)

		h := preprocessor.NewHandler(mockAPIRepo, mockSummaryRepo, mockArticleRepo, mockJobRepo, logger)

		resp, err := h.QueueSummarize(context.Background(), connect.NewRequest(&preprocessorv2.QueueSummarizeRequest{
			ArticleId: canonical,
			Origin:    preprocessorv2.SummarizeOrigin_SUMMARIZE_ORIGIN_SYSTEM,
		}))

		require.NoError(t, err)
		assert.Equal(t, "job-connect-sys", resp.Msg.GetJobId())
		assert.Equal(t, "pending", resp.Msg.GetStatus())
	})

	t.Run("rejects unspecified or unknown origin with InvalidArgument", func(t *testing.T) {
		invalidOrigins := []preprocessorv2.SummarizeOrigin{
			preprocessorv2.SummarizeOrigin_SUMMARIZE_ORIGIN_UNSPECIFIED,
			preprocessorv2.SummarizeOrigin(99),
			preprocessorv2.SummarizeOrigin(-1),
		}
		for _, invalidOrigin := range invalidOrigins {
			ctrl := gomock.NewController(t)

			mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
			mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
			mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
			mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

			h := preprocessor.NewHandler(mockAPIRepo, mockSummaryRepo, mockArticleRepo, mockJobRepo, logger)

			resp, err := h.QueueSummarize(context.Background(), connect.NewRequest(&preprocessorv2.QueueSummarizeRequest{
				ArticleId: canonical,
				Origin:    invalidOrigin,
			}))

			require.Error(t, err, "expected error for origin %v", invalidOrigin)
			assert.Nil(t, resp)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

			ctrl.Finish()
		}
	})

	t.Run("rejects non-UUID article_id with InvalidArgument", func(t *testing.T) {
		for _, invalidID := range []string{"", "not-a-uuid", "article-1"} {
			ctrl := gomock.NewController(t)

			mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
			mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
			mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
			mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

			h := preprocessor.NewHandler(mockAPIRepo, mockSummaryRepo, mockArticleRepo, mockJobRepo, logger)

			resp, err := h.QueueSummarize(context.Background(), connect.NewRequest(&preprocessorv2.QueueSummarizeRequest{
				ArticleId: invalidID,
				Origin:    preprocessorv2.SummarizeOrigin_SUMMARIZE_ORIGIN_USER,
			}))

			require.Error(t, err, "expected error for %q", invalidID)
			assert.Nil(t, resp)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

			ctrl.Finish()
		}
	})
}
