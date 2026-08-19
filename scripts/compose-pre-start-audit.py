#!/usr/bin/env python3
"""Fail when Wave 1b chown-only hooks are not `pre_start` on the consumer.

P2-13 / PM-2026-037: `news-creator-volume-init` and
`knowledge-embedder-local-volume-init` were the two safe single-consumer
chown-only one-shots. They must live as `pre_start` on the consumer so
`up --no-deps --force-recreate` still runs the chown. Sibling init
services and `service_completed_successfully` edges onto them are debt.

Usage: python3 scripts/compose-pre-start-audit.py
Exit 0 when the two consumers carry alpine chown hooks and the sibling
init services are gone; 1 otherwise.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

_SCRIPTS = Path(__file__).resolve().parent
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

from compose_include import production_services  # noqa: E402

# Wave 1b scope. Do not add Atlas/DB migrators, step-ca-bootstrap,
# oauth-token-init, clickhouse-migrator, or recap artefacts here.
MIGRATED_CHOWN_HOOKS = {
    "news-creator-backend": {
        "path": "/home/ollama-user/.ollama",
        "uid_gid": "2000:2000",
        "removed_init": "news-creator-volume-init",
    },
    "knowledge-embedder-local": {
        "path": "/home/ollama-user/.ollama",
        "uid_gid": "2000:2000",
        "removed_init": "knowledge-embedder-local-volume-init",
    },
}

# Wave 4 final in-process PKI cutover: the parent must own the cert
# volume so it can write 0400 keys. UID/GID match the runtime user.
# No sibling pki-agent sidecar (dual writer).
WAVE4_PKI_CERT_HOOKS = {
    "alt-backend": {
        "path": "/certs",
        "uid_gid": "65532:65532",
        "removed_sidecar": "pki-agent-alt-backend",
    },
    "alt-harvester": {
        "path": "/certs",
        "uid_gid": "65532:65532",
        "removed_sidecar": "pki-agent-alt-harvester",
    },
    "alt-data-hub": {
        "path": "/certs",
        "uid_gid": "65532:65532",
        "removed_sidecar": "pki-agent-alt-data-hub",
    },
    "alt-notifier": {
        "path": "/certs",
        "uid_gid": "65532:65532",
        "removed_sidecar": "pki-agent-alt-notifier",
    },
    "alt-butterfly-facade": {
        "path": "/certs",
        "uid_gid": "65532:65532",
        "removed_sidecar": "pki-agent-alt-butterfly-facade",
    },
    "auth-hub": {
        "path": "/certs",
        "uid_gid": "65532:65532",
        "removed_sidecar": "pki-agent-auth-hub",
    },
    "search-indexer": {
        "path": "/certs",
        "uid_gid": "65532:65532",
        "removed_sidecar": "pki-agent-search-indexer",
    },
    "rag-orchestrator": {
        "path": "/certs",
        "uid_gid": "65532:65532",
        "removed_sidecar": "pki-agent-rag-orchestrator",
    },
    "pre-processor": {
        "path": "/certs",
        "uid_gid": "65532:65532",
        "removed_sidecar": "pki-agent-pre-processor",
    },
    "tag-generator": {
        "path": "/certs",
        "uid_gid": "1000:1000",
        "removed_sidecar": "pki-agent-tag-generator",
    },
    "recap-worker": {
        "path": "/certs",
        "uid_gid": "999:999",
        "removed_sidecar": "pki-agent-recap-worker",
    },
    "acolyte-orchestrator": {
        "path": "/certs",
        "uid_gid": "1000:1000",
        "removed_sidecar": "pki-agent-acolyte-orchestrator",
    },
    "recap-subworker": {
        "path": "/certs",
        "uid_gid": "999:999",
        "removed_sidecar": "pki-agent-recap-subworker",
    },
    "news-creator": {
        "path": "/certs",
        "uid_gid": "1000:1000",
        "removed_sidecar": "pki-agent-news-creator",
    },
}


def hook_command_text(hook: dict) -> str:
    command = hook.get("command")
    if isinstance(command, list):
        return " ".join(str(part) for part in command)
    if command is None:
        return ""
    return str(command)


def is_chown_hook(hook: object, path: str, uid_gid: str) -> bool:
    if not isinstance(hook, dict):
        return False
    text = hook_command_text(hook)
    image = str(hook.get("image") or "")
    if "alpine" not in image:
        return False
    if "chown" not in text:
        return False
    if uid_gid not in text:
        return False
    return path in text


def is_pki_cert_chown_hook(hook: object, path: str, uid_gid: str) -> bool:
    """Wave 4 cert volume: chown to the distroless uid, never world-writable."""
    if not is_chown_hook(hook, path, uid_gid):
        return False
    text = hook_command_text(hook)
    if "0777" in text or " chmod 777" in f" {text}":
        return False
    return "0750" in text or "0700" in text or "chmod 750" in text or "chmod 700" in text


def audit_migrated_chown_hooks(services: dict[str, dict]) -> list[str]:
    violations: list[str] = []
    for consumer, spec in MIGRATED_CHOWN_HOOKS.items():
        removed = spec["removed_init"]
        if removed in services:
            violations.append(f"sibling init service {removed} still declared")
        svc = services.get(consumer)
        if not isinstance(svc, dict):
            violations.append(f"{consumer} missing from compose")
            continue
        hooks = svc.get("pre_start") or []
        if not isinstance(hooks, list) or not any(
            is_chown_hook(hook, spec["path"], spec["uid_gid"]) for hook in hooks
        ):
            violations.append(
                f"{consumer} missing pre_start alpine chown hook for {spec['path']}"
            )
        deps = svc.get("depends_on") or {}
        if isinstance(deps, dict) and removed in deps:
            violations.append(f"{consumer} still depends_on {removed}")
        elif isinstance(deps, list) and removed in deps:
            violations.append(f"{consumer} still depends_on {removed}")
    return violations


def audit_wave4_pki_cert_hooks(services: dict[str, dict]) -> list[str]:
    violations: list[str] = []
    for consumer, spec in WAVE4_PKI_CERT_HOOKS.items():
        removed = spec["removed_sidecar"]
        if removed in services:
            violations.append(f"dual writer: {removed} still declared beside {consumer}")
        svc = services.get(consumer)
        if not isinstance(svc, dict):
            violations.append(f"{consumer} missing from compose")
            continue
        hooks = svc.get("pre_start") or []
        if not isinstance(hooks, list) or not any(
            is_pki_cert_chown_hook(hook, spec["path"], spec["uid_gid"]) for hook in hooks
        ):
            violations.append(
                f"{consumer} missing pre_start alpine chown 0750 hook for {spec['path']}"
            )
        deps = svc.get("depends_on") or {}
        if isinstance(deps, dict) and removed in deps:
            violations.append(f"{consumer} still depends_on {removed}")
        elif isinstance(deps, list) and removed in deps:
            violations.append(f"{consumer} still depends_on {removed}")
    return violations


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.parse_args()
    services = production_services()
    found = audit_migrated_chown_hooks(services)
    found.extend(audit_wave4_pki_cert_hooks(services))
    if found:
        print("compose-pre-start-audit FAILED — chown hooks drifted:")
        for item in found:
            print(f"  - {item}")
        return 1
    print(
        f"OK: {len(MIGRATED_CHOWN_HOOKS)} Wave 1b consumer(s) and "
        f"{len(WAVE4_PKI_CERT_HOOKS)} Wave 4 PKI parent(s) carry pre_start "
        "chown hooks; sibling init/sidecar writers are gone"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
