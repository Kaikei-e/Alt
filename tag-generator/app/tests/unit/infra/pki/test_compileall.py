"""PKI / inbound TLS / mTLS modules must compile on 3.13+ and import when disabled."""

from __future__ import annotations

import ast
import compileall
import importlib
import os
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

from tag_generator.infra.pki.config import MODE_DISABLED
from tag_generator.infra.pki.start import start_enrollment


class _Log:
    def __init__(self) -> None:
        self.events: list[str] = []

    def info(self, event: str, **_kwargs: object) -> None:
        self.events.append(event)

    def error(self, event: str, **_kwargs: object) -> None:
        self.events.append(event)


ROOT = Path(__file__).resolve().parents[4]
PKI_DIR = ROOT / "tag_generator" / "infra" / "pki"
INBOUND = ROOT / "tag_generator" / "infra" / "inbound_tls.py"
MTLS = ROOT / "tag_generator" / "infra" / "mtls_client.py"


def _startup_files() -> list[Path]:
    files = sorted(PKI_DIR.glob("*.py"))
    files.extend((INBOUND, MTLS))
    return files


def test_pki_inbound_mtls_compileall() -> None:
    assert compileall.compile_dir(str(PKI_DIR), quiet=1, force=True) is True
    assert compileall.compile_file(str(INBOUND), quiet=1, force=True) is True
    assert compileall.compile_file(str(MTLS), quiet=1, force=True) is True


def test_startup_modules_ast_parse_on_current_python() -> None:
    for path in _startup_files():
        ast.parse(path.read_text(encoding="utf-8"), filename=str(path))


def _python313() -> str | None:
    found = shutil.which("python3.13")
    if found:
        return found
    pyenv_root = Path(os.environ.get("PYENV_ROOT", Path.home() / ".pyenv"))
    versions = pyenv_root / "versions"
    if versions.is_dir():
        matches = sorted(versions.glob("3.13*/bin/python"))
        if matches:
            return str(matches[-1])
    return None


def test_python_313_ast_parse_and_py_compile() -> None:
    py313 = _python313()
    if py313 is None:
        pytest.skip("python3.13 is not installed")
    for path in _startup_files():
        subprocess.run(
            [
                py313,
                "-c",
                "import ast, pathlib, sys; ast.parse(pathlib.Path(sys.argv[1]).read_text(encoding='utf-8'))",
                str(path),
            ],
            check=True,
        )
        subprocess.run([py313, "-m", "py_compile", str(path)], check=True)


@pytest.mark.asyncio
async def test_disabled_mode_imports_and_starts(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("PKI_ENROLLMENT", raising=False)
    monkeypatch.delenv("PKI_ENROLLMENT_FILE", raising=False)
    importlib.invalidate_caches()
    inbound = importlib.import_module("tag_generator.infra.inbound_tls")
    mtls = importlib.import_module("tag_generator.infra.mtls_client")
    manager = importlib.import_module("tag_generator.infra.pki.manager")
    issuer = importlib.import_module("tag_generator.infra.pki.native_issuer")
    assert inbound.resolve_inbound_tls_bind is not None
    assert mtls.build_ssl_context is not None
    assert manager.Manager is not None
    assert issuer.NativeStepCAIssuer is not None
    log = _Log()
    handle = await start_enrollment("tag-generator", logger=log)
    try:
        assert handle is None
        assert "pki_enrollment_disabled" in log.events
        assert MODE_DISABLED
        assert sys.version_info >= (3, 13)
    finally:
        if handle is not None:
            await handle.aclose()


def test_pki_direct_deps_are_capped() -> None:
    text = (ROOT / "pyproject.toml").read_text(encoding="utf-8")
    assert "jwcrypto>=1.5.8,<2" in text
    assert "cryptography>=50.0.0,<51" in text
    assert "prometheus-client>=0.26.0,<1" in text
    assert "httpx>=0.28.1,<0.29" in text
