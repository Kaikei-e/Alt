"""Tests for path validation utilities."""

import pytest

from recap_subworker.infra.path_validation import validate_path


class TestValidatePath:
    """Tests for validate_path() - CodeQL py/path-injection sanitizer."""

    def test_absolute_path_within_allowed_dir(self, tmp_path):
        base = tmp_path / "data"
        base.mkdir()
        target = base / "file.json"
        target.touch()

        result = validate_path(str(target), base_dirs=[base])
        assert result == target.resolve()

    def test_relative_path_resolved_against_first_base(self, tmp_path):
        base = tmp_path / "data"
        base.mkdir()
        target = base / "file.json"
        target.touch()

        result = validate_path("file.json", base_dirs=[base])
        assert result == target.resolve()

    def test_subdirectory_path(self, tmp_path):
        base = tmp_path / "data"
        sub = base / "sub"
        sub.mkdir(parents=True)
        target = sub / "file.json"
        target.touch()

        result = validate_path(str(target), base_dirs=[base])
        assert result == target.resolve()

    def test_multiple_base_dirs_second_matches(self, tmp_path):
        base1 = tmp_path / "data"
        base1.mkdir()
        base2 = tmp_path / "resources"
        base2.mkdir()
        target = base2 / "file.json"
        target.touch()

        result = validate_path(str(target), base_dirs=[base1, base2])
        assert result == target.resolve()

    def test_path_equals_base_dir(self, tmp_path):
        base = tmp_path / "data"
        base.mkdir()

        result = validate_path(str(base), base_dirs=[base])
        assert result == base.resolve()

    def test_rejects_path_traversal(self, tmp_path):
        base = tmp_path / "data"
        base.mkdir()

        with pytest.raises(ValueError, match="not within allowed directories"):
            validate_path("../../../etc/passwd", base_dirs=[base])

    def test_rejects_absolute_path_outside(self, tmp_path):
        base = tmp_path / "data"
        base.mkdir()

        with pytest.raises(ValueError, match="not within allowed directories"):
            validate_path("/etc/passwd", base_dirs=[base])

    def test_rejects_prefix_attack(self, tmp_path):
        """Ensure /app/data-evil doesn't match /app/data."""
        base = tmp_path / "data"
        base.mkdir()
        evil = tmp_path / "data-evil"
        evil.mkdir()
        target = evil / "file.json"
        target.touch()

        with pytest.raises(ValueError, match="not within allowed directories"):
            validate_path(str(target), base_dirs=[base])

    def test_rejects_symlink_escaping(self, tmp_path):
        """Symlinks pointing outside allowed dirs should be rejected."""
        base = tmp_path / "data"
        base.mkdir()
        outside = tmp_path / "outside"
        outside.mkdir()
        secret = outside / "secret.txt"
        secret.touch()

        link = base / "link.txt"
        link.symlink_to(secret)

        with pytest.raises(ValueError, match="not within allowed directories"):
            validate_path(str(link), base_dirs=[base])

    def test_empty_base_dirs_raises(self):
        with pytest.raises(ValueError, match="No base directories configured"):
            validate_path("file.json", base_dirs=[])

    def test_defaults_to_allowed_base_dirs(self):
        """Using default base_dirs rejects paths outside ALLOWED_BASE_DIRS."""
        with pytest.raises(ValueError, match="not within allowed directories"):
            validate_path("/etc/passwd")
