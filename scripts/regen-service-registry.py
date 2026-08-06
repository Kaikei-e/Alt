#!/usr/bin/env python3
"""Regenerate service-registry artifacts from services.yaml.

Single source of truth: services.yaml at the repo root.
Derived files:
  - .github/workflows/docker-build.yaml  (SERVICES=(...) block)
  - c2quay.yml                           (environments.production.services)
  - .github/generated/alt-deploy-service-paths.js
      (reference snippet for alt-deploy's release-deploy.yaml SERVICE_PATHS)

Usage:
  scripts/regen-service-registry.py --check   # exit 1 if drift detected
  scripts/regen-service-registry.py --write   # rewrite derived files

Uses only the standard library so CI can invoke it without any env setup.
"""
from __future__ import annotations

import argparse
import difflib
import re
import sys
from pathlib import Path
from typing import Any

# Stdlib YAML parser via re — we only need a subset. But PyYAML is
# essentially universally installed on GitHub-hosted runners. Prefer it
# when available so we don't reimplement YAML parsing.
try:
    import yaml
except ImportError:  # pragma: no cover
    sys.stderr.write(
        "PyYAML is required. Install via `pip install pyyaml` or use an "
        "image that preinstalls it.\n"
    )
    sys.exit(2)

ROOT = Path(__file__).resolve().parent.parent
REGISTRY = ROOT / "services.yaml"
DOCKER_BUILD = ROOT / ".github" / "workflows" / "docker-build.yaml"
C2QUAY = ROOT / "c2quay.yml"
ALT_DEPLOY_SNIPPET = ROOT / ".github" / "generated" / "alt-deploy-service-paths.js"

# Sentinel markers that bracket the generated regions in the hand-edited
# files. Anything inside the sentinels is rewritten in place; anything
# outside is preserved verbatim.
SVC_BEGIN = "# <<<SERVICES-REGISTRY-BEGIN>>>"
SVC_END = "# <<<SERVICES-REGISTRY-END>>>"


def load_registry() -> dict[str, Any]:
    with REGISTRY.open() as f:
        data = yaml.safe_load(f)
    if not isinstance(data, dict) or "services" not in data:
        raise SystemExit(f"{REGISTRY}: missing top-level `services` list")
    return data


def pacticipants(registry: dict[str, Any]) -> list[dict[str, Any]]:
    """Services that go through the Pact gate (local + external)."""
    return [
        s for s in registry["services"]
        if s.get("kind") in ("pacticipant", "external")
    ]


def local_pacticipants(registry: dict[str, Any]) -> list[dict]:
    """Pacticipants built + deployed locally (excludes `external`)."""
    return [s for s in registry["services"] if s.get("kind") == "pacticipant"]


def runtime_services(registry: dict[str, Any]) -> list[dict]:
    """Non-pacticipant images built via docker-build but not gated by Pact."""
    return [s for s in registry["services"] if s.get("kind") == "runtime"]


def local_build_services(registry: dict[str, Any]) -> list[dict]:
    """Services that have a Dockerfile in this repo (pacticipant + runtime)."""
    return [
        s for s in registry["services"]
        if s.get("kind") in ("pacticipant", "runtime")
    ]


# ---------------------------------------------------------------------------
# Renderers
# ---------------------------------------------------------------------------

def build_spec(service: dict[str, Any]) -> dict[str, Any]:
    """Resolve a service's build inputs, applying the name-derived defaults.

    Historically `name` was the directory, the image suffix and the
    change-detection prefix all at once. That breaks as soon as one directory
    produces more than one image (the alt-backend trio), so `build:` in
    services.yaml can state the three separately. Everything omitted keeps the
    old behaviour, which is why the 21 pre-existing entries need no edit:

      dir        defaults to name
      dockerfile defaults to "" — docker-build.yaml then discovers it under
                 dir/, exactly as before
      args       defaults to {} — no extra --build-arg
    """
    build = service.get("build") or {}
    name = service["name"]
    args = build.get("args") or {}
    return {
        "dir": build.get("dir", name),
        "dockerfile": build.get("dockerfile", ""),
        # Only BINARY is consumed today; keeping it a dict means adding a
        # second arg does not need another matrix key.
        "binary": str(args.get("BINARY", "")),
    }


