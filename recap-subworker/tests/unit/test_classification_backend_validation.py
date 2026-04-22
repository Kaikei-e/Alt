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


class TestJoblibArtifactValidation:
    """classification_backend='joblib' 時の artefact が空ディレクトリ bind-mount に
    なっていないことを起動時に fail-close で検出する。

    Docker が存在しないホストファイルを file-scoped bind mount しようとすると、
    コンテナ側に空ディレクトリを自動生成する。その結果 joblib.load() が
    IsADirectoryError で落ち、Classification worker pool 初期化が 300s 後に
    タイムアウトする。Settings 構築時点で止める。
    """

    def test_joblib_backend_rejects_directory_at_model_path_ja(self, monkeypatch, tmp_path):
        stub_dir = tmp_path / "genre_classifier_ja.joblib"
        stub_dir.mkdir()

        monkeypatch.setenv("RECAP_CLASSIFICATION_BACKEND", "joblib")
        monkeypatch.setenv("RECAP_SUBWORKER_GENRE_CLASSIFIER_MODEL_PATH_JA", str(stub_dir))

        with pytest.raises(ValidationError) as exc_info:
            Settings(_env_file=None)

        msg = str(exc_info.value)
        assert "joblib artefact" in msg or "is a directory" in msg
        assert str(stub_dir) in msg

    def test_joblib_backend_rejects_directory_at_tfidf_path(self, monkeypatch, tmp_path):
        stub_dir = tmp_path / "tfidf_vectorizer_ja.joblib"
        stub_dir.mkdir()

        monkeypatch.setenv("RECAP_CLASSIFICATION_BACKEND", "joblib")
        monkeypatch.setenv("RECAP_SUBWORKER_TFIDF_VECTORIZER_PATH_JA", str(stub_dir))

        with pytest.raises(ValidationError) as exc_info:
            Settings(_env_file=None)

        assert str(stub_dir) in str(exc_info.value)

    def test_joblib_backend_rejects_directory_at_thresholds_path(self, monkeypatch, tmp_path):
        stub_dir = tmp_path / "genre_thresholds_ja.json"
        stub_dir.mkdir()

        monkeypatch.setenv("RECAP_CLASSIFICATION_BACKEND", "joblib")
        monkeypatch.setenv("RECAP_SUBWORKER_GENRE_THRESHOLDS_PATH_JA", str(stub_dir))

        with pytest.raises(ValidationError) as exc_info:
            Settings(_env_file=None)

        assert str(stub_dir) in str(exc_info.value)

    def test_joblib_backend_enumerates_every_misconfigured_path(self, monkeypatch, tmp_path):
        bad_model = tmp_path / "genre_classifier_ja.joblib"
        bad_tfidf = tmp_path / "tfidf_vectorizer_ja.joblib"
        bad_model.mkdir()
        bad_tfidf.mkdir()

        monkeypatch.setenv("RECAP_CLASSIFICATION_BACKEND", "joblib")
        monkeypatch.setenv("RECAP_SUBWORKER_GENRE_CLASSIFIER_MODEL_PATH_JA", str(bad_model))
        monkeypatch.setenv("RECAP_SUBWORKER_TFIDF_VECTORIZER_PATH_JA", str(bad_tfidf))

        with pytest.raises(ValidationError) as exc_info:
            Settings(_env_file=None)

        msg = str(exc_info.value)
        assert str(bad_model) in msg
        assert str(bad_tfidf) in msg

    def test_joblib_backend_accepts_regular_file(self, monkeypatch, tmp_path):
        real_model = tmp_path / "genre_classifier_ja.joblib"
        real_model.write_bytes(b"fake joblib payload")

        monkeypatch.setenv("RECAP_CLASSIFICATION_BACKEND", "joblib")
        monkeypatch.setenv("RECAP_SUBWORKER_GENRE_CLASSIFIER_MODEL_PATH_JA", str(real_model))

        settings = Settings(_env_file=None)
        assert settings.classification_backend == "joblib"
        assert settings.genre_classifier_model_path_ja == str(real_model)

    def test_joblib_backend_tolerates_missing_path(self, monkeypatch, tmp_path):
        missing = tmp_path / "nonexistent" / "genre_classifier_ja.joblib"

        monkeypatch.setenv("RECAP_CLASSIFICATION_BACKEND", "joblib")
        monkeypatch.setenv("RECAP_SUBWORKER_GENRE_CLASSIFIER_MODEL_PATH_JA", str(missing))

        # Missing paths are not a validation failure — only directory-shaped
        # bind-mount placeholders are. Missing files still fall through to
        # the runtime FileNotFoundError guard in GenreClassifierService.
        settings = Settings(_env_file=None)
        assert settings.genre_classifier_model_path_ja == str(missing)

    def test_learning_machine_backend_skips_joblib_check(self, monkeypatch, tmp_path):
        ja_dir = tmp_path / "v0_ja"
        ja_dir.mkdir()
        stub_dir = tmp_path / "genre_classifier_ja.joblib"
        stub_dir.mkdir()

        monkeypatch.setenv("RECAP_CLASSIFICATION_BACKEND", "learning_machine")
        monkeypatch.setenv("RECAP_LEARNING_MACHINE_STUDENT_JA_DIR", str(ja_dir))
        monkeypatch.setenv("RECAP_SUBWORKER_GENRE_CLASSIFIER_MODEL_PATH_JA", str(stub_dir))

        # learning_machine backend ignores joblib paths entirely; the
        # directory-shaped joblib path must not fail the validator.
        settings = Settings(_env_file=None)
        assert settings.classification_backend == "learning_machine"
