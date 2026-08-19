#!/usr/bin/env python3
"""Wave 4 PKI acceptance contract — final in-process cutover.

Subject-scoped enrollment: every east-west parent mints with its own JWK
provisioner. The pki-agent workload fleet must be 0. Parent-only
force-recreate keeps :9443 reachable because inbound TLS lives in the
parent process.

This file does not start or destroy a live stack and never reads secret
bytes.

Run:
    python3 scripts/tests/test-pki-wave4-acceptance.py
Exit 0 on green, non-zero on red.
"""

from __future__ import annotations

import importlib.util
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))
sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))

from compose_include import load_yaml, production_compose_files  # noqa: E402
import safe_log  # noqa: E402
from safe_log import check  # noqa: E402

safe_log.reset()

spec = importlib.util.spec_from_file_location(
    "netns_cascade_audit", ROOT / "scripts" / "compose-netns-cascade-audit.py"
)
assert spec is not None and spec.loader is not None, "audit script not importable"
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)

MTLS = ("MTLS_CERT_FILE", "MTLS_KEY_FILE", "MTLS_CA_FILE")
DATAHUB = ("DATAHUB_TLS_CERT_FILE", "DATAHUB_TLS_KEY_FILE", "DATAHUB_TLS_CA_FILE")
DISTROLESS = ("65532", "65532")
APPUSER = ("1000", "1000")
RECAP = ("999", "999")

# 14 workload parents. Compose service name == CERT_SUBJECT.
INPROCESS = {
    "alt-backend": {
        "cert_vol": "alt_backend_certs",
        "tls": MTLS,
        "uid_gid": DISTROLESS,
        "ops_env": "OPS_LISTEN",
        "main": ROOT / "alt-backend" / "app" / "cmd" / "backend" / "main.go",
        "start": "bootstrap.StartEnrollment",
    },
    "alt-harvester": {
        "cert_vol": "alt_harvester_certs",
        "tls": MTLS,
        "uid_gid": DISTROLESS,
        "ops_env": "OPS_LISTEN",
        "main": ROOT / "alt-backend" / "app" / "cmd" / "harvester" / "main.go",
        "start": "bootstrap.StartEnrollment",
    },
    "alt-data-hub": {
        "cert_vol": "alt_data_hub_certs",
        "tls": DATAHUB,
        "uid_gid": DISTROLESS,
        "ops_env": "OPS_LISTEN",
        "main": ROOT / "alt-backend" / "app" / "cmd" / "datahub" / "main.go",
        "start": "bootstrap.StartEnrollment",
    },
    "alt-notifier": {
        "cert_vol": "alt_notifier_certs",
        "tls": MTLS,
        "uid_gid": DISTROLESS,
        "ops_env": "OPS_LISTEN",
        "main": ROOT / "alt-backend" / "app" / "cmd" / "notifier" / "main.go",
        "start": "bootstrap.StartEnrollment",
    },
    "alt-butterfly-facade": {
        "cert_vol": "alt_butterfly_facade_certs",
        "tls": MTLS,
        "uid_gid": DISTROLESS,
        "ops_env": "OPS_LISTEN",
        "main": ROOT / "alt-butterfly-facade" / "main.go",
        "start": "pki.Start(",
    },
    "auth-hub": {
        "cert_vol": "auth_hub_certs",
        "tls": MTLS,
        "uid_gid": DISTROLESS,
        "ops_env": "OPS_LISTEN",
        "main": ROOT / "auth-hub" / "cmd" / "auth-hub" / "main.go",
        "start": "pki.Start(",
    },
    "search-indexer": {
        "cert_vol": "search_indexer_certs",
        "tls": MTLS,
        "uid_gid": DISTROLESS,
        "ops_env": "OPS_LISTEN",
        "main": ROOT / "search-indexer" / "app" / "bootstrap" / "app.go",
        "start": "pki.Start(",
    },
    "rag-orchestrator": {
        "cert_vol": "rag_orchestrator_certs",
        "tls": MTLS,
        "uid_gid": DISTROLESS,
        "ops_env": "OPS_LISTEN",
        "main": ROOT / "rag-orchestrator" / "cmd" / "server" / "main.go",
        "start": "pki.StartWithRegisterer(",
    },
    "pre-processor": {
        "cert_vol": "pre_processor_certs",
        "tls": MTLS,
        "uid_gid": DISTROLESS,
        "ops_env": "OPS_LISTEN",
        "main": ROOT / "pre-processor" / "app" / "bootstrap" / "lifecycle.go",
        "start": "startEnrollment(",
    },
    "tag-generator": {
        "cert_vol": "tag_generator_certs",
        "tls": MTLS,
        "uid_gid": APPUSER,
        "ops_env": "OPS_LISTEN",
        "main": ROOT / "tag-generator" / "app" / "tag_generator" / "infra" / "pki" / "start.py",
        "start": "start_ops",
    },
    "recap-worker": {
        "cert_vol": "recap_worker_certs",
        "tls": MTLS,
        "uid_gid": RECAP,
        "ops_env": "PKI_METRICS_BIND",
        "main": ROOT / "recap-worker" / "recap-worker" / "src" / "pki" / "start.rs",
        "start": "PKI_METRICS_BIND",
    },
    "acolyte-orchestrator": {
        "cert_vol": "acolyte_orchestrator_certs",
        "tls": MTLS,
        "uid_gid": APPUSER,
        "ops_env": "OPS_LISTEN",
        "main": ROOT / "acolyte-orchestrator" / "acolyte" / "infra" / "pki" / "start.py",
        "start": "start_ops",
    },
    "recap-subworker": {
        "cert_vol": "recap_subworker_certs",
        "tls": MTLS,
        "uid_gid": RECAP,
        "ops_env": "OPS_LISTEN",
        "main": ROOT / "recap-subworker" / "recap_subworker" / "app" / "infra" / "pki" / "start.py",
        "start": "start_ops",
    },
    "news-creator": {
        "cert_vol": "news_creator_certs",
        "tls": MTLS,
        "uid_gid": APPUSER,
        "ops_env": "OPS_LISTEN",
        "main": ROOT / "news-creator" / "app" / "news_creator" / "infra" / "pki" / "start.py",
        "start": "start_ops",
    },
}

