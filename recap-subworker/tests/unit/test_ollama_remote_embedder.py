"""Tests for ollama-remote embedding backend."""

import numpy as np
import pytest
from unittest.mock import Mock, patch

from recap_subworker.infra.config import Settings
from recap_subworker.services.embedder import Embedder, EmbedderConfig


class TestOllamaRemoteConfig:
    """ollama-remote 設定のテスト"""

    def test_ollama_embed_url_default_none(self):
        """ollama_embed_url のデフォルトは None"""
        settings = Settings()
        assert settings.ollama_embed_url is None

    def test_ollama_embed_model_default(self):
        """ollama_embed_model のデフォルトは mxbai-embed-large"""
        settings = Settings()
        assert settings.ollama_embed_model == "mxbai-embed-large"

    def test_ollama_embed_timeout_default(self):
        """ollama_embed_timeout のデフォルトは 30.0"""
        settings = Settings()
        assert settings.ollama_embed_timeout == 30.0

    def test_env_var_loading(self, monkeypatch):
        """環境変数から読み込み"""
        monkeypatch.setenv("OLLAMA_EMBED_URL", "http://remote-host:11436")
        monkeypatch.setenv("OLLAMA_EMBED_MODEL", "nomic-embed-text")
        monkeypatch.setenv("OLLAMA_EMBED_TIMEOUT", "60.0")
        settings = Settings()
        assert settings.ollama_embed_url == "http://remote-host:11436"
        assert settings.ollama_embed_model == "nomic-embed-text"
        assert settings.ollama_embed_timeout == 60.0

    def test_prefixed_env_var_loading(self, monkeypatch):
        """RECAP_SUBWORKER_ プレフィックス付き環境変数から読み込み"""
        monkeypatch.setenv("RECAP_SUBWORKER_OLLAMA_EMBED_URL", "http://another-host:11436")
        settings = Settings()
        assert settings.ollama_embed_url == "http://another-host:11436"

    def test_model_backend_ollama_remote(self):
        """model_backend に ollama-remote を設定可能"""
        settings = Settings(model_backend="ollama-remote")
        assert settings.model_backend == "ollama-remote"


