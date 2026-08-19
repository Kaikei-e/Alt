#!/usr/bin/env python3
"""Structural pins for docs/runbooks/pki-agent-recovery.md.

Forward cutover/recovery must stop+rm leftover `alt` project pki-agent-*
containers (exact compose labels) and fail-closed on a zero docker ps
BEFORE any PKI_ENROLLMENT=enabled parent `up`. `compose up --remove-orphans`
does not provide that ordering and is not required.

Must chown emergency leaves to the parent runtime UID, wipe every compose
cert volume, and probe :9110 from a toolbox on the Compose network
(parent images are distroless and have no wget).

Does not start Docker. Compose is the source of truth for the 14 volumes
and the 65532/1000/999 ownership map.

Run:
    python3 scripts/tests/test-pki-agent-recovery-runbook.py
"""

from __future__ import annotations

import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))

from compose_include import load_yaml, production_compose_files, production_services  # noqa: E402
import yaml  # noqa: E402

RUNBOOK = ROOT / "docs" / "runbooks" / "pki-agent-recovery.md"
WORKFLOW = ROOT / ".github" / "workflows" / "compose-audit.yaml"
COMPOSE_PROJECT = "alt"
RUNBOOK_REL = "docs/runbooks/pki-agent-recovery.md"
TEST_REL = "scripts/tests/test-pki-agent-recovery-runbook.py"
PKI_COMPOSE_REL = "compose/pki.yaml"
COMPOSE_YAML_GLOB = "compose/**.yaml"
WORKFLOW_REL = ".github/workflows/compose-audit.yaml"
PKI_SCRIPTS_GLOB = "pki-agent/scripts/**"
DEPLOY_SH = "scripts/deploy.sh"
LEFTOVER_SWEEP = "scripts/retire-alt-pki-agent-leftovers.sh"
TEST_DEPLOY = "tests/scripts/test_deploy.sh"
TEST_CASCADE = "tests/scripts/test_cascade_pki_sidecars.sh"
PYTHON3_TEST = re.compile(r"^python3\s+(\S+)\s*$")
BASH_TEST = re.compile(r"^bash\s+(\S+\.sh)\s*$")

PASS = 0
FAIL = 0

# pre_start must chown the volume *contents* (`-R`). A directory-only
# chown leaves 0400 keys owned by root after a previous mint, and the
# parent cannot rewrite them. Optional -R so an older non-recursive
# form still parses until every stack file has moved.
CHOWN_CERTS = re.compile(r"chown\s+(?:-R\s+)?(\d+):(\d+)\s+/certs")
BASH_FENCE = re.compile(r"```(?:bash)?\n(.*?)```", re.S)
ASSOC_ARRAY = re.compile(
    r"declare\s+-A\s+(?P<name>[A-Z_]+)=\((?P<body>.*?)\)",
    re.S,
)
ASSOC_ENTRY = re.compile(r"\[(?P<key>[^\]]+)\]=(?P<val>[^\s]+)")
COMPOSE_UP = re.compile(r"docker\s+compose\b.*\bup\b", re.I)
DOCKER_EXEC_WGET = re.compile(r"docker\s+exec\b[^\n]*\bwget\b")
SWEEP_FN = "retire_alt_pki_agent_leftovers"
PROJECT_LABEL = "com.docker.compose.project=alt"
SERVICE_LABEL = "com.docker.compose.service"
STOP_EXACT = re.compile(r"docker\s+stop\s+--")
RM_EXACT = re.compile(r"docker\s+rm\s+-f\s+--")


def check(name: str, condition: bool, detail: str = "") -> None:
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
        return
    print(f"  FAIL  {name}")
    if detail:
        print(f"        {detail}")
    FAIL += 1


def bash_fences(md: str) -> list[str]:
    return BASH_FENCE.findall(md)


def join_continuations(block: str) -> str:
    return re.sub(r"\\\n", " ", block)


def parse_assoc_arrays(md: str) -> dict[str, dict[str, str]]:
    found: dict[str, dict[str, str]] = {}
    for match in ASSOC_ARRAY.finditer(md):
        body = re.sub(r"#.*", "", match.group("body"))
        entries = {
            m.group("key"): m.group("val").strip("'\"")
            for m in ASSOC_ENTRY.finditer(body)
        }
        found[match.group("name")] = entries
    return found


