#!/usr/bin/env python3
"""Deterministic Wave 4 provisioner-mapping tests (no live step-ca).

Parses bootstrap/verify scripts as text. Docker-free helper functions are
extracted from the source and run on their own; the scripts themselves never
run. Does not start Docker, does not print or invent secret bytes.
"""

from __future__ import annotations

import pathlib
import re
import stat
import subprocess
import sys
import tempfile

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
    "knowledge-sovereign",
    "recap-evaluator",
)


def extract_subjects(text: str, array_name: str) -> list[str]:
    m = re.search(rf"{array_name}=\(\n(.*?)^\)", text, re.S | re.M)
    if not m:
        return []
    body = re.sub(r"#.*", "", m.group(1))
    return re.findall(r"^\s*([A-Za-z0-9.-]+)\s*$", body, re.M)


def shell_function(text: str, name: str) -> str:
    m = re.search(rf"^{name}\(\) \{{\n.*?^\}}\n", text, re.S | re.M)
    return m.group(0) if m else ""


def run_shell_function(definition: str, call: str, *args: str) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        ["bash", "-c", f"{definition}\n{call}", "pki-test", *args],
        capture_output=True,
        check=False,
    )


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

print("bootstrap host password files stay readable by the deploy runner")
ensure_pw = shell_function(bootstrap, "ensure_host_password_file")
with tempfile.TemporaryDirectory() as tmp:
    new_file = pathlib.Path(tmp) / "store" / "pki-agent-example-jwk.txt"
    # A restrictive caller umask must not leak into the canonical store mode.
    created = run_shell_function(ensure_pw, 'umask 077; ensure_host_password_file "$1"', str(new_file))
    check(
        "a new host password file is created non-empty with mode 0644",
        created.returncode == 0
        and new_file.is_file()
        and new_file.stat().st_size > 0
        and stat.S_IMODE(new_file.stat().st_mode) == 0o644,
        f"rc={created.returncode}",
    )
    existing = pathlib.Path(tmp) / "pki-agent-existing-jwk.txt"
    existing.write_bytes(b"existing-placeholder\n")
    existing.chmod(0o640)
    kept = run_shell_function(ensure_pw, 'ensure_host_password_file "$1"', str(existing))
    check(
        "an existing host password file keeps its bytes and its mode",
        kept.returncode == 0
        and existing.read_bytes() == b"existing-placeholder\n"
        and stat.S_IMODE(existing.stat().st_mode) == 0o640,
        f"rc={kept.returncode}",
    )

print("the X.509 template never touches a fixed host path")
# The template decides which SANs a provisioner may sign. A predictable host
# path under /tmp can be pre-created or symlinked by any local user between the
# write and the `docker cp`, so the bytes are streamed into step-ca instead.
check(
    "bootstrap writes nothing under the host /tmp",
    not re.search(r'>\s*"?/tmp/', bootstrap) and ".tpl.host" not in bootstrap,
)
check(
    "bootstrap does not `docker cp` the template from a host file",
    "docker cp" not in bootstrap,
)
copy_tpl = shell_function(bootstrap, "copy_template_into_ca")
check(
    "copy_template_into_ca streams render_x509_template over docker exec stdin",
    "render_x509_template" in copy_tpl and 'docker exec -i -u 0 "$STEP_CA" sh -c' in copy_tpl,
)
render_m = re.search(r"^render_x509_template\(\) \{\n.*?^  fi\n\}\n", bootstrap, re.S | re.M)
render_tpl = render_m.group(0) if render_m else ""
for subject, sans in (("alt-backend", 2), ("localhost", 1)):
    rendered = run_shell_function(render_tpl, 'render_x509_template "$1"', subject)
    out = rendered.stdout.decode("utf-8", "replace")
    check(
        f"render_x509_template {subject} pins CN, token sub and SAN count",
        rendered.returncode == 0
        and f'"commonName": "{subject}"' in out
        and f'{{{{- if ne .Token.sub "{subject}" }}}}' in out
        and f"{{{{- if gt (len .Token.sans) {sans} }}}}" in out
        and "{{- $has_subject := false }}" in out,
        f"rc={rendered.returncode}",
    )
