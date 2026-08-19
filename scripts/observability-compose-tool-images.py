#!/usr/bin/env python3
"""Parse Prometheus and Alertmanager images from compose/observability.yaml.

Uses PyYAML so comments cannot select versions. Missing images, :latest,
and unpinned tags fail closed. No grep, no fallback.

Run:
    python3 scripts/observability-compose-tool-images.py --export-shell
    python3 scripts/observability-compose-tool-images.py --github-output
"""

from __future__ import annotations

import argparse
import pathlib
import re
import sys

try:
    import yaml
except ImportError as exc:  # pragma: no cover - CI installs PyYAML
    raise SystemExit("PyYAML is required: pip install 'pyyaml==6.*'") from exc

REPO_ROOT = pathlib.Path(__file__).resolve().parents[1]
DEFAULT_COMPOSE = REPO_ROOT / "compose" / "observability.yaml"

# GitHub release assets want 3.1.0, compose pins prom/prometheus:v3.1.0.
PINNED_TAG_RE = re.compile(
    r"^(?P<repo>[^:@\s]+/[^:@\s]+):(?P<tag>v?(?P<version>\d+(?:\.\d+)+))$"
)
EXPECTED_REPOS = {
    "prometheus": "prom/prometheus",
    "alertmanager": "prom/alertmanager",
}


def parse_tool_images(
    path: pathlib.Path, *, require_alertmanager: bool = True
) -> dict[str, str]:
    """Return pinned images/versions from the compose services mapping.

    Raises ValueError when the pin is missing, :latest, or unpinned.
    """
    try:
        raw = path.read_text(encoding="utf-8")
    except OSError as exc:
        raise ValueError(f"cannot read compose file {path}: {exc}") from exc

    doc = yaml.safe_load(raw)
    if not isinstance(doc, dict):
        raise ValueError(f"{path}: compose root must be a mapping")

    services = doc.get("services")
    if not isinstance(services, dict):
        raise ValueError(f"{path}: compose file has no services mapping")

    prometheus_image = _service_image(services, "prometheus", path)
    result = {
        "prometheus_image": prometheus_image,
        "prometheus_version": _pinned_version(prometheus_image, "prometheus"),
    }

    if require_alertmanager or "alertmanager" in services:
        alertmanager_image = _service_image(services, "alertmanager", path)
        result["alertmanager_image"] = alertmanager_image
        result["alertmanager_version"] = _pinned_version(
            alertmanager_image, "alertmanager"
        )
    elif require_alertmanager:
        raise ValueError(f"{path}: missing alertmanager image")

    return result


def _service_image(services: dict, name: str, path: pathlib.Path) -> str:
    service = services.get(name)
    if not isinstance(service, dict):
        raise ValueError(f"{path}: missing {name} service")
    image = service.get("image")
    if image is None or (isinstance(image, str) and not image.strip()):
        raise ValueError(f"{path}: missing {name} image")
    if not isinstance(image, str):
        raise ValueError(
            f"{path}: {name} image must be a string pin, got {type(image).__name__}"
        )
    return image.strip()


def _pinned_version(image: str, service: str) -> str:
    expected_repo = EXPECTED_REPOS[service]
    lowered = image.lower()
    if lowered.endswith(":latest") or lowered.endswith("/latest"):
        raise ValueError(f"{service} image must not use :latest, got {image!r}")

    match = PINNED_TAG_RE.match(image)
    if match is None:
        raise ValueError(
            f"{service} image must be {expected_repo}:vMAJOR.MINOR.PATCH, got {image!r}"
        )
    repo = match.group("repo")
    if repo != expected_repo:
        raise ValueError(
            f"{service} image repo must be {expected_repo}, got {repo!r}"
        )
    return match.group("version")


def github_output_lines(parsed: dict[str, str]) -> str:
    prometheus = parsed["prometheus_version"]
    alertmanager = parsed.get("alertmanager_version", "")
    return f"prometheus={prometheus}\nalertmanager={alertmanager}\n"


def shell_export_lines(parsed: dict[str, str]) -> str:
    lines = [f"PROM_IMAGE={parsed['prometheus_image']}"]
    if "alertmanager_image" in parsed:
        lines.append(f"AM_IMAGE={parsed['alertmanager_image']}")
    return "\n".join(lines) + "\n"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--compose",
        type=pathlib.Path,
        default=DEFAULT_COMPOSE,
        help="compose/observability.yaml path",
    )
    parser.add_argument(
        "--github-output",
        action="store_true",
        help="print prometheus=/alertmanager= lines for GITHUB_OUTPUT",
    )
    parser.add_argument(
        "--export-shell",
        action="store_true",
        help="print PROM_IMAGE=/AM_IMAGE= assignments for eval",
    )
    parser.add_argument(
        "--require-alertmanager",
        action=argparse.BooleanOptionalAction,
        default=True,
    )
    args = parser.parse_args(argv)
    try:
        parsed = parse_tool_images(
            args.compose, require_alertmanager=args.require_alertmanager
        )
    except ValueError as exc:
        print(f"::error::{exc}", file=sys.stderr)
        return 1
    if args.github_output:
        sys.stdout.write(github_output_lines(parsed))
        return 0
    if args.export_shell:
        sys.stdout.write(shell_export_lines(parsed))
        return 0
    sys.stdout.write(
        f"prometheus {parsed['prometheus_image']} (v{parsed['prometheus_version']})\n"
    )
    if "alertmanager_image" in parsed:
        sys.stdout.write(
            f"alertmanager {parsed['alertmanager_image']} "
            f"(v{parsed['alertmanager_version']})\n"
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
