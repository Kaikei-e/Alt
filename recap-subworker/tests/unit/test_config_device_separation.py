"""Tests for classification_device separation configuration."""

import pytest

from recap_subworker.infra.config import Settings


class TestDeviceSeparation:
    """classification_device 分離設定のテスト"""

    def test_classification_device_defaults_to_device(self):
        """classification_device 未設定時は device を継承"""
        settings = Settings(device="cuda")
        assert settings.classification_device == "cuda"

    def test_classification_device_independent(self):
        """classification_device は独立して設定可能"""
        settings = Settings(device="cuda", classification_device="cpu")
        assert settings.device == "cuda"
        assert settings.classification_device == "cpu"

    def test_backward_compatibility_cpu(self):
        """後方互換: 単一 device=cpu 設定で両方が同じ値"""
        settings = Settings(device="cpu")
        assert settings.device == "cpu"
        assert settings.classification_device == "cpu"

    def test_backward_compatibility_cuda(self):
        """後方互換: 単一 device=cuda 設定で両方が同じ値"""
        settings = Settings(device="cuda")
        assert settings.device == "cuda"
        assert settings.classification_device == "cuda"

    def test_env_var_loading(self, monkeypatch):
        """環境変数から読み込み"""
        monkeypatch.setenv("RECAP_SUBWORKER_DEVICE", "cuda")
        monkeypatch.setenv("RECAP_SUBWORKER_CLASSIFICATION_DEVICE", "cpu")
        settings = Settings()
        assert settings.device == "cuda"
        assert settings.classification_device == "cpu"

    def test_env_var_classification_device_only(self, monkeypatch):
        """classification_device のみ環境変数で設定"""
        monkeypatch.setenv("RECAP_SUBWORKER_DEVICE", "cuda")
        # RECAP_SUBWORKER_CLASSIFICATION_DEVICE は未設定
        settings = Settings()
        assert settings.device == "cuda"
        assert settings.classification_device == "cuda"

    def test_alternative_env_var_prefix(self, monkeypatch):
        """RECAP_ プレフィックスの環境変数でも動作"""
        monkeypatch.setenv("RECAP_SUBWORKER_DEVICE", "cuda")
        monkeypatch.setenv("RECAP_CLASSIFICATION_DEVICE", "cpu")
        settings = Settings()
        assert settings.device == "cuda"
        assert settings.classification_device == "cpu"

    def test_serialization_to_json(self):
        """model_dump でシリアライズ可能"""
        settings = Settings(device="cuda", classification_device="cpu")
        dumped = settings.model_dump(mode="json")
        assert dumped["device"] == "cuda"
        assert dumped["classification_device"] == "cpu"

    def test_deserialization_from_dict(self):
        """dict からデシリアライズ可能"""
        settings = Settings(device="cuda", classification_device="cpu")
        dumped = settings.model_dump(mode="json")
        restored = Settings(**dumped)
        assert restored.device == "cuda"
        assert restored.classification_device == "cpu"
