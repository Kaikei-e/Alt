#!/usr/bin/env python3
"""Tests for scripts/compose-file-bind-audit.py.

The detector exists to catch the PM-2026-036 class: a short-syntax
file-scoped bind whose missing source becomes an empty directory. A
parser that only looks at `type: bind` after `compose config` would miss
every current production mount, because config expands short syntax.

Run:
    python3 scripts/tests/test-compose-file-bind-audit.py
"""

from __future__ import annotations

import importlib.util
import pathlib
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))

spec = importlib.util.spec_from_file_location(
    "file_bind_audit", SCRIPTS / "compose-file-bind-audit.py"
)
assert spec is not None and spec.loader is not None
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)

PASS = 0
FAIL = 0


def check(name, condition):
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
    else:
        print(f"  FAIL  {name}")
        FAIL += 1


print("file_bind_violations")

SHORT_FILE = {
    "prometheus": {
        "volumes": [
            "../observability/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro",
        ]
    }
}
found = audit.file_bind_violations(SHORT_FILE, pathlib.Path("/compose"))
check(
    "short-syntax file bind is a violation",
    any("prometheus" in v and "prometheus.yml" in v for v in found),
)

SHORT_DIR = {
    "plecto-proxy": {
        "volumes": ["../plecto:/etc/plecto:ro"],
    }
}
check(
    "short-syntax directory bind is not a file-bind violation",
    audit.file_bind_violations(SHORT_DIR, pathlib.Path("/compose")) == [],
)

NAMED = {
    "news-creator": {
        "volumes": ["news_creator_models:/home/ollama-user/.ollama"],
    }
}
check(
    "a named volume is not a file bind",
    audit.file_bind_violations(NAMED, pathlib.Path("/compose")) == [],
)

SOCK = {
    "docker-socket-proxy": {
        "volumes": ["/var/run/docker.sock:/var/run/docker.sock:ro"],
    }
}
found = audit.file_bind_violations(SOCK, pathlib.Path("/compose"))
check(
    "short-syntax docker.sock is a PM-036 file-class violation",
    any("docker.sock" in v for v in found),
)

SOCK_LONG_OK = {
    "docker-socket-proxy": {
        "volumes": [
            {
                "type": "bind",
                "source": "/var/run/docker.sock",
                "target": "/var/run/docker.sock",
                "read_only": True,
                "bind": {"create_host_path": False},
            }
        ]
    }
}
check(
    "long-syntax docker.sock with create_host_path: false is clean",
    audit.file_bind_violations(SOCK_LONG_OK, pathlib.Path("/compose")) == [],
)

LONG_OK = {
    "prometheus": {
        "volumes": [
            {
                "type": "bind",
                "source": "../observability/prometheus/prometheus.yml",
                "target": "/etc/prometheus/prometheus.yml",
                "read_only": True,
                "bind": {"create_host_path": False},
            }
        ]
    }
}
check(
    "long-syntax file bind with create_host_path: false is clean",
    audit.file_bind_violations(LONG_OK, pathlib.Path("/compose")) == [],
)

LONG_FOOTGUN = {
    "prometheus": {
        "volumes": [
            {
                "type": "bind",
                "source": "../observability/prometheus/prometheus.yml",
                "target": "/etc/prometheus/prometheus.yml",
                "read_only": True,
            }
        ]
    }
}
found = audit.file_bind_violations(LONG_FOOTGUN, pathlib.Path("/compose"))
check(
    "long-syntax file bind without create_host_path: false is a violation",
    any("prometheus" in v and "create_host_path" in v for v in found),
)

EXTENSIONLESS = {
    "restic-backup": {
        "volumes": [
            "../secrets/ssh/id_ed25519_backup:/root/.ssh/id_ed25519:ro",
            "../secrets/ssh/known_hosts:/root/.ssh/known_hosts:ro",
        ]
    }
}
found = audit.file_bind_violations(EXTENSIONLESS, pathlib.Path("/compose"))
check(
    "extensionless ssh key and known_hosts short binds are file binds",
    len(found) == 2,
)

with tempfile.TemporaryDirectory() as tmp:
    host = pathlib.Path(tmp)
    (host / "real.conf").write_text("x\n", encoding="utf-8")
    EXISTING = {
        "svc": {
            "volumes": [f"{host / 'real.conf'}:/etc/real.conf:ro"],
        }
    }
    found = audit.file_bind_violations(EXISTING, host)
    check(
        "a source that exists as a file is a file bind even without a well-known suffix on the target",
        any("real.conf" in v for v in found),
    )

ARTEFACT_SHORT = {
    "recap-subworker": {
        "volumes": [
            "${RECAP_SUBWORKER_DATA_HOST_PATH:-/var/lib/alt-recap-subworker-data}:/app/data:ro",
        ]
    }
}
found = audit.file_bind_violations(ARTEFACT_SHORT, pathlib.Path("/compose"))
check(
    "short-syntax recap artefact directory bind is a violation",
    any("recap-subworker" in v and "/app/data" in v for v in found),
)

ARTEFACT_LONG_OK = {
    "recap-subworker": {
        "volumes": [
            {
                "type": "bind",
                "source": "${RECAP_SUBWORKER_DATA_HOST_PATH:-/var/lib/alt-recap-subworker-data}",
                "target": "/app/data",
                "read_only": True,
                "bind": {"create_host_path": False},
            }
        ]
    }
}
check(
    "long-syntax recap artefact directory bind with create_host_path: false is clean",
    audit.file_bind_violations(ARTEFACT_LONG_OK, pathlib.Path("/compose")) == [],
)