def env_map(svc: dict) -> dict[str, str]:
    raw = svc.get("environment")
    if isinstance(raw, dict):
        return {str(k): str(v) for k, v in raw.items()}
    out: dict[str, str] = {}
    if isinstance(raw, list):
        for item in raw:
            if isinstance(item, str) and "=" in item:
                key, value = item.split("=", 1)
                out[key] = value
    return out


def cert_volume_for(svc: dict) -> str | None:
    for item in svc.get("volumes") or []:
        if isinstance(item, str):
            parts = item.split(":")
            if len(parts) >= 2 and parts[1] == "/certs" and parts[0].endswith("_certs"):
                return parts[0]
            continue
        if not isinstance(item, dict):
            continue
        source = str(item.get("source") or "")
        target = str(item.get("target") or "")
        if target == "/certs" and source.endswith("_certs"):
            return source
    return None


def hook_command_text(hook: dict) -> str:
    command = hook.get("command")
    if isinstance(command, list):
        return " ".join(str(part) for part in command)
    if command is None:
        return ""
    return str(command)


def load_github_workflow(path: pathlib.Path) -> dict:
    data = yaml.safe_load(path.read_text(encoding="utf-8"))
    return data if isinstance(data, dict) else {}


def workflow_on(cfg: dict) -> dict:
    # YAML 1.1 treats the key `on` as boolean true.
    raw = cfg.get("on")
    if raw is None:
        raw = cfg.get(True)
    return raw if isinstance(raw, dict) else {}


def event_paths(on: dict, event: str) -> set[str]:
    block = on.get(event) or {}
    if not isinstance(block, dict):
        return set()
    return {str(p) for p in (block.get("paths") or [])}


def covers_compose_pki(paths: set[str]) -> bool:
    return PKI_COMPOSE_REL in paths or COMPOSE_YAML_GLOB in paths


def executed_run_scripts(cfg: dict) -> set[str]:
    """Script paths invoked by a job step `run:` line.

    Comments are stripped by the YAML parser, so a commented-out mention
    of this test does not count as an executed step.
    """
    found: set[str] = set()
    for job in (cfg.get("jobs") or {}).values():
        if not isinstance(job, dict):
            continue
        for step in job.get("steps") or []:
            if not isinstance(step, dict):
                continue
            run = step.get("run")
            if not isinstance(run, str):
                continue
            for line in run.splitlines():
                stripped = line.strip()
                if not stripped or stripped.startswith("#"):
                    continue
                match = PYTHON3_TEST.match(stripped) or BASH_TEST.match(stripped)
                if match:
                    found.add(match.group(1))
    return found


def executed_python_tests(cfg: dict) -> set[str]:
    return {p for p in executed_run_scripts(cfg) if p.endswith(".py")}


def pre_start_certs_uid(svc: dict) -> str | None:
    for hook in svc.get("pre_start") or []:
        if not isinstance(hook, dict):
            continue
        match = CHOWN_CERTS.search(hook_command_text(hook))
        if match:
            return match.group(1)
    return None


def compose_cert_volumes() -> dict[str, str]:
    """compose volume name -> docker volume name (`-p alt`)."""
    names: dict[str, str] = {}
    for path in production_compose_files():
        data = load_yaml(path)
        for name in data.get("volumes") or {}:
            if str(name).endswith("_certs"):
                names[str(name)] = f"{COMPOSE_PROJECT}_{name}"
    return names


def has_exact_label_sweep(text: str) -> bool:
    """Project=alt + service prefix pki-agent- + stop/rm --. Not a name= glob."""
    return (
        PROJECT_LABEL in text
        and SERVICE_LABEL in text
        and "pki-agent-" in text
        and STOP_EXACT.search(text) is not None
        and RM_EXACT.search(text) is not None
    )


def has_zero_ps_gate(text: str) -> bool:
    return (
        PROJECT_LABEL in text
        and "pki-agent-" in text
        and "exit 1" in text
        and "docker ps" in text
    )


def sweep_function_body(md: str) -> str:
    match = re.search(rf"{re.escape(SWEEP_FN)}\(\)\s*\{{(.*?)^\}}", md, re.S | re.M)
    return match.group(0) if match else ""


