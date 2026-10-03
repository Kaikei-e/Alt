#!/usr/bin/env python3
"""Fail when a production compose service uses an unguarded file bind.

PM-2026-036: a missing file-scoped bind source is created as an empty
directory with no warning. Short syntax cannot set `create_host_path:
false`. Long syntax without that flag has the same default (create).
Wave 1 converted production mounts to `configs:` (in-repo static files)
or long-syntax bind + `create_host_path: false` (host-only files and
artefact directories). This audit is the zero-violation gate.

It also keeps deploy/host-prereqs.yaml in lockstep with compose: every
host-path bind, the docker socket group, GPU runtime consumers, required
.env keys and gitignored env_files compose relies on must be declared
there, and nothing declared there may outlive its compose use.

Usage: python3 scripts/compose-file-bind-audit.py
Exit 0 when clean, 1 with a per-violation report otherwise.
"""

from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
from collections.abc import Iterable
from pathlib import Path
from typing import NamedTuple

_SCRIPTS = Path(__file__).resolve().parent
REPO_ROOT = _SCRIPTS.parent
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

from compose_include import (  # noqa: E402
    iter_production_services,
    load_yaml,
    production_compose_files,
)

MANIFEST_PATH = REPO_ROOT / "deploy" / "host-prereqs.yaml"

