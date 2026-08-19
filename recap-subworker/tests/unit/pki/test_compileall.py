"""PKI modules must compile and import in disabled mode (service startup)."""

from __future__ import annotations

import compileall
import importlib
import logging
import re
from pathlib import Path

from recap_subworker.app.infra.pki.config import MODE_DISABLED
from recap_subworker.app.infra.pki.start import start

ROOT = Path(__file__).resolve().parents[3]
PKI_DIR = ROOT / "recap_subworker" / "app" / "infra" / "pki"
INBOUND_TLS = ROOT / "recap_subworker" / "app" / "infra" / "inbound_tls.py"
INBOUND_SERVER = ROOT / "recap_subworker" / "app" / "infra" / "inbound_server.py"
INBOUND_TLS_TEST = ROOT / "tests" / "unit" / "test_inbound_tls_hot_reload.py"
_UNPARENTHESIZED_EXCEPT = re.compile(
    r"^\s*except\s+(?!\()([\w.]+\s*,\s*)+[\w.]+",
    re.MULTILINE,
)


def _portable_py_roots() -> list[Path]:
    return [
        PKI_DIR,
        INBOUND_TLS,
        INBOUND_SERVER,
        INBOUND_TLS_TEST,
        ROOT / "tests" / "unit" / "pki",
    ]


def test_pki_package_compileall() -> None:
    ok = compileall.compile_dir(str(PKI_DIR), quiet=1, force=True)
    assert ok is True
    assert compileall.compile_file(str(INBOUND_TLS), quiet=1, force=True) is True
    assert compileall.compile_file(str(INBOUND_SERVER), quiet=1, force=True) is True
    assert compileall.compile_file(str(INBOUND_TLS_TEST), quiet=1, force=True) is True


def test_pki_and_inbound_tls_except_syntax_is_313_portable() -> None:
    offenders: list[str] = []
    for root in _portable_py_roots():
        paths = [root] if root.is_file() else sorted(root.rglob("*.py"))
        for path in paths:
            if "health_deep" in path.parts:
                continue
            text = path.read_text(encoding="utf-8")
            if _UNPARENTHESIZED_EXCEPT.search(text):
                offenders.append(str(path.relative_to(ROOT)))
    assert offenders == []


def test_disabled_mode_imports_and_starts(monkeypatch, caplog) -> None:
    monkeypatch.delenv("PKI_ENROLLMENT", raising=False)
    monkeypatch.delenv("PKI_ENROLLMENT_FILE", raising=False)
    importlib.invalidate_caches()
    inbound = importlib.import_module("recap_subworker.app.infra.inbound_server")
    manager = importlib.import_module("recap_subworker.app.infra.pki.manager")
    issuer = importlib.import_module("recap_subworker.app.infra.pki.native_issuer")
    assert inbound.serve_app is not None
    assert manager.Manager is not None
    assert issuer.NativeStepCAIssuer is not None
    caplog.set_level(logging.INFO)
    handle = start("recap-subworker")
    try:
        assert "pki_enrollment_disabled" in caplog.text
        assert MODE_DISABLED
    finally:
        if handle is not None:
            handle.stop()
