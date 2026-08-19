"""Atomic PEM leaf + key install with durable journal recovery."""

# ruff: noqa: TRY003

from __future__ import annotations

import json
import os
import ssl
import tempfile
from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives.serialization import (
    Encoding,
    PublicFormat,
    load_pem_private_key,
)

from tag_generator.infra.pki.config import PkiError
from tag_generator.infra.pki.filesafe import (
    CERT_DIR_PERM,
    MAX_ROOT_PEM_BYTES,
    assert_trusted_dest,
    mkdir_trusted,
    read_regular_no_follow,
)

JOURNAL_NAME = ".pki-pair.journal"
_BACKUP_SUFFIX = ".pki-bak"


class CertNotFoundError(PkiError):
    """Cert file absent from disk."""


class CertParseFailedError(PkiError):
    """File present but not a valid X.509 PEM."""


class CertPairMismatchError(CertParseFailedError):
    """Cert and key exist but do not form a usable TLS pair."""


def backup_path(final_path: Path) -> Path:
    return final_path.with_name(final_path.name + _BACKUP_SUFFIX)


def atomic_replace(src: Path, dst: Path) -> None:
    """Rename src onto dst. Tests inject failures here."""
    src.replace(dst)


def _fsync_dir(directory: Path) -> None:
    fd = os.open(str(directory), os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def _pair_matches(cert_pem: bytes, key_pem: bytes) -> bool:
    try:
        cert = x509.load_pem_x509_certificate(cert_pem)
        key = load_pem_private_key(key_pem, password=None)
        cert_spki = cert.public_key().public_bytes(Encoding.DER, PublicFormat.SubjectPublicKeyInfo)
        key_spki = key.public_key().public_bytes(Encoding.DER, PublicFormat.SubjectPublicKeyInfo)
    except (
        ValueError,
        TypeError,
        AttributeError,
    ):
        return False
    return cert_spki == key_spki


class CertFile:
    """Reads and atomically replaces a PEM leaf + key pair."""

    def __init__(self, cert_path: str, key_path: str) -> None:
        self.cert_path = Path(cert_path)
        self.key_path = Path(key_path)

    def _journal_path(self) -> Path:
        return self.cert_path.parent / JOURNAL_NAME

    def load(self) -> x509.Certificate:
        self.recover()
        try:
            raw = read_regular_no_follow(str(self.cert_path), MAX_ROOT_PEM_BYTES)
        except FileNotFoundError as err:
            raise CertNotFoundError("pki: cert not found") from err
        except PkiError as err:
            raise CertParseFailedError("pki: cert parse failed") from err
        try:
            key_raw = read_regular_no_follow(str(self.key_path), MAX_ROOT_PEM_BYTES)
        except FileNotFoundError as err:
            raise CertPairMismatchError("pki: cert/key pair mismatch") from err
        except PkiError as err:
            raise CertParseFailedError("pki: cert parse failed") from err
        try:
            cert = x509.load_pem_x509_certificate(raw)
        except ValueError as err:
            raise CertParseFailedError("pki: cert parse failed") from err
        if not _pair_matches(raw, key_raw):
            raise CertPairMismatchError("pki: cert/key pair mismatch")
        self._assert_ssl_pair()
        return cert

    def write(self, cert_pem: bytes, key_pem: bytes) -> None:
        """Stage both PEMs, journal the swap, rename key then cert, then pair-check."""
        self.recover()
        mkdir_trusted(str(self.cert_path.parent), CERT_DIR_PERM)
        mkdir_trusted(str(self.key_path.parent), CERT_DIR_PERM)
        assert_trusted_dest(str(self.cert_path))
        assert_trusted_dest(str(self.key_path))
        had_previous = self.cert_path.is_file() and self.key_path.is_file()
        if had_previous:
            self._backup_existing()
        self._write_journal("prepared", had_previous)
        cert_tmp = _write_temp(self.cert_path, cert_pem, 0o444)
        try:
            key_tmp = _write_temp(self.key_path, key_pem, 0o400)
        except OSError:
            cert_tmp.unlink(missing_ok=True)
            self._rollback(had_previous)
            raise
        try:
            atomic_replace(key_tmp, self.key_path)
            self._write_journal("key_installed", had_previous)
            atomic_replace(cert_tmp, self.cert_path)
        except OSError:
            cert_tmp.unlink(missing_ok=True)
            key_tmp.unlink(missing_ok=True)
            self._rollback(had_previous)
            raise
        try:
            self._assert_ssl_pair()
            if not _pair_matches(cert_pem, key_pem):
                raise CertPairMismatchError("pki: cert/key pair mismatch")
        except (
            OSError,
            ssl.SSLError,
            CertParseFailedError,
        ):
            self._rollback(had_previous)
            raise
        self._cleanup_txn()

    def recover(self) -> None:
        """Complete or roll back a write interrupted between renames."""
        journal = self._journal_path()
        if not journal.is_file():
            return
        try:
            payload = json.loads(journal.read_bytes())
        except (
            OSError,
            json.JSONDecodeError,
            UnicodeDecodeError,
        ):
            return
        if not isinstance(payload, dict):
            return
        if self._dests_pair():
            self._cleanup_txn()
            return
        if payload.get("had_previous") and self._bak_pair():
            self._restore_from_bak()
            self._cleanup_txn()
            return
        self._cleanup_txn()

    def _dests_pair(self) -> bool:
        try:
            cert_pem = read_regular_no_follow(str(self.cert_path), MAX_ROOT_PEM_BYTES)
            key_pem = read_regular_no_follow(str(self.key_path), MAX_ROOT_PEM_BYTES)
        except (
            FileNotFoundError,
            PkiError,
        ):
            return False
        return _pair_matches(cert_pem, key_pem)

    def _bak_pair(self) -> bool:
        bak_cert = backup_path(self.cert_path)
        bak_key = backup_path(self.key_path)
        try:
            cert_pem = read_regular_no_follow(str(bak_cert), MAX_ROOT_PEM_BYTES)
            key_pem = read_regular_no_follow(str(bak_key), MAX_ROOT_PEM_BYTES)
        except (
            FileNotFoundError,
            PkiError,
        ):
            return False
        return _pair_matches(cert_pem, key_pem)

    def _backup_existing(self) -> None:
        cert_pem = read_regular_no_follow(str(self.cert_path), MAX_ROOT_PEM_BYTES)
        key_pem = read_regular_no_follow(str(self.key_path), MAX_ROOT_PEM_BYTES)
        bak_cert = backup_path(self.cert_path)
        bak_key = backup_path(self.key_path)
        cert_tmp = _write_temp(bak_cert, cert_pem, 0o444)
        key_tmp = _write_temp(bak_key, key_pem, 0o400)
        atomic_replace(cert_tmp, bak_cert)
        atomic_replace(key_tmp, bak_key)
        _fsync_dir(bak_cert.parent)
        if bak_key.parent != bak_cert.parent:
            _fsync_dir(bak_key.parent)

    def _restore_from_bak(self) -> None:
        bak_cert = backup_path(self.cert_path)
        bak_key = backup_path(self.key_path)
        atomic_replace(bak_key, self.key_path)
        atomic_replace(bak_cert, self.cert_path)
        _fsync_dir(self.cert_path.parent)
        if self.key_path.parent != self.cert_path.parent:
            _fsync_dir(self.key_path.parent)

    def _rollback(self, had_previous: bool) -> None:
        if had_previous and self._bak_pair():
            self._restore_from_bak()
        elif not had_previous:
            self.cert_path.unlink(missing_ok=True)
            self.key_path.unlink(missing_ok=True)
        self._cleanup_txn()

    def _write_journal(self, phase: str, had_previous: bool) -> None:
        payload = json.dumps({"v": 1, "phase": phase, "had_previous": had_previous}).encode("utf-8")
        path = self._journal_path()
        tmp = _write_temp(path, payload, 0o600)
        atomic_replace(tmp, path)
        _fsync_dir(path.parent)

    def _cleanup_txn(self) -> None:
        self._journal_path().unlink(missing_ok=True)
        backup_path(self.cert_path).unlink(missing_ok=True)
        backup_path(self.key_path).unlink(missing_ok=True)
        for directory in {self.cert_path.parent, self.key_path.parent}:
            for leftover in directory.glob(".pki-enroll-*"):
                leftover.unlink(missing_ok=True)
            _fsync_dir(directory)

    def _assert_ssl_pair(self) -> None:
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        try:
            ctx.load_cert_chain(certfile=str(self.cert_path), keyfile=str(self.key_path))
        except (ssl.SSLError, OSError) as err:
            raise CertPairMismatchError("pki: cert/key pair mismatch") from err


def _write_temp(final_path: Path, data: bytes, mode: int) -> Path:
    assert_trusted_dest(str(final_path))
    fd, name = tempfile.mkstemp(prefix=".pki-enroll-", dir=str(final_path.parent))
    tmp = Path(name)
    try:
        os.write(fd, data)
        os.fsync(fd)
        os.fchmod(fd, mode)
    except OSError:
        tmp.unlink(missing_ok=True)
        raise
    finally:
        os.close(fd)
    return tmp
