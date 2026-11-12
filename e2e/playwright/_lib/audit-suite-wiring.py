#!/usr/bin/env python3
"""Check that each suite's declared wiring matches what compose actually does.

    python3 e2e/playwright/_lib/audit-suite-wiring.py

Three ways a suite can be wired wrong while every test still reports green.
All three were real, found on the same afternoon:

1. **A built image nothing consumes.** knowledge-sovereign's compose entry had
   `build:` and no `image:`, so the GHCR-tagged image CI produced with buildx
   and the registry layer cache was ignored and compose rebuilt the service
   from scratch on every run. The suite passed throughout; it just paid for the
   build twice and used the cache never.

2. **An image tag never forwarded.** compose keys each GHCR image off its own
   `<SERVICE>_IMAGE_TAG`, defaulting to `main`. A suite that builds an image at
   `:ci` but does not forward the tag runs against `:main` — the *previous*
   release of the very service it is meant to be testing. Nothing fails; the
   suite simply stops being about this commit.

3. **A suite that exists in one place only.** A directory with no `suites.yaml`
   entry never runs in CI. An entry with no directory fails the matrix.

Case 2 has a legitimate opposite: a dependency that is *not* co-versioned with
the service under test should stay on `:main`, because pinning it to this
suite's tag demands a build the dispatch SHA may never have produced. That is a
decision, not an oversight, so it has to be written down — `unpinnedImages:` in
suites.yaml is where. The audit then distinguishes "deliberately on main" from
"forgotten", which is the whole point (CLAUDE.md rule 8, applied to CI wiring).
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

import yaml


class ComposeSafeLoader(yaml.SafeLoader):
    """SafeLoader that accepts Docker Compose `!override` and `!reset` tags."""


for _compose_tag in ("!override", "!reset"):
    ComposeSafeLoader.add_constructor(
        _compose_tag,
        lambda loader, node: (
            loader.construct_sequence(node)
            if isinstance(node, yaml.SequenceNode)
            else loader.construct_mapping(node)
            if isinstance(node, yaml.MappingNode)
            else loader.construct_scalar(node)
        ),
    )

ROOT = Path(__file__).resolve().parents[3]
PW_ROOT = ROOT / "e2e" / "playwright"
SUITES_YAML = PW_ROOT / "suites.yaml"
COMPOSE = ROOT / "compose" / "compose.staging.yaml"

TAG_VAR = re.compile(r"\$\{([A-Z0-9_]+_IMAGE_TAG)")
SUITE_UP = re.compile(r"^[ \t]*suite_up(?:[ \t]+((?:\\\n|[^\n])+))?", re.MULTILINE)
COMPOSE_BUILD = re.compile(
    r"^[ \t]*docker[ \t]+compose[ \t]+(?:\\\n|[^\n])*?\bbuild\b((?:\\\n|[^\n])*)",
    re.MULTILINE,
)


SUITE_INIT_BODY = re.compile(r"^suite_init\(\)\s*\{(.*?)^\}", re.MULTILINE | re.DOTALL)


def suite_init_mints_token_keys(suite_lib_text: str) -> bool:
    """compose.staging.yaml needs STAGING_TOKEN_KEY_DIR on every render, so
    suite_init (which every suite calls) has to mint the keys."""
    body = SUITE_INIT_BODY.search(suite_lib_text)
    if body is None:
        return False
    code = "\n".join(l for l in body.group(1).splitlines() if not l.strip().startswith("#"))
    return re.search(r"^\s*suite_token_keys\b", code, re.MULTILINE) is not None


def main() -> int:
    with SUITES_YAML.open() as handle:
        manifest = yaml.safe_load(handle) or {}
    suites = manifest.get("suites", {})
    stubs = manifest.get("stubs", {})

    with COMPOSE.open() as handle:
        compose = yaml.safe_load(handle) or {}
    services = compose.get("services", {})

    # service -> the *_IMAGE_TAG its `image:` interpolates, when it has one.
    tag_var: dict[str, str] = {}
    for name, svc in services.items():
        match = TAG_VAR.search(str(svc.get("image") or ""))
        if match:
            tag_var[name] = match.group(1)

    # profile -> the services it activates.
    profiles: dict[str, list[str]] = {}
    for name, svc in services.items():
        for profile in svc.get("profiles", []) or []:
            profiles.setdefault(profile, []).append(name)

    problems: list[str] = []

    if not suite_init_mints_token_keys((PW_ROOT / "_lib" / "suite.sh").read_text()):
        problems.append(
            "_lib/suite.sh: suite_init does not call suite_token_keys, so a suite would render "
            "compose.staging.yaml without STAGING_TOKEN_KEY_DIR"
        )

    # (3) suites.yaml and the directory tree must agree.
    on_disk = {
        path.parent.name
        for path in PW_ROOT.glob("*/playwright.config.ts")
    }
    if (PW_ROOT / "alt-frontend-sv" / "run.sh").is_file() and (
        ROOT / "alt-frontend-sv" / "playwright.config.ts"
    ).is_file():
        on_disk.add("alt-frontend-sv")
    for name in sorted(on_disk - set(suites)):
        problems.append(
            f"{name}: has a playwright.config.ts but no `suites:` entry, so it never runs in CI"
        )
    for name in sorted(set(suites) - on_disk):
        problems.append(f"{name}: declared in suites.yaml but has no playwright.config.ts")

    if list((suites.get("alt-frontend-sv") or {}).get("images") or []) != ["alt-frontend-sv"]:
        problems.append(
            "alt-frontend-sv: suites.yaml must register alt-frontend-sv with "
            "images: [alt-frontend-sv] so PR CI builds the app image from the working tree"
        )

    for name in sorted(suites):
        entry = suites[name] or {}
        declared_images = list(entry.get("images") or [])
        unpinned = set(entry.get("unpinnedImages") or [])

        # alt-frontend-sv runs outside compose.staging.yaml; its run.sh wiring
        # is checked in (5)-(10) and (12) below.
        if name == "alt-frontend-sv":
            continue

        # (1) every declared image must be something compose can consume.
        for image in declared_images:
            if image in stubs:
                continue
            svc = services.get(image)
            if svc is None:
                problems.append(
                    f"{name}: builds '{image}', which is not a service in "
                    f"compose.staging.yaml and not a stub — nothing will use it"
                )
            elif image not in tag_var:
                problems.append(
                    f"{name}: builds '{image}', but compose.staging.yaml's entry for it has no "
                    f"`image:` with a *_IMAGE_TAG — compose will rebuild from source and ignore "
                    f"the build. Add an `image:` key, or drop it from `images:`"
                )

        run_sh = PW_ROOT / name / "run.sh"
        if not run_sh.is_file():
            problems.append(f"{name}: no run.sh")
            continue

        # Only the arguments of an actual `suite_image_tags` call count. Scanning
        # the whole file would match the var names these scripts quote in their
        # comments while *explaining* why they are not forwarded — the audit
        # would then pass on precisely the file that documents the gap.
        run_text = run_sh.read_text()
        forwarded: set[str] = set()
        for line in run_text.splitlines():
            stripped = line.strip()
            if stripped.startswith("#") or not stripped.startswith("suite_image_tags"):
                continue
            forwarded.update(re.findall(r"[A-Z0-9_]+_IMAGE_TAG", stripped))

        # (4) a suite that declares pre-built images must not pass `--build` to
        #     `suite_up` (with or without a service list) or run `docker compose
        #     ... build` on those services — compose would rebuild from source
        #     and overwrite the pre-built image.
        run_code = "\n".join(
            line for line in run_text.splitlines() if not line.strip().startswith("#")
        )
        for match in SUITE_UP.finditer(run_code):
            raw_args = (match.group(1) or "").replace("\\\n", " ")
            tokens = raw_args.split()
            if "--build" in tokens and declared_images:
                service_args = [t for t in tokens if not t.startswith("-")]
                rebuilt = (
                    list(declared_images)
                    if not service_args
                    else [img for img in declared_images if img in service_args]
                )
                if rebuilt:
                    problems.append(
                        f"{name}: run.sh passes --build to suite_up for pre-built image(s) "
                        f"{rebuilt} — compose will rebuild from source and ignore the "
                        f"pre-built image"
                    )

        built_services: set[str] = set()
        for match in COMPOSE_BUILD.finditer(run_code):
            raw_args = (match.group(1) or "").replace("\\\n", " ")
            tokens = raw_args.split()
            service_args = [t for t in tokens if not t.startswith("-")]
            built_services.update(service_args)
            if declared_images:
                rebuilt = (
                    list(declared_images)
                    if not service_args
                    else [img for img in declared_images if img in service_args]
                )
                if rebuilt:
                    problems.append(
                        f"{name}: run.sh runs 'docker compose ... build' for pre-built image(s) "
                        f"{rebuilt} — only local-only helper services may be built here"
                    )

        if name == "rag-orchestrator":
            if services.get("rag-db", {}).get("image"):
                problems.append(
                    "rag-orchestrator: compose.staging.yaml sets an `image:` on rag-db, "
                    "which prevents per-run rebuilds when rag-db/Dockerfile changes"
                )
            if built_services != {"rag-db", "rag-db-migrator"}:
                problems.append(
                    "rag-orchestrator: run.sh must build exactly ['rag-db', 'rag-db-migrator'] "
                    f"via docker compose build before suite_up (got {sorted(built_services)})"
                )

        # (2) every taggable image in the profile is either forwarded or
        #     recorded as deliberately left on `:main`.
        for svc_name in sorted(profiles.get(name, [])):
            var = tag_var.get(svc_name)
            if var is None or var in forwarded or svc_name in unpinned:
                continue
            problems.append(
                f"{name}: compose service '{svc_name}' resolves through ${{{var}}}, which run.sh "
                f"does not forward — it will run at `:main`. Either add it to "
                f"`suite_image_tags`, or record it under `unpinnedImages:` in suites.yaml with "
                f"the reason"
            )

        # An `unpinnedImages` entry that is not in the profile is stale.
        for svc_name in sorted(unpinned - set(profiles.get(name, []))):
            problems.append(
                f"{name}: `unpinnedImages` lists '{svc_name}', which is not in the {name} profile"
            )

    # (5) alt-frontend-sv runs outside compose.staging.yaml, but its run.sh
    #     must still boot the pre-built alt-alt-frontend-sv image with
    #     PW_NO_WEBSERVER=1 and avoid per-run apt/Bun/full-app installs.
    frontend_run_sh = PW_ROOT / "alt-frontend-sv" / "run.sh"
    if not frontend_run_sh.is_file():
        problems.append("alt-frontend-sv: no run.sh")
    else:
        fe_code = "\n".join(
            line
            for line in frontend_run_sh.read_text().splitlines()
            if not line.strip().startswith("#")
        )
        if "alt-alt-frontend-sv" not in fe_code or "IMAGE_TAG" not in fe_code:
            problems.append(
                "alt-frontend-sv: run.sh does not reference pre-built "
                "alt-alt-frontend-sv:${IMAGE_TAG} image"
            )
        if "PW_NO_WEBSERVER" not in fe_code:
            problems.append(
                "alt-frontend-sv: run.sh does not set PW_NO_WEBSERVER=1 — "
                "playwright.config.ts will rebuild the SvelteKit app from source"
            )
        for forbidden in ("apt-get", "bun.sh/install", "bun install", "npm install"):
            if forbidden in fe_code:
                problems.append(
                    f"alt-frontend-sv: run.sh still invokes '{forbidden}' per run"
                )

        # (6) run.sh must use the shared lockfile-managed e2e/playwright manifest
        #     with `npm ci ... --ignore-scripts`, and the two E2E runner package
        #     versions in e2e/playwright/package.json must match alt-frontend-sv/bun.lock.
        if not re.search(r"npm\s+ci\b[^\n]*--ignore-scripts\b", fe_code):
            problems.append(
                "alt-frontend-sv: run.sh must install runner deps with "
                "`npm ci ... --ignore-scripts`"
            )
        pw_pkg_path = PW_ROOT / "package.json"
        bun_lock_path = ROOT / "alt-frontend-sv" / "bun.lock"
        if pw_pkg_path.is_file() and bun_lock_path.is_file():
            pw_pkg = json.loads(pw_pkg_path.read_text())
            pw_deps = {
                **(pw_pkg.get("dependencies") or {}),
                **(pw_pkg.get("devDependencies") or {}),
            }
            bun_lock_text = bun_lock_path.read_text()
            for pkg_name in ("@playwright/test", "axe-playwright"):
                pinned = pw_deps.get(pkg_name)
                m = re.search(
                    rf'^\s*"{re.escape(pkg_name)}"\s*:\s*\["{re.escape(pkg_name)}@([^"]+)"',
                    bun_lock_text,
                    re.MULTILINE,
                )
                lock_ver = m.group(1) if m else None
                if pinned != lock_ver:
                    problems.append(
                        f"alt-frontend-sv: e2e/playwright/package.json pins "
                        f"{pkg_name}={pinned!r}, but alt-frontend-sv/bun.lock "
                        f"resolves {lock_ver!r}"
                    )

        # (7) pre-launch port guard, container State.Running check, and curl --max-time.
        app_run_pos = fe_code.find("docker run -d")
        pre_launch = fe_code[:app_run_pos] if app_run_pos != -1 else ""
        post_launch = fe_code[app_run_pos:] if app_run_pos != -1 else ""
        if not re.search(r"curl\b[^\n]*--max-time\b[^\n]*/health", pre_launch):
            problems.append(
                "alt-frontend-sv: run.sh must fail before `docker run -d` if "
                "127.0.0.1:${PORT}/health already answers (using curl --max-time)"
            )
        if "State.Running" not in post_launch:
            problems.append(
                "alt-frontend-sv: run.sh health wait must check `docker inspect` "
                "State.Running so an exited container fails immediately"
            )
        if not re.search(r"curl\b[^\n]*--max-time\b[^\n]*/health", post_launch):
            problems.append(
                "alt-frontend-sv: run.sh health wait curl must pass --max-time"
            )

        # (8) tmpfs mount over /work/repo/alt-frontend-sv/node_modules.
        if "destination=/work/repo/alt-frontend-sv/node_modules" not in post_launch:
            problems.append(
                "alt-frontend-sv: run.sh must mount a tmpfs over "
                "/work/repo/alt-frontend-sv/node_modules to prevent local "
                "node_modules from shadowing /work/node_modules"
            )

        # (9) stale-container sweep must be scoped to this run/project, not global.
        fe_joined = re.sub(r"\\\n[ \t]*", " ", fe_code)
        ps_lines = [m.group(0) for m in re.finditer(r"docker ps[^\n]+", fe_joined)]
        if not ps_lines or any(
            "alt.e2e.suite=alt-frontend-sv" not in ps_line
            or (
                "com.docker.compose.project=" not in ps_line
                and "APP_CONTAINER" not in ps_line
                and "SAFE_RUN_ID" not in ps_line
            )
            for ps_line in ps_lines
        ):
            problems.append(
                "alt-frontend-sv: run.sh stale-container sweep must be scoped "
                "to this run/project, not every alt-frontend-sv container on the daemon"
            )

        # (10) env key parity with alt-frontend-sv/playwright.config.ts webServer.env.
        pw_cfg = ROOT / "alt-frontend-sv" / "playwright.config.ts"
        if pw_cfg.is_file():
            cfg_text = pw_cfg.read_text()
            env_block_match = re.search(
                r"webServer\s*:[\s\S]*?\benv\s*:\s*\{([\s\S]*?)\}",
                cfg_text,
            )
            app_cmd_match = re.search(
                r"docker run -d\s+((?:\\\n|[^\n])+)",
                fe_code,
            )
            if env_block_match and app_cmd_match:
                expected_keys = set(
                    re.findall(
                        r"^\s*([A-Z][A-Z0-9_]*)\s*:",
                        env_block_match.group(1),
                        re.MULTILINE,
                    )
                )
                app_cmd = app_cmd_match.group(1).replace("\\\n", " ")
                passed_keys = set(re.findall(r"-e\s+([A-Z][A-Z0-9_]*)=", app_cmd))
                allowed_extra = {"HOST", "NODE_ENV", "PROTOCOL_HEADER"}
                missing_keys = sorted(expected_keys - passed_keys)
                unexpected_keys = sorted((passed_keys - expected_keys) - allowed_extra)
                if missing_keys or unexpected_keys:
                    problems.append(
                        "alt-frontend-sv: run.sh app container env keys drifted from "
                        f"playwright.config.ts webServer.env (missing={missing_keys}, "
                        f"unexpected={unexpected_keys})"
                    )

            # (12) Guard (a): PROTOCOL_HEADER=x-forwarded-proto is passed ONLY to the
            #      E2E app container in alt-frontend-sv/run.sh, the Playwright container
            #      gets E2E_FORWARDED_PROTO=http, and playwright.config.ts opts into
            #      extraHTTPHeaders x-forwarded-proto only when E2E_FORWARDED_PROTO is set.
            pw_cmd_match = re.search(
                r"docker run --rm\s+((?:\\\n|[^\n])+)",
                fe_code,
            )
            app_cmd_str = app_cmd_match.group(1).replace("\\\n", " ") if app_cmd_match else ""
            pw_cmd_str = pw_cmd_match.group(1).replace("\\\n", " ") if pw_cmd_match else ""
            other_proto_refs = [
                other_run.parent.name
                for other_run in sorted(PW_ROOT.glob("*/run.sh"))
                if other_run != frontend_run_sh
                and "PROTOCOL_HEADER"
                in "\n".join(
                    line
                    for line in other_run.read_text().splitlines()
                    if not line.strip().startswith("#")
                )
            ]
            if (
                not re.search(
                    r"(?:^|\s)-e\s+PROTOCOL_HEADER=x-forwarded-proto(?:\s|$)",
                    app_cmd_str,
                )
                or len(re.findall(r"\bPROTOCOL_HEADER\b", fe_code)) != 1
                or re.search(r"\bPROTOCOL_HEADER\b", pw_cmd_str)
                or other_proto_refs
            ):
                problems.append(
                    "alt-frontend-sv: run.sh must pass -e PROTOCOL_HEADER=x-forwarded-proto "
                    "only to the E2E app container"
                )
            if (
                not re.search(
                    r"(?:^|\s)-e\s+E2E_FORWARDED_PROTO=http(?:\s|$)",
                    pw_cmd_str,
                )
                or "E2E_FORWARDED_PROTO" in app_cmd_str
            ):
                problems.append(
                    "alt-frontend-sv: run.sh must pass -e E2E_FORWARDED_PROTO=http "
                    "to the Playwright container"
                )
            if (
                "E2E_FORWARDED_PROTO" not in cfg_text
                or "extraHTTPHeaders" not in cfg_text
                or "x-forwarded-proto" not in cfg_text
                or (
                    env_block_match
                    and "PROTOCOL_HEADER" in env_block_match.group(1)
                )
            ):
                problems.append(
                    "alt-frontend-sv: playwright.config.ts must gate extraHTTPHeaders "
                    "x-forwarded-proto on E2E_FORWARDED_PROTO without setting "
                    "PROTOCOL_HEADER in webServer.env"
                )

    # (13) Guard (b): no compose/*.yaml service definition for alt-frontend-sv
    #      may set PROTOCOL_HEADER (production derives https behind its TLS proxy).
    for compose_file in sorted((ROOT / "compose").glob("*.yaml")):
        with compose_file.open() as handle:
            compose_doc = yaml.load(handle, Loader=ComposeSafeLoader) or {}
        if not isinstance(compose_doc, dict):
            continue
        fe_svc = (compose_doc.get("services") or {}).get("alt-frontend-sv")
        if not isinstance(fe_svc, dict):
            continue
        env_spec = fe_svc.get("environment") or {}
        env_keys: set[str] = set()
        if isinstance(env_spec, dict):
            env_keys.update(str(k) for k in env_spec)
        elif isinstance(env_spec, list):
            for entry_item in env_spec:
                env_keys.add(str(entry_item).split("=", 1)[0].strip())
        if "PROTOCOL_HEADER" in env_keys or "PROTOCOL_HEADER" in json.dumps(fe_svc):
            problems.append(
                f"{compose_file.relative_to(ROOT)}: alt-frontend-sv service must not set "
                "PROTOCOL_HEADER (production must keep deriving https behind its proxy)"
            )

    # (11) suite.sh's suite_init must clear SLICE and SLICE_DIR so an inherited
    #      environment variable cannot make suite_up reuse or suite_cleanup delete a
    #      foreign slice.
    suite_sh = PW_ROOT / "_lib" / "suite.sh"
    if not suite_sh.is_file():
        problems.append("suite.sh: missing e2e/playwright/_lib/suite.sh")
    else:
        suite_sh_code = "\n".join(
            line
            for line in suite_sh.read_text().splitlines()
            if not line.strip().startswith("#")
        )
        init_match = re.search(
            r"^[ \t]*suite_init\s*\(\)\s*\{([\s\S]*?)^\}",
            suite_sh_code,
            re.MULTILINE,
        )
        init_body = init_match.group(1) if init_match else ""
        if not re.search(r"\bunset\b[^\n]*\bSLICE\b", init_body) or not re.search(
            r"\bunset\b[^\n]*\bSLICE_DIR\b", init_body
        ):
            problems.append(
                "suite.sh: suite_init must `unset SLICE SLICE_DIR` so an inherited "
                "environment variable cannot make suite_up reuse or suite_cleanup "
                "delete a foreign slice"
            )

    if problems:
        print(f"suite wiring audit found {len(problems)} problem(s):\n", file=sys.stderr)
        for problem in problems:
            print(f"  - {problem}", file=sys.stderr)
        return 1

    print(f"suite wiring audit: {len(suites)} suites consistent with compose.staging.yaml")
    return 0


if __name__ == "__main__":
    sys.exit(main())
