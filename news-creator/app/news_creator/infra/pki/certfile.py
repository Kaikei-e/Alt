"""Atomic PEM leaf + key install with journaled rollback."""

# ruff: noqa: TRY003

from __future__ import annotations

import ctypes
import json
import os
import ssl
import tempfile
from datetime import datetime
from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives.serialization import (
    Encoding,
    PublicFormat,
    load_pem_private_key,
)
from cryptography.x509.oid import NameOID

from news_creator.infra.pki.config import PkiError
from news_creator.infra.pki.filesafe import (
    CERT_DIR_PERM,
    MAX_ROOT_PEM_BYTES,
    assert_trusted_dest,
    mkdir_trusted,
    read_regular_no_follow,
)

_JOURNAL_NAME = ".pki-install.journal"
_BAK_SUFFIX = ".pki-bak"
_ENROLL_PREFIX = ".pki-enroll-"
_STAGE_PREFIX = ".pki-stage-"
_RESTORE_PREFIX = ".pki-restore-"


class CertNotFoundError(PkiError):
    """Cert file absent from disk."""


class CertParseError(PkiError):
    """File present but not a valid X.509 PEM."""


CertParseFailedError = CertParseError


class LoadedCert:
    """Parsed leaf used by the state machine."""

    __slots__ = ("common_name", "not_after", "not_before")

    def __init__(
        self, not_before: datetime, not_after: datetime, common_name: str
    ) -> None:
        self.not_before = not_before
        self.not_after = not_after
        self.common_name = common_name


class CertFile:
    """Reads and atomically replaces a PEM leaf + key pair."""

    def __init__(self, cert_path: str, key_path: str) -> None:
        self.cert_path = Path(cert_path)
        self.key_path = Path(key_path)

    def load(self) -> LoadedCert:
        self.recover()
        try:
            cert_raw = read_regular_no_follow(str(self.cert_path), MAX_ROOT_PEM_BYTES)
        except FileNotFoundError as err:
            raise CertNotFoundError("pki: cert not found") from err
        except PkiError as err:
            raise CertParseError("pki: cert parse failed") from err
        try:
            key_raw = read_regular_no_follow(str(self.key_path), MAX_ROOT_PEM_BYTES)
        except FileNotFoundError as err:
            raise CertParseError("pki: cert/key pair mismatch") from err
        except PkiError as err:
            raise CertParseError("pki: cert parse failed") from err
        _assert_pair(cert_raw, key_raw)
        try:
            cert = x509.load_pem_x509_certificate(cert_raw)
        except ValueError as err:
            raise CertParseError("pki: cert parse failed") from err
        cn = cert.subject.get_attributes_for_oid(NameOID.COMMON_NAME)
        name = cn[0].value if cn else ""
        if not isinstance(name, str):
            name = str(name)
        return LoadedCert(cert.not_valid_before_utc, cert.not_valid_after_utc, name)

    def write(self, cert_pem: bytes, key_pem: bytes) -> None:
        """Stage both PEMs, journal the install, rename key then cert, roll back on failure."""
        mkdir_trusted(str(self.cert_path.parent), CERT_DIR_PERM)
        mkdir_trusted(str(self.key_path.parent), CERT_DIR_PERM)
        assert_trusted_dest(str(self.cert_path))
        assert_trusted_dest(str(self.key_path))
        self.recover()
        self._backup_current()
        self._write_journal()
        cert_tmp = _write_temp(self.cert_path, cert_pem, 0o444, _ENROLL_PREFIX)
        try:
            key_tmp = _write_temp(self.key_path, key_pem, 0o400, _ENROLL_PREFIX)
        except OSError:
            cert_tmp.unlink(missing_ok=True)
            raise
        try:
            key_tmp.replace(self.key_path)
            cert_tmp.replace(self.cert_path)
        except OSError:
            cert_tmp.unlink(missing_ok=True)
            key_tmp.unlink(missing_ok=True)
            self._rollback_partial()
            raise
        try:
            _assert_pair(
                read_regular_no_follow(str(self.cert_path), MAX_ROOT_PEM_BYTES),
                read_regular_no_follow(str(self.key_path), MAX_ROOT_PEM_BYTES),
            )
        except FileNotFoundError:
            self._rollback_partial()
            raise
        except PkiError:
            self._rollback_partial()
            raise
        self._commit()

    def recover(self) -> None:
        """Restore the last-good pair after a crash between dest renames."""
        if self._dest_pair_ok():
            self._commit()
            return
        if self._backup_pair_ok():
            self._restore_backup()
            if self._dest_pair_ok():
                self._commit()
                return
        self._cleanup_temps()

    def _journal_path(self) -> Path:
        return self.cert_path.parent / _JOURNAL_NAME

    def _cert_bak_path(self) -> Path:
        return self.cert_path.parent / f"{self.cert_path.name}{_BAK_SUFFIX}"

    def _key_bak_path(self) -> Path:
        return self.key_path.parent / f"{self.key_path.name}{_BAK_SUFFIX}"

    def _backup_current(self) -> None:
        if _regular_exists(self.cert_path):
            raw = read_regular_no_follow(str(self.cert_path), MAX_ROOT_PEM_BYTES)
            _install(self._cert_bak_path(), raw, 0o444, _STAGE_PREFIX)
        if _regular_exists(self.key_path):
            raw = read_regular_no_follow(str(self.key_path), MAX_ROOT_PEM_BYTES)
            _install(self._key_bak_path(), raw, 0o400, _STAGE_PREFIX)

    def _write_journal(self) -> None:
        payload = json.dumps(
            {
                "state": "installing",
                "cert": str(self.cert_path),
                "key": str(self.key_path),
                "cert_bak": str(self._cert_bak_path()),
                "key_bak": str(self._key_bak_path()),
            },
            separators=(",", ":"),
        ).encode("utf-8")
        _install(self._journal_path(), payload, 0o600, _STAGE_PREFIX)

    def _rollback_partial(self) -> None:
        self._restore_backup()
        self._commit()

    def _restore_backup(self) -> None:
        if _regular_exists(self._cert_bak_path()):
            raw = read_regular_no_follow(str(self._cert_bak_path()), MAX_ROOT_PEM_BYTES)
            _install(self.cert_path, raw, 0o444, _RESTORE_PREFIX)
        if _regular_exists(self._key_bak_path()):
            raw = read_regular_no_follow(str(self._key_bak_path()), MAX_ROOT_PEM_BYTES)
            _install(self.key_path, raw, 0o400, _RESTORE_PREFIX)

    def _commit(self) -> None:
        self._journal_path().unlink(missing_ok=True)
        self._cert_bak_path().unlink(missing_ok=True)
        self._key_bak_path().unlink(missing_ok=True)
        self._cleanup_temps()

    def _cleanup_temps(self) -> None:
        for directory in {self.cert_path.parent, self.key_path.parent}:
            if not directory.is_dir():
                continue
            for child in directory.iterdir():
                if child.name.startswith(
                    (_ENROLL_PREFIX, _STAGE_PREFIX, _RESTORE_PREFIX)
                ):
                    child.unlink(missing_ok=True)

    def _dest_pair_ok(self) -> bool:
        return _pair_files_ok(self.cert_path, self.key_path)

    def _backup_pair_ok(self) -> bool:
        return _pair_files_ok(self._cert_bak_path(), self._key_bak_path())


