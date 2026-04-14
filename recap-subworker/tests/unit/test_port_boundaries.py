"""Clean-Architecture boundary test: guard against external ML lib imports
leaking into usecase / domain layers.

Phase 3 installs this invariant incrementally. For the tranche committed
with Phase 3A, ``usecase/`` is the only tree that must be strictly
boundary-clean; ``domain/`` retains a short whitelist of files that
still reach into numpy / scipy / sklearn and will be migrated in
Phase 3B. Each whitelist entry is a conscious tech-debt marker —
adding a new whitelist row should require a matching ADR rationale.

External libraries considered "outside" the pure layer:

- numpy      (foundational, but still expected only at gateway / driver)
- scipy, sklearn, hdbscan, umap, faiss, sentence_transformers, torch

numpy usage inside ``port/`` is tolerated because ports use
``numpy.ndarray`` exclusively as a structural type alias for embeddings
and cluster labels; swapping the alias is a Phase 4+ concern.
"""

from __future__ import annotations

import ast
from pathlib import Path

PACKAGE_ROOT = Path(__file__).resolve().parents[2] / "recap_subworker"

EXTERNAL_ML_PACKAGES = {
    "numpy",
    "scipy",
    "sklearn",
    "hdbscan",
    "umap",
    "faiss",
    "sentence_transformers",
    "torch",
    "transformers",
    "statsmodels",
}

# Tech-debt whitelist. Each entry MUST be removed when the corresponding
# migration completes. Grow only with an ADR rationale.
DOMAIN_WHITELIST: set[Path] = {
    PACKAGE_ROOT / "domain" / "topics.py",           # c-TF-IDF via sklearn (Phase 3B)
    PACKAGE_ROOT / "domain" / "selectors.py",        # numpy MMR helper (Phase 3B)
    PACKAGE_ROOT / "domain" / "classification" / "model.py",  # numpy genre scoring (Phase 3B)
    PACKAGE_ROOT / "domain" / "analysis" / "stats.py",  # scipy / statsmodels (Phase 3B)
}

PORT_WHITELIST: set[Path] = {
    PACKAGE_ROOT / "port" / "embedder.py",           # numpy.ndarray type alias
    PACKAGE_ROOT / "port" / "clusterer.py",          # numpy.ndarray + labels alias
}


def _iter_py(directory: Path) -> list[Path]:
    return [p for p in directory.rglob("*.py") if "__pycache__" not in p.parts]


def _external_ml_imports(tree: ast.Module) -> set[str]:
    hits: set[str] = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                head = alias.name.split(".", 1)[0]
                if head in EXTERNAL_ML_PACKAGES:
                    hits.add(head)
        elif isinstance(node, ast.ImportFrom) and node.module:
            head = node.module.split(".", 1)[0]
            if head in EXTERNAL_ML_PACKAGES:
                hits.add(head)
    return hits


def test_usecase_layer_has_no_external_ml_imports() -> None:
    """Phase 2+ usecases must be strictly free of ML library imports."""
    offenders: list[str] = []
    for file_path in _iter_py(PACKAGE_ROOT / "usecase"):
        tree = ast.parse(file_path.read_text(encoding="utf-8"), filename=str(file_path))
        hits = _external_ml_imports(tree)
        if hits:
            offenders.append(
                f"{file_path.relative_to(PACKAGE_ROOT.parent)}: {sorted(hits)}"
            )
    assert not offenders, (
        "usecase/ must not import ML libraries:\n  " + "\n  ".join(offenders)
    )


def test_domain_layer_external_imports_within_whitelist() -> None:
    """Domain files importing ML libs must be on the tech-debt whitelist."""
    unexpected: list[str] = []
    for file_path in _iter_py(PACKAGE_ROOT / "domain"):
        tree = ast.parse(file_path.read_text(encoding="utf-8"), filename=str(file_path))
        hits = _external_ml_imports(tree)
        if hits and file_path not in DOMAIN_WHITELIST:
            unexpected.append(
                f"{file_path.relative_to(PACKAGE_ROOT.parent)}: {sorted(hits)}"
            )
    assert not unexpected, (
        "New domain/ files must not reach for ML libraries. "
        "If a legacy file was migrated, remove it from DOMAIN_WHITELIST instead:\n  "
        + "\n  ".join(unexpected)
    )


def test_port_layer_external_imports_within_whitelist() -> None:
    """Port files are type-only for now; any new ML import must be reviewed."""
    unexpected: list[str] = []
    for file_path in _iter_py(PACKAGE_ROOT / "port"):
        tree = ast.parse(file_path.read_text(encoding="utf-8"), filename=str(file_path))
        hits = _external_ml_imports(tree)
        if hits and file_path not in PORT_WHITELIST:
            unexpected.append(
                f"{file_path.relative_to(PACKAGE_ROOT.parent)}: {sorted(hits)}"
            )
    assert not unexpected, (
        "New port/ files must stay free of ML libraries:\n  "
        + "\n  ".join(unexpected)
    )


def test_stopwords_moved_out_of_domain() -> None:
    """Phase 3A invariant: stopwords helpers live in infra, not domain."""
    legacy = PACKAGE_ROOT / "domain" / "stopwords.py"
    relocated = PACKAGE_ROOT / "infra" / "stopwords.py"
    if legacy.exists():
        # Acceptable only if it is a re-export shim (no direct sklearn/nltk imports).
        tree = ast.parse(legacy.read_text(encoding="utf-8"), filename=str(legacy))
        assert not _external_ml_imports(tree), (
            "domain/stopwords.py must not import ML libs directly. "
            "Move the implementation to infra/stopwords.py and keep a re-export."
        )
    assert relocated.exists(), (
        "Phase 3A expects infra/stopwords.py as the canonical owner of the "
        "stopword tables (sklearn + nltk dependencies live in infra/)."
    )