CONFIGS_ONLY = {
    "prometheus": {
        "configs": [
            {
                "source": "prometheus_yml",
                "target": "/etc/prometheus/prometheus.yml",
                "mode": 0o444,
            }
        ]
    }
}
check(
    "a configs: mount is not a volume bind violation",
    audit.file_bind_violations(CONFIGS_ONLY, pathlib.Path("/compose")) == [],
)

print("ephemeral_source_violations")

# The deploy job checks Alt out fresh into the runner workspace, so a bind
# whose source is a repo-relative path only holds what git tracks. A
# gitignored source resolves to a path that is absent there — and with
# `create_host_path: false` the roll fails at preflight, while without it
# Engine mounts a silently empty directory (PM-2026-036 again, one layer up).
IGNORED_RELATIVE = {
    "recap-subworker": {
        "volumes": [
            {
                "type": "bind",
                "source": "../recap-subworker/recap_subworker/learning_machine/artifacts",
                "target": "/app/recap_subworker/learning_machine/artifacts",
                "read_only": True,
                "bind": {"create_host_path": False},
            }
        ]
    }
}
check(
    "repo-relative bind whose source is gitignored is a violation",
    any(
        "recap-subworker" in v and "artifacts" in v
        for v in audit.ephemeral_source_violations(
            IGNORED_RELATIVE, pathlib.Path("/repo/compose"), is_ignored=lambda _p: True
        )
    ),
)
check(
    "repo-relative bind whose source is tracked is clean",
    audit.ephemeral_source_violations(
        IGNORED_RELATIVE, pathlib.Path("/repo/compose"), is_ignored=lambda _p: False
    )
    == [],
)

HOST_ABSOLUTE = {
    "recap-subworker": {
        "volumes": [
            {
                "type": "bind",
                "source": "${RECAP_SUBWORKER_ARTIFACTS_HOST_PATH:-/var/lib/alt-recap-subworker-artifacts}",
                "target": "/app/recap_subworker/learning_machine/artifacts",
                "read_only": True,
                "bind": {"create_host_path": False},
            }
        ]
    }
}
check(
    "host-path bind is never checked against gitignore",
    audit.ephemeral_source_violations(
        HOST_ABSOLUTE, pathlib.Path("/repo/compose"), is_ignored=lambda _p: True
    )
    == [],
)

# restic SSH keys live under gitignored `secrets/`. A path relative to this
# compose file follows the `secrets` symlink on an operator host (so a local
# audit can miss them) and resolves inside the deploy checkout on CI, where
# the files are absent. Same host-path rule as recap-subworker artefacts.
RESTIC_SSH_RELATIVE = {
    "restic-backup": {
        "volumes": [
            {
                "type": "bind",
                "source": "../secrets/ssh/id_ed25519_backup",
                "target": "/root/.ssh/id_ed25519",
                "read_only": True,
                "bind": {"create_host_path": False},
            }
        ]
    }
}
check(
    "repo-relative restic SSH key bind is a gitignored-source violation",
    any(
        "restic-backup" in v and "id_ed25519_backup" in v
        for v in audit.ephemeral_source_violations(
            RESTIC_SSH_RELATIVE, pathlib.Path("/repo/compose"), is_ignored=lambda _p: True
        )
    ),
)

RESTIC_SSH_HOST = {
    "restic-backup": {
        "volumes": [
            {
                "type": "bind",
                "source": "${RESTIC_SSH_KEY_HOST_PATH:-/var/lib/alt-restic/ssh/id_ed25519_backup}",
                "target": "/root/.ssh/id_ed25519",
                "read_only": True,
                "bind": {"create_host_path": False},
            },
            {
                "type": "bind",
                "source": "${RESTIC_SSH_KNOWN_HOSTS_HOST_PATH:-/var/lib/alt-restic/ssh/known_hosts}",
                "target": "/root/.ssh/known_hosts",
                "read_only": True,
                "bind": {"create_host_path": False},
            },
        ]
    }
}
check(
    "host-path restic SSH binds are never checked against gitignore",
    audit.ephemeral_source_violations(
        RESTIC_SSH_HOST, pathlib.Path("/repo/compose"), is_ignored=lambda _p: True
    )
    == [],
)

NAMED_VOLUME = {
    "recap-subworker": {"volumes": ["recap_subworker_certs:/certs"]}
}
check(
    "named volume is not a source path",
    audit.ephemeral_source_violations(
        NAMED_VOLUME, pathlib.Path("/repo/compose"), is_ignored=lambda _p: True
    )
    == [],
)

print("production")
prod = audit.audit_production()
check("production compose has 0 unguarded file binds", prod == [])
ephemeral = audit.audit_production_sources()
check("production compose has 0 gitignored bind sources", ephemeral == [])
for v in ephemeral:
    print(f"    leftover: {v}")
if prod:
    for v in prod:
        print(f"    leftover: {v}")

restic_ssh_sources = []
for _path, name, svc in audit.iter_production_services():
    if name != "restic-backup":
        continue
    for raw in svc.get("volumes") or []:
        if not isinstance(raw, dict):
            continue
        if not str(raw.get("target") or "").startswith("/root/.ssh/"):
            continue
        restic_ssh_sources.append(str(raw.get("source") or ""))
check(
    "production restic SSH binds use host-path interpolation",
    any("${RESTIC_SSH_KEY_HOST_PATH" in s for s in restic_ssh_sources)
    and any("${RESTIC_SSH_KNOWN_HOSTS_HOST_PATH" in s for s in restic_ssh_sources)
    and not any("../secrets/" in s for s in restic_ssh_sources),
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
