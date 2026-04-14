"""Tests for dispatch_metrics — distributed BE observability instruments."""

import pytest
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import InMemoryMetricReader
from opentelemetry.sdk.metrics.view import (
    ExplicitBucketHistogramAggregation,
    View,
)


DURATION_BUCKETS = [0.5, 1, 2, 5, 10, 20, 30, 60, 120, 180, 300]


@pytest.fixture
def metrics_reader():
    from news_creator.gateway import dispatch_metrics

    reader = InMemoryMetricReader()
    view = View(
        instrument_name="newscreator.distributed_be.request.duration",
        aggregation=ExplicitBucketHistogramAggregation(DURATION_BUCKETS),
    )
    provider = MeterProvider(metric_readers=[reader], views=[view])
    meter = provider.get_meter("news_creator.distributed_be")
    dispatch_metrics.reset_metrics_for_tests(meter)
    yield reader
    dispatch_metrics.reset_metrics_for_tests(None)
    provider.shutdown()


def _collect(reader: InMemoryMetricReader) -> dict[str, list]:
    data = reader.get_metrics_data()
    result: dict[str, list] = {}
    if data is None:
        return result
    for rm in data.resource_metrics:
        for sm in rm.scope_metrics:
            for metric in sm.metrics:
                result.setdefault(metric.name, []).extend(metric.data.data_points)
    return result


def _points_for(reader: InMemoryMetricReader, name: str):
    return _collect(reader).get(name, [])


def _find(points, **attrs):
    matches = [
        p
        for p in points
        if all(p.attributes.get(k) == v for k, v in attrs.items())
    ]
    return matches


class TestDispatchContext:
    @pytest.mark.asyncio
    async def test_success_increments_dispatches_and_records_duration(self, metrics_reader):
        from news_creator.gateway import dispatch_metrics

        async with dispatch_metrics.dispatch_context(
            remote_url="http://100.74.178.93:11434",
            model="gemma4-e4b-q4km",
        ) as obs:
            obs.set_outcome("success")

        dispatches = _find(
            _points_for(metrics_reader, "newscreator.distributed_be.dispatches"),
            remote_url="http://100.74.178.93:11434",
            model="gemma4-e4b-q4km",
            outcome="success",
        )
        assert len(dispatches) == 1
        assert dispatches[0].value == 1

        duration_points = _find(
            _points_for(metrics_reader, "newscreator.distributed_be.request.duration"),
            remote_url="http://100.74.178.93:11434",
            outcome="success",
        )
        assert len(duration_points) == 1
        assert duration_points[0].count == 1
        assert list(duration_points[0].explicit_bounds) == DURATION_BUCKETS

    @pytest.mark.asyncio
    async def test_inflight_gauge_tracks_enter_exit(self, metrics_reader):
        from news_creator.gateway import dispatch_metrics

        async with dispatch_metrics.dispatch_context(
            remote_url="http://remote-a:11434",
            model="gemma4-e4b-q4km",
        ) as obs:
            # Inside the context: inflight should be 1 for this remote
            mid = _find(
                _points_for(metrics_reader, "newscreator.distributed_be.inflight"),
                remote_url="http://remote-a:11434",
            )
            assert len(mid) == 1
            assert mid[0].value == 1
            obs.set_outcome("success")

        # After exit: inflight drops back to 0
        end = _find(
            _points_for(metrics_reader, "newscreator.distributed_be.inflight"),
            remote_url="http://remote-a:11434",
        )
        assert len(end) == 1
        assert end[0].value == 0

    @pytest.mark.asyncio
    async def test_exception_inside_context_records_failure_and_decrements_inflight(
        self, metrics_reader
    ):
        from news_creator.gateway import dispatch_metrics

        with pytest.raises(RuntimeError, match="boom"):
            async with dispatch_metrics.dispatch_context(
                remote_url="http://remote-a:11434",
                model="gemma4-e4b-q4km",
            ):
                raise RuntimeError("boom")

        # inflight must be back to 0 despite exception
        inflight = _find(
            _points_for(metrics_reader, "newscreator.distributed_be.inflight"),
            remote_url="http://remote-a:11434",
        )
        assert len(inflight) == 1
        assert inflight[0].value == 0

        # default outcome when exception propagates is "failure"
        dispatches = _find(
            _points_for(metrics_reader, "newscreator.distributed_be.dispatches"),
            remote_url="http://remote-a:11434",
            outcome="failure",
        )
        assert len(dispatches) == 1
        assert dispatches[0].value == 1

    @pytest.mark.asyncio
    async def test_outcome_defaults_to_failure_when_not_set_and_no_exception(
        self, metrics_reader
    ):
        from news_creator.gateway import dispatch_metrics

        async with dispatch_metrics.dispatch_context(
            remote_url="http://remote-a:11434",
            model="gemma4-e4b-q4km",
        ):
            pass  # caller forgot to set_outcome

        dispatches = _find(
            _points_for(metrics_reader, "newscreator.distributed_be.dispatches"),
            remote_url="http://remote-a:11434",
            outcome="failure",
        )
        assert len(dispatches) == 1
        assert dispatches[0].value == 1

    @pytest.mark.asyncio
    async def test_timeout_outcome_label(self, metrics_reader):
        from news_creator.gateway import dispatch_metrics

        async with dispatch_metrics.dispatch_context(
            remote_url="http://remote-a:11434",
            model="gemma4-e4b-q4km",
        ) as obs:
            obs.set_outcome("timeout")

        dispatches = _find(
            _points_for(metrics_reader, "newscreator.distributed_be.dispatches"),
            remote_url="http://remote-a:11434",
            outcome="timeout",
        )
        assert len(dispatches) == 1

    @pytest.mark.asyncio
    async def test_per_remote_per_model_labels_are_independent(self, metrics_reader):
        from news_creator.gateway import dispatch_metrics

        async with dispatch_metrics.dispatch_context(
            remote_url="http://remote-a:11434", model="gemma4-e4b-q4km"
        ) as obs:
            obs.set_outcome("success")
        async with dispatch_metrics.dispatch_context(
            remote_url="http://remote-a:11434", model="gemma4-e4b-q4km"
        ) as obs:
            obs.set_outcome("success")
        async with dispatch_metrics.dispatch_context(
            remote_url="http://remote-b:11434", model="gemma4-e4b-q4km"
        ) as obs:
            obs.set_outcome("success")

        points = _points_for(metrics_reader, "newscreator.distributed_be.dispatches")
        a = _find(points, remote_url="http://remote-a:11434", outcome="success")
        b = _find(points, remote_url="http://remote-b:11434", outcome="success")
        assert a[0].value == 2
        assert b[0].value == 1