INPROCESS_PARENTS = tuple(INPROCESS)
assert len(INPROCESS_PARENTS) == 14

PATTERN_B_PARENTS = (
    "tag-generator",
    "acolyte-orchestrator",
    "recap-subworker",
    "news-creator",
)

EXPECTED_ALLOWED_PEERS = {
    "tag-generator": "alt-butterfly-facade,alt-backend,search-indexer,recap-worker",
    "acolyte-orchestrator": "alt-butterfly-facade",
    "recap-subworker": "recap-worker",
    "news-creator": "recap-worker,acolyte-orchestrator,rag-orchestrator,recap-evaluator",
}

INBOUND_ENABLE = {
    "tag-generator": ("INBOUND_TLS_ENABLED", "true"),
    "acolyte-orchestrator": ("INBOUND_TLS_ENABLED", "true"),
    "recap-subworker": ("INBOUND_MTLS", "true"),
    "news-creator": ("INBOUND_MTLS", "true"),
}

FORBIDDEN_PROXY_ENV = (
    "PROXY_LISTEN",
    "PROXY_UPSTREAM",
    "PROXY_RESPONSE_HEADER_TIMEOUT",
)

PRESERVED_NON_PKI_SCRAPES = (
    "alt-butterfly-facade:9250",
    "pre-processor:9201",
    "recap-worker:9005",
    "recap-subworker:8002",
    "news-creator:11434",
    "rag-orchestrator:9010",
)

CUTOVER_SIDECARS = tuple(f"pki-agent-{name}" for name in INPROCESS_PARENTS)
PKI_FLEET_SIZE = 0


def env_map(svc: dict) -> dict[str, str]:
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


def interp_default(value: str) -> str:
    match = re.fullmatch(r"\$\{[^:}]+:-(.*)\}", value.strip())
    if match:
        return match.group(1)
    return value


def volume_entries(svc: dict) -> list[str]:
    out: list[str] = []
    for item in svc.get("volumes") or []:
        if isinstance(item, str):
            out.append(item)
            continue
        if not isinstance(item, dict):
            continue
        source = str(item.get("source") or "")
        target = str(item.get("target") or "")
        mode = "ro" if item.get("read_only") else "rw"
        if source and target:
            out.append(f"{source}:{target}:{mode}")
    return out


