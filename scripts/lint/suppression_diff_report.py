#!/usr/bin/env python3
"""Thin wrapper. The implementation lives in scripts/altharness (Go)."""

from __future__ import annotations

import os
import sys
from pathlib import Path


def main() -> None:
    root = Path(__file__).resolve().parents[2]
    launcher = root / "scripts" / "altharness" / "alt-harness"
    os.execv(launcher, [str(launcher), "suppression-diff", *sys.argv[1:]])  # noqa: S606


if __name__ == "__main__":
    main()
