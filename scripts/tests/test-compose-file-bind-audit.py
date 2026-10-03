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

print("security artifact lifecycle guards")
for source, target in [
    ("/host/dev-key.pub", "/etc/plecto/.plecto/dev-key.pub"),
    ("/host/signed-filter", "/etc/plecto/artifacts/stale-chunk-heal"),
    ("/host/ch-backups", "/backups/clickhouse"),
]:
    check(
        f"missing security payload {target} cannot be auto-created by short syntax",
        bool(audit.file_bind_violations(
            {"svc": {"volumes": [f"{source}:{target}:ro"]}},
            pathlib.Path("/compose"),
        )),
    )
    mount = {"type": "bind", "source": source, "target": target,
             "read_only": True, "bind": {"create_host_path": False}}
    check(
        f"guarded security payload {target} is accepted",
        audit.file_bind_violations({"svc": {"volumes": [mount]}},
                                   pathlib.Path("/compose")) == [],
    )
    mount["bind"] = {}
    check(
        f"security payload {target} without explicit guard is rejected",
        bool(audit.file_bind_violations({"svc": {"volumes": [mount]}},
                                       pathlib.Path("/compose"))),
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

print("host prerequisite manifest")

# A host-path bind is something the deploy host must provide before compose
# can start the service. 2026-10-02: several were added with nothing that
# provisioned them, and the deploy only failed when the container was created.
# Every such bind must be declared in deploy/host-prereqs.yaml, and every
# declaration must still match a bind compose actually makes.
PLECTO_KEY_BIND = {
    "type": "bind",
    "source": "${PLECTO_PUBLIC_KEY_HOST_PATH:-/var/lib/alt-plecto/dev-key.pub}",
    "target": "/etc/plecto/.plecto/dev-key.pub",
    "read_only": True,
    "bind": {"create_host_path": False},
}
CH_BACKUP_BIND = {
    "type": "bind",
    "source": "${CLICKHOUSE_BACKUP_HOST_PATH:-/var/lib/alt-clickhouse-backups}",
    "target": "/backups/clickhouse",
    "bind": {"create_host_path": False},
}
HOST_SERVICES = {
    "plecto-proxy": {"volumes": [PLECTO_KEY_BIND, "../plecto/manifest.toml:/etc/plecto/manifest.toml:ro"]},
    "clickhouse": {"volumes": [CH_BACKUP_BIND, "clickhouse_data:/var/lib/clickhouse"]},
    "restic-backup": {"volumes": [CH_BACKUP_BIND]},
}


def host_entry(**overrides):
    entry = {
        "id": "plecto-public-key",
        "env": "PLECTO_PUBLIC_KEY_HOST_PATH",
        "path": "/var/lib/alt-plecto/dev-key.pub",
        "kind": "file",
        "owner": "0:0",
        "mode": "0644",
        "create_host_path": False,
        "mounts": [
            {"service": "plecto-proxy", "target": "/etc/plecto/.plecto/dev-key.pub", "read_only": True}
        ],
        "provision": "install -D -m 0644 key /var/lib/alt-plecto/dev-key.pub",
    }
    entry.update(overrides)
    return entry


CH_ENTRY = {
    "id": "clickhouse-backups",
    "env": "CLICKHOUSE_BACKUP_HOST_PATH",
    "path": "/var/lib/alt-clickhouse-backups",
    "kind": "dir",
    "owner": "101:101",
    "mode": "0700",
    "create_host_path": False,
    "mounts": [
        {"service": "clickhouse", "target": "/backups/clickhouse", "read_only": False},
        {"service": "restic-backup", "target": "/backups/clickhouse", "read_only": False},
    ],
    "provision": "install -d -o 101 -g 101 -m 0700 /var/lib/alt-clickhouse-backups",
}


def path_violations(services, entries):
    mounts = audit.host_path_mounts(services.items())
    return audit.host_path_violations(mounts, {"host_paths": entries})


check(
    "every host-path bind declared with matching mounts is clean",
    path_violations(HOST_SERVICES, [host_entry(), CH_ENTRY]) == [],
)
found = path_violations(HOST_SERVICES, [CH_ENTRY])
check(
    "a guarded host-path bind with no manifest entry is a violation",
    any("PLECTO_PUBLIC_KEY_HOST_PATH" in v and "plecto-proxy" in v for v in found),
)
found = path_violations({"clickhouse": HOST_SERVICES["clickhouse"]}, [CH_ENTRY, host_entry()])
check(
    "a manifest entry no compose bind uses any more is a violation",
    any("plecto-public-key" in v for v in found),
)
check(
    "a manifest entry missing one of the services that mounts the path is a violation",
    any(
        "clickhouse-backups" in v and "restic-backup" in v
        for v in path_violations(
            HOST_SERVICES,
            [host_entry(), dict(CH_ENTRY, mounts=CH_ENTRY["mounts"][:1])],
        )
    ),
)
check(
    "a manifest create_host_path that disagrees with compose is a violation",
    any(
        "plecto-public-key" in v and "create_host_path" in v
        for v in path_violations(HOST_SERVICES, [host_entry(create_host_path=True), CH_ENTRY])
    ),
)
UNGUARDED_ABSOLUTE = {"recap-worker": {"volumes": ["/opt/rustbert-cache:/opt/rustbert-cache:rw"]}}
check(
    "an unguarded absolute host bind still needs a manifest entry",
    any("/opt/rustbert-cache" in v for v in path_violations(UNGUARDED_ABSOLUTE, [])),
)
check(
    "an unguarded absolute host bind is declared with create_host_path: true",
    path_violations(
        UNGUARDED_ABSOLUTE,
        [
            {
                "id": "rustbert-cache",
                "env": None,
                "path": "/opt/rustbert-cache",
                "kind": "dir",
                "owner": "999:999",
                "mode": "0700",
                "create_host_path": True,
                "mounts": [{"service": "recap-worker", "target": "/opt/rustbert-cache", "read_only": False}],
                "provision": "install -d -o 999 -g 999 -m 0700 /opt/rustbert-cache",
            }
        ],
    )
    == [],
)
check(
    "repo-relative binds and named volumes need no manifest entry",
    path_violations(
        {"svc": {"volumes": ["../plecto:/etc/plecto:ro", "named:/data", "/tmp"]}}, []
    )
    == [],
)
for field, bad in [("kind", "symlink"), ("owner", "root"), ("mode", "644"), ("provision", "")]:
    check(
        f"a manifest entry with invalid {field} is a violation",
        any(
            "plecto-public-key" in v and field in v
            for v in path_violations(HOST_SERVICES, [host_entry(**{field: bad}), CH_ENTRY])
        ),
    )

SOCKET_ENTRY = {
    "id": "docker-socket",
    "env": None,
    "path": "/var/run/docker.sock",
    "kind": "socket",
    "owner": "0:984",
    "mode": "0660",
    "create_host_path": False,
    "mounts": [{"service": "docker-socket-proxy-ro", "target": "/var/run/docker.sock", "read_only": True}],
    "provision": "Docker Engine creates it",
}


def socket_services(user):
    return {
        "docker-socket-proxy-ro": {
            "user": user,
            "volumes": [
                {
                    "type": "bind",
                    "source": "/var/run/docker.sock",
                    "target": "/var/run/docker.sock",
                    "read_only": True,
                    "bind": {"create_host_path": False},
                }
            ],
        }
    }


check(
    "a socket consumer whose user: gid matches the declared socket group is clean",
    audit.socket_group_violations(socket_services("65534:984"), {"host_paths": [SOCKET_ENTRY]}) == [],
)
check(
    "a socket consumer hardcoding a different gid than the declared socket group is a violation",
    any(
        "docker-socket-proxy-ro" in v and "985" in v
        for v in audit.socket_group_violations(
            socket_services("65534:985"), {"host_paths": [SOCKET_ENTRY]}
        )
    ),
)

GPU_SERVICES = {
    "news-creator-backend": {
        "deploy": {"resources": {"reservations": {"devices": [{"driver": "nvidia", "count": "all"}]}}}
    },
    "irodori-tts": {"runtime": "nvidia"},
    "alt-backend": {},
}
check(
    "declared GPU services matching compose are clean",
    audit.gpu_violations(
        GPU_SERVICES,
        {"host_facts": {"nvidia_container_runtime": {"services": ["irodori-tts", "news-creator-backend"]}}},
    )
    == [],
)
check(
    "a GPU service missing from the manifest is a violation",
    any(
        "irodori-tts" in v
        for v in audit.gpu_violations(
            GPU_SERVICES,
            {"host_facts": {"nvidia_container_runtime": {"services": ["news-creator-backend"]}}},
        )
    ),
)

COMPOSE_TEXT = """
services:
  db:
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      REQUIRED: ${MUST_SET:?set it}
      WITH_DEFAULT: ${HAS_DEFAULT:-x}
      ALSO_DEFAULT: ${HAS_DASH_DEFAULT-x}
    command: sh -c 'cat $${NOT_COMPOSE}'
    # ${COMMENTED_OUT}
"""
check(
    "required env keys are the interpolations with no default",
    audit.undefaulted_env_keys([COMPOSE_TEXT]) == {"POSTGRES_USER", "MUST_SET"},
)
check(
    "required_env matching compose is clean",
    audit.required_env_violations([COMPOSE_TEXT], {"required_env": ["MUST_SET", "POSTGRES_USER"]}) == [],
)
found = audit.required_env_violations([COMPOSE_TEXT], {"required_env": ["MUST_SET", "STALE_KEY"]})
check(
    "an undeclared required key and a stale declared key are both violations",
    any("POSTGRES_USER" in v for v in found) and any("STALE_KEY" in v for v in found),
)

ENV_FILE_ENTRIES = [
    (pathlib.Path("/repo/compose"), "alt-backend", {"env_file": ["../.env"]}),
    (pathlib.Path("/repo/compose"), "restic-backup", {"env_file": ["../scripts/backup/alt-backup.env"]}),
    (pathlib.Path("/repo/compose"), "tracked", {"env_file": "../config/tracked.env"}),
    (pathlib.Path("/repo/compose"), "optional", {"env_file": [{"path": "../optional.env", "required": False}]}),
]


def ignored_env(path):
    return path.name in {".env", "alt-backup.env", "optional.env"}


check(
    "gitignored env_files declared in the manifest are clean",
    audit.env_file_violations(
        ENV_FILE_ENTRIES,
        {"env_files": [{"path": ".env", "provision": "x"}, {"path": "scripts/backup/alt-backup.env", "provision": "x"}]},
        is_ignored=ignored_env,
        repo_root=pathlib.Path("/repo"),
    )
    == [],
)
found = audit.env_file_violations(
    ENV_FILE_ENTRIES,
    {"env_files": [{"path": ".env", "provision": "x"}, {"path": "gone.env", "provision": "x"}]},
    is_ignored=ignored_env,
    repo_root=pathlib.Path("/repo"),
)
check(
    "an undeclared gitignored env_file and a stale declared one are both violations",
    any("scripts/backup/alt-backup.env" in v for v in found) and any("gone.env" in v for v in found),
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

shared_backup_mounts = {}
for _path, name, svc in audit.iter_production_services():
    if name not in {"clickhouse", "restic-backup"}:
        continue
    for mount in svc.get("volumes") or []:
        if isinstance(mount, dict) and mount.get("target") == "/backups/clickhouse":
            shared_backup_mounts[name] = mount
check(
    "ClickHouse and Restic share one guarded writable native-backup directory",
    set(shared_backup_mounts) == {"clickhouse", "restic-backup"}
    and len({m.get("source") for m in shared_backup_mounts.values()}) == 1
    and all(not m.get("read_only", False)
            and (m.get("bind") or {}).get("create_host_path") is False
            for m in shared_backup_mounts.values()),
)

prereqs = audit.audit_host_prereqs()
check("production compose matches deploy/host-prereqs.yaml", prereqs == [])
for v in prereqs:
    print(f"    leftover: {v}")

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