def volume_is_ro(entry: str, source: str, target: str) -> bool:
    parts = entry.split(":")
    if len(parts) < 2:
        return False
    src, dst = parts[0], parts[1]
    mode = parts[2] if len(parts) > 2 else "rw"
    return src == source and dst == target and mode == "ro"


def volume_is_rw(entry: str, source: str, target: str) -> bool:
    parts = entry.split(":")
    if len(parts) < 2:
        return False
    src, dst = parts[0], parts[1]
    mode = parts[2] if len(parts) > 2 else "rw"
    return src == source and dst == target and mode != "ro"


def secret_names(svc: dict) -> set[str]:
    names: set[str] = set()
    for item in svc.get("secrets") or []:
        if isinstance(item, str):
            names.add(item)
            continue
        if isinstance(item, dict) and item.get("source"):
            names.add(str(item["source"]))
    return names


def production_secrets() -> dict[str, dict]:
    secrets: dict[str, dict] = {}
    for path in production_compose_files():
        data = load_yaml(path)
        for name, spec in (data.get("secrets") or {}).items():
            if isinstance(spec, dict):
                secrets[name] = spec
    return secrets


def hook_command_text(hook: dict) -> str:
    command = hook.get("command")
    if isinstance(command, list):
        return " ".join(str(part) for part in command)
    if command is None:
        return ""
    return str(command)


def is_certs_chown_hook(hook: object, uid: str, gid: str) -> bool:
    if not isinstance(hook, dict):
        return False
    text = hook_command_text(hook)
    image = str(hook.get("image") or "")
    if "alpine" not in image:
        return False
    if "chown" not in text or "/certs" not in text:
        return False
    if f"{uid}:{gid}" not in text:
        return False
    if "0777" in text or "777" in text.split():
        return False
    return "0750" in text or "0700" in text or "750" in text or "700" in text


def published_ports(svc: dict) -> str:
    ports = svc.get("ports") or []
    return " ".join(str(p) for p in ports)


print("pki-agent workload fleet is 0")

services = audit.production_services()
pki_agents = {
    name: svc
    for name, svc in services.items()
    if name.startswith("pki-agent-")
}
check(
    "pki-agent workload fleet is 0",
    len(pki_agents) == PKI_FLEET_SIZE,
    f"got {sorted(pki_agents)} — leftover sidecars are dual writers",
)
for sidecar in CUTOVER_SIDECARS:
    check(
        f"{sidecar} is not declared",
        sidecar not in services,
        "in-process parent owns enrollment; a leftover sidecar is a dual writer",
    )

bootstrap = (ROOT / "pki-agent" / "scripts" / "bootstrap-pki-provisioner.sh").read_text(
    encoding="utf-8"
)
check(
    "bootstrap script does not add a single shared pki-agent JWK for every subject",
    "step ca provisioner add pki-agent" not in bootstrap,
    "pki-agent/scripts/bootstrap-pki-provisioner.sh still creates one JWK named pki-agent",
)

print("zero network_mode: service: pki sidecars")

netns = audit.netns_sidecars(services)
pki_netns = {name: parent for name, parent in netns.items() if name.startswith("pki-agent-")}
check(
    "no pki-agent uses network_mode: service:<parent>",
    pki_netns == {},
    f"forbidden netns-sharing pki sidecars: {pki_netns!r}",
)

print("Pattern B inbound TLS stays in the parent")

for parent in PATTERN_B_PARENTS:
    parent_svc = services.get(parent) or {}
    parent_env = env_map(parent_svc)
    for key in FORBIDDEN_PROXY_ENV:
        check(
            f"{parent} has no {key}",
            not parent_env.get(key),
            f"{key}={parent_env.get(key)!r} would terminate inbound TLS outside the parent",
        )
    enable_key, enable_val = INBOUND_ENABLE[parent]
    check(
        f"{parent} explicitly enables inbound mTLS ({enable_key}={enable_val})",
        interp_default(parent_env.get(enable_key, "")).lower() == enable_val,
        f"{enable_key}={parent_env.get(enable_key)!r}",
    )
    allowed = interp_default(parent_env.get("MTLS_ALLOWED_PEERS", ""))
    check(
        f"{parent} MTLS_ALLOWED_PEERS matches former PROXY_ALLOWED_PEERS",
        allowed == EXPECTED_ALLOWED_PEERS[parent] and allowed != "",
        f"got {allowed!r}, want {EXPECTED_ALLOWED_PEERS[parent]!r}",
    )
    trusted = interp_default(parent_env.get("PEER_IDENTITY_TRUSTED", "off")).lower()
    check(
        f"{parent} does not trust inbound X-Alt-Peer-Identity from the network",
        trusted in {"off", "false", "0", ""},
        f"PEER_IDENTITY_TRUSTED={parent_env.get('PEER_IDENTITY_TRUSTED')!r}",
    )
    check(
        f"{parent} does not publish plaintext :9443 on the host",
        "9443" not in published_ports(parent_svc),
        f"ports={parent_svc.get('ports')!r}",
    )

