#!/usr/bin/env python3
"""Unit tests for scripts/codegen/sync_go_canonical.py."""

from __future__ import annotations

import importlib.util
import pathlib
import shutil
import sys
import tempfile
import types
import unittest
from unittest import mock

REPO_ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT_PATH = REPO_ROOT / "scripts" / "codegen" / "sync_go_canonical.py"


def load_sync_module() -> types.ModuleType:
    spec = importlib.util.spec_from_file_location("sync_go_canonical", SCRIPT_PATH)
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    with mock.patch.dict(sys.modules, {"sync_go_canonical": module}):
        spec.loader.exec_module(module)
    return module


class TestSyncGoCanonical(unittest.TestCase):
    def setUp(self) -> None:
        self.mod = load_sync_module()

    def _make_temp_repo(self) -> pathlib.Path:
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        root = pathlib.Path(td.name)
        shutil.copytree(
            REPO_ROOT / "scripts" / "codegen" / "go_canonical",
            root / "scripts" / "codegen" / "go_canonical",
        )
        return root

    def test_check_fails_when_targets_missing_and_passes_after_sync(self) -> None:
        root = self._make_temp_repo()
        assert self.mod.sync(root, check=True) == 1
        assert self.mod.sync(root, check=False) == 0
        assert self.mod.sync(root, check=True) == 0

        generated = root / "auth-hub" / "internal" / "httpserver" / "httpserver_gen.go"
        assert generated.is_file()
        first_line = generated.read_text(encoding="utf-8").splitlines()[0]
        assert first_line == self.mod.HEADER_PREFIX

    def test_check_detects_tampered_generated_file(self) -> None:
        root = self._make_temp_repo()
        assert self.mod.sync(root, check=False) == 0

        target = (
            root / "search-indexer" / "app" / "internal" / "connectguard" / "connectguard_gen.go"
        )
        target.write_text(target.read_text(encoding="utf-8") + "\n// tampered\n", encoding="utf-8")

        assert self.mod.sync(root, check=True) == 1

    def test_check_detects_orphan_generated_file(self) -> None:
        root = self._make_temp_repo()
        assert self.mod.sync(root, check=False) == 0

        orphan = root / "orphan-service" / "internal" / "fsperm" / "fsperm_gen.go"
        orphan.parent.mkdir(parents=True, exist_ok=True)
        orphan.write_text(self.mod.GENERATED_HEADER + "package fsperm\n", encoding="utf-8")

        assert self.mod.sync(root, check=True) == 1

    def test_repo_canonical_files_in_sync(self) -> None:
        assert self.mod.sync(REPO_ROOT, check=True) == 0


if __name__ == "__main__":
    unittest.main()
