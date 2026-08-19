#!/usr/bin/env python3
"""Tests for Wave 1b chown-only `pre_start` migration.

Static checks pin the two consumers. Live isolated fixtures (60s) prove
`up --no-deps --force-recreate` runs the hook and a failing hook blocks
start. Live tests use a locally present alpine image and never touch
project `alt`.

Run:
    python3 scripts/tests/test-compose-pre-start-audit.py
"""

from __future__ import annotations

import importlib.util
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
import time

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))

from compose_include import production_services  # noqa: E402

spec = importlib.util.spec_from_file_location(
    "pre_start_audit", SCRIPTS / "compose-pre-start-audit.py"
)
assert spec is not None and spec.loader is not None
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)

PASS = 0
FAIL = 0
LIVE_TIMEOUT = 60
ALPINE_CANDIDATES = ("alpine:3.21", "alpine:3.20", "alpine:latest")


def check(name, condition):
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
    else:
        print(f"  FAIL  {name}")
        FAIL += 1


print("is_chown_hook")
HOOK = {
    "image": "alpine:3.21",
    "user": "0:0",
    "command": [
        "sh",
        "-c",
        "mkdir -p /home/ollama-user/.ollama && chown -R 2000:2000 /home/ollama-user/.ollama",
    ],
}
check(
    "alpine chown of the models path is accepted",
    audit.is_chown_hook(HOOK, "/home/ollama-user/.ollama", "2000:2000"),
)
check(
    "inherited ollama image is not an appropriate chown image",
    not audit.is_chown_hook(
        {
            "image": "ollama/ollama:0.22.1",
            "command": ["chown -R 2000:2000 /home/ollama-user/.ollama"],
        },
        "/home/ollama-user/.ollama",
        "2000:2000",
    ),
)
check(
    "echo is not a chown hook",
    not audit.is_chown_hook(
        {"image": "alpine:3.21", "command": ["echo", "ok"]},
        "/home/ollama-user/.ollama",
        "2000:2000",
    ),
)

PKI_HOOK = {
    "image": "alpine:3.21",
    "user": "0:0",
    "command": [
        "sh",
        "-c",
        "mkdir -p /certs && chown -R 65532:65532 /certs && chmod 0750 /certs",
    ],
}
check(
    "distroless cert volume chown 0750 is accepted",
    audit.is_pki_cert_chown_hook(PKI_HOOK, "/certs", "65532:65532"),
)
check(
    "world-writable 0777 cert dir is rejected",
    not audit.is_pki_cert_chown_hook(
        {
            "image": "alpine:3.21",
            "command": ["sh", "-c", "chown 65532:65532 /certs && chmod 0777 /certs"],
        },
        "/certs",
        "65532:65532",
    ),
)

check(
    "a chown that skips the files already in the volume is rejected",
    not audit.is_pki_cert_chown_hook(
        {
            "image": "alpine:3.21",
            "command": [
                "sh",
                "-c",
                "mkdir -p /certs && chown 65532:65532 /certs && chmod 0750 /certs",
            ],
        },
        "/certs",
        "65532:65532",
    ),
)

print("audit_wave4_pki_cert_hooks")


def _pki_hook(uid_gid: str) -> dict:
    uid, gid = uid_gid.split(":")
    return {
        "image": "alpine:3.21",
        "user": "0:0",
        "command": [
            "sh",
            "-c",
            f"mkdir -p /certs && chown {uid}:{gid} /certs && chmod 0750 /certs",
        ],
    }


PKI_GREEN = {
    name: {
        "pre_start": [_pki_hook(spec["uid_gid"])],
        "volumes": [f"{name.replace('-', '_')}_certs:/certs"],
    }
    for name, spec in audit.WAVE4_PKI_CERT_HOOKS.items()
}
check("wave 4 parents are clean", audit.audit_wave4_pki_cert_hooks(PKI_GREEN) == [])
dual = dict(PKI_GREEN)
dual["pki-agent-alt-backend"] = {"environment": ["CERT_SUBJECT=alt-backend"]}
dual_found = audit.audit_wave4_pki_cert_hooks(dual)
check(
    "leftover sidecar beside an in-process parent is a dual-writer violation",
    any("dual writer" in v and "pki-agent-alt-backend" in v for v in dual_found),
)

print("audit_migrated_chown_hooks")
GREEN = {
    "news-creator-backend": {
        "restart": "unless-stopped",
        "volumes": ["news_creator_models:/home/ollama-user/.ollama"],
        "pre_start": [HOOK],
    },
    "knowledge-embedder-local": {
        "restart": "unless-stopped",
        "volumes": ["knowledge_embedder_local_models:/home/ollama-user/.ollama"],
        "pre_start": [HOOK],
    },
}
check("migrated consumers are clean", audit.audit_migrated_chown_hooks(GREEN) == [])