print("14 in-process parents (final compose cutover)")

secrets = production_secrets()
seen_subjects: dict[str, str] = {}
seen_provisioners: dict[str, str] = {}
seen_password_files: dict[str, str] = {}
seen_secret_names: dict[str, str] = {}

for parent, meta in INPROCESS.items():
    sidecar = f"pki-agent-{parent}"
    parent_svc = services.get(parent) or {}
    parent_env = env_map(parent_svc)
    vols = volume_entries(parent_svc)
    cert_vol = meta["cert_vol"]
    secret_id = f"pki-agent-{parent}-jwk"
    jwk_file = f"../secrets/pki-agent-{parent}-jwk.txt"
    provisioner = parent_env.get("STEP_CA_PROVISIONER", "")
    password_file = parent_env.get("STEP_CA_PROVISIONER_PASSWORD_FILE", "")
    subject = parent_env.get("CERT_SUBJECT", "")
    cert_path = parent_env.get("CERT_PATH", "")
    key_path = parent_env.get("KEY_PATH", "")
    ca_url = parent_env.get("STEP_CA_URL", "")
    ca_root = parent_env.get("STEP_CA_ROOT_FILE", "")
    sans = parent_env.get("CERT_SANS", "")
    tls_cert, tls_key, tls_ca = meta["tls"]
    ops_env = meta["ops_env"]
    uid, gid = meta["uid_gid"]

    check(
        f"{parent} is not a dual writer with {sidecar}",
        sidecar not in services,
        f"{sidecar} still declared; sidecar + parent on {cert_vol} is forbidden",
    )
    check(
        f"{parent} has no depends_on {sidecar}",
        sidecar not in (parent_svc.get("depends_on") or {}),
        f"depends_on still lists {sidecar}",
    )
    check(
        f"{parent} explicitly sets PKI_ENROLLMENT=enabled",
        interp_default(parent_env.get("PKI_ENROLLMENT", "")).lower() == "enabled",
        f"PKI_ENROLLMENT={parent_env.get('PKI_ENROLLMENT')!r} — "
        "old images that ignore this env plus new compose (no sidecar) have no cert writer",
    )
    check(
        f"{parent} CERT_SUBJECT={parent}",
        subject == parent,
        f"CERT_SUBJECT={subject!r}",
    )
    check(
        f"{parent} CERT_SANS includes subject and localhost",
        sans == f"{parent},localhost",
        f"CERT_SANS={sans!r}",
    )
    check(
        f"{parent} STEP_CA_PROVISIONER is subject-scoped (not shared pki-agent)",
        provisioner == f"pki-agent-{parent}" and provisioner != "pki-agent",
        f"STEP_CA_PROVISIONER={provisioner!r}",
    )
    check(
        f"{parent} provisioner password file is the subject-scoped JWK secret",
        password_file == f"/run/secrets/{secret_id}",
    )
    check(
        f"{parent} does not mount step_ca_root_password",
        "step_ca_root_password" not in secret_names(parent_svc)
        and "step_ca_root_password" not in password_file,
    )
    check(
        f"{parent} mounts only its matching JWK secret {secret_id}",
        secret_id in secret_names(parent_svc)
        and not any(
            name.startswith("pki-agent-") and name.endswith("-jwk") and name != secret_id
            for name in secret_names(parent_svc)
        ),
        f"secrets={sorted(secret_names(parent_svc))}",
    )
    spec = secrets.get(secret_id) or {}
    check(
        f"compose declares {secret_id} with fail-fast file: {jwk_file}",
        spec.get("file") == jwk_file,
        f"got {spec!r}",
    )
    check(
        f"repo does not contain secret bytes for {jwk_file}",
        not (ROOT / "secrets" / f"pki-agent-{parent}-jwk.txt").is_file(),
        "do not check in provisioner passwords",
    )
    check(
        f"{parent} CERT_PATH matches the TLS reloader cert path",
        cert_path == "/certs/svc-cert.pem" and parent_env.get(tls_cert) == cert_path,
        f"CERT_PATH={cert_path!r} reloader={parent_env.get(tls_cert)!r}",
    )
    check(
        f"{parent} KEY_PATH matches the TLS reloader key path",
        key_path == "/certs/svc-key.pem" and parent_env.get(tls_key) == key_path,
        f"KEY_PATH={key_path!r} reloader={parent_env.get(tls_key)!r}",
    )
    check(
        f"{parent} STEP_CA_ROOT_FILE matches the TLS reloader CA path",
        ca_root == "/trust/ca-bundle.pem" and parent_env.get(tls_ca) == ca_root,
        f"STEP_CA_ROOT_FILE={ca_root!r} reloader={parent_env.get(tls_ca)!r}",
    )
    check(
        f"{parent} STEP_CA_URL is https step-ca",
        ca_url == "https://step-ca:9000",
        f"STEP_CA_URL={ca_url!r}",
    )
    check(
        f"{parent} mounts {cert_vol} at /certs writable (parent is the writer)",
        any(volume_is_rw(v, cert_vol, "/certs") for v in vols),
        f"volumes={vols!r}",
    )
    check(
        f"{parent} mounts pki_trust_bundle at /trust read-only",
        any(volume_is_ro(v, "pki_trust_bundle", "/trust") for v in vols),
        f"volumes={vols!r}",
    )
    hooks = parent_svc.get("pre_start") or []
    check(
        f"{parent} pre_start chowns /certs to {uid}:{gid} mode 0750 (not world-writable)",
        isinstance(hooks, list) and any(is_certs_chown_hook(h, uid, gid) for h in hooks),
        f"pre_start={hooks!r}",
    )
    check(
        f"{parent} depends_on step-ca-bootstrap (trust bundle before enroll)",
        "step-ca-bootstrap" in (parent_svc.get("depends_on") or {}),
        f"depends_on={parent_svc.get('depends_on')!r}",
    )
    ops_val = parent_env.get(ops_env, "")
    if ops_env == "PKI_METRICS_BIND":
        allowed_ops = {"0.0.0.0:9110"}
        detail = (
            f"{ops_env}={ops_val!r} — Rust SocketAddr rejects bare :9110; "
            "compose must set 0.0.0.0:9110"
        )
    else:
        allowed_ops = {":9110", "0.0.0.0:9110"}
        detail = f"{ops_env}={ops_val!r} — loopback default is unreachable to Prometheus"
    check(
        f"{parent} binds private ops via {ops_env}={sorted(allowed_ops)[-1] if ops_env == 'PKI_METRICS_BIND' else ':9110'}",
        ops_val in allowed_ops,
        detail,
    )
    check(
        f"{parent} does not publish :9110 on the host",
        "9110" not in published_ports(parent_svc),
        f"ports={parent_svc.get('ports')!r}",
    )
    main_path = meta["main"]
    main_body = main_path.read_text(encoding="utf-8") if main_path.is_file() else ""
    check(
        f"{parent} image wires in-process enrollment ({meta['start']})",
        meta["start"] in main_body,
        f"{main_path} missing {meta['start']!r} — rolling old binaries onto this compose "
        f"leaves nobody writing {cert_vol}",
    )

    seen_subjects[parent] = subject
    seen_provisioners[parent] = provisioner
    seen_password_files[parent] = password_file
    seen_secret_names[parent] = secret_id