def render_docker_build_block(registry: dict[str, Any]) -> str:
    """Bash body for docker-build.yaml's generated registry block.

    Emits the SERVICES array plus three parallel lookup maps keyed by service
    name. A map (rather than packing fields into the array entries) keeps the
    array itself byte-identical in shape to the hand-written original, so the
    diff on a normal service addition stays one line.
    """
    lines = ["          SERVICES=("]
    lines.append("            # --- 13 Pact pacticipants (mirrors c2quay.yml) ---")
    for s in local_pacticipants(registry):
        lines.append(f'            "{s["name"]}"')
    lines.append("            # --- Additional runtime services used by compose ---")
    for s in runtime_services(registry):
        lines.append(f'            "{s["name"]}"')
    lines.append("          )")

    specs = {s["name"]: build_spec(s) for s in local_build_services(registry)}

    lines.append("")
    lines.append("          # Build inputs per service. BUILD_DIR doubles as the")
    lines.append("          # change-detection prefix, so several services can share one")
    lines.append("          # source directory. An empty BUILD_DOCKERFILE means \"discover")
    lines.append("          # it under BUILD_DIR\"; an empty BUILD_BINARY means no")
    lines.append("          # --build-arg BINARY. Defaults reproduce the historical")
    lines.append("          # name-is-the-directory rule.")
    for var, key in (
        ("BUILD_DIR", "dir"),
        ("BUILD_DOCKERFILE", "dockerfile"),
        ("BUILD_BINARY", "binary"),
    ):
        lines.append(f"          declare -A {var}=(")
        for name, spec in specs.items():
            lines.append(f'            ["{name}"]="{spec[key]}"')
        lines.append("          )")
    return "\n".join(lines) + "\n"


def render_c2quay_block(registry: dict[str, Any]) -> str:
    """YAML fragment for c2quay.yml environments.production.services.

    Entries with `kind: external` (services gated via Pact but not run by
    docker compose on this host, e.g. tts-speaker on a remote GPU host) are
    emitted with `gate_only: true` so c2quay includes them in the Pact
    can-i-deploy gate and records their deployment, without attempting to
    start/recreate them via compose. See docs/adr/0013-gate-only-services.md
    in the c2quay repo.
    """
    # Align pacticipant colons for readability (matches current style).
    names = [s["name"] for s in pacticipants(registry)]
    width = max(len(n) for n in names)
    out = []
    for s in pacticipants(registry):
        name = s["name"]
        pad = " " * (width - len(name))
        if s.get("kind") == "external":
            out.append(
                f"      {name}:{pad} {{ pacticipant: {name}, gate_only: true }}"
            )
        else:
            out.append(f"      {name}:{pad} {{ pacticipant: {name} }}")
    return "\n".join(out) + "\n"


def render_alt_deploy_service_paths(registry: dict[str, Any]) -> str:
    """JavaScript object literal for alt-deploy release-deploy.yaml.

    alt-deploy lives in a separate repo and is regenerated out-of-band;
    we commit the snippet here so drift is visible in Alt's PR review.
    """
    out = ["// AUTOGENERATED by scripts/regen-service-registry.py."]
    out.append("// Paste the object body into alt-deploy's")
    out.append("// .github/workflows/release-deploy.yaml SERVICE_PATHS.")
    out.append("//")
    out.append("// Regenerate with `scripts/regen-service-registry.py --write`.")
    out.append("const SERVICE_PATHS = {")
    for s in local_pacticipants(registry):
        name = s["name"]
        compose_files = s.get("compose_files", [])
        # Source path comes from build.dir, not the name: a service whose
        # image is built out of a shared directory must react to changes in
        # that directory, not in a directory named after itself that may not
        # exist. e2e paths stay keyed by name — those fixtures are per
        # pacticipant, not per source tree.
        #
        # The suite path is e2e/playwright/<name>/ since the Hurl suites were
        # retired; ADR-000766's dispatch contract (`e2e/<framework>/<svc>/run.sh`)
        # is unchanged, only the framework directory moved.
        source_dir = build_spec(s)["dir"]
        lines = [
            f"  '{name}': [",
            f"    '{source_dir}/',",
            f"    'e2e/playwright/{name}/',",
            f"    'e2e/fixtures/{name}/',",
        ]
        for cf in compose_files:
            lines.append(f"    'compose/{cf}',")
        lines.append("  ],")
        out.extend(lines)
    out.append("};")
    return "\n".join(out) + "\n"