FILE_SUFFIXES = {
    ".conf",
    ".yml",
    ".yaml",
    ".json",
    ".toml",
    ".ini",
    ".xml",
    ".pem",
    ".pub",
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
    "/etc/plecto/artifacts/",
    "/backups/clickhouse",
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
    # `compose config` without --profile backup drops this service from the
    # rendered model, so no roll can materialise the source in the workspace.
    # The source is also the live restic repository and the tree
    # `altctl migrate backup -o ./backups` writes to; moving it would fork
    # the two and orphan the offsite copy lineage.
    ("restic-backup", "../backups"): (
        "backup profile only; excluded from the rendered model, and the source "
        "is the live restic repo shared with altctl's ./backups output"
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


# --- deploy/host-prereqs.yaml ------------------------------------------------

_SINGLE_INTERPOLATION = re.compile(
    r"^\$\{(?P<env>[A-Za-z_][A-Za-z0-9_]*)(?:(?P<op>:?[-?])(?P<arg>[^}]*))?\}$"
)
_UNDEFAULTED_ENV = re.compile(
    r"(?<!\$)\$(?:\{(?P<braced>[A-Za-z_][A-Za-z0-9_]*)(?::?\?[^}]*)?\}"
    r"|(?P<bare>[A-Za-z_][A-Za-z0-9_]*))"
)
HOST_PATH_KINDS = {"file", "dir", "socket"}
_OWNER = re.compile(r"^\d+:\d+$")
_MODE = re.compile(r"^0[0-7]{3}$")


class HostMount(NamedTuple):
    service: str
    target: str
    read_only: bool
    create_host_path: bool


HostKey = tuple  # (env var or None, default/literal path or None)


def host_source(source: str) -> HostKey | None:
    """(env, path) for a bind source the host must provide, else None.

    `${VAR:-/default}` and `${VAR}` are host paths chosen by the deploy
    host's .env; an absolute path is a host path compose hardcodes.
    Repo-relative sources come from the checkout and are not host
    prerequisites (gitignored ones are caught by ephemeral_source_violations).
    """
    s = source.strip()
    m = _SINGLE_INTERPOLATION.match(s)
    if m:
        has_default = (m.group("op") or "").endswith("-")
        return m.group("env"), (m.group("arg") if has_default else None)
    if s.startswith(("/", "$")):
        return None, s
    return None


def _short_read_only(entry: str) -> bool:
    bits = entry.split(":")
    return len(bits) >= 3 and "ro" in bits[-1].split(",")


def host_path_mounts(
    services: Iterable[tuple[str, dict]],
) -> dict[HostKey, set[HostMount]]:
    found: dict[HostKey, set[HostMount]] = {}
    for name, svc in services:
        for raw in svc.get("volumes") or []:
            if isinstance(raw, str):
                split = split_short_volume(raw)
                read_only = _short_read_only(raw)
                create = True
            elif isinstance(raw, dict):
                split = _long_bind(raw)
                read_only = bool(raw.get("read_only"))
                bind = raw.get("bind") if isinstance(raw.get("bind"), dict) else {}
                create = bind.get("create_host_path") is not False
            else:
                continue
            if split is None:
                continue
            source, target = split
            key = host_source(source)
            if key is None:
                continue
            found.setdefault(key, set()).add(HostMount(name, target, read_only, create))
    return found


def _key_label(key: HostKey) -> str:
    env, path = key
    if env is None:
        return str(path)
    return f"${{{env}:-{path}}}" if path is not None else f"${{{env}}}"


def _entry_schema_violations(entry: dict) -> list[str]:
    label = entry.get("id") or _key_label((entry.get("env"), entry.get("path")))
    found = []
    if not isinstance(entry.get("id"), str) or not entry["id"]:
        found.append(f"host_paths entry {label}: missing id")
    if entry.get("env") is None and not entry.get("path"):
        found.append(f"host_paths entry {label}: needs env, path, or both")
    if entry.get("kind") not in HOST_PATH_KINDS:
        found.append(f"host_paths entry {label}: kind must be one of {sorted(HOST_PATH_KINDS)}")
    owner = entry.get("owner")
    if owner is not None and not (isinstance(owner, str) and _OWNER.match(owner)):
        found.append(f"host_paths entry {label}: owner must be \"uid:gid\" or null")
    mode = entry.get("mode")
    if mode is not None and not (isinstance(mode, str) and _MODE.match(mode)):
        found.append(f"host_paths entry {label}: mode must be a quoted octal like \"0644\" or null")
    if not isinstance(entry.get("create_host_path"), bool):
        found.append(f"host_paths entry {label}: create_host_path must be true or false")
    provision = entry.get("provision")
    if not isinstance(provision, str) or not provision.strip():
        found.append(f"host_paths entry {label}: provision recipe is empty")
    return found


def _declared_mounts(entry: dict) -> set[tuple[str, str, bool]]:
    return {
        (m.get("service"), m.get("target"), bool(m.get("read_only", False)))
        for m in entry.get("mounts") or []
    }


def host_path_violations(
    mounts_by_source: dict[HostKey, set[HostMount]], manifest: dict
) -> list[str]:
    found: list[str] = []
    declared: dict[HostKey, dict] = {}
    for entry in manifest.get("host_paths") or []:
        found.extend(_entry_schema_violations(entry))
        key = (entry.get("env"), entry.get("path"))
        if key in declared:
            found.append(f"host_paths entry {entry.get('id')}: duplicates {declared[key].get('id')}")
        declared[key] = entry

    for key in sorted(mounts_by_source, key=_key_label):
        mounts = mounts_by_source[key]
        users = ", ".join(sorted({m.service for m in mounts}))
        entry = declared.get(key)
        if entry is None:
            found.append(
                f"host path {_key_label(key)} is bind-mounted by {users} but has no "
                f"deploy/host-prereqs.yaml host_paths entry"
            )
            continue
        actual = {(m.service, m.target, m.read_only) for m in mounts}
        listed = _declared_mounts(entry)
        for service, target, read_only in sorted(actual - listed):
            found.append(
                f"{entry.get('id')}: compose mounts it in {service} at {target} "
                f"(read_only={read_only}), which the manifest does not list"
            )
        for service, target, read_only in sorted(listed - actual):
            found.append(
                f"{entry.get('id')}: manifest lists {service} at {target} "
                f"(read_only={read_only}), which compose does not mount"
            )
        creates = {m.create_host_path for m in mounts}
        if creates != {entry.get("create_host_path")}:
            found.append(
                f"{entry.get('id')}: manifest says create_host_path: "
                f"{entry.get('create_host_path')}, compose has {sorted(creates)}"
            )

    for key, entry in declared.items():
        if key not in mounts_by_source:
            found.append(
                f"{entry.get('id')}: no production compose bind uses {_key_label(key)} "
                f"any more; remove the entry"
            )
    return found


def _user_gid(user) -> str | None:
    parts = str(user).split(":")
    return parts[1] if len(parts) == 2 and parts[1].isdigit() else None


def socket_group_violations(services: dict[str, dict], manifest: dict) -> list[str]:
    """A socket consumer's hardcoded `user: uid:gid` must use the socket's group."""
    found: list[str] = []
    for entry in manifest.get("host_paths") or []:
        if entry.get("kind") != "socket" or not entry.get("owner"):
            continue
        socket_gid = str(entry["owner"]).split(":")[1]
        for mount in entry.get("mounts") or []:
            svc = services.get(mount.get("service")) or {}
            gid = _user_gid(svc.get("user", ""))
            if gid is not None and gid != socket_gid:
                found.append(
                    f"{mount.get('service')} runs as group {gid}, but {entry.get('id')} "
                    f"declares the socket group as {socket_gid}"
                )
    return found


def _uses_nvidia(svc: dict) -> bool:
    if svc.get("runtime") == "nvidia":
        return True
    devices = (
        ((svc.get("deploy") or {}).get("resources") or {}).get("reservations") or {}
    ).get("devices") or []
    return any(isinstance(d, dict) and d.get("driver") == "nvidia" for d in devices)


def gpu_violations(services: dict[str, dict], manifest: dict) -> list[str]:
    actual = {name for name, svc in services.items() if _uses_nvidia(svc)}
    fact = (manifest.get("host_facts") or {}).get("nvidia_container_runtime") or {}
    declared = set(fact.get("services") or [])
    found = [
        f"{name} reserves an NVIDIA GPU but is missing from host_facts.nvidia_container_runtime"
        for name in sorted(actual - declared)
    ]
    found.extend(
        f"host_facts.nvidia_container_runtime lists {name}, which no longer reserves a GPU"
        for name in sorted(declared - actual)
    )
    return found


def undefaulted_env_keys(texts: Iterable[str]) -> set[str]:
    """Interpolations compose resolves from .env with no default value."""
    keys: set[str] = set()
    for text in texts:
        for line in text.splitlines():
            if line.lstrip().startswith("#"):
                continue
            for m in _UNDEFAULTED_ENV.finditer(line):
                keys.add(m.group("braced") or m.group("bare"))
    return keys


def required_env_violations(texts: Iterable[str], manifest: dict) -> list[str]:
    actual = undefaulted_env_keys(texts)
    declared = set(manifest.get("required_env") or [])
    found = [
        f"compose interpolates ${{{key}}} with no default, but required_env does not list it"
        for key in sorted(actual - declared)
    ]
    found.extend(
        f"required_env lists {key}, which compose no longer interpolates without a default"
        for key in sorted(declared - actual)
    )
    return found


def _env_file_paths(raw) -> list[str]:
    if raw is None:
        return []
    items = raw if isinstance(raw, list) else [raw]
    paths = []
    for item in items:
        if isinstance(item, str):
            paths.append(item)
        elif isinstance(item, dict) and item.get("required", True) is not False:
            paths.append(str(item.get("path")))
    return paths


def env_file_violations(
    entries: Iterable[tuple[Path, str, dict]],
    manifest: dict,
    is_ignored=_default_is_ignored,
    repo_root: Path = REPO_ROOT,
) -> list[str]:
    """Required env_files a fresh checkout lacks must be declared."""
    actual: dict[str, set[str]] = {}
    for compose_dir, name, svc in entries:
        for rel in _env_file_paths(svc.get("env_file")):
            resolved = Path(os.path.normpath(compose_dir / rel))
            if not is_ignored(resolved):
                continue
            actual.setdefault(resolved.relative_to(repo_root).as_posix(), set()).add(name)
    declared = {e.get("path") for e in manifest.get("env_files") or []}
    found = [
        f"env_file {path} (used by {', '.join(sorted(actual[path]))}) is gitignored, "
        f"so the host must provide it, but env_files does not list it"
        for path in sorted(set(actual) - declared)
    ]
    found.extend(
        f"env_files lists {path}, which no production service requires any more"
        for path in sorted(declared - set(actual))
    )
    return found


def audit_host_prereqs(manifest_path: Path = MANIFEST_PATH) -> list[str]:
    if not manifest_path.is_file():
        return [f"missing host prerequisite manifest {manifest_path}"]
    manifest = load_yaml(manifest_path)
    entries = iter_production_services()
    services = {name: svc for _path, name, svc in entries}
    texts = [p.read_text(encoding="utf-8") for p in production_compose_files()]
    found = host_path_violations(
        host_path_mounts((name, svc) for _path, name, svc in entries), manifest
    )
    found.extend(socket_group_violations(services, manifest))
    found.extend(gpu_violations(services, manifest))
    found.extend(required_env_violations(texts, manifest))
    found.extend(
        env_file_violations(
            ((path.parent, name, svc) for path, name, svc in entries), manifest
        )
    )
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
    prereqs = audit_host_prereqs()
    if prereqs:
        print(f"Host prerequisites out of sync with {MANIFEST_PATH.relative_to(REPO_ROOT)}:")
        for v in prereqs:
            print(f"  - {v}")
        print(
            "\nThe deploy preflight provisions and checks the host from that "
            "manifest. Declare each new host path (owner, mode, provisioning "
            "recipe) there, or drop entries compose no longer uses."
        )
        return 1
    acknowledged = acknowledged_workspace_sources()
    if acknowledged:
        print("Known gitignored bind sources (staged debt, not gating):")
        for entry in acknowledged:
            print(f"  - {entry}")
    print(
        "OK: 0 unguarded file binds, 0 unacknowledged gitignored bind sources, "
        "host prerequisite manifest in sync"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