check(
    "in-process CERT_SUBJECT values are distinct",
    len(set(seen_subjects.values())) == len(INPROCESS_PARENTS),
    f"{seen_subjects!r}",
)
check(
    "in-process provisioner names are distinct",
    len(set(seen_provisioners.values())) == len(INPROCESS_PARENTS),
    f"{seen_provisioners!r}",
)
check(
    "in-process JWK secret files are distinct",
    len(set(seen_password_files.values())) == len(INPROCESS_PARENTS),
)
check(
    "in-process compose secret names are distinct",
    len(set(seen_secret_names.values())) == len(INPROCESS_PARENTS),
    f"{seen_secret_names!r}",
)

ca_workloads = {
    name
    for name, svc in services.items()
    if name not in {"step-ca", "step-ca-bootstrap"}
    and "step_ca_root_password" in secret_names(svc)
}
check(
    "shared step_ca_root_password is confined to step-ca, not workloads",
    ca_workloads == set(),
    f"workloads mounting root secret: {sorted(ca_workloads)}",
)

shared_prov = {
    name
    for name, svc in services.items()
    if name not in {"step-ca", "step-ca-bootstrap"}
    and env_map(svc).get("STEP_CA_PROVISIONER") == "pki-agent"
}
check(
    "no current compose workload uses shared STEP_CA_PROVISIONER=pki-agent",
    shared_prov == set(),
    f"{sorted(shared_prov)}",
)