def rollback_restore_section(md: str) -> str:
    """Rollback stop through the restore snippet, not the rest of the runbook."""
    idx = md.find("Rollback stop")
    if idx < 0:
        return ""
    rest = md[idx:]
    nxt = re.search(r"\n(?:## |### |許可される)", rest[1:])
    return rest if nxt is None else rest[: nxt.start() + 1]


def calls_sweep(block: str) -> bool:
    """True when the helper is invoked, not merely named in a comment."""
    for line in join_continuations(block).splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if stripped == SWEEP_FN or stripped.startswith(f"{SWEEP_FN} ") or stripped.startswith(f"{SWEEP_FN}()"):
            return True
    return False


def is_forward_parent_up_fence(block: str) -> bool:
    joined = join_continuations(block)
    if not COMPOSE_UP.search(joined):
        return False
    return (
        "--force-recreate" in joined
        or "SUBJECTS[@]" in joined
        or "<subject>" in joined
    )


def inprocess_parents() -> dict[str, tuple[str, str]]:
    """service -> (compose cert volume, runtime uid)."""
    parents: dict[str, tuple[str, str]] = {}
    for name, svc in production_services().items():
        if env_map(svc).get("PKI_ENROLLMENT") != "enabled":
            continue
        volume = cert_volume_for(svc)
        uid = pre_start_certs_uid(svc)
        if volume and uid:
            parents[name] = (volume, uid)
    return parents


print("pki-agent recovery runbook structure")

check(
    "pre_start chown regex matches recursive -R (volume contents, not just the dir)",
    CHOWN_CERTS.search(
        "mkdir -p /certs && chown -R 65532:65532 /certs && chmod 0750 /certs"
    )
    is not None,
)
check(
    "pre_start chown regex still matches a non-recursive chown of /certs",
    CHOWN_CERTS.search("chown 65532:65532 /certs") is not None,
)

runbook = RUNBOOK.read_text(encoding="utf-8")
parents = inprocess_parents()
cert_volumes = compose_cert_volumes()
assoc = parse_assoc_arrays(runbook)

check(
    "runbook file exists",
    RUNBOOK.is_file(),
    str(RUNBOOK),
)
check(
    "compose declares exactly 14 cert volumes",
    len(cert_volumes) == 14,
    f"got {sorted(cert_volumes)}",
)
check(
    "compose has exactly 14 in-process PKI parents",
    len(parents) == 14,
    f"got {sorted(parents)}",
)
check(
    "runtime UIDs are the 65532/1000/999 set",
    {uid for _, uid in parents.values()} <= {"65532", "1000", "999"}
    and {"65532", "1000", "999"} <= {uid for _, uid in parents.values()},
    f"uids={sorted({uid for _, uid in parents.values()})}",
)

cert_uid = assoc.get("CERT_UID", {})
cert_volume = assoc.get("CERT_VOLUME", {})
check(
    "runbook CERT_UID maps all 14 parents to compose pre_start UIDs",
    cert_uid == {name: uid for name, (_, uid) in parents.items()},
    f"runbook={dict(sorted(cert_uid.items()))} compose={ {n: u for n, (_, u) in sorted(parents.items())} }",
)
expected_docker_vols = {
    name: f"{COMPOSE_PROJECT}_{volume}" for name, (volume, _) in parents.items()
}
check(
    "runbook CERT_VOLUME maps all 14 parents to compose docker volume names",
    cert_volume == expected_docker_vols,
    f"runbook={dict(sorted(cert_volume.items()))} compose={dict(sorted(expected_docker_vols.items()))}",
)

runbook_vol_tokens = set(re.findall(r"alt_[a-z0-9_]+_certs", runbook))
check(
    "runbook names every compose cert docker volume (incl. alt-notifier)",
    set(cert_volumes.values()) <= runbook_vol_tokens,
    f"missing {sorted(set(cert_volumes.values()) - runbook_vol_tokens)}",
)
check(
    "fleet wipe iterates CERT_VOLUME rather than a drifting literal list",
    "CERT_VOLUME[@]" in runbook or "CERT_VOLUME[*]" in runbook,
    "wipe loop must expand CERT_VOLUME so alt-notifier cannot drop out",
)

