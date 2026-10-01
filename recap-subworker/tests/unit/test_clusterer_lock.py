"""Tests for native clustering lock serialization in Clusterer."""

from __future__ import annotations

import threading
import time
from unittest.mock import patch

import numpy as np

from recap_subworker.infra.config import Settings
from recap_subworker.services import clusterer as clusterer_module
from recap_subworker.services.clusterer import Clusterer


def test_native_clustering_lock_exists():
    """Module-level _NATIVE_CLUSTERING_LOCK must exist and be a threading.Lock."""
    assert hasattr(clusterer_module, "_NATIVE_CLUSTERING_LOCK"), (
        "clusterer module must define _NATIVE_CLUSTERING_LOCK"
    )
    assert isinstance(clusterer_module._NATIVE_CLUSTERING_LOCK, type(threading.Lock()))


def test_concurrent_clustering_calls_never_overlap():
    """Two concurrent calls through the lock-guarded section must never overlap."""
    settings = Settings()
    settings.enable_umap_force = False
    settings.enable_umap_auto = False
    settings.noise_recluster_enabled = False
    clusterer = Clusterer(settings)

    embeddings = np.random.rand(20, 5)

    active_count = 0
    max_concurrent = 0
    lock_stats = threading.Lock()

    def fake_run_with_timeout(func, timeout_seconds, *args, **kwargs):
        nonlocal active_count, max_concurrent
        with lock_stats:
            active_count += 1
            max_concurrent = max(max_concurrent, active_count)
        time.sleep(0.1)
        with lock_stats:
            active_count -= 1
        return np.zeros(20, dtype=int), np.ones(20, dtype=float)

    with patch.object(clusterer, "_run_with_timeout", side_effect=fake_run_with_timeout):
        with patch.object(clusterer, "_calculate_dbcv", return_value=0.5):
            with patch.object(clusterer, "_calculate_silhouette", return_value=0.5):
                t1 = threading.Thread(
                    target=clusterer.cluster,
                    args=(embeddings,),
                    kwargs={"min_cluster_size": 2, "min_samples": 1},
                )
                t2 = threading.Thread(
                    target=clusterer.cluster,
                    args=(embeddings,),
                    kwargs={"min_cluster_size": 2, "min_samples": 1},
                )
                t1.start()
                t2.start()
                t1.join()
                t2.join()

    assert max_concurrent == 1, (
        f"Native clustering calls overlapped! max_concurrent was {max_concurrent}"
    )


def test_timed_out_guarded_call_releases_lock():
    """A timed-out guarded call releases the lock so the next call proceeds."""
    settings = Settings()
    settings.enable_umap_force = False
    settings.enable_umap_auto = False
    settings.noise_recluster_enabled = False
    settings.hdbscan_timeout_seconds = 1
    clusterer = Clusterer(settings)

    embeddings = np.random.rand(20, 5)

    # Simulate timeout in call 1 (returns None from _run_with_timeout, triggers fallback)
    with patch.object(clusterer, "_run_with_timeout", return_value=None):
        with patch.object(
            clusterer,
            "_fallback_minibatch_kmeans",
            return_value=(np.zeros(20, dtype=int), np.ones(20, dtype=float)),
        ):
            with patch.object(clusterer, "_calculate_dbcv", return_value=None):
                with patch.object(clusterer, "_calculate_silhouette", return_value=None):
                    res1 = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)
                    assert res1.used_fallback is True

    # Lock must be released immediately after call 1 finishes
    lock = getattr(clusterer_module, "_NATIVE_CLUSTERING_LOCK", None)
    assert lock is not None, "_NATIVE_CLUSTERING_LOCK must be defined"
    assert not lock.locked(), "Lock remained acquired after timed-out call"

    # Call 2 proceeds and succeeds without blocking
    with patch.object(
        clusterer,
        "_run_with_timeout",
        return_value=(np.zeros(20, dtype=int), np.ones(20, dtype=float)),
    ):
        with patch.object(clusterer, "_calculate_dbcv", return_value=0.5):
            with patch.object(clusterer, "_calculate_silhouette", return_value=0.5):
                res2 = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)
                assert res2.used_fallback is False


def test_lock_wait_logging():
    """Log at debug when waiting for lock, and at info if wait exceeds 1 s."""
    settings = Settings()
    settings.enable_umap_force = False
    settings.enable_umap_auto = False
    settings.noise_recluster_enabled = False
    clusterer = Clusterer(settings)

    embeddings = np.random.rand(20, 5)

    lock = getattr(clusterer_module, "_NATIVE_CLUSTERING_LOCK", None)
    assert lock is not None, "_NATIVE_CLUSTERING_LOCK must be defined"

    # Acquire lock in main thread, simulate a run that waits in a second thread
    lock.acquire()
    thread_finished = threading.Event()

    def run_cluster():
        with patch.object(
            clusterer,
            "_run_with_timeout",
            return_value=(np.zeros(20, dtype=int), np.ones(20, dtype=float)),
        ):
            with patch.object(clusterer, "_calculate_dbcv", return_value=0.5):
                with patch.object(clusterer, "_calculate_silhouette", return_value=0.5):
                    clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)
        thread_finished.set()

    with (
        patch.object(clusterer_module._LOGGER, "debug") as mock_debug,
        patch.object(clusterer_module._LOGGER, "info") as mock_info,
    ):
        calls = []
        real_monotonic = time.monotonic

        def fake_monotonic():
            val = real_monotonic() + len(calls) * 1.5
            calls.append(val)
            return val

        with patch.object(clusterer_module.time, "monotonic", side_effect=fake_monotonic):
            t = threading.Thread(target=run_cluster)
            t.start()
            time.sleep(0.05)
            # Release lock so thread can acquire it
            lock.release()
            thread_finished.wait(timeout=2.0)
            t.join()

        mock_debug.assert_called()
        mock_info.assert_called()


