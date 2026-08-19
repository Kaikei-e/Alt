#!/usr/bin/env python3
"""Deterministic Wave 4 provisioner-mapping tests (no live step-ca).

Parses bootstrap/verify scripts as text. Does not start Docker, does not
print or invent secret bytes.
"""

from __future__ import annotations

import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))
sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))

import safe_log  # noqa: E402
from safe_log import check  # noqa: E402

safe_log.reset()

BOOTSTRAP = ROOT / "pki-agent" / "scripts" / "bootstrap-pki-provisioner.sh"
VERIFY = ROOT / "pki-agent" / "scripts" / "verify-cn-allowlist.sh"
WORKFLOW = ROOT / ".github" / "workflows" / "compose-audit.yaml"
PKI_SCRIPTS_GLOB = "pki-agent/scripts/**"

WORKLOAD = (
    "alt-backend",
    "alt-harvester",
    "alt-data-hub",
    "alt-notifier",
    "alt-butterfly-facade",
    "auth-hub",
    "pre-processor",
    "search-indexer",
    "tag-generator",
    "recap-worker",
    "acolyte-orchestrator",
    "recap-subworker",
    "news-creator",
    "rag-orchestrator",
)


def extract_subjects(text: str, array_name: str) -> list[str]:
    m = re.search(rf"{array_name}=\(\n(.*?)^\)", text, re.S | re.M)
    if not m:
        return []
    body = re.sub(r"#.*", "", m.group(1))
    return re.findall(r"^\s*([A-Za-z0-9.-]+)\s*$", body, re.M)


bootstrap = BOOTSTRAP.read_text(encoding="utf-8")
verify = VERIFY.read_text(encoding="utf-8")

print("subject-scoped bootstrap mapping")

check(
    "bootstrap does not add a shared JWK via `step ca provisioner add pki-agent`",
    "step ca provisioner add pki-agent" not in bootstrap,
)
check(
    "bootstrap never uses the CA root password as a provisioner --password-file",
    "--password-file" in bootstrap
    and "step_ca_root_password" not in re.findall(
        r"--password-file\s+(\S+)", bootstrap
    ),
)
check(
    "bootstrap names provisioners pki-agent-<subject> via helper",
    "pki-agent-%s" in bootstrap and "provisioner_name_for" in bootstrap,
)
check(
    "bootstrap host secret files are pki-agent-<subject>-jwk.txt",
    "pki-agent-%s-jwk.txt" in bootstrap,
)
check(
    "bootstrap does not echo password file contents",
    not re.search(r"echo\s+.*password", bootstrap, re.I)
    and "cat \"$host_pw\"" not in bootstrap
    and "cat $host_pw" not in bootstrap,
)
check(
    "localhost is allowlisted but does not get a JWK provisioner",
    "localhost" in bootstrap and "skip provisioner for allowlist-only name localhost" in bootstrap,
)

boot_subjects = extract_subjects(bootstrap, "SUBJECTS")
verify_cns = extract_subjects(verify, "EXPECTED_CNS")
check(
    "bootstrap SUBJECTS and verify EXPECTED_CNS stay in lockstep",
    boot_subjects == verify_cns and boot_subjects[-1:] == ["localhost"],
    f"bootstrap={boot_subjects!r} verify={verify_cns!r}",
)
check(
    "all 14 workload CNs are in the allowlist",
    set(WORKLOAD).issubset(set(boot_subjects)) and len(WORKLOAD) == 14,
    f"missing={sorted(set(WORKLOAD) - set(boot_subjects))}",
)

print("compose declares 14 subject-scoped JWK secrets; no shared workload provisioner")

from compose_include import load_yaml, production_compose_files, production_services  # noqa: E402


def _env_map(svc: dict) -> dict[str, str]:
    out: dict[str, str] = {}
    raw = svc.get("environment") or []
    if isinstance(raw, dict):
        for key, value in raw.items():
            if isinstance(key, str) and value is not None:
                out[key] = str(value)
        return out
    for item in raw:
        if not isinstance(item, str) or "=" not in item:
            continue
        key, value = item.split("=", 1)
        out[key] = value
    return out


def _secret_names(svc: dict) -> set[str]:
    names: set[str] = set()
    for item in svc.get("secrets") or []:
        if isinstance(item, str):
            names.add(item)
        elif isinstance(item, dict) and item.get("source"):
            names.add(str(item["source"]))
    return names


compose_secrets: dict[str, dict] = {}
for path in production_compose_files():
    data = load_yaml(path)
    for name, spec in (data.get("secrets") or {}).items():
        if isinstance(spec, dict):
            compose_secrets[name] = spec

for subject in WORKLOAD:
    secret_id = f"pki-agent-{subject}-jwk"
    spec = compose_secrets.get(secret_id) or {}
    check(
        f"compose declares fail-fast {secret_id}",
        spec.get("file") == f"../secrets/{secret_id}.txt",
        f"got {spec!r}",
    )

prod = production_services()
shared = [
    name
    for name, svc in prod.items()
    if name not in {"step-ca", "step-ca-bootstrap"}
    and _env_map(svc).get("STEP_CA_PROVISIONER") == "pki-agent"
]
check(
    "no compose workload uses shared STEP_CA_PROVISIONER=pki-agent",
    shared == [],
    f"{shared}",
)
root_workloads = [
    name
    for name, svc in prod.items()
    if name not in {"step-ca", "step-ca-bootstrap"}
    and "step_ca_root_password" in _secret_names(svc)
]
check(
    "no compose workload mounts step_ca_root_password",
    root_workloads == [],
    f"{root_workloads}",
)
pki_agents = [name for name in prod if name.startswith("pki-agent-")]
check(
    "compose declares 0 pki-agent workload sidecars",
    pki_agents == [],
    f"{pki_agents}",
)

print("verify script source")
check(
    "verify mints with a subject-scoped provisioner helper, not a shared name",
    "provisioner_name_for" in verify and "SMOKE_PROVISIONER" in verify,
)
check(
    "verify does not pass the CA root password as --password-file",
    "step_ca_root_password" not in verify,
)
check(
    "verify smoke subject is alt-backend (first cohort)",
    "SMOKE_SUBJECT=alt-backend" in verify,
)

print("compose-audit path coverage for provisioner scripts")
from compose_include import load_yaml  # noqa: E402

workflow_cfg = load_yaml(WORKFLOW) if WORKFLOW.is_file() else {}
on = workflow_cfg.get("on")
if on is None:
    on = workflow_cfg.get(True)
if not isinstance(on, dict):
    on = {}
for event in ("push", "pull_request"):
    block = on.get(event) or {}
    paths = {str(p) for p in (block.get("paths") or [])} if isinstance(block, dict) else set()
    check(
        f"{event} path filter includes {PKI_SCRIPTS_GLOB}",
        PKI_SCRIPTS_GLOB in paths,
        f"missing {PKI_SCRIPTS_GLOB}; provisioner contract would skip",
    )

print(f"\n{safe_log.PASS} passed, {safe_log.FAIL} failed")
sys.exit(1 if safe_log.FAIL else 0)
