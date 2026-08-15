"""Path validation utilities for recap-subworker.

Normalizes user-supplied paths and confines them to the allow-listed base
directories (CWE-22).
"""

from __future__ import annotations

import os
from pathlib import Path

# NOTE: changing this list must be coordinated with the API-layer contract.
ALLOWED_BASE_DIRS: list[Path] = [
    Path("/app/data"),
    Path("/app/resources"),
]


def validate_path(user_path: str, base_dirs: list[Path] | None = None) -> Path:
    """Validate a user-supplied path and return it as a safe Path.

    - normalizes the path (collapses `..` and friends)
    - resolves relative paths against base_dirs[0]
    - resolves symlinks, then requires the result to live under one of the
      allow-listed base directories

    Args:
        user_path: the path as supplied (absolute or relative)
        base_dirs: allow-listed base directories; ALLOWED_BASE_DIRS when None

    Returns:
        Path: the validated absolute filesystem path.

    Raises:
        ValueError: when the path is outside every allowed directory.
    """
    if base_dirs is None:
        base_dirs = ALLOWED_BASE_DIRS

    if not base_dirs:
        raise ValueError("No base directories configured for path validation")

    normalized = os.path.normpath(user_path)

    # NOTE: CodeQL py/path-injection recognizes the os.path API
    # (normpath/realpath + commonpath) as a sanitizer, not pathlib.
    if not os.path.isabs(normalized):  # noqa: PTH117
        normalized = os.path.normpath(os.path.join(str(base_dirs[0]), normalized))  # noqa: PTH118

    # realpath() resolves symlinks before the containment check.
    real_path = os.path.realpath(normalized)

    for base_dir in base_dirs:
        real_base = os.path.realpath(str(base_dir))
        # commonpath blocks prefix attacks (/app/data-evil does not match
        # /app/data) and is a CodeQL-recognized sanitizer. Different drives
        # or mix-ups raise ValueError → not allowed.
        try:
            if os.path.commonpath([real_path, real_base]) == real_base:
                return Path(real_path)
        except ValueError:
            continue

    raise ValueError(
        f"Path '{user_path}' is not within allowed directories: {[str(d) for d in base_dirs]}"
    )


def require_existing_path(user_path: str, base_dirs: list[Path] | None = None) -> Path:
    """Validate *and* confirm the path exists under an allow-listed base.

    Filesystem access (`os.path.exists`) runs only after a CodeQL-visible
    sanitizer on the same variable: normalize → commonpath/prefix-check →
    `safe_path = real_path` → exists(`safe_path`). The sink is flattened out
    of the allow-list loop so the query can prove the guard.
    """
    if base_dirs is None:
        base_dirs = ALLOWED_BASE_DIRS

    if not base_dirs:
        raise ValueError("No base directories configured for path validation")

    # Lexical allow-list (raises ValueError if the path escapes).
    validate_path(user_path, base_dirs)

    # Inline the official CodeQL pattern so taint does not survive through the
    # helper: normpath(+join) → realpath → prefix-check → same var at the sink.
    normalized = os.path.normpath(user_path)
    if not os.path.isabs(normalized):  # noqa: PTH117
        normalized = os.path.normpath(os.path.join(str(base_dirs[0]), normalized))  # noqa: PTH118

    real_path = os.path.realpath(normalized)

    matched_base: str | None = None
    for base_dir in base_dirs:
        real_base = os.path.realpath(str(base_dir))
        try:
            if os.path.commonpath([real_path, real_base]) == real_base:
                matched_base = real_base
                break
        except ValueError:
            continue

    if matched_base is None:
        raise ValueError(
            f"Path '{user_path}' is not within allowed directories: {[str(d) for d in base_dirs]}"
        )

    # Official sanitizer (docs): prefix-check THEN use the same variable.
    # commonpath above already blocked prefix attacks (/app/data-evil);
    # startswith(matched_base) is the exact barrier CodeQL models.
    if not real_path.startswith(matched_base):
        raise ValueError(
            f"Path '{user_path}' is not within allowed directories: {[str(d) for d in base_dirs]}"
        )

    safe_path = real_path
    if not os.path.exists(safe_path):  # noqa: PTH110
        raise FileNotFoundError(safe_path)
    return Path(safe_path)