# ---------------------------------------------------------------------------
# Patchers (for in-place files with sentinel markers)
# ---------------------------------------------------------------------------

def patch_between_sentinels(
    path: Path,
    new_body: str,
    begin_marker: str,
    end_marker: str,
) -> str:
    """Replace the text between begin_marker and end_marker with new_body.

    Preserves the leading whitespace of each sentinel line so the host
    file's indentation (`          #` in docker-build, `      #` in
    c2quay) survives. new_body is responsible for its own indentation.
    """
    text = path.read_text()
    pat = re.compile(
        r"(^[ \t]*)" + re.escape(begin_marker) + r".*?^([ \t]*)" + re.escape(end_marker),
        re.DOTALL | re.MULTILINE,
    )
    m = pat.search(text)
    if not m:
        raise SystemExit(
            f"{path}: sentinel markers not found.\n"
            f"  Expected '{begin_marker}' and '{end_marker}' on their own lines.\n"
            f"  Initialize the block manually, then rerun this script."
        )
    begin_indent, end_indent = m.group(1), m.group(2)
    replacement = f"{begin_indent}{begin_marker}\n{new_body}{end_indent}{end_marker}"
    return pat.sub(replacement, text, count=1)


def patch_docker_build(registry: dict[str, Any]) -> tuple[Path, str]:
    new_body = render_docker_build_block(registry)
    text = patch_between_sentinels(DOCKER_BUILD, new_body, SVC_BEGIN, SVC_END)
    return DOCKER_BUILD, text


def patch_c2quay(registry: dict[str, Any]) -> tuple[Path, str]:
    new_body = render_c2quay_block(registry)
    text = patch_between_sentinels(C2QUAY, new_body, SVC_BEGIN, SVC_END)
    return C2QUAY, text


def write_alt_deploy_snippet(registry: dict[str, Any]) -> tuple[Path, str]:
    return ALT_DEPLOY_SNIPPET, render_alt_deploy_service_paths(registry)


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def diff(path: Path, new_text: str) -> str:
    old = path.read_text() if path.exists() else ""
    if old == new_text:
        return ""
    return "".join(
        difflib.unified_diff(
            old.splitlines(keepends=True),
            new_text.splitlines(keepends=True),
            fromfile=f"a/{path.relative_to(ROOT)}",
            tofile=f"b/{path.relative_to(ROOT)}",
        )
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--check", action="store_true",
                      help="exit 1 if generated files are out of date")
    mode.add_argument("--write", action="store_true",
                      help="rewrite generated files in place")
    args = parser.parse_args()

    registry = load_registry()

    outputs: list[tuple[Path, str]] = [
        patch_docker_build(registry),
        patch_c2quay(registry),
        write_alt_deploy_snippet(registry),
    ]

    if args.write:
        for path, new_text in outputs:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(new_text)
            print(f"wrote {path.relative_to(ROOT)}")
        return 0

    drift = False
    for path, new_text in outputs:
        d = diff(path, new_text)
        if d:
            drift = True
            print(d, end="")
            print(f"::error file={path.relative_to(ROOT)}::drifts from services.yaml; "
                  f"run scripts/regen-service-registry.py --write")
    if drift:
        return 1
    print("services registry is in sync.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
