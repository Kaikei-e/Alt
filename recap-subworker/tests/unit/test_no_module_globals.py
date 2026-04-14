"""Guard test: forbid module-level mutable singletons in the main process tree.

Phase 1 invariant (see docs/ADR/000727): ServiceContainer owns the lifecycle
of engine, session factory, semaphores, and cached pipelines. Module-level
global variables in main-process source files are disallowed because they
leak across TestClient instances and defeat lifespan-based ownership.

Subprocess workers (gunicorn_conf.py, pipeline_worker.py, classification_worker)
are explicitly excluded: they run in spawned child processes where module
globals are the only place per-worker singletons can live.
"""

from __future__ import annotations

import ast
from pathlib import Path

PACKAGE_ROOT = Path(__file__).resolve().parents[2] / "recap_subworker"

# Files that intentionally host per-subprocess module state.
EXCLUDED_FILES = {
    PACKAGE_ROOT / "infra" / "gunicorn_conf.py",
    PACKAGE_ROOT / "services" / "pipeline_worker.py",
    PACKAGE_ROOT / "services" / "classification_worker.py",
}

FORBIDDEN_NAMES = {
    "_ENGINE",
    "_ENGINE_PID",
    "_SESSION_FACTORY",
    "_CONTAINER",
    "_container",
    "_PIPELINE",
    "_EXTRACT_SEMAPHORE",
    "_extract_semaphore",
}


def _iter_package_files() -> list[Path]:
    return [p for p in PACKAGE_ROOT.rglob("*.py") if p not in EXCLUDED_FILES]


def _module_level_assignments(tree: ast.Module) -> set[str]:
    names: set[str] = set()
    for node in tree.body:
        if isinstance(node, ast.Assign):
            for target in node.targets:
                if isinstance(target, ast.Name):
                    names.add(target.id)
        elif isinstance(node, ast.AnnAssign) and isinstance(node.target, ast.Name):
            names.add(node.target.id)
    return names


def test_no_forbidden_module_globals() -> None:
    offenders: list[str] = []
    for file_path in _iter_package_files():
        tree = ast.parse(file_path.read_text(encoding="utf-8"), filename=str(file_path))
        assigned = _module_level_assignments(tree)
        violations = assigned & FORBIDDEN_NAMES
        for name in violations:
            offenders.append(f"{file_path.relative_to(PACKAGE_ROOT.parent)}: {name}")
    assert not offenders, (
        "Forbidden module-level globals detected:\n  " + "\n  ".join(sorted(offenders))
    )