def test_abandoned_hdbscan_forces_fallback_until_completed():
    """While an abandoned HDBSCAN future runs, next calls skip HDBSCAN and use fallback."""
    settings = Settings()
    settings.enable_umap_force = False
    settings.enable_umap_auto = False
    settings.noise_recluster_enabled = False
    settings.hdbscan_timeout_seconds = 1
    clusterer = Clusterer(settings)

    embeddings = np.random.rand(20, 5)

    hdbscan_call_count = 0
    fake_running_event = threading.Event()
    fake_proceed_event = threading.Event()

    class FakeHDBSCAN:
        def __init__(self, **kwargs):
            pass

        def fit(self, X):
            nonlocal hdbscan_call_count
            hdbscan_call_count += 1
            fake_running_event.set()
            fake_proceed_event.wait()
            self.labels_ = np.zeros(len(X), dtype=int)
            self.probabilities_ = np.ones(len(X), dtype=float)
            return self

    with (
        patch("recap_subworker.services.clusterer.HDBSCAN", FakeHDBSCAN),
        patch.object(clusterer, "_calculate_dbcv", return_value=None),
        patch.object(clusterer, "_calculate_silhouette", return_value=None),
        patch.object(
            clusterer,
            "_fallback_minibatch_kmeans",
            return_value=(np.zeros(20, dtype=int), np.ones(20, dtype=float)),
        ) as mock_fallback,
        patch.object(clusterer_module._LOGGER, "warning") as mock_warning,
    ):
        try:
            # Call 1: HDBSCAN runs in background thread, times out
            res1 = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)
            assert res1.used_fallback is True
            assert hdbscan_call_count == 1
            assert fake_running_event.is_set()

            # Call 2: Abandoned HDBSCAN is still running. Next call must skip HDBSCAN
            res2 = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)
            assert res2.used_fallback is True
            assert hdbscan_call_count == 1, (
                "HDBSCAN was invoked while abandoned HDBSCAN future was still running"
            )
            assert mock_fallback.call_count == 2
            warning_calls = [
                c
                for c in mock_warning.call_args_list
                if c.kwargs.get("reason") == "abandoned_hdbscan_still_running"
            ]
            assert len(warning_calls) == 1

            # Finish the abandoned HDBSCAN future
            fake_proceed_event.set()
            time.sleep(0.05)

            # Call 3: Now that abandoned HDBSCAN is done, HDBSCAN is used again
            res3 = clusterer.cluster(embeddings, min_cluster_size=2, min_samples=1)
            assert res3.used_fallback is False
            assert hdbscan_call_count == 2
        finally:
            fake_proceed_event.set()


def test_abandoned_future_cleared_by_done_callback():
    """Setting an abandoned future marks it running; done-callback clears it."""
    import concurrent.futures

    executor = concurrent.futures.ThreadPoolExecutor(max_workers=1)
    proceed = threading.Event()

    def task():
        proceed.wait()
        return "done"

    future = executor.submit(task)
    try:
        clusterer_module._set_abandoned_hdbscan_future(future)
        assert clusterer_module._is_abandoned_hdbscan_running() is True
        assert clusterer_module._ABANDONED_HDBSCAN_FUTURE is future

        # Trigger completion
        proceed.set()
        future.result(timeout=1.0)
        time.sleep(0.02)

        assert clusterer_module._is_abandoned_hdbscan_running() is False
        assert clusterer_module._ABANDONED_HDBSCAN_FUTURE is None
    finally:
        proceed.set()
        executor.shutdown(wait=True)


def test_abandoned_future_tracked_for_differently_named_function():
    """_run_with_timeout tracks abandoned future even when func.__name__ does not contain 'hdbscan'."""
    settings = Settings()
    clusterer = Clusterer(settings)

    proceed_event = threading.Event()

    def custom_algorithm_worker():
        proceed_event.wait(timeout=5.0)
        return np.zeros(10, dtype=int), np.ones(10, dtype=float)

    try:
        res = clusterer._run_with_timeout(
            custom_algorithm_worker, timeout_seconds=0.05, track_abandoned=True
        )
        assert res is None
        assert clusterer_module._is_abandoned_hdbscan_running() is True
    finally:
        proceed_event.set()
        time.sleep(0.05)
