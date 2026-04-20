"""Tests for classification_backend default and learning_machine artifact validation."""

import pytest
from pydantic import ValidationError

from recap_subworker.infra.config import Settings


class TestClassificationBackendDefault:
    """classification_backend のデフォルトが joblib であることを保証する。"""

    def test_default_classification_backend_is_joblib(self, monkeypatch):
        monkeypatch.delenv("RECAP_CLASSIFICATION_BACKEND", raising=False)
        monkeypatch.delenv("RECAP_SUBWORKER_CLASSIFICATION_BACKEND", raising=False)
        settings = Settings(_env_file=None)
        assert settings.classification_backend == "joblib"


class TestLearningMachineArtifactValidation:
    """classification_backend='learning_machine' 時の artifact 存在チェック。"""

    def test_learning_machine_backend_rejects_missing_artifacts(self, monkeypatch, tmp_path):
        monkeypatch.setenv("RECAP_CLASSIFICATION_BACKEND", "learning_machine")
        monkeypatch.setenv(
            "RECAP_LEARNING_MACHINE_STUDENT_JA_DIR", str(tmp_path / "nonexistent_ja")
        )
        monkeypatch.setenv(
            "RECAP_LEARNING_MACHINE_STUDENT_EN_DIR", str(tmp_path / "nonexistent_en")
        )

        with pytest.raises(ValidationError) as exc_info:
            Settings(_env_file=None)

        assert "learning_machine artifacts missing" in str(exc_info.value)

    def test_learning_machine_backend_accepts_ja_only(self, monkeypatch, tmp_path):
        ja_dir = tmp_path / "v0_ja"
        ja_dir.mkdir()

        monkeypatch.setenv("RECAP_CLASSIFICATION_BACKEND", "learning_machine")
        monkeypatch.setenv("RECAP_LEARNING_MACHINE_STUDENT_JA_DIR", str(ja_dir))
        monkeypatch.setenv(
            "RECAP_LEARNING_MACHINE_STUDENT_EN_DIR", str(tmp_path / "nonexistent_en")
        )

        settings = Settings(_env_file=None)
        assert settings.classification_backend == "learning_machine"
        assert settings.learning_machine_student_ja_dir == str(ja_dir)

    def test_learning_machine_backend_accepts_en_only(self, monkeypatch, tmp_path):
        en_dir = tmp_path / "v0_en"
        en_dir.mkdir()

        monkeypatch.setenv("RECAP_CLASSIFICATION_BACKEND", "learning_machine")
        monkeypatch.setenv(
            "RECAP_LEARNING_MACHINE_STUDENT_JA_DIR", str(tmp_path / "nonexistent_ja")
        )
        monkeypatch.setenv("RECAP_LEARNING_MACHINE_STUDENT_EN_DIR", str(en_dir))

        settings = Settings(_env_file=None)
        assert settings.classification_backend == "learning_machine"
        assert settings.learning_machine_student_en_dir == str(en_dir)

    def test_joblib_backend_skips_artifact_check(self, monkeypatch, tmp_path):
        monkeypatch.setenv("RECAP_CLASSIFICATION_BACKEND", "joblib")
        monkeypatch.setenv(
            "RECAP_LEARNING_MACHINE_STUDENT_JA_DIR", str(tmp_path / "nonexistent_ja")
        )
        monkeypatch.setenv(
            "RECAP_LEARNING_MACHINE_STUDENT_EN_DIR", str(tmp_path / "nonexistent_en")
        )

        settings = Settings(_env_file=None)
        assert settings.classification_backend == "joblib"