sweep_body = sweep_function_body(runbook)
check(
    f"forward retirement helper `{SWEEP_FN}` uses exact project+service labels then stop/rm --",
    has_exact_label_sweep(sweep_body)
    and has_zero_ps_gate(sweep_body)
    and "docker ps -a" in sweep_body
    and "pki-agent-*" in sweep_body,
    "need docker ps -a --filter label=com.docker.compose.project=alt, "
    "format compose.service, pki-agent-* prefix, docker stop -- / rm -f --, then exit 1",
)
check(
    "retirement sweep does not glob other projects via name=pki-agent",
    "name=pki-agent" not in runbook,
    "name= filter can match non-alt projects; use compose project+service labels",
)
check(
    "matching pki-agent=0 is not an automatic fresh install",
    "ALT_ACK_FRESH_INSTALL=1" in sweep_body
    and "fresh-install or already retired" not in sweep_body,
    "empty project listing needs exact ACK; visible anchors are a steady no-op",
)
check(
    "rollback does not require ALT_ACK_FRESH_INSTALL (do not weaken alt-deploy)",
    "ALT_ACK_FRESH_INSTALL" not in rollback_restore_section(runbook),
)

fn_def_at = runbook.find(f"{SWEEP_FN}()")
parent_up_fences = [
    join_continuations(block)
    for block in bash_fences(runbook)
    if is_forward_parent_up_fence(block)
]
check(
    "at least one forward parent up fence exists (cutover/recovery)",
    bool(parent_up_fences),
    "Step 4 / fleet recovery must compose-up enrollment parents",
)
missing_sweep_before_up: list[str] = []
for fence in parent_up_fences:
    up_match = COMPOSE_UP.search(fence)
    if up_match is None:
        continue
    prefix = fence[: up_match.start()]
    if not calls_sweep(prefix):
        missing_sweep_before_up.append(fence[:120])
check(
    "every PKI_ENROLLMENT parent up is preceded by the retirement sweep+zero gate",
    not missing_sweep_before_up and fn_def_at >= 0,
    f"missing call before up: {missing_sweep_before_up}",
)

rollback_section = rollback_restore_section(runbook)
rollback_fences = [join_continuations(b) for b in BASH_FENCE.findall(rollback_section)]
rollback_ups = [f for f in rollback_fences if COMPOSE_UP.search(f)]
check(
    "rollback restores sidecar-era compose/sidecars and does not sweep after restore",
    bool(rollback_ups)
    and all(not calls_sweep(fence) for fence in rollback_ups)
    and STOP_EXACT.search(rollback_section) is None
    and RM_EXACT.search(rollback_section) is None
    and ("sidecar" in rollback_section.lower() or "pki-agent" in rollback_section),
    "rollback up must restore target sidecars before old parent images; no stop/rm sweep after",
)
check(
    "rollback compose up is --no-deps pki-agent sidecars, not whole-stack up -d",
    bool(rollback_ups)
    and all("--no-deps" in fence and "pki-agent" in fence for fence in rollback_ups),
    "mixed-mode SHAs already had PKI_ENROLLMENT=enabled on some parents; "
    "whole-stack up -d recreates the world on current images",
)
check(
    "rollback pins images to a sidecar-era SHA (alt-deploy or exact phases)",
    "SHA" in rollback_section
    and (
        "alt-deploy" in rollback_section.lower()
        or "deploy.md" in rollback_section
        or "docs/runbooks/deploy" in rollback_section
        or "image pin" in rollback_section.lower()
        or "イメージを同じ SHA" in rollback_section
        or "同じ SHA" in rollback_section
    ),
    "delegate to SHA-aligned alt-deploy / deploy.md, or write exact safe phases",
)
check(
    "wikilink is [[000747]] not [[ADR-000747]]",
    "[[ADR-000747]]" not in runbook and "[[000747]]" in runbook,
)
check(
    "Step 6 does not instruct editing 000747 Decision",
    "000747]] を更新" not in runbook
    and "Update ADR-000747" not in runbook
    and "ADR-000747 を更新" not in runbook,
    "evaluate a new ADR or postmortem; do not edit [[000747]] Decision",
)
check(
    "Pattern B leftover is not described as live cert-only netns topology",
    "sidecars are cert-only on their own netns" not in runbook,
    "inbound TLS is in the parent; leftover pki-agent-* is a dual writer",
)