legacy = {
    "news-creator-backend": {
        "depends_on": {
            "news-creator-volume-init": {
                "condition": "service_completed_successfully",
            }
        }
    },
    "news-creator-volume-init": {"restart": "no", "image": "alpine:3.21"},
    "knowledge-embedder-local": {
        "depends_on": {
            "knowledge-embedder-local-volume-init": {
                "condition": "service_completed_successfully",
            }
        }
    },
    "knowledge-embedder-local-volume-init": {"restart": "no"},
}
legacy_found = audit.audit_migrated_chown_hooks(legacy)
check(
    "sibling init service is a violation",
    any("news-creator-volume-init" in v and "sibling" in v for v in legacy_found),
)
check(
    "missing pre_start on the consumer is a violation",
    any("news-creator-backend" in v and "pre_start" in v for v in legacy_found),
)
check(
    "stale depends_on the init service is a violation",
    any("depends_on" in v and "news-creator-volume-init" in v for v in legacy_found),
)

print("production pin")
prod = production_services()
prod_found = audit.audit_migrated_chown_hooks(prod)
check("production Wave 1b chown hooks are pre_start", prod_found == [])
check("news-creator-volume-init is gone", "news-creator-volume-init" not in prod)
check(
    "knowledge-embedder-local-volume-init is gone",
    "knowledge-embedder-local-volume-init" not in prod,
)
pki_found = audit.audit_wave4_pki_cert_hooks(prod)
check("production Wave 4 PKI cert chown hooks are pre_start", pki_found == [])
for sidecar in (spec["removed_sidecar"] for spec in audit.WAVE4_PKI_CERT_HOOKS.values()):
    check(f"{sidecar} is gone (no dual writer)", sidecar not in prod)


def _which_alpine() -> str | None:
    docker = shutil.which("docker")
    if docker is None:
        return None
    for image in ALPINE_CANDIDATES:
        proc = subprocess.run(
            [docker, "image", "inspect", image],
            capture_output=True,
            text=True,
            check=False,
            timeout=10,
        )
        if proc.returncode == 0:
            return image
    return None


def _compose_supports_pre_start() -> bool:
    docker = shutil.which("docker")
    if docker is None:
        return False
    proc = subprocess.run(
        [docker, "compose", "version"],
        capture_output=True,
        text=True,
        check=False,
        timeout=10,
    )
    if proc.returncode != 0:
        return False
    text = (proc.stdout or proc.stderr or "").strip()
    try:
        spec = importlib.util.spec_from_file_location(
            "feature_gate", SCRIPTS / "compose-feature-gate.py"
        )
        assert spec is not None and spec.loader is not None
        gate = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(gate)
        return gate.pre_start_supported(gate.parse_compose_version(text))
    except (ValueError, AssertionError):
        return False


def _compose(project: str, compose_file: pathlib.Path, *args: str, timeout: int = LIVE_TIMEOUT):
    return subprocess.run(
        ["docker", "compose", "-p", project, "-f", str(compose_file), *args],
        capture_output=True,
        text=True,
        check=False,
        timeout=timeout,
    )


def _down(project: str, compose_file: pathlib.Path) -> None:
    try:
        _compose(project, compose_file, "down", "-v", "--remove-orphans", timeout=30)
    except (subprocess.TimeoutExpired, FileNotFoundError, OSError):
        pass


print("live isolated pre_start fixtures")
alpine = _which_alpine()
if not _compose_supports_pre_start():
    print("  SKIP  host Compose is below 5.4.0; not running live pre_start")
elif alpine is None:
    print("  SKIP  no local alpine image; not pulling")
