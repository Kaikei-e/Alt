#!/usr/bin/env python3
"""Fail when a production compose service uses an unguarded file bind.

PM-2026-036: a missing file-scoped bind source is created as an empty
directory with no warning. Short syntax cannot set `create_host_path:
false`. Long syntax without that flag has the same default (create).
Wave 1 converted production mounts to `configs:` (in-repo static files)
or long-syntax bind + `create_host_path: false` (host-only files and
artefact directories). This audit is the zero-violation gate.

Usage: python3 scripts/compose-file-bind-audit.py
Exit 0 when clean, 1 with a per-violation report otherwise.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

_SCRIPTS = Path(__file__).resolve().parent
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

from compose_include import iter_production_services  # noqa: E402

FILE_SUFFIXES = {
    ".conf",
    ".yml",
    ".yaml",
    ".json",
    ".toml",
    ".ini",
    ".xml",
    ".pem",
    ".crt",
    ".key",
    ".sh",
    ".js",
    ".html",
    ".txt",
    ".cfg",
    ".sql",
    ".joblib",
    ".pkl",
    ".env",
}

EXTENSIONLESS_FILE_BASENAMES = {
    "authorized_keys",
    "id_dsa",
    "id_ecdsa",
    "id_ed25519",
    "id_ed25519_backup",
    "id_rsa",
    "known_hosts",
}

SOCKET_BASENAMES = {"docker.sock"}

# Host artefact directories that must refuse a missing source. Generic
# in-repo directory binds (plecto, grafana provisioning, …) stay out of
# this set — Wave 1 only gates the PM-036/037 class.
ARTEFACT_DIRECTORY_MARKERS = (
    "RECAP_SUBWORKER_DATA_HOST_PATH",
    "alt-recap-subworker-data",
    "learning_machine/artifacts",
)

SHORT_MODES = {"ro", "rw", "z", "Z", "consistent", "delegated", "cached"}


def _basename(path: str) -> str:
    return Path(path.replace("\\", "/")).name


def split_short_volume(entry: str) -> tuple[str, str] | None:
    """Return (source, target) for a short-syntax bind, or None if unnamed/named-only."""
    rest = entry.strip()
    if not rest or rest.startswith("#"):
        return None
    bits = rest.split(":")
    if bits and bits[-1] in SHORT_MODES:
        rest = rest[: -(len(bits[-1]) + 1)]
    idx = rest.rfind(":/")
    if idx == -1:
        if ":" not in rest:
            return None
        source, target = rest.split(":", 1)
    else:
        source, target = rest[:idx], rest[idx + 1 :]
    source, target = source.strip(), target.strip()
    if not source or not target:
        return None
    return source, target


def is_named_volume(source: str) -> bool:
    s = source.strip()
    if s.startswith((".", "/", "$", "~")):
        return False
    return "/" not in s


def is_socket_bind(source: str, target: str) -> bool:
    return _basename(source) in SOCKET_BASENAMES or _basename(target) in SOCKET_BASENAMES


def looks_like_file(source: str, target: str, compose_dir: Path | None) -> bool:
    if is_socket_bind(source, target):
        return True
    for part in (source, target):
        base = _basename(part)
        suffix = Path(base).suffix.lower()
        if suffix in FILE_SUFFIXES:
            return True
        if base in EXTENSIONLESS_FILE_BASENAMES:
            return True
    if compose_dir is not None and not source.startswith("$"):
        path = Path(source)
        if not path.is_absolute():
            path = compose_dir / source
        try:
            if path.is_file():
                return True
        except OSError:
            return False
    return False


def looks_like_artefact_directory(source: str, target: str) -> bool:
    haystack = f"{source}:{target}"
    return any(marker in haystack for marker in ARTEFACT_DIRECTORY_MARKERS)


def needs_create_host_path_guard(
    source: str, target: str, compose_dir: Path | None
) -> bool:
    return looks_like_file(source, target, compose_dir) or looks_like_artefact_directory(
        source, target
    )


def _long_bind(entry: dict) -> tuple[str, str] | None:
    typ = entry.get("type")
    source = entry.get("source") or entry.get("src")
    target = entry.get("target") or entry.get("destination")
    if not isinstance(source, str) or not isinstance(target, str):
        return None
    if typ not in (None, "bind"):
        return None
    if typ is None and is_named_volume(source):
        return None
    return source, target


def file_bind_violations(
    services: dict[str, dict],
    compose_dir: Path | None = None,
) -> list[str]:
    found: list[str] = []
    for name, svc in sorted(services.items()):
        for raw in svc.get("volumes") or []:
            if isinstance(raw, str):
                split = split_short_volume(raw)
                if split is None:
                    continue
                source, target = split
                if is_named_volume(source):
                    continue
                if needs_create_host_path_guard(source, target, compose_dir):
                    found.append(
                        f"{name} short-syntax file bind {source} -> {target} "
                        f"(cannot set create_host_path: false; PM-2026-036)"
                    )
                continue
            if not isinstance(raw, dict):
                continue
            split = _long_bind(raw)
            if split is None:
                continue
            source, target = split
            if is_named_volume(source) or not needs_create_host_path_guard(
                source, target, compose_dir
            ):
                continue
            bind = raw.get("bind") if isinstance(raw.get("bind"), dict) else {}
            if bind.get("create_host_path") is False:
                continue
            found.append(
                f"{name} long-syntax file bind {source} -> {target} "
                f"without create_host_path: false"
            )
    return found


def audit_production() -> list[str]:
    found: list[str] = []
    for path, name, svc in iter_production_services():
        found.extend(file_bind_violations({name: svc}, path.parent))
    return found


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--expect-violations",
        action="store_true",
        help="Wave 0 only: exit 0 when violations exist. Do not use after Wave 1.",
    )
    args = parser.parse_args()
    found = audit_production()
    if args.expect_violations:
        if found:
            print(
                f"OK: file-bind detector is armed ({len(found)} production "
                f"violation(s) still present; Wave 1 must clear them)"
            )
            for v in found:
                print(f"  - {v}")
            return 0
        print(
            "file-bind-audit --expect-violations FAILED — production compose "
            "has no short-syntax file binds. If Wave 1 already migrated them, "
            "drop --expect-violations and keep the zero-violation gate."
        )
        return 1
    if found:
        print("Short-syntax / unguarded file binds (PM-2026-036 class):")
        for v in found:
            print(f"  - {v}")
        print(
            "\nConvert to long-syntax bind with create_host_path: false, "
            "or a configs: entry. Short syntax cannot refuse a missing file."
        )
        return 1
    print("OK: 0 unguarded file binds")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
