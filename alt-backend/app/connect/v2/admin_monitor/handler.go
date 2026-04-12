// Package admin_monitor provides the Connect-RPC handler for AdminMonitorService,
// which serves Prometheus-backed observability data to the Admin UI.
package admin_monitor

import (
	"alt/domain"
	adminmonitorv1 "alt/gen/proto/alt/admin_monitor/v1"
	"alt/gen/proto/alt/admin_monitor/v1/adminmonitorv1connect"
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"
)

// Usecase is the orchestration boundary required by the handler.
// Implementations pipe to AdminMetricsPort under the hood.
type Usecase interface {
	GetCatalog() []domain.MetricCatalogEntry
	GetSnapshot(ctx context.Context, keys []domain.MetricKey, window domain.RangeWindow, step domain.Step) (*domain.MetricsSnapshot, error)
	StreamSnapshots(ctx context.Context, keys []domain.MetricKey, window domain.RangeWindow, step domain.Step) (<-chan *domain.MetricsSnapshot, error)
}

type Handler struct {
	uc     Usecase
	logger *slog.Logger
}

var _ adminmonitorv1connect.AdminMonitorServiceHandler = (*Handler)(nil)

func NewHandler(uc Usecase, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{uc: uc, logger: logger}
}

func (h *Handler) Catalog(ctx context.Context, req *connect.Request[adminmonitorv1.CatalogRequest]) (*connect.Response[adminmonitorv1.CatalogResponse], error) {
	entries := h.uc.GetCatalog()
	out := &adminmonitorv1.CatalogResponse{Entries: make([]*adminmonitorv1.CatalogEntry, 0, len(entries))}
	for _, e := range entries {
		out.Entries = append(out.Entries, &adminmonitorv1.CatalogEntry{
			Key:         string(e.Key),
			Title:       e.Title,
			Unit:        e.Unit,
			Description: e.Description,
			GrafanaUrl:  e.GrafanaURL,
			Kind:        kindToProto(e.Kind),
		})
	}
	return connect.NewResponse(out), nil
}

func (h *Handler) Snapshot(ctx context.Context, req *connect.Request[adminmonitorv1.SnapshotRequest]) (*connect.Response[adminmonitorv1.SnapshotResponse], error) {
	keys := toKeys(req.Msg.Keys)
	snap, err := h.uc.GetSnapshot(ctx, keys, windowFromProto(req.Msg.Window), stepFromProto(req.Msg.Step))
	if err != nil {
		return nil, connectError(err)
	}
	resp := &adminmonitorv1.SnapshotResponse{
		Time:    snap.Time.UTC().Format(time.RFC3339Nano),
		Metrics: snapshotToProto(snap),
	}
	connectResp := connect.NewResponse(resp)
	// Backbone SSE-like behavior: defeat intermediate buffering.
	connectResp.Header().Set("X-Accel-Buffering", "no")
	return connectResp, nil
}

func (h *Handler) Watch(ctx context.Context, req *connect.Request[adminmonitorv1.WatchRequest], stream *connect.ServerStream[adminmonitorv1.WatchResponse]) error {
	stream.ResponseHeader().Set("X-Accel-Buffering", "no")
	stream.ResponseHeader().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	keys := toKeys(req.Msg.Keys)
	ch, err := h.uc.StreamSnapshots(ctx, keys, windowFromProto(req.Msg.Window), stepFromProto(req.Msg.Step))
	if err != nil {
		return connectError(err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case snap, ok := <-ch:
			if !ok {
				return nil
			}
			msg := &adminmonitorv1.WatchResponse{
				Time:    snap.Time.UTC().Format(time.RFC3339Nano),
				Metrics: snapshotToProto(snap),
			}
			if err := stream.Send(msg); err != nil {
				h.logger.Info("admin_monitor watch send failed", "err", err)
				return nil
			}
		}
	}
}

func snapshotToProto(snap *domain.MetricsSnapshot) []*adminmonitorv1.MetricResult {
	out := make([]*adminmonitorv1.MetricResult, 0, len(snap.Metrics))
	for _, mr := range snap.Metrics {
		series := make([]*adminmonitorv1.Series, 0, len(mr.Series))
		for _, s := range mr.Series {
			pts := make([]*adminmonitorv1.Point, 0, len(s.Points))
			for _, p := range s.Points {
				pts = append(pts, &adminmonitorv1.Point{Time: p.Time.UTC().Format(time.RFC3339Nano), Value: p.Value})
			}
			series = append(series, &adminmonitorv1.Series{Labels: s.Labels, Points: pts})
		}
		out = append(out, &adminmonitorv1.MetricResult{
			Key:        string(mr.Key),
			Kind:       kindToProto(mr.Kind),
			Unit:       mr.Unit,
			GrafanaUrl: mr.GrafanaURL,
			Series:     series,
			Degraded:   mr.Degraded,
			Reason:     mr.Reason,
			Warnings:   mr.Warnings,
		})
	}
	return out
}

func toKeys(in []string) []domain.MetricKey {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.MetricKey, 0, len(in))
	for _, s := range in {
		out = append(out, domain.MetricKey(s))
	}
	return out
}

func windowFromProto(w adminmonitorv1.RangeWindow) domain.RangeWindow {
	switch w {
	case adminmonitorv1.RangeWindow_RANGE_WINDOW_5M:
		return domain.RangeWindow5m
	case adminmonitorv1.RangeWindow_RANGE_WINDOW_15M:
		return domain.RangeWindow15m
	case adminmonitorv1.RangeWindow_RANGE_WINDOW_1H:
		return domain.RangeWindow1h
	case adminmonitorv1.RangeWindow_RANGE_WINDOW_6H:
		return domain.RangeWindow6h
	case adminmonitorv1.RangeWindow_RANGE_WINDOW_24H:
		return domain.RangeWindow24h
	}
	return ""
}

func stepFromProto(s adminmonitorv1.Step) domain.Step {
	switch s {
	case adminmonitorv1.Step_STEP_15S:
		return domain.Step15s
	case adminmonitorv1.Step_STEP_30S:
		return domain.Step30s
	case adminmonitorv1.Step_STEP_1M:
		return domain.Step1m
	case adminmonitorv1.Step_STEP_5M:
		return domain.Step5m
	}
	return ""
}

func kindToProto(k domain.SeriesKind) adminmonitorv1.SeriesKind {
	switch k {
	case domain.SeriesKindInstant:
		return adminmonitorv1.SeriesKind_SERIES_KIND_INSTANT
	case domain.SeriesKindRange:
		return adminmonitorv1.SeriesKind_SERIES_KIND_RANGE
	}
	return adminmonitorv1.SeriesKind_SERIES_KIND_UNSPECIFIED
}

// connectError translates usecase errors to Connect codes.
// - input validation (unknown metric, bad window/step) -> CodeInvalidArgument
// - other errors -> CodeInternal
func connectError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	if strings.Contains(low, "unknown metric") || strings.Contains(low, "invalid window") || strings.Contains(low, "invalid step") || strings.Contains(low, "keys required") || strings.Contains(low, "window/step ratio") {
		return connect.NewError(connect.CodeInvalidArgument, errors.New(msg))
	}
	return connect.NewError(connect.CodeInternal, errors.New(msg))
}