else:
    marker = f"pre-start-ran-{int(time.time())}"
    with tempfile.TemporaryDirectory(
        prefix="alt-wave1b-prestart-", ignore_cleanup_errors=True
    ) as raw:
        tmp = pathlib.Path(raw)
        data_dir = tmp / "data"
        data_dir.mkdir()
        ok_file = tmp / "ok.yaml"
        ok_file.write_text(
            f"""\
name: alt-wave1b-prestart-ok
services:
  probe:
    image: {alpine}
    command: ["sleep", "8"]
    volumes:
      - type: bind
        source: {data_dir}
        target: /data
        bind:
          create_host_path: false
    pre_start:
      - image: {alpine}
        user: "0:0"
        command: ["sh", "-c", "chown -R 2000:2000 /data && echo {marker} > /data/hook.txt && chmod 0666 /data/hook.txt && chmod 0777 /data"]
""",
            encoding="utf-8",
        )
        fail_file = tmp / "fail.yaml"
        fail_file.write_text(
            f"""\
name: alt-wave1b-prestart-fail
services:
  probe:
    image: {alpine}
    command: ["sleep", "8"]
    pre_start:
      - image: {alpine}
        command: ["sh", "-c", "echo hook-failed; exit 1"]
""",
            encoding="utf-8",
        )
        try:
            up_ok = _compose(
                "alt-wave1b-prestart-ok",
                ok_file,
                "up",
                "-d",
                "--no-deps",
                "--force-recreate",
                "probe",
            )
            combined_ok = (up_ok.stdout or "") + (up_ok.stderr or "")
            hook_txt = data_dir / "hook.txt"
            hook_ran = (
                up_ok.returncode == 0
                and hook_txt.is_file()
                and marker in hook_txt.read_text(encoding="utf-8")
            )
            check(
                "up --no-deps --force-recreate runs pre_start",
                hook_ran,
            )
            if not hook_ran:
                print(f"    detail: exit={up_ok.returncode} {combined_ok.strip()[-400:]}")
        except subprocess.TimeoutExpired:
            check("up --no-deps --force-recreate runs pre_start", False)
            print("    detail: timed out after 60s")
        finally:
            _down("alt-wave1b-prestart-ok", ok_file)

        try:
            up_fail = _compose(
                "alt-wave1b-prestart-fail",
                fail_file,
                "up",
                "-d",
                "--no-deps",
                "--force-recreate",
                "probe",
            )
            combined_fail = (up_fail.stdout or "") + (up_fail.stderr or "")
            running = subprocess.run(
                [
                    "docker",
                    "compose",
                    "-p",
                    "alt-wave1b-prestart-fail",
                    "-f",
                    str(fail_file),
                    "ps",
                    "-q",
                    "probe",
                ],
                capture_output=True,
                text=True,
                check=False,
                timeout=15,
            )
            started = bool((running.stdout or "").strip())
            check(
                "hook failure prevents service start",
                up_fail.returncode != 0 and not started,
            )
            if up_fail.returncode == 0:
                print(f"    detail: {combined_fail.strip()[-400:]}")
        except subprocess.TimeoutExpired:
            check("hook failure prevents service start", False)
            print("    detail: timed out after 60s")
        finally:
            _down("alt-wave1b-prestart-fail", fail_file)

print("live isolated Wave 4 cert-volume pre_start")
if not _compose_supports_pre_start():
    print("  SKIP  host Compose is below 5.4.0; not running live pre_start")
elif alpine is None:
    print("  SKIP  no local alpine image; not pulling")
else:
    with tempfile.TemporaryDirectory(
        prefix="alt-wave4-pki-prestart-", ignore_cleanup_errors=True
    ) as raw:
        tmp = pathlib.Path(raw)
        certs = tmp / "certs"
        certs.mkdir()
        compose_file = tmp / "certs.yaml"
        compose_file.write_text(
            f"""\
name: alt-wave4-pki-prestart
services:
  probe:
    image: {alpine}
    user: "65532:65532"
    command: ["sleep", "8"]
    volumes:
      - type: bind
        source: {certs}
        target: /certs
        bind:
          create_host_path: false
    pre_start:
      - image: {alpine}
        user: "0:0"
        command:
          - sh
          - -c
          - mkdir -p /certs && chown 65532:65532 /certs && chmod 0750 /certs
""",
            encoding="utf-8",
        )
        try:
            up1 = _compose(
                "alt-wave4-pki-prestart",
                compose_file,
                "up",
                "-d",
                "--no-deps",
                "--force-recreate",
                "probe",
            )
            stat1 = certs.stat()
            mode1 = stat1.st_mode & 0o777
            check(
                "up --no-deps --force-recreate chowns cert dir to 0750",
                up1.returncode == 0 and mode1 == 0o750,
            )
            if up1.returncode != 0:
                print(f"    detail: {(up1.stdout or '') + (up1.stderr or '')}".strip()[-400:])
            up2 = _compose(
                "alt-wave4-pki-prestart",
                compose_file,
                "up",
                "-d",
                "--no-deps",
                "--force-recreate",
                "probe",
            )
            mode2 = certs.stat().st_mode & 0o777
            check(
                "second --force-recreate is idempotent (still 0750)",
                up2.returncode == 0 and mode2 == 0o750,
            )
        except subprocess.TimeoutExpired:
            check("up --no-deps --force-recreate chowns cert dir to 0750", False)
            check("second --force-recreate is idempotent (still 0750)", False)
        finally:
            _down("alt-wave4-pki-prestart", compose_file)
            # Hook leaves the bind-mounted dir owned by 65532:65532 mode 0750.
            # Restore host ownership so TemporaryDirectory cleanup can unlink it.
            uid = os.getuid()
            gid = os.getgid()
            subprocess.run(
                [
                    "docker",
                    "run",
                    "--rm",
                    "-v",
                    f"{certs}:/certs",
                    alpine,
                    "sh",
                    "-c",
                    f"chown -R {uid}:{gid} /certs && chmod 0755 /certs",
                ],
                capture_output=True,
                text=True,
                check=False,
                timeout=30,
            )

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
