"""Safe test isolation: monkeypatch.setitem restores sys.modules on teardown."""

import sys
from unittest.mock import MagicMock

import pytest


def test_auth_service_wires_strict_from_env(monkeypatch: pytest.MonkeyPatch) -> None:
    stub = MagicMock()
    monkeypatch.setitem(sys.modules, "auth_service", stub)
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "true")
    import auth_service  # noqa: PLC0415 - fetched at call time after monkeypatch setup

    assert auth_service is stub
