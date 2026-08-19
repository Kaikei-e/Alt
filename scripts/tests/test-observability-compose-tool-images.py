#!/usr/bin/env python3
"""RED/GREEN tests: compose tool images must come from PyYAML, not grep.

Comments in compose/observability.yaml must not select Prometheus or
Alertmanager versions. Missing images and :latest are fail-closed.

Run:
    python3 scripts/tests/test-observability-compose-tool-images.py
"""

from __future__ import annotations

import importlib.util
import pathlib
import sys
import tempfile
import textwrap

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"

spec = importlib.util.spec_from_file_location(
    "compose_tool_images", SCRIPTS / "observability-compose-tool-images.py"
)
assert spec is not None and spec.loader is not None
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)

PASS = 0
FAIL = 0


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


def parse(text: str, *, require_am: bool = True) -> tuple[dict[str, str] | None, str]:
    with tempfile.NamedTemporaryFile("w", suffix=".yaml", encoding="utf-8", delete=False) as handle:
        handle.write(text)
        path = pathlib.Path(handle.name)
    try:
        return mod.parse_tool_images(path, require_alertmanager=require_am), ""
    except (SystemExit, ValueError, RuntimeError) as exc:
        return None, str(exc)
    finally:
        path.unlink(missing_ok=True)


print("compose tool images (PyYAML, comments cannot select versions)")

comment_trap = textwrap.dedent(
    """\
    services:
      prometheus:
        # image: prom/prometheus:v9.9.9
        image: prom/prometheus:v3.1.0
      alertmanager:
        # image: prom/alertmanager:latest
        image: prom/alertmanager:v0.28.1
    """
)
got, err = parse(comment_trap)
check("commented v9.9.9 / latest do not win", got is not None, err)
if got:
    check("prometheus version is 3.1.0 from the mapping", got["prometheus_version"] == "3.1.0", str(got))
    check("alertmanager version is 0.28.1 from the mapping", got["alertmanager_version"] == "0.28.1", str(got))
    check(
        "full prometheus image is pinned",
        got["prometheus_image"] == "prom/prometheus:v3.1.0",
        str(got),
    )

missing, err = parse("services:\n  grafana:\n    image: grafana/grafana:11.0.0\n")
check("missing prometheus image fails", missing is None and "prometheus" in err.lower(), err)

latest, err = parse(
    textwrap.dedent(
        """\
        services:
          prometheus:
            image: prom/prometheus:latest
          alertmanager:
            image: prom/alertmanager:v0.28.1
        """
    )
)
check("prometheus :latest fails", latest is None and "latest" in err.lower(), err)

unpinned, err = parse(
    textwrap.dedent(
        """\
        services:
          prometheus:
            image: prom/prometheus
          alertmanager:
            image: prom/alertmanager:v0.28.1
        """
    )
)
check("unpinned prometheus image fails", unpinned is None, err)

prod = mod.parse_tool_images(
    ROOT / "compose" / "observability.yaml", require_alertmanager=True
)
check("production prometheus is v3.1.0", prod["prometheus_version"] == "3.1.0", str(prod))
check("production alertmanager is v0.28.1", prod["alertmanager_version"] == "0.28.1", str(prod))

validate = (ROOT / "scripts" / "observability-validate.sh").read_text(encoding="utf-8")
check(
    "validate.sh does not grep compose images",
    "grep -oE" not in validate and "image_from_compose" not in validate,
    "grep/fallback can select a commented tag or :latest",
)
check(
    "validate.sh has no :latest image fallback",
    "prom/prometheus:latest" not in validate and "prom/alertmanager:latest" not in validate,
)
check(
    "validate.sh calls observability-compose-tool-images.py",
    "scripts/observability-compose-tool-images.py" in validate,
)

workflow = (ROOT / ".github" / "workflows" / "observability-validate.yaml").read_text(
    encoding="utf-8"
)
check(
    "workflow does not grep compose image tags",
    "grep -oE" not in workflow,
)
# Stronger: the version step must invoke the PyYAML parser.
check(
    "workflow resolves versions via PyYAML parser",
    "observability-compose-tool-images.py" in workflow,
    "comments cannot select versions if the parser loads the YAML mapping",
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
