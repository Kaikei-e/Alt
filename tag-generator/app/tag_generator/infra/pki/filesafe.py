"""O_NOFOLLOW secret/cert I/O with size caps. Mirrors Go filesafe.go."""

# ruff: noqa: TRY003

from __future__ import annotations

import contextlib
import errno
import os
import stat
from pathlib import Path

from tag_generator.infra.pki.config import PkiError

MAX_PASSWORD_BYTES = 4 << 10
MAX_ENV_FILE_BYTES = 4 << 10
MAX_ROOT_PEM_BYTES = 1 << 20
CERT_DIR_PERM = 0o750


def assert_trusted_parent(path: str) -> None:
    if not path:
        raise PkiError("pki: empty path")
    directory = str(Path(path).parent)
    try:
        info = os.lstat(directory)
    except OSError as err:
        raise PkiError(f"pki: lstat parent of {path!r}") from err
    if stat.S_ISLNK(info.st_mode):
        raise PkiError(f"pki: parent directory of {path!r} is a symlink")
    if not stat.S_ISDIR(info.st_mode):
        raise PkiError(f"pki: parent of {path!r} is not a directory")


def assert_trusted_dest(path: str) -> None:
    assert_trusted_parent(path)
    try:
        info = os.lstat(path)
    except FileNotFoundError:
        return
    except OSError as err:
        raise PkiError(f"pki: lstat {path!r}") from err
    if stat.S_ISLNK(info.st_mode):
        raise PkiError(f"pki: {path!r} is a symlink")


def mkdir_trusted(path: str, mode: int = CERT_DIR_PERM) -> None:
    Path(path).mkdir(parents=True, exist_ok=True, mode=mode)
    with contextlib.suppress(OSError):
        Path(path).chmod(mode)
    assert_trusted_parent(str(Path(path) / ".keep"))


def read_regular_no_follow(path: str, max_bytes: int) -> bytes:
    assert_trusted_parent(path)
    target = Path(path)
    directory = str(target.parent)
    base = target.name
    if base in {".", ".."} or os.sep in base:
        raise PkiError(f"pki: invalid basename for {path!r}")
    flags_dir = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | getattr(os, "O_CLOEXEC", 0)
    try:
        dir_fd = os.open(directory, flags_dir)
    except OSError as err:
        if err.errno == errno.ELOOP:
            raise PkiError(f"pki: parent directory of {path!r} is a symlink") from err
        raise PkiError(f"pki: open parent of {path!r}") from err
    try:
        flags = os.O_RDONLY | os.O_NOFOLLOW | getattr(os, "O_CLOEXEC", 0)
        try:
            fd = os.open(base, flags, dir_fd=dir_fd)
        except OSError as err:
            if err.errno == errno.ELOOP:
                raise PkiError(f"pki: {path!r} is a symlink") from err
            if err.errno == errno.ENOENT:
                raise FileNotFoundError(path) from err
            raise PkiError(f"pki: open {path!r}") from err
    finally:
        os.close(dir_fd)
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode):
            raise PkiError(f"pki: {path!r} is not a regular file")
        if info.st_mode & 0o002:
            raise PkiError(f"pki: {path!r} is world-writable")
        if info.st_size > max_bytes:
            raise PkiError(f"pki: {path!r} exceeds {max_bytes}-byte cap")
        raw = os.read(fd, max_bytes + 1)
        if len(raw) > max_bytes:
            raise PkiError(f"pki: {path!r} exceeds {max_bytes}-byte cap")
        return raw
    finally:
        os.close(fd)
