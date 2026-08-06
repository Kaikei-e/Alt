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

import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[3]
PW_ROOT = ROOT / "e2e" / "playwright"
SUITES_YAML = PW_ROOT / "suites.yaml"
COMPOSE = ROOT / "compose" / "compose.staging.yaml"

TAG_VAR = re.compile(r"\$\{([A-Z0-9_]+_IMAGE_TAG)")


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

    # (3) suites.yaml and the directory tree must agree.
    on_disk = {
        path.parent.name
        for path in PW_ROOT.glob("*/playwright.config.ts")
        # The browser suite is a dispatch shim: it builds no images and runs
        # under the Playwright browser image, so it is deliberately not in the
        # manifest. See e2e/playwright/README.md.
        if path.parent.name != "alt-frontend-sv"
    }
    for name in sorted(on_disk - set(suites)):
        problems.append(
            f"{name}: has a playwright.config.ts but no `suites:` entry, so it never runs in CI"
        )
    for name in sorted(set(suites) - on_disk):
        problems.append(f"{name}: declared in suites.yaml but has no playwright.config.ts")

    for name in sorted(suites):
        entry = suites[name] or {}
        declared_images = list(entry.get("images") or [])
        unpinned = set(entry.get("unpinnedImages") or [])

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
        forwarded: set[str] = set()
        for line in run_sh.read_text().splitlines():
            stripped = line.strip()
            if stripped.startswith("#") or not stripped.startswith("suite_image_tags"):
                continue
            forwarded.update(re.findall(r"[A-Z0-9_]+_IMAGE_TAG", stripped))

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

    if problems:
        print(f"suite wiring audit found {len(problems)} problem(s):\n", file=sys.stderr)
        for problem in problems:
            print(f"  - {problem}", file=sys.stderr)
        return 1

    print(f"suite wiring audit: {len(suites)} suites consistent with compose.staging.yaml")
    return 0


if __name__ == "__main__":
    sys.exit(main())
