"""O_NOFOLLOW certificate file helpers."""

from __future__ import annotations

import os
import stat
from pathlib import Path


class PkiError(RuntimeError):
    """Raised when a PKI path check fails."""


def assert_trusted_parent(path: str) -> None:
    if not path:
        msg = "pki: empty path"
        raise PkiError(msg)
    info = os.lstat(str(Path(path).parent))
    if stat.S_ISLNK(info.st_mode):
        msg = f"pki: parent directory of {path!r} is a symlink"
        raise PkiError(msg)
