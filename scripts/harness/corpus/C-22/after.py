# ruff: noqa: TRY003, PLC0415
"""O_NOFOLLOW certificate file helpers."""

from __future__ import annotations

import os
import stat
from pathlib import Path


class PkiError(RuntimeError):
    """Raised when a PKI path check fails."""


def assert_trusted_parent(path: str) -> None:
    if not path:
        raise PkiError("pki: empty path")
    info = os.lstat(str(Path(path).parent))
    if stat.S_ISLNK(info.st_mode):
        raise PkiError(f"pki: parent directory of {path!r} is a symlink")
