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
import subprocess
import sys
from pathlib import Path

_SCRIPTS = Path(__file__).resolve().parent
REPO_ROOT = _SCRIPTS.parent
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


# (service, source) pairs that still bind a gitignored repo path, with why
# they have not moved to a host path yet. They predate the check and each
# needs its payload staged on the prod host first, so they are reported as
# debt rather than failing the gate — a *new* one still fails.
KNOWN_WORKSPACE_SOURCES = {
    ("restic-backup", "../backups"): "backup profile only; never rolled by deploy",
    ("restic-backup", "../backups/postgres"): "backup profile only; never rolled by deploy",
    ("knowledge-sovereign", "../backups/sovereign-snapshots"): (
        "snapshot sink; a deploy roll writes into the workspace copy instead of the host one"
    ),
}


def _default_is_ignored(path: Path) -> bool:
    """True when git excludes `path`, so a fresh checkout will not have it."""
    proc = subprocess.run(
        ["git", "-C", str(REPO_ROOT), "check-ignore", "-q", str(path)],
        capture_output=True,
        check=False,
    )
    return proc.returncode == 0


def ephemeral_source_violations(
    services: dict[str, dict],
    compose_dir: Path | None = None,
    is_ignored=_default_is_ignored,
) -> list[str]:
    """Bind sources that a fresh checkout of this repo would not contain.

    `create_host_path: false` makes a missing source fail loudly rather than
    materialise as an empty directory, but it cannot say *where* the source
    should live. A repo-relative source resolves against the compose file, so
    on the deploy runner it lands inside the per-job workspace — which holds
    only tracked files. Point such a mount at a host path (see
    RECAP_SUBWORKER_DATA_HOST_PATH) instead.

    Interpolated and absolute sources are the host-path form and are skipped:
    their value comes from the host's .env, which this audit cannot resolve.
    """
    if compose_dir is None:
        return []
    found: list[str] = []
    for name, svc in sorted(services.items()):
        for raw in svc.get("volumes") or []:
            if isinstance(raw, str):
                split = split_short_volume(raw)
            elif isinstance(raw, dict):
                split = _long_bind(raw)
            else:
                continue
            if split is None:
                continue
            source, target = split
            if is_named_volume(source) or "${" in source or source.startswith("/"):
                continue
            resolved = (compose_dir / source).resolve()
            if not is_ignored(resolved):
                continue
            found.append(
                f"{name} bind source {source} -> {target} is gitignored, so the "
                f"deploy checkout resolves it to an empty path (use a host path)"
            )
    return found


def audit_production() -> list[str]:
    found: list[str] = []
    for path, name, svc in iter_production_services():
        found.extend(file_bind_violations({name: svc}, path.parent))
    return found


def audit_production_sources() -> list[str]:
    """Unacknowledged gitignored bind sources across production compose."""
    found: list[str] = []
    for path, name, svc in iter_production_services():
        for violation in ephemeral_source_violations({name: svc}, path.parent):
            source = violation.split(" bind source ", 1)[1].split(" -> ", 1)[0]
            if (name, source) in KNOWN_WORKSPACE_SOURCES:
                continue
            found.append(violation)
    return found


def acknowledged_workspace_sources() -> list[str]:
    return [
        f"{service} {source} — {why}"
        for (service, source), why in sorted(KNOWN_WORKSPACE_SOURCES.items())
    ]


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
    ephemeral = audit_production_sources()
    if ephemeral:
        print("Bind sources a fresh checkout cannot provide:")
        for v in ephemeral:
            print(f"  - {v}")
        print(
            "\nThe deploy runner checks Alt out per job, so only tracked files "
            "are there. Move the source to a host path with a ${VAR:-/var/lib/...} "
            "default, as recap-subworker /app/data already does."
        )
        return 1
    acknowledged = acknowledged_workspace_sources()
    if acknowledged:
        print("Known gitignored bind sources (staged debt, not gating):")
        for entry in acknowledged:
            print(f"  - {entry}")
    print("OK: 0 unguarded file binds, 0 unacknowledged gitignored bind sources")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
