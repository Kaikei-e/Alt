"""RED test: ``predict_batch`` must export the chosen ``top_genre`` as a Prometheus Counter.

Why this metric exists: the 2026-04-14..2026-04-27 ``no_evidence`` regression
took 13 days to surface because the classifier output distribution was never
visible in operator dashboards. ``recap_outputs`` only stores genres that
*passed* the full pipeline, so when classification collapsed to two top-1
classes (``consumer_tech``, ``politics_government``) the metric system showed
nothing wrong at the classifier layer.

Counter (rather than Histogram) is enough here: cardinality is bounded by the
30 canonical genres, and Grafana can compute the active-genre count over a
1-hour window with ``count(rate(classifier_top_genre_total[1h]) > 0)``.
"""

from __future__ import annotations

import gc
import importlib

import pytest


def _fresh_telemetry():
    """Return a freshly imported telemetry module so we own the registry."""
    # prometheus_client uses a module-level registry; reload to avoid label
    # bleed between tests within the same process.
    gc.collect()
    return importlib.import_module("recap_subworker.infra.telemetry")


def test_classifier_top_genre_total_counter_is_exported() -> None:
    """``CLASSIFIER_TOP_GENRE_TOTAL`` must exist in telemetry as a labelled Counter."""
    telemetry = _fresh_telemetry()
    assert hasattr(telemetry, "CLASSIFIER_TOP_GENRE_TOTAL"), (
        "telemetry module must expose CLASSIFIER_TOP_GENRE_TOTAL — see ADR-000835 "
        "stage 3 observability addendum."
    )
    counter = telemetry.CLASSIFIER_TOP_GENRE_TOTAL
    # prometheus_client.Counter._name and _labelnames are private but stable.
    assert counter._name == "recap_subworker_classifier_top_genre"
    assert counter._labelnames == ("genre",)


def test_predict_batch_increments_top_genre_counter() -> None:
    """One increment per classified text, labelled by chosen top_genre."""
    telemetry = _fresh_telemetry()
    counter = telemetry.CLASSIFIER_TOP_GENRE_TOTAL

    # Snapshot the pre-existing value so the test is order-independent within
    # the registry's lifetime.
    def _value(label: str) -> float:
        return counter.labels(genre=label)._value.get()  # type: ignore[attr-defined]

    pre_a = _value("ai_data")
    pre_b = _value("politics_government")

    from recap_subworker.services.classifier import GenreClassifierService

    # Build a stub GenreClassifierService instance via __new__ to avoid the
    # heavy embedder/joblib load in unit context. Then exercise the post-
    # prediction emit path directly.
    service = GenreClassifierService.__new__(GenreClassifierService)

    # The implementation under test should expose a small helper or inline emit
    # that increments ``CLASSIFIER_TOP_GENRE_TOTAL`` for each result entry. We
    # assert via the public API by feeding fabricated results through whichever
    # private method the impl wires up. We tolerate both shapes:
    #
    #   1) ``service._record_top_genres([{"top_genre": ...}, ...])`` helper
    #   2) An inline loop in ``predict_batch`` that emits each entry
    #
    # Helper-shape is preferred (see bp-python: small composable units).
    assert hasattr(service, "_record_top_genres"), (
        "GenreClassifierService must expose _record_top_genres(results) so the "
        "classifier observability is testable without embedder / joblib load."
    )

    service._record_top_genres(  # type: ignore[attr-defined]
        [
            {"top_genre": "ai_data"},
            {"top_genre": "ai_data"},
            {"top_genre": "politics_government"},
        ]
    )

    assert _value("ai_data") == pytest.approx(pre_a + 2)
    assert _value("politics_government") == pytest.approx(pre_b + 1)


def test_predict_batch_skips_emit_for_missing_top_genre() -> None:
    """Defensive: missing or empty ``top_genre`` must not raise / leak labels."""
    telemetry = _fresh_telemetry()
    counter = telemetry.CLASSIFIER_TOP_GENRE_TOTAL

    from recap_subworker.services.classifier import GenreClassifierService

    service = GenreClassifierService.__new__(GenreClassifierService)
    pre_unknown = counter.labels(genre="unknown")._value.get()  # type: ignore[attr-defined]
    pre_empty = counter.labels(genre="")._value.get()  # type: ignore[attr-defined]

    service._record_top_genres(  # type: ignore[attr-defined]
        [
            {"top_genre": ""},
            {"confidence": 0.1},  # missing key entirely
        ]
    )

    # No new labels for "unknown" or "". The impl drops the entry rather than
    # creating empty / catch-all labels.
    assert counter.labels(genre="unknown")._value.get() == pre_unknown  # type: ignore[attr-defined]
    assert counter.labels(genre="")._value.get() == pre_empty  # type: ignore[attr-defined]
