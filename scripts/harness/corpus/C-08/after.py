"""BUG: top-level import combined with raw sys.modules mutation creates order dependence."""

import importlib
import sys
from unittest.mock import MagicMock

import auth_service
import pytest


def test_replace_auth_service_module() -> None:
    # BUG: direct mutation leaks across tests and breaks importlib.reload on stale bindings.
    sys.modules["auth_service"] = MagicMock()


def test_auth_service_wires_strict_from_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "true")
    importlib.reload(auth_service)
    assert auth_service is not None