with tempfile.TemporaryDirectory() as tmp:
    target = pathlib.Path(tmp) / "pki-agent-alt-backend.tpl"
    fake_exec = 'docker() { [ "$1 $2" = "exec -i" ] || return 9; shift 7; sh -c "$1"; }'
    streamed = run_shell_function(
        f"{fake_exec}\n{render_tpl}\n{copy_tpl}",
        'STEP_CA=step-ca; copy_template_into_ca alt-backend "$1"',
        str(target),
    )
    expected = run_shell_function(render_tpl, 'render_x509_template "$1"', "alt-backend").stdout
    check(
        "copy_template_into_ca lands exactly the rendered template in the container path",
        streamed.returncode == 0 and target.is_file() and target.read_bytes() == expected,
        f"rc={streamed.returncode}",
    )

boot_subjects = extract_subjects(bootstrap, "SUBJECTS")
verify_cns = extract_subjects(verify, "EXPECTED_CNS")
check(
    "bootstrap SUBJECTS and verify EXPECTED_CNS stay in lockstep",
    boot_subjects == verify_cns and boot_subjects[-1:] == ["localhost"],
    f"bootstrap={boot_subjects!r} verify={verify_cns!r}",
)
check(
    "all 16 production workload CNs are in the allowlist",
    set(WORKLOAD).issubset(set(boot_subjects)) and len(WORKLOAD) == 16,
    f"missing={sorted(set(WORKLOAD) - set(boot_subjects))}",
)