leftover_ps = PROJECT_LABEL in runbook and "docker ps" in runbook
check(
    "runbook does not treat Prometheus pki-agent scrape as leftover detector",
    leftover_ps
    and (
        "scrape" in runbook.lower()
        or "9510" in runbook
        or "prometheus" in runbook.lower()
    )
    and (
        "頼ら" in runbook
        or "do not rely" in runbook.lower()
        or "not rely" in runbook.lower()
        or "no longer scrape" in runbook.lower()
        or "scrape しない" in runbook
        or "scrapeしない" in runbook
    ),
    "say leftover detection is docker ps labels; Prometheus no longer scrapes pki-agent",
)

check(
    "runbook has no docker exec ... wget (parent images are distroless)",
    DOCKER_EXEC_WGET.search(runbook) is None,
    "probe :9110 from a toolbox container on the Compose network",
)

probe_blocks = [
    join_continuations(block)
    for block in bash_fences(runbook)
    if ":9110" in block and "metrics" in block
]
toolbox_probe = any(
    "docker run" in block
    and "--network" in block
    and ("alt_alt-network" in block or "alt-network" in block)
    and ":9110" in block
    and "wget" in block
    for block in probe_blocks
)
loop_subjects: set[str] = set()
for block in probe_blocks:
    listed = re.search(r"for\s+s\s+in\s+(.*?);", block, re.S)
    if listed:
        loop_subjects.update(re.findall(r"[A-Za-z0-9.-]+", listed.group(1)))
    if re.search(r"http://\$\{?s\}?:9110", block):
        loop_subjects.update(parents)
check(
    "toolbox probe hits all 14 parent :9110 endpoints via Compose DNS",
    toolbox_probe and set(parents) <= loop_subjects,
    f"probe_ok={toolbox_probe} missing={sorted(set(parents) - loop_subjects)}",
)

check(
    "emergency mint sets cert 0444 and key 0400",
    "chmod 0444 /certs/svc-cert.pem" in runbook
    and "chmod 0400 /certs/svc-key.pem" in runbook,
)
check(
    "emergency mint chown uses CERT_UID, not a blanket 65532",
    "CERT_UID" in runbook
    and not re.search(r"chown\s+65532:65532\s+/certs/svc-cert", runbook),
    "copyable mint must chown from the per-subject UID table",
)

print("compose-audit workflow wiring")
workflow_cfg = load_github_workflow(WORKFLOW)
on = workflow_on(workflow_cfg)
executed = executed_python_tests(workflow_cfg)
check(
    "compose-audit.yaml exists and parses as YAML mapping",
    WORKFLOW.is_file() and bool(workflow_cfg.get("jobs")),
    str(WORKFLOW),
)
for event in ("push", "pull_request"):
    paths = event_paths(on, event)
    check(
        f"{event} path filter includes recovery runbook",
        RUNBOOK_REL in paths,
        f"missing {RUNBOOK_REL} in {sorted(paths)}",
    )
    check(
        f"{event} path filter includes recovery runbook test",
        TEST_REL in paths,
        f"missing {TEST_REL}",
    )
    check(
        f"{event} path filter covers compose PKI files",
        covers_compose_pki(paths),
        f"need {PKI_COMPOSE_REL} or {COMPOSE_YAML_GLOB}",
    )
    check(
        f"{event} path filter includes compose-audit workflow",
        WORKFLOW_REL in paths,
        f"missing {WORKFLOW_REL}",
    )
check(
    "compose-audit executes recovery runbook test via run: (not a comment)",
    TEST_REL in executed,
    f"python3 steps={sorted(executed)}",
)
executed_all = executed_run_scripts(workflow_cfg)
for required in (PKI_SCRIPTS_GLOB, DEPLOY_SH, LEFTOVER_SWEEP, TEST_DEPLOY, TEST_CASCADE):
    for event in ("push", "pull_request"):
        paths = event_paths(on, event)
        check(
            f"{event} path filter includes {required}",
            required in paths,
            f"missing {required} in {sorted(paths)}",
        )
check(
    "compose-audit executes test_deploy.sh via run: (not a comment)",
    TEST_DEPLOY in executed_all,
    f"run scripts={sorted(executed_all)}",
)
check(
    "compose-audit executes test_cascade_pki_sidecars.sh via run: (not a comment)",
    TEST_CASCADE in executed_all,
    f"run scripts={sorted(executed_all)}",
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
