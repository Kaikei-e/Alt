#!/usr/bin/env python3
"""Single-source-of-truth wrapper delegating to .github/workflows/scripts/workflow-safety-check.py.

Retained at scripts/workflow-safety-check.py for backward compatibility with
docs and tests that import or invoke this path directly.
"""

from __future__ import annotations

import importlib.util
from pathlib import Path

_CANONICAL_PATH = (
    Path(__file__).resolve().parent.parent
    / ".github"
    / "workflows"
    / "scripts"
    / "workflow-safety-check.py"
)
_SPEC = importlib.util.spec_from_file_location("_canonical_workflow_safety_check", _CANONICAL_PATH)
if _SPEC is None or _SPEC.loader is None:
    msg = f"Cannot load canonical workflow-safety-check module from {_CANONICAL_PATH}"
    raise RuntimeError(msg)

_CANONICAL_MODULE = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(_CANONICAL_MODULE)

_PRESERVE_DUNDERS = frozenset(
    {"__name__", "__doc__", "__file__", "__loader__", "__package__", "__spec__"}
)
globals().update(
    {
        name: value
        for name, value in vars(_CANONICAL_MODULE).items()
        if name not in _PRESERVE_DUNDERS
    }
)

if __name__ == "__main__":
    raise SystemExit(_CANONICAL_MODULE.main())