print("compose declares 16 subject-scoped JWK secrets; no shared workload provisioner")

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
    "compose declares no pki-agent-* workload (every subject enrolls in-process)",
    pki_agents == [],
    f"{pki_agents}",
)
for subject in WORKLOAD:
    env = _env_map(prod.get(subject) or {})
    check(
        f"{subject} enrolls in-process with its own provisioner",
        env.get("PKI_ENROLLMENT") == "enabled"
        and env.get("STEP_CA_PROVISIONER") == f"pki-agent-{subject}"
        and f"pki-agent-{subject}-jwk" in _secret_names(prod.get(subject) or {}),
        f"PKI_ENROLLMENT={env.get('PKI_ENROLLMENT')!r} provisioner={env.get('STEP_CA_PROVISIONER')!r}",
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

print("verify rejection assertions prove no certificate was issued")
attempt = shell_function(verify, "attempt_issue")
rejected_with = shell_function(verify, "rejected_with")
check(
    "attempt_issue runs the request as root inside step-ca",
    'docker exec -u 0 "$STEP_CA" sh -c' in attempt,
)
check(
    "attempt_issue clears /tmp/c.pem /tmp/k.pem before and after the request",
    attempt.count("rm -f /tmp/c.pem /tmp/k.pem") >= 2,
)
check(
    "attempt_issue reports NO_TOKEN instead of requesting with an empty token",
    "NO_TOKEN" in attempt and "step ca certificate" in attempt and "--force" in attempt,
)
check(
    "attempt_issue says REJECTED only on non-zero exit with no cert file, else ISSUED",
    "-ne 0" in attempt and "! -e /tmp/c.pem" in attempt and "REJECTED" in attempt and "ISSUED" in attempt,
)
check(
    "verify no longer greps a pipe whose exit status is discarded",
    "not allowed|forbidden|denied|policy" not in verify,
)
check(
    "assertion 3 expects the template's sub mismatch rejection",
    'rejected_with "$out" "sub mismatch"' in verify
    and "Invalid OTT claims: sub mismatch" in bootstrap,
)
check(
    "assertion 4 expects the template's unapproved SAN rejection",
    'rejected_with "$out" "unapproved SAN"' in verify
    and "Invalid OTT claims: unapproved SAN" in bootstrap,
)

REJECTION_CASES = (
    ("template rejection", "Invalid OTT claims: sub mismatch\nREJECTED\n", "sub mismatch", True),
    ("SAN rejection", "Invalid OTT claims: unapproved SAN\nREJECTED\n", "unapproved SAN", True),
    ("certificate issued", "Invalid OTT claims: sub mismatch\nISSUED\n", "sub mismatch", False),
    ("rejected for another reason", "not allowed by policy\nREJECTED\n", "sub mismatch", False),
    ("no token minted", "NO_TOKEN\n", "sub mismatch", False),
    ("no output", "", "sub mismatch", False),
    ("verdict not last", "sub mismatch\nREJECTED\nError response from daemon\n", "sub mismatch", False),
)
for label, output, expected, should_pass in REJECTION_CASES:
    result = run_shell_function(rejected_with, 'rejected_with "$1" "$2"', output, expected)
    check(
        f"rejected_with: {label} -> {'PASS' if should_pass else 'FAIL'}",
        (result.returncode == 0) is should_pass,
        f"rc={result.returncode}",
    )

print("verify leaves no signed leaf or key behind in the step-ca container")
check(
    "every step ca certificate call goes through attempt_issue's cleanup",
    verify.count("step ca certificate") == 1 and attempt.count("step ca certificate") == 1,
    f"verify={verify.count('step ca certificate')}",
)
check(
    "assertion 5 (smoke) signs through attempt_issue and passes only on ISSUED",
    'out="$(attempt_issue "$SMOKE_SUBJECT" "$SMOKE_SUBJECT")"' in verify
    and 'tail -n 1)" = "ISSUED"' in verify,
)

# Run attempt_issue against a fake `docker` (runs the in-container script
# locally) and a fake `step`, with the cert paths moved into a temp dir.
FAKE_STEP = """#!/bin/sh
case "$1 $2" in
  "ca token") [ "$MODE" = notoken ] && exit 1; echo fake-token; exit 0 ;;
  "ca certificate")
    if [ "$MODE" = issue ]; then echo cert > "$4"; echo key > "$5"; exit 0; fi
    echo "Invalid OTT claims: sub mismatch" >&2; exit 1 ;;
esac
exit 2
"""
with tempfile.TemporaryDirectory() as tmp:
    tmpdir = pathlib.Path(tmp)
    (tmpdir / "bin").mkdir()
    fake_step = tmpdir / "bin" / "step"
    fake_step.write_text(FAKE_STEP, encoding="utf-8")
    fake_step.chmod(0o755)
    cert, key = tmpdir / "c.pem", tmpdir / "k.pem"
    local_attempt = attempt.replace("/tmp/c.pem", str(cert)).replace("/tmp/k.pem", str(key))
    fake_docker = 'docker() { shift 4; PATH="$FAKE_BIN:$PATH" sh -c "$3"; }'
    for mode, verdict in (("issue", "ISSUED"), ("reject", "REJECTED"), ("notoken", "NO_TOKEN")):
        cert.write_text("stale\n", encoding="utf-8")
        key.write_text("stale\n", encoding="utf-8")
        result = run_shell_function(
            f"{fake_docker}\n{local_attempt}",
            'export MODE="$1" FAKE_BIN="$2"; STEP_CA=step-ca SMOKE_PROVISIONER=pki-agent-alt-backend '
            'SMOKE_PW_CA=/nonexistent; attempt_issue alt-backend alt-backend',
            mode,
            str(tmpdir / "bin"),
        )
        lines = result.stdout.decode("utf-8", "replace").strip().splitlines()
        check(
            f"attempt_issue ({mode}) ends with {verdict} and leaves no cert or key file",
            lines[-1:] == [verdict] and not cert.exists() and not key.exists(),
            f"last={lines[-1:]!r} cert={cert.exists()} key={key.exists()}",
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
