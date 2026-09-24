"""Clean Architecture layer boundary fitness tests using AST analysis.

Enforces:
- handler -> usecase, domain
- usecase -> port, domain (no infra libraries like httpx, no gateway, no driver)
- port -> domain only (no infra, no gateway, no driver, no usecase)
- domain -> stdlib only (no infra, no outer layers)
"""

from __future__ import annotations

import ast
from pathlib import Path

ACOLYTE_ROOT = Path(__file__).resolve().parents[2] / "acolyte"

FORBIDDEN_INFRA_IN_CORE = {
    "httpx",
    "asyncpg",
    "redis",
    "sqlalchemy",
    "fastapi",
    "starlette",
    "psycopg",
    "psycopg_pool",
    "pyqwest",
}


def _iter_py(directory: Path) -> list[Path]:
    return [p for p in directory.rglob("*.py") if "__pycache__" not in p.parts]


def _resolve_relative_import(file_path: Path, level: int, module: str | None) -> str:
    rel_parts = file_path.relative_to(ACOLYTE_ROOT).parts
    pkg_parts = ("acolyte", *rel_parts[:-1])
    if level > len(pkg_parts):
        base_parts: tuple[str, ...] = ()
    elif level == 1:
        base_parts = pkg_parts
    else:
        base_parts = pkg_parts[: -(level - 1)]
    base = ".".join(base_parts)
    if module:
        return f"{base}.{module}" if base else module
    return base


def _collect_imports(tree: ast.Module, file_path: Path) -> list[tuple[str, int]]:
    imports: list[tuple[str, int]] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                imports.append((alias.name, node.lineno))
        elif isinstance(node, ast.ImportFrom):
            if node.level and node.level > 0:
                mod = _resolve_relative_import(file_path, node.level, node.module)
                imports.append((mod, node.lineno))
                if not node.module:
                    for alias in node.names:
                        sub = f"{mod}.{alias.name}" if mod else alias.name
                        imports.append((sub, node.lineno))
            else:
                mod = node.module or ""
                imports.append((mod, node.lineno))
    return imports


def _scan_layer_imports(
    layer_name: str,
    *,
    forbidden_layers: tuple[str, ...],
    forbidden_infra: set[str] | None = None,
) -> list[str]:
    files = _iter_py(ACOLYTE_ROOT / layer_name)
    assert files, f"No python files found in layer '{layer_name}'"

    offenders: list[str] = []
    infra = forbidden_infra or set()

    for file_path in files:
        tree = ast.parse(file_path.read_text(encoding="utf-8"), filename=str(file_path))
        rel = file_path.relative_to(ACOLYTE_ROOT.parent)
        for mod, lineno in _collect_imports(tree, file_path):
            head = mod.split(".", 1)[0]
            if head in infra:
                offenders.append(f"{rel}:{lineno} imports forbidden infra '{mod}'")
            elif any(mod == pkg or mod.startswith(f"{pkg}.") for pkg in forbidden_layers):
                offenders.append(f"{rel}:{lineno} imports forbidden layer '{mod}'")

    return offenders


def test_usecase_layer_has_no_infra_or_outer_imports() -> None:
    offenders = _scan_layer_imports(
        "usecase",
        forbidden_layers=("acolyte.gateway", "acolyte.driver", "acolyte.handler"),
        forbidden_infra=FORBIDDEN_INFRA_IN_CORE,
    )
    assert not offenders, "usecase layer has forbidden imports:\n  " + "\n  ".join(offenders)


def test_port_layer_has_no_infra_or_outer_imports() -> None:
    offenders = _scan_layer_imports(
        "port",
        forbidden_layers=(
            "acolyte.gateway",
            "acolyte.driver",
            "acolyte.handler",
            "acolyte.usecase",
        ),
        forbidden_infra=FORBIDDEN_INFRA_IN_CORE,
    )
    assert not offenders, "port layer has forbidden imports:\n  " + "\n  ".join(offenders)


def test_domain_layer_has_no_infra_or_outer_imports() -> None:
    offenders = _scan_layer_imports(
        "domain",
        forbidden_layers=(
            "acolyte.gateway",
            "acolyte.driver",
            "acolyte.handler",
            "acolyte.usecase",
            "acolyte.port",
        ),
        forbidden_infra=FORBIDDEN_INFRA_IN_CORE,
    )
    assert not offenders, "domain layer has forbidden imports:\n  " + "\n  ".join(offenders)


def test_handler_layer_has_no_driver_or_gateway_imports() -> None:
    offenders = _scan_layer_imports(
        "handler",
        forbidden_layers=("acolyte.gateway", "acolyte.driver"),
    )
    assert not offenders, "handler layer has forbidden imports:\n  " + "\n  ".join(offenders)
