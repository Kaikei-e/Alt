package knowledge_home

import (
	"context"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/orchestrator/usecase/get_knowledge_home_usecase"
	"alt/orchestrator/usecase/track_home_action_usecase"
	"alt/orchestrator/usecase/track_home_seen_usecase"
	"alt/utils/logger"
	altotel "alt/utils/otel"
)

func TestGetKnowledgeHome_RequestsTotalCarriesStatus(t *testing.T) {
	logger.InitLogger()

	reader := sdkmetric.NewManualReader()
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	metrics, err := altotel.NewKnowledgeHomeMetrics()
	require.NoError(t, err)

	homePort := &mockHomeItemsPort{
		items: nil,
		err:   assert.AnError,
	}
	digestPort := &mockTodayDigestPort{err: assert.AnError}
	getHome := get_knowledge_home_usecase.NewGetKnowledgeHomeUsecase(homePort, digestPort, nil, nil, nil, nil)
	handler := NewHandler(
		getHome,
		track_home_seen_usecase.NewTrackHomeSeenUsecase(&mockUserEventPort{}, nil),
		track_home_action_usecase.NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil, nil, nil, nil, nil),
		nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		metrics,
		slog.Default(),
	)

	_, err = handler.GetKnowledgeHome(testUserContext(), connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{Limit: 20}))
	require.Error(t, err)

	okHandler, _, _ := setupHandler()
	okHandler.metrics = metrics
	_, err = okHandler.GetKnowledgeHome(testUserContext(), connect.NewRequest(&knowledgehomev1.GetKnowledgeHomeRequest{Limit: 20}))
	require.NoError(t, err)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	got := requestStatusValues(rm)
	assert.Contains(t, got, "error", "availability SLO filters status=error; a failure must increment that series")
	assert.Contains(t, got, "ok", "successful Home reads must increment status=ok so the error ratio has a denominator")
}

func requestStatusValues(rm metricdata.ResourceMetrics) []string {
	var out []string
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "alt_home_requests_total" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range sum.DataPoints {
				for _, kv := range dp.Attributes.ToSlice() {
					if string(kv.Key) == "status" {
						out = append(out, kv.Value.AsString())
					}
				}
			}
		}
	}
	return out
}