print("observability: parent :9110, no sidecar :9510, preserve non-PKI scrapes")

prom_cfg = load_yaml(ROOT / "observability" / "prometheus" / "prometheus.yml")
rules_cfg = load_yaml(
    ROOT / "observability" / "prometheus" / "rules" / "pki-agent-alerts.yml"
)
obs_spec = importlib.util.spec_from_file_location(
    "obs_audit", ROOT / "scripts" / "observability-config-audit.py"
)
assert obs_spec is not None and obs_spec.loader is not None
obs_audit = importlib.util.module_from_spec(obs_spec)
obs_spec.loader.exec_module(obs_audit)
pki_violations = obs_audit.audit_pki_ops_surface(prom_cfg, rules_cfg)
check(
    "observability YAML pin: 14 parent:9110 jobs, /metrics, no pki-agent, 14 absent()",
    pki_violations == [],
    "; ".join(pki_violations),
)
all_targets: list[str] = []
for job in prom_cfg.get("scrape_configs") or []:
    if not isinstance(job, dict):
        continue
    for block in job.get("static_configs") or []:
        if not isinstance(block, dict):
            continue
        for target in block.get("targets") or []:
            if isinstance(target, str):
                all_targets.append(target)

check(
    "prometheus scrape_configs have no pki-agent job or pki-agent targets",
    not any(
        isinstance(job, dict) and job.get("job_name") == "pki-agent"
        for job in (prom_cfg.get("scrape_configs") or [])
    )
    and not any(t.startswith("pki-agent") for t in all_targets),
    f"targets={all_targets!r}",
)
for preserved in PRESERVED_NON_PKI_SCRAPES:
    check(
        f"prometheus still scrapes non-PKI {preserved}",
        preserved in all_targets,
        f"{preserved} missing — silent drop of existing metrics",
    )


runbook = (ROOT / "docs" / "runbooks" / "pki-agent-recovery.md").read_text(encoding="utf-8")
check(
    "recovery runbook documents 14 in-process parents and 0 workload sidecars",
    "14" in runbook and "in-process" in runbook.lower() and "0" in runbook,
    "runbook still describes a mixed sidecar fleet",
)
check(
    "recovery runbook names all 14 operator JWK files",
    all(f"pki-agent-{name}-jwk.txt" in runbook for name in INPROCESS_PARENTS),
    "missing operator secret-file instructions for the final 14",
)
check(
    "recovery runbook identifies old images + new compose as unsafe",
    "unsafe" in runbook.lower() and "old image" in runbook.lower(),
    "missing rolling-compat stop: old image cannot enroll, new compose has no sidecar",
)
check(
    "recovery runbook documents deploy order and rollback stop",
    "deploy order" in runbook.lower() and "rollback" in runbook.lower(),
    "missing deploy-order / rollback-stop instructions",
)
check(
    "recovery runbook restore-sidecars-before-old-images on rollback",
    "sidecar" in runbook.lower() and "rollback" in runbook.lower(),
    "rollback must restore sidecars/compose before old images",
)
check(
    "recovery runbook does not add a plaintext fallback",
    "plaintext fallback" not in runbook.lower()
    and "InsecureSkipVerify" not in runbook,
)

print(f"\n{safe_log.PASS} passed, {safe_log.FAIL} failed")
sys.exit(1 if safe_log.FAIL else 0)
