"""Atomic cert/key install: 0444/0400, no temps left, missing is distinct."""

from __future__ import annotations

from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID

from news_creator.infra.pki.certfile import CertFile, CertNotFoundError, CertParseError


def new_self_signed_pem(cn: str, nb: datetime, na: datetime) -> tuple[bytes, bytes]:
    key = ec.generate_private_key(ec.SECP256R1())
    cert = (
        x509.CertificateBuilder()
        .subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, cn)]))
        .issuer_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, cn)]))
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(nb)
        .not_valid_after(na)
        .add_extension(x509.SubjectAlternativeName([x509.DNSName(cn)]), critical=False)
        .sign(key, hashes.SHA256())
    )
    cert_pem = cert.public_bytes(serialization.Encoding.PEM)
    key_pem = key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.TraditionalOpenSSL,
        encryption_algorithm=serialization.NoEncryption(),
    )
    return cert_pem, key_pem


def test_certfile_write_and_load(tmp_path: Path) -> None:
    cf = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    nb = datetime.now(UTC) - timedelta(minutes=1)
    cert, key = new_self_signed_pem("news-creator", nb, nb + timedelta(hours=1))
    cf.write(cert, key)
    assert (tmp_path / "svc-cert.pem").stat().st_mode & 0o777 == 0o444
    assert (tmp_path / "svc-key.pem").stat().st_mode & 0o777 == 0o400
    loaded = cf.load()
    assert loaded.common_name == "news-creator"


def test_certfile_load_missing(tmp_path: Path) -> None:
    cf = CertFile(str(tmp_path / "absent.pem"), str(tmp_path / "absent.key"))
    with pytest.raises(CertNotFoundError):
        cf.load()


def test_certfile_write_leaves_no_temp_on_success(tmp_path: Path) -> None:
    cf = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    nb = datetime.now(UTC)
    cert, key = new_self_signed_pem("news-creator", nb, nb + timedelta(hours=1))
    cf.write(cert, key)
    leftovers = [
        p.name
        for p in tmp_path.iterdir()
        if p.name.startswith(".pki-enroll-")
        or p.name.startswith(".pki-install")
        or p.name.endswith(".pki-bak")
    ]
    assert leftovers == []


class _SimulatedCrash(Exception):
    """Stands in for a process crash between dest key and dest cert rename."""


def test_certfile_load_pair_mismatch_is_corrupt(tmp_path: Path) -> None:
    cf = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    nb = datetime.now(UTC) - timedelta(minutes=1)
    cert, key = new_self_signed_pem("news-creator", nb, nb + timedelta(hours=1))
    cf.write(cert, key)
    _, other_key = new_self_signed_pem("other", nb, nb + timedelta(hours=1))
    key_path = tmp_path / "svc-key.pem"
    key_path.chmod(0o600)
    key_path.write_bytes(other_key)
    key_path.chmod(0o400)
    with pytest.raises(CertParseError, match="pair"):
        cf.load()


def test_certfile_second_rename_failure_restores_old_key(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    cf = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    nb = datetime.now(UTC) - timedelta(minutes=1)
    old_cert, old_key = new_self_signed_pem("old-leaf", nb, nb + timedelta(hours=1))
    cf.write(old_cert, old_key)
    new_cert, new_key = new_self_signed_pem("new-leaf", nb, nb + timedelta(hours=2))
    orig = Path.replace

    def boom(self: Path, target: Path | str, *args: object, **kwargs: object) -> Path:
        dest = Path(target)
        if dest == Path(cf.cert_path) and self.name.startswith(".pki-enroll-"):
            raise OSError("injected second rename failure")
        return orig(self, dest, *args, **kwargs)

    monkeypatch.setattr(Path, "replace", boom)
    with pytest.raises(OSError, match="injected second rename failure"):
        cf.write(new_cert, new_key)
    loaded = CertFile(str(cf.cert_path), str(cf.key_path)).load()
    assert loaded.common_name == "old-leaf"
    assert Path(cf.key_path).read_bytes() == old_key
    assert Path(cf.cert_path).read_bytes() == old_cert
    leftovers = [
        p.name for p in tmp_path.iterdir() if p.name.startswith(".pki-enroll-")
    ]
    assert leftovers == []


def test_certfile_crash_between_renames_recovers_on_restart(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    cf = CertFile(str(tmp_path / "svc-cert.pem"), str(tmp_path / "svc-key.pem"))
    nb = datetime.now(UTC) - timedelta(minutes=1)
    old_cert, old_key = new_self_signed_pem("old-leaf", nb, nb + timedelta(hours=1))
    cf.write(old_cert, old_key)
    new_cert, new_key = new_self_signed_pem("new-leaf", nb, nb + timedelta(hours=2))
    orig = Path.replace

    def crash_after_key(
        self: Path, target: Path | str, *args: object, **kwargs: object
    ) -> Path:
        dest = Path(target)
        result = orig(self, dest, *args, **kwargs)
        if dest == Path(cf.key_path) and self.name.startswith(".pki-enroll-"):
            raise _SimulatedCrash("crash after key rename")
        return result

    monkeypatch.setattr(Path, "replace", crash_after_key)
    with pytest.raises(_SimulatedCrash):
        cf.write(new_cert, new_key)
    monkeypatch.setattr(Path, "replace", orig)
    restarted = CertFile(str(cf.cert_path), str(cf.key_path))
    loaded = restarted.load()
    assert loaded.common_name == "old-leaf"
    assert Path(cf.key_path).read_bytes() == old_key
    assert Path(cf.cert_path).read_bytes() == old_cert
