"""Atomic cert/key install with journal recovery and pair checks."""

# ruff: noqa: TRY003

from __future__ import annotations

import json
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from cryptography import x509
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat

from tag_generator.infra.pki import certfile as certfile_mod
from tag_generator.infra.pki.certfile import (
    JOURNAL_NAME,
    CertFile,
    CertNotFoundError,
    CertPairMismatchError,
    CertParseFailedError,
)
from tests.unit.infra.pki.helpers import self_signed_pem

SUBJECT = "tag-generator"
NB = datetime.now(tz=UTC) - timedelta(minutes=1)
NA = NB + timedelta(hours=1)


def _spki(cert_pem: bytes) -> bytes:
    cert = x509.load_pem_x509_certificate(cert_pem)
    return cert.public_key().public_bytes(Encoding.DER, PublicFormat.SubjectPublicKeyInfo)


def test_certfile_write_and_load(tmp_path: Path) -> None:
    files = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    cert, key = self_signed_pem(SUBJECT, NB, NA)
    files.write(cert, key)
    assert files.cert_path.stat().st_mode & 0o777 == 0o444
    assert files.key_path.stat().st_mode & 0o777 == 0o400
    loaded = files.load()
    assert loaded.subject.rfc4514_string() == f"CN={SUBJECT}"


def test_certfile_load_missing(tmp_path: Path) -> None:
    files = CertFile(str(tmp_path / "absent.pem"), str(tmp_path / "absent.key"))
    with pytest.raises(CertNotFoundError):
        files.load()


def test_certfile_write_leaves_no_temp_or_journal_on_success(tmp_path: Path) -> None:
    files = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    cert, key = self_signed_pem(SUBJECT, NB, NA)
    files.write(cert, key)
    leftovers = [p.name for p in tmp_path.iterdir() if p.name.startswith(".pki-enroll-") or p.name.endswith(".pki-bak")]
    assert leftovers == []
    assert not (tmp_path / JOURNAL_NAME).exists()


def test_certfile_load_pair_mismatch_is_corrupt(tmp_path: Path) -> None:
    files = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    cert_a, key_a = self_signed_pem(SUBJECT, NB, NA)
    _cert_b, key_b = self_signed_pem("other", NB, NA)
    files.write(cert_a, key_a)
    files.key_path.chmod(0o600)
    files.key_path.write_bytes(key_b)
    files.key_path.chmod(0o400)
    with pytest.raises(CertPairMismatchError):
        files.load()
    with pytest.raises(CertParseFailedError):
        files.load()


def test_write_rolls_back_when_cert_rename_fails(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    files = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    old_cert, old_key = self_signed_pem(SUBJECT, NB, NA)
    new_cert, new_key = self_signed_pem(SUBJECT, NB, NA + timedelta(hours=1))
    files.write(old_cert, old_key)
    real = certfile_mod.atomic_replace

    def _boom(src: Path, dst: Path) -> None:
        if dst == files.cert_path and src.name.startswith(".pki-enroll-"):
            raise OSError("injected cert rename failure")
        real(src, dst)

    monkeypatch.setattr(certfile_mod, "atomic_replace", _boom)
    with pytest.raises(OSError, match="injected"):
        files.write(new_cert, new_key)
    loaded = files.load()
    assert _spki(loaded.public_bytes(Encoding.PEM)) == _spki(old_cert)


def test_restart_load_recovers_crash_between_renames(tmp_path: Path) -> None:
    files = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    old_cert, old_key = self_signed_pem(SUBJECT, NB, NA)
    _new_cert, new_key = self_signed_pem(SUBJECT, NB, NA + timedelta(hours=1))
    files.write(old_cert, old_key)
    bak_cert = certfile_mod.backup_path(files.cert_path)
    bak_key = certfile_mod.backup_path(files.key_path)
    bak_cert.write_bytes(old_cert)
    bak_key.write_bytes(old_key)
    files.key_path.chmod(0o600)
    files.key_path.write_bytes(new_key)
    files.key_path.chmod(0o400)
    (tmp_path / JOURNAL_NAME).write_text(
        json.dumps({"v": 1, "phase": "key_installed", "had_previous": True}),
        encoding="utf-8",
    )
    restarted = CertFile(str(files.cert_path), str(files.key_path))
    loaded = restarted.load()
    assert _spki(loaded.public_bytes(Encoding.PEM)) == _spki(old_cert)
    assert not (tmp_path / JOURNAL_NAME).exists()


def test_restart_load_commits_when_both_renames_succeeded(tmp_path: Path) -> None:
    files = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    old_cert, old_key = self_signed_pem(SUBJECT, NB, NA)
    new_cert, new_key = self_signed_pem(SUBJECT, NB, NA + timedelta(hours=1))
    files.write(new_cert, new_key)
    bak_cert = certfile_mod.backup_path(files.cert_path)
    bak_key = certfile_mod.backup_path(files.key_path)
    bak_cert.write_bytes(old_cert)
    bak_key.write_bytes(old_key)
    (tmp_path / JOURNAL_NAME).write_text(
        json.dumps({"v": 1, "phase": "key_installed", "had_previous": True}),
        encoding="utf-8",
    )
    restarted = CertFile(str(files.cert_path), str(files.key_path))
    loaded = restarted.load()
    assert _spki(loaded.public_bytes(Encoding.PEM)) == _spki(new_cert)
    assert not bak_cert.exists()
    assert not bak_key.exists()
    assert not (tmp_path / JOURNAL_NAME).exists()
