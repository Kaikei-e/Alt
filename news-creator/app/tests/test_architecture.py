import ast
from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent.parent / "news_creator"


def _resolve_module(py_file: Path, node: ast.ImportFrom) -> str:
    if node.level == 0:
        return node.module or ""
    rel_path = py_file.relative_to(BASE_DIR.parent)
    pkg_parts = list(rel_path.parent.parts)
    slice_idx = max(0, len(pkg_parts) - (node.level - 1))
    target_parts = pkg_parts[:slice_idx]
    if node.module:
        return ".".join(target_parts + node.module.split("."))
    return ".".join(target_parts)


def _scan_violations(
    layer_dir: Path,
    forbidden_prefixes: set[str],
) -> list[str]:
    violations: list[str] = []
    for py_file in sorted(layer_dir.glob("**/*.py")):
        if "__pycache__" in py_file.parts:
            continue
        source = py_file.read_text(encoding="utf-8")
        tree = ast.parse(source, filename=str(py_file))
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                for alias in node.names:
                    for prefix in forbidden_prefixes:
                        if alias.name == prefix or alias.name.startswith(f"{prefix}."):
                            violations.append(
                                f"{py_file.name}:{node.lineno}: import {alias.name}"
                            )
            elif isinstance(node, ast.ImportFrom):
                target_module = _resolve_module(py_file, node)
                for prefix in forbidden_prefixes:
                    if target_module == prefix or target_module.startswith(
                        f"{prefix}."
                    ):
                        imported_names = ", ".join(a.name for a in node.names)
                        violations.append(
                            f"{py_file.name}:{node.lineno}: from {target_module} import {imported_names}"
                        )
                    elif not node.module:
                        for alias in node.names:
                            full_name = f"{target_module}.{alias.name}"
                            if full_name == prefix or full_name.startswith(
                                f"{prefix}."
                            ):
                                violations.append(
                                    f"{py_file.name}:{node.lineno}: from {target_module} import {alias.name}"
                                )
    return violations


def test_handlers_do_not_import_driver_or_gateway():
    handler_dir = BASE_DIR / "handler"
    forbidden = {"news_creator.driver", "news_creator.gateway"}
    violations = _scan_violations(handler_dir, forbidden)
    assert not violations, (
        f"Found {len(violations)} Clean Architecture violations in handlers:\n"
        + "\n".join(violations)
    )


def test_usecases_do_not_import_infra_or_outer():
    usecase_dir = BASE_DIR / "usecase"
    forbidden = {
        "news_creator.driver",
        "news_creator.gateway",
        "news_creator.handler",
        "httpx",
        "asyncpg",
        "redis",
        "sqlalchemy",
        "fastapi",
        "starlette",
        "psycopg",
    }
    violations = _scan_violations(usecase_dir, forbidden)
    assert not violations, (
        f"Found {len(violations)} Clean Architecture violations in usecases:\n"
        + "\n".join(violations)
    )