class TestOllamaRemoteEmbedder:
    """OllamaRemoteAdapter のテスト"""

    def test_ollama_remote_requires_url(self):
        """ollama_embed_url がない場合はエラー"""
        config = EmbedderConfig(
            model_id="test",
            distill_model_id="test",
            backend="ollama-remote",
            device="cpu",
            batch_size=8,
            cache_size=100,
            ollama_embed_url=None,
        )
        embedder = Embedder(config)

        with pytest.raises(ValueError, match="ollama_embed_url is required"):
            embedder.encode(["test sentence"])

    def test_ollama_remote_encode(self):
        """OllamaRemoteAdapter で正しくエンコードできる"""
        with patch("httpx.Client") as mock_client_class:
            mock_client = Mock()
            mock_client_class.return_value = mock_client

            # Each short text is embedded with one API call
            mock_response1 = Mock()
            mock_response1.json.return_value = {"embeddings": [[0.1, 0.2, 0.3]]}
            mock_response1.raise_for_status = Mock()

            mock_response2 = Mock()
            mock_response2.json.return_value = {"embeddings": [[0.4, 0.5, 0.6]]}
            mock_response2.raise_for_status = Mock()

            mock_client.post.side_effect = [mock_response1, mock_response2]

            config = EmbedderConfig(
                model_id="test",
                distill_model_id="test",
                backend="ollama-remote",
                device="cpu",
                batch_size=8,
                cache_size=100,
                ollama_embed_url="http://test-host:11436",
                ollama_embed_model="test-model",
                ollama_embed_timeout=30.0,
            )
            embedder = Embedder(config)

            result = embedder.encode(["sentence 1", "sentence 2"])

            assert result.shape == (2, 3)
            # Each text gets one API call (since they're short)
            assert mock_client.post.call_count == 2
            call_args = mock_client.post.call_args
            assert "http://test-host:11436/api/embed" in call_args[0]

    def test_ollama_remote_batching(self):
        """バッチ処理が正しく動作する（短いテキストは各1回のAPI呼び出し）"""
        with patch("httpx.Client") as mock_client_class:
            mock_client = Mock()
            mock_client_class.return_value = mock_client

            # Each short text gets one API call
            mock_response1 = Mock()
            mock_response1.json.return_value = {"embeddings": [[0.1, 0.2]]}
            mock_response1.raise_for_status = Mock()

            mock_response2 = Mock()
            mock_response2.json.return_value = {"embeddings": [[0.3, 0.4]]}
            mock_response2.raise_for_status = Mock()

            mock_response3 = Mock()
            mock_response3.json.return_value = {"embeddings": [[0.5, 0.6]]}
            mock_response3.raise_for_status = Mock()

            mock_client.post.side_effect = [mock_response1, mock_response2, mock_response3]

            config = EmbedderConfig(
                model_id="test",
                distill_model_id="test",
                backend="ollama-remote",
                device="cpu",
                batch_size=2,  # batch_size affects outer loop batching, not API calls
                cache_size=100,
                ollama_embed_url="http://test-host:11436",
            )
            embedder = Embedder(config)

            result = embedder.encode(["s1", "s2", "s3"])

            assert result.shape == (3, 2)
            # Each short text gets one API call
            assert mock_client.post.call_count == 3

    def test_ollama_remote_normalization(self):
        """正規化が正しく動作する"""
        with patch("httpx.Client") as mock_client_class:
            mock_client = Mock()
            mock_client_class.return_value = mock_client

            mock_response = Mock()
            mock_response.json.return_value = {
                "embeddings": [[3.0, 4.0]]  # norm = 5
            }
            mock_response.raise_for_status = Mock()
            mock_client.post.return_value = mock_response

            config = EmbedderConfig(
                model_id="test",
                distill_model_id="test",
                backend="ollama-remote",
                device="cpu",
                batch_size=8,
                cache_size=100,
                ollama_embed_url="http://test-host:11436",
            )
            embedder = Embedder(config)

            result = embedder.encode(["test"])

            # After normalization: [3/5, 4/5] = [0.6, 0.8]
            assert np.allclose(result[0], [0.6, 0.8])

    def test_ollama_remote_empty_input(self):
        """空の入力で空の配列を返す"""
        config = EmbedderConfig(
            model_id="test",
            distill_model_id="test",
            backend="ollama-remote",
            device="cpu",
            batch_size=8,
            cache_size=100,
            ollama_embed_url="http://test-host:11436",
        )
        embedder = Embedder(config)

        result = embedder.encode([])

        assert result.shape[0] == 0

    def test_ollama_remote_close(self):
        """close() でリソースが解放される"""
        with patch("httpx.Client") as mock_client_class:
            mock_client = Mock()
            mock_client_class.return_value = mock_client

            mock_response = Mock()
            mock_response.json.return_value = {"embeddings": [[0.1, 0.2]]}
            mock_response.raise_for_status = Mock()
            mock_client.post.return_value = mock_response

            config = EmbedderConfig(
                model_id="test",
                distill_model_id="test",
                backend="ollama-remote",
                device="cpu",
                batch_size=8,
                cache_size=100,
                ollama_embed_url="http://test-host:11436",
            )
            embedder = Embedder(config)

            # Trigger model loading
            embedder.encode(["test"])

            # Close embedder
            embedder.close()

            mock_client.close.assert_called_once()
            assert embedder._model is None

    def test_ollama_remote_long_text_chunking(self):
        """長いテキストはチャンク分割して平均化される"""
        with patch("httpx.Client") as mock_client_class:
            mock_client = Mock()
            mock_client_class.return_value = mock_client

            # Long text will be split into 2 chunks, each gets embedded separately
            mock_response1 = Mock()
            mock_response1.json.return_value = {"embeddings": [[0.2, 0.4]]}
            mock_response1.raise_for_status = Mock()

            mock_response2 = Mock()
            mock_response2.json.return_value = {"embeddings": [[0.4, 0.6]]}
            mock_response2.raise_for_status = Mock()

            mock_client.post.side_effect = [mock_response1, mock_response2]

            config = EmbedderConfig(
                model_id="test",
                distill_model_id="test",
                backend="ollama-remote",
                device="cpu",
                batch_size=8,
                cache_size=100,
                ollama_embed_url="http://test-host:11436",
            )
            embedder = Embedder(config)

            # Create a text longer than MAX_CHUNK_CHARS (400)
            long_text = "x" * 800  # Will be split into 2 chunks

            result = embedder.encode([long_text])

            # Single embedding, averaged from 2 chunks
            assert result.shape == (1, 2)
            # After averaging [0.2, 0.4] and [0.4, 0.6] = [0.3, 0.5], normalized
            # Norm of [0.3, 0.5] = sqrt(0.09 + 0.25) = sqrt(0.34) ≈ 0.583
            # Normalized: [0.3/0.583, 0.5/0.583] ≈ [0.514, 0.857]
            # Each chunk gets a separate API call
            assert mock_client.post.call_count == 2
