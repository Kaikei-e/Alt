"""Enrollment must complete before the in-process :9443 listener binds."""

from __future__ import annotations

import pytest

from recap_subworker.app.infra import inbound_server


def test_serve_app_enrolls_before_tls(monkeypatch: pytest.MonkeyPatch) -> None:
    order: list[str] = []

    def fake_start(name: str) -> None:
        assert name == "recap-subworker"
        order.append("pki")

    def fake_after(**kwargs: object) -> None:
        del kwargs
        order.append("tls")

    monkeypatch.setattr(inbound_server, "start_pki_enrollment", fake_start)
    monkeypatch.setattr(inbound_server, "_serve_after_enrollment", fake_after)
    inbound_server.serve_app(host="127.0.0.1", port=0, log_level="info")
    assert order == ["pki", "tls"]


def test_serve_app_fail_fast_skips_listener(monkeypatch: pytest.MonkeyPatch) -> None:
    tls_called = False

    def fake_start(name: str) -> None:
        del name
        raise RuntimeError("pki: enroll failed after 1 attempts")

    def fake_after(**kwargs: object) -> None:
        del kwargs
        nonlocal tls_called
        tls_called = True

    monkeypatch.setattr(inbound_server, "start_pki_enrollment", fake_start)
    monkeypatch.setattr(inbound_server, "_serve_after_enrollment", fake_after)
    with pytest.raises(RuntimeError, match="enroll failed"):
        inbound_server.serve_app(host="127.0.0.1", port=0, log_level="info")
    assert tls_called is False
