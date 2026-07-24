"""CI-time drift gate: locked scikit-learn version vs. shipped classifier artefact.

The genre_classifier_*.joblib artefacts (recap-subworker/data/) are gitignored,
host-mounted files installed by runner bootstrap (see the recap-subworker
volumes comment in compose/recap.yaml and docs/runbooks/runner-setup.md
§2.6) — a fresh CI checkout never has them, so this gate cannot open their
.meta.json sidecar directly. Instead it treats uv.lock as the build-time
source of truth for "what scikit-learn will actually ship" and compares its
resolved minor version against the sklearn_version the shipped artefacts
were trained under. At runtime, recap_subworker.services.classifier.
_guard_metadata_against_runtime performs the equivalent comparison (minor
precision, ADR-000835 stage 3) and raises ConfigValidationError on mismatch,
which is why the 2026-07-08 scikit-learn 1.8.0 -> 1.9.0 lock bump broke the
classification worker pool at startup without any code change. This test
catches that class of drift at `uv lock` time, before an image is built.

Bump EXPECTED_ARTIFACT_SKLEARN_MINOR only together with retraining the
shipped artefacts and updating their .meta.json sidecars.
"""

from __future__ import annotations

import tomllib
from pathlib import Path

# Recorded in recap-subworker/data/genre_classifier_ja.meta.json
# ("sklearn_version": "1.8.0") as of the last retrain, 2026-04-23. That file
# is gitignored/host-only, so the expectation is pinned here explicitly
# rather than read at test time.
EXPECTED_ARTIFACT_SKLEARN_MINOR = "1.8"

UV_LOCK_PATH = Path(__file__).resolve().parents[2] / "uv.lock"


def _locked_scikit_learn_version() -> str:
    with UV_LOCK_PATH.open("rb") as f:
        lock = tomllib.load(f)
    for package in lock["package"]:
        if package["name"] == "scikit-learn":
            return str(package["version"])
    raise AssertionError("scikit-learn package entry not found in uv.lock")


def _minor(version: str) -> str:
    major, minor, *_ = version.split(".")
    return f"{major}.{minor}"


def test_locked_sklearn_minor_matches_shipped_classifier_artifact() -> None:
    locked_version = _locked_scikit_learn_version()
    locked_minor = _minor(locked_version)
    assert locked_minor == EXPECTED_ARTIFACT_SKLEARN_MINOR, (
        f"recap-subworker/uv.lock resolves scikit-learn=={locked_version} "
        f"(minor {locked_minor}), but the shipped genre_classifier joblib "
        f"artefacts were trained under scikit-learn "
        f"{EXPECTED_ARTIFACT_SKLEARN_MINOR}.x. "
        "recap_subworker.services.classifier._guard_metadata_against_runtime "
        "fail-closes at startup on exactly this mismatch (ADR-000835 stage "
        "3), so the classification worker pool never initializes. Either "
        "pin scikit-learn in pyproject.toml to the artefact's trained minor "
        "version and re-run `uv lock`, or retrain the artefacts under the "
        "new scikit-learn and update EXPECTED_ARTIFACT_SKLEARN_MINOR plus "
        "the artefacts' .meta.json sidecars together."
    )