class TestRecordFallback:
    def test_counter_increments_with_labels(self, metrics_reader):
        from news_creator.gateway import dispatch_metrics

        dispatch_metrics.record_fallback(
            from_remote_url="http://remote-a:11434",
            to="local",
            reason="exhausted",
        )

        points = _find(
            _points_for(metrics_reader, "newscreator.distributed_be.fallbacks"),
            from_remote_url="http://remote-a:11434",
            to="local",
            reason="exhausted",
        )
        assert len(points) == 1
        assert points[0].value == 1

    def test_distinct_reasons_produce_distinct_series(self, metrics_reader):
        from news_creator.gateway import dispatch_metrics

        dispatch_metrics.record_fallback(
            from_remote_url="http://remote-a:11434", to="next_remote", reason="error"
        )
        dispatch_metrics.record_fallback(
            from_remote_url="http://remote-a:11434", to="next_remote", reason="error"
        )
        dispatch_metrics.record_fallback(
            from_remote_url="http://remote-a:11434", to="local", reason="exhausted"
        )

        points = _points_for(metrics_reader, "newscreator.distributed_be.fallbacks")
        retry = _find(points, to="next_remote", reason="error")
        local = _find(points, to="local", reason="exhausted")
        assert retry[0].value == 2
        assert local[0].value == 1


class TestRecordCooldown:
    def test_counter_increments(self, metrics_reader):
        from news_creator.gateway import dispatch_metrics

        dispatch_metrics.record_cooldown(
            remote_url="http://remote-a:11434", reason="error"
        )
        dispatch_metrics.record_cooldown(
            remote_url="http://remote-a:11434", reason="error"
        )

        points = _find(
            _points_for(metrics_reader, "newscreator.distributed_be.cooldowns"),
            remote_url="http://remote-a:11434",
            reason="error",
        )
        assert points[0].value == 2
