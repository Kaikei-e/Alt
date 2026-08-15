"""Defensive tests: Livedoor tarball extract must not escape dest_dir (CWE-22)."""

from __future__ import annotations

import io
import tarfile
from pathlib import Path

import pytest

from recap_subworker.infra.classifier.collect_external_data import extract_tar_under_dir


def _write_tar_gz(path: Path, members: list[tuple[str, bytes]]) -> None:
    """Build a gzip tar with the given member names (including traversal names)."""
    with tarfile.open(path, "w:gz") as tar:
        for name, data in members:
            info = tarfile.TarInfo(name=name)
            info.size = len(data)
            tar.addfile(info, io.BytesIO(data))


class TestExtractTarUnderDir:
    def test_relative_dotdot_member_does_not_write_outside_dest(self, tmp_path: Path) -> None:
        dest = tmp_path / "data"
        dest.mkdir()
        tar_path = tmp_path / "ldcc.tar.gz"
        _write_tar_gz(tar_path, [("../evil.txt", b"pwned")])

        with pytest.raises(ValueError, match="escapes"):
            extract_tar_under_dir(tar_path, dest)

        assert not (tmp_path / "evil.txt").exists()

    def test_absolute_member_does_not_write_outside_dest(self, tmp_path: Path) -> None:
        dest = tmp_path / "data"
        dest.mkdir()
        evil_abs = tmp_path / "evil"
        tar_path = tmp_path / "ldcc.tar.gz"
        _write_tar_gz(tar_path, [(str(evil_abs), b"pwned")])

        with pytest.raises(ValueError, match="escapes"):
            extract_tar_under_dir(tar_path, dest)

        assert not evil_abs.exists()

    def test_rejects_escaping_member_before_extracting_others(self, tmp_path: Path) -> None:
        dest = tmp_path / "data"
        dest.mkdir()
        tar_path = tmp_path / "ldcc.tar.gz"
        _write_tar_gz(
            tar_path,
            [
                ("ok.txt", b"hello"),
                ("../evil.txt", b"pwned"),
            ],
        )

        with pytest.raises(ValueError, match="escapes"):
            extract_tar_under_dir(tar_path, dest)

        assert not (dest / "ok.txt").exists()
        assert not (tmp_path / "evil.txt").exists()

    def test_safe_members_extract_under_dest(self, tmp_path: Path) -> None:
        dest = tmp_path / "data"
        dest.mkdir()
        tar_path = tmp_path / "ldcc.tar.gz"
        _write_tar_gz(tar_path, [("text/ok.txt", b"hello")])

        extract_tar_under_dir(tar_path, dest)

        assert (dest / "text" / "ok.txt").read_bytes() == b"hello"


class TestProcessAgNewsFailsLoudOnBrokenDatasetsInstall:
    def test_missing_datasets_package_raises_instead_of_returning_empty(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """A broken `datasets` install must not degrade to a Japanese-only corpus."""
        import sys

        from recap_subworker.infra.classifier import collect_external_data

        monkeypatch.setitem(sys.modules, "datasets", None)

        with pytest.raises(ImportError):
            collect_external_data.process_ag_news()