def _regular_exists(path: Path) -> bool:
    try:
        return path.is_file() and not path.is_symlink()
    except OSError:
        return False


def _pair_files_ok(cert_path: Path, key_path: Path) -> bool:
    try:
        cert_raw = read_regular_no_follow(str(cert_path), MAX_ROOT_PEM_BYTES)
        key_raw = read_regular_no_follow(str(key_path), MAX_ROOT_PEM_BYTES)
        _assert_pair(cert_raw, key_raw)
    except FileNotFoundError:
        return False
    except PkiError:
        return False
    except ValueError:
        return False
    return True


def _assert_pair(cert_pem: bytes, key_pem: bytes) -> None:
    try:
        cert = x509.load_pem_x509_certificate(cert_pem)
        key = load_pem_private_key(key_pem, password=None)
    except (ValueError, TypeError) as err:
        raise CertParseError("pki: cert/key pair parse failed") from err
    cert_spki = cert.public_key().public_bytes(
        Encoding.DER, PublicFormat.SubjectPublicKeyInfo
    )
    key_spki = key.public_key().public_bytes(
        Encoding.DER, PublicFormat.SubjectPublicKeyInfo
    )
    if cert_spki != key_spki:
        raise CertParseError("pki: cert/key pair mismatch")
    _ssl_load_pair(cert_pem, key_pem)


def _ssl_load_pair(cert_pem: bytes, key_pem: bytes) -> None:
    cert_fd = -1
    key_fd = -1
    try:
        cert_fd = _anonymous_fd("pki-pair-cert")
        key_fd = _anonymous_fd("pki-pair-key")
        os.write(cert_fd, cert_pem)
        os.write(key_fd, key_pem)
        os.lseek(cert_fd, 0, os.SEEK_SET)
        os.lseek(key_fd, 0, os.SEEK_SET)
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        ctx.load_cert_chain(
            certfile=f"/proc/self/fd/{cert_fd}", keyfile=f"/proc/self/fd/{key_fd}"
        )
    except ssl.SSLError as err:
        raise CertParseError("pki: cert/key pair mismatch") from err
    finally:
        if cert_fd >= 0:
            os.close(cert_fd)
        if key_fd >= 0:
            os.close(key_fd)


def _anonymous_fd(name: str) -> int:
    create = getattr(os, "memfd_create", None)
    if callable(create):
        fd = create(name, getattr(os, "MFD_CLOEXEC", 1))
        if not isinstance(fd, int) or fd < 0:
            raise PkiError("pki: anonymous secret fd unavailable")
        return fd
    libc = ctypes.CDLL("libc.so.6", use_errno=True)
    libc.memfd_create.argtypes = [ctypes.c_char_p, ctypes.c_uint]
    libc.memfd_create.restype = ctypes.c_int
    fd = libc.memfd_create(name.encode("ascii", "replace"), 1)
    if fd >= 0:
        return fd
    err = ctypes.get_errno()
    raise PkiError("pki: anonymous secret fd unavailable") from OSError(
        err, os.strerror(err)
    )


def _install(final_path: Path, data: bytes, mode: int, prefix: str) -> None:
    assert_trusted_dest(str(final_path))
    tmp = _write_temp(final_path, data, mode, prefix)
    tmp.replace(final_path)


def _write_temp(final_path: Path, data: bytes, mode: int, prefix: str) -> Path:
    assert_trusted_dest(str(final_path))
    fd, name = tempfile.mkstemp(prefix=prefix, dir=str(final_path.parent))
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
