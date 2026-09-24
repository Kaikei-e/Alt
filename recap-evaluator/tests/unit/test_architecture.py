"""Clean Architecture boundary test for recap-evaluator.

Scans AST of python source files to verify that layer boundaries conform to
Clean Architecture:
- handler must not import gateway, infra, or driver
- usecase must not import gateway, infra, driver, handler, or scheduler
- evaluator must not import handler, usecase, scheduler, gateway, or driver/infra
- port must not import evaluator, scheduler, gateway, driver/infra, usecase, or handler
- scheduler must not import gateway, infra, or driver (may import usecase)
- domain must not import outer layers
"""

from __future__ import annotations

import ast
from pathlib import Path

import pytest


def _resolve_import_targets(
    node: ast.Import | ast.ImportFrom,
    py_file: Path,
    src_dir: Path,
) -> list[str]:
    targets: list[str] = []
    if isinstance(node, ast.Import):
        for alias in node.names:
            targets.append(alias.name)
    elif isinstance(node, ast.ImportFrom):
        if node.level > 0:
            rel_path = py_file.resolve().relative_to(src_dir.resolve())
            package_parts = rel_path.parent.parts
            cutoff = len(package_parts) - (node.level - 1)
            if cutoff < 0:
                raise ValueError(
                    f"Relative import level {node.level} goes beyond src root in {py_file}"
                )
            base_parts = list(package_parts[:cutoff])
            if node.module:
                full_module = ".".join([*base_parts, node.module])
                targets.append(full_module)
                for alias in node.names:
                    targets.append(f"{full_module}.{alias.name}")
            else:
                for alias in node.names:
                    targets.append(".".join([*base_parts, alias.name]))
        else:
            module = node.module or ""
            targets.append(module)
            for alias in node.names:
                targets.append(f"{module}.{alias.name}" if module else alias.name)
    return targets


def _scan_violations(
    directory: Path,
    forbidden_prefixes: tuple[str, ...],
    src_dir: Path,
) -> list[str]:
    violations: list[str] = []
    py_files = [p for p in sorted(directory.glob("**/*.py")) if "__pycache__" not in p.parts]
    assert py_files, f"No python files found in {directory} (trivial pass prevented)"

    for py_file in py_files:
        tree = ast.parse(py_file.read_text(encoding="utf-8"), filename=str(py_file))
        for node in ast.walk(tree):
            if isinstance(node, (ast.Import, ast.ImportFrom)):
                for target in _resolve_import_targets(node, py_file, src_dir):
                    matched = False
                    for prefix in forbidden_prefixes:
                        if target == prefix or target.startswith(f"{prefix}."):
                            violations.append(f"{py_file.name}:{node.lineno}: import from {target}")
                            matched = True
                            break
                    if matched:
                        break
    return violations


def _get_src_dir() -> Path:
    return Path(__file__).resolve().parents[2] / "src"


def test_handler_does_not_import_gateway_or_infra():
    src_dir = _get_src_dir()
    base_dir = src_dir / "recap_evaluator" / "handler"
    violations = _scan_violations(
        base_dir,
        (
            "recap_evaluator.gateway",
            "recap_evaluator.infra",
            "recap_evaluator.driver",
        ),
        src_dir,
    )
    assert not violations, f"Handler layer violations: {violations}"


def test_usecase_does_not_import_gateway_or_infra():
    src_dir = _get_src_dir()
    base_dir = src_dir / "recap_evaluator" / "usecase"
    violations = _scan_violations(
        base_dir,
        (
            "recap_evaluator.gateway",
            "recap_evaluator.infra",
            "recap_evaluator.driver",
            "recap_evaluator.handler",
            "recap_evaluator.scheduler",
        ),
        src_dir,
    )
    assert not violations, f"Usecase layer violations: {violations}"


def test_evaluator_does_not_import_forbidden_layers():
    src_dir = _get_src_dir()
    base_dir = src_dir / "recap_evaluator" / "evaluator"
    violations = _scan_violations(
        base_dir,
        (
            "recap_evaluator.gateway",
            "recap_evaluator.infra",
            "recap_evaluator.driver",
            "recap_evaluator.handler",
            "recap_evaluator.usecase",
            "recap_evaluator.scheduler",
        ),
        src_dir,
    )
    assert not violations, f"Evaluator layer violations: {violations}"


def test_port_does_not_import_outer_layers():
    src_dir = _get_src_dir()
    base_dir = src_dir / "recap_evaluator" / "port"
    violations = _scan_violations(
        base_dir,
        (
            "recap_evaluator.gateway",
            "recap_evaluator.infra",
            "recap_evaluator.driver",
            "recap_evaluator.usecase",
            "recap_evaluator.handler",
            "recap_evaluator.evaluator",
            "recap_evaluator.scheduler",
        ),
        src_dir,
    )
    assert not violations, f"Port layer violations: {violations}"


def test_domain_does_not_import_outer_layers():
    src_dir = _get_src_dir()
    base_dir = src_dir / "recap_evaluator" / "domain"
    violations = _scan_violations(
        base_dir,
        (
            "recap_evaluator.gateway",
            "recap_evaluator.infra",
            "recap_evaluator.driver",
            "recap_evaluator.usecase",
            "recap_evaluator.handler",
            "recap_evaluator.port",
            "recap_evaluator.evaluator",
            "recap_evaluator.scheduler",
        ),
        src_dir,
    )
    assert not violations, f"Domain layer violations: {violations}"


def test_scheduler_does_not_import_gateway_or_driver():
    src_dir = _get_src_dir()
    base_dir = src_dir / "recap_evaluator" / "scheduler"
    violations = _scan_violations(
        base_dir,
        (
            "recap_evaluator.gateway",
            "recap_evaluator.infra",
            "recap_evaluator.driver",
        ),
        src_dir,
    )
    assert not violations, f"Scheduler layer violations: {violations}"


def test_architecture_rule_detector_flags_forbidden_import(tmp_path: Path):
    src_dir = tmp_path / "src"
    test_dir = src_dir / "recap_evaluator" / "evaluator"
    test_dir.mkdir(parents=True)
    bad_file = test_dir / "bad.py"
    bad_file.write_text("import recap_evaluator.gateway.ollama_gateway\n")
    violations = _scan_violations(test_dir, ("recap_evaluator.gateway",), src_dir)
    assert len(violations) == 1
    assert "bad.py:1: import from recap_evaluator.gateway.ollama_gateway" in violations[0]


def test_architecture_rule_detector_resolves_relative_imports(tmp_path: Path):
    src_dir = tmp_path / "src"
    test_dir = src_dir / "recap_evaluator" / "evaluator"
    test_dir.mkdir(parents=True)
    bad_file = test_dir / "eval.py"
    bad_file.write_text("from ..gateway.ollama_gateway import OllamaGateway\n")
    violations = _scan_violations(test_dir, ("recap_evaluator.gateway",), src_dir)
    assert len(violations) == 1
    assert "eval.py:1: import from recap_evaluator.gateway.ollama_gateway" in violations[0]


def test_architecture_fails_on_empty_directory(tmp_path: Path):
    empty_dir = tmp_path / "empty"
    empty_dir.mkdir()
    with pytest.raises(AssertionError, match="No python files found"):
        _scan_violations(empty_dir, ("recap_evaluator.gateway",), tmp_path)
