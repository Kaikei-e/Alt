"""
Unit tests for ModelManager class.
Tests model loading, shared instance management, and error handling.
"""

import threading
from unittest.mock import MagicMock, mock_open, patch

import pytest

from tag_extractor.model_manager import ModelConfig, ModelManager, get_model_manager, reset_model_manager


class TestModelManager:
    """Test cases for ModelManager functionality."""

    def setup_method(self):
        """Reset shared ModelManager instance for each test."""
        reset_model_manager()

    def test_get_model_manager_returns_shared_instance(self):
        """Test that get_model_manager returns the same instance."""
        manager1 = get_model_manager()
        manager2 = get_model_manager()
        assert manager1 is manager2
        assert isinstance(manager1, ModelManager)

    def test_new_instances_are_independent(self):
        """Test that direct construction creates independent instances."""
        manager1 = ModelManager()
        manager2 = ModelManager()
        assert manager1 is not manager2

    def test_get_model_manager_thread_safety(self):
        """Test that get_model_manager is thread-safe."""
        instances = []

        def create_manager():
            instances.append(get_model_manager())

        threads = [threading.Thread(target=create_manager) for _ in range(10)]
        for thread in threads:
            thread.start()
        for thread in threads:
            thread.join()

        # All instances should be the same shared object
        assert all(instance is instances[0] for instance in instances)

    def test_models_loaded_initially_false(self):
        """Test that models are not loaded initially."""
        manager = ModelManager()
        assert not manager.is_loaded()

    @patch("tag_extractor.model_manager.SentenceTransformer")
    @patch("tag_extractor.model_manager.KeyBERT")
    @patch("tag_extractor.model_manager.Tagger")
    def test_get_models_loads_successfully(self, mock_tagger, mock_keybert, mock_sentence_transformer):
        """Test successful model loading."""
        # Arrange
        mock_embedder = MagicMock()
        mock_kb = MagicMock()
        mock_ja_tagger = MagicMock()

        mock_sentence_transformer.return_value = mock_embedder
        mock_keybert.return_value = mock_kb
        mock_tagger.return_value = mock_ja_tagger

        manager = ModelManager()
        config = ModelConfig(model_name="test-model", device="cpu")

        # Act
        embedder, keybert, ja_tagger = manager.get_models(config)

        # Assert
        assert embedder is mock_embedder
        assert keybert is mock_kb
        assert ja_tagger is mock_ja_tagger
        assert manager.is_loaded()

    @patch("tag_extractor.model_manager.OnnxEmbeddingModel")
    @patch("tag_extractor.model_manager.KeyBERT")
    @patch("tag_extractor.model_manager.Tagger")
    def test_runtime_metadata_reports_onnx_backend(self, mock_tagger, mock_keybert, mock_onnx):
        """Ensure runtime metadata exposes ONNX backend details."""
        mock_embedder = MagicMock()
        mock_embedder.describe.return_value = {
            "backend": "onnx",
            "providers": ["CPUExecutionProvider"],
            "model_path": "/models/onnx/model.onnx",
        }
        mock_kb = MagicMock()
        mock_ja_tagger = MagicMock()

        mock_onnx.return_value = mock_embedder
        mock_keybert.return_value = mock_kb
        mock_tagger.return_value = mock_ja_tagger

        manager = ModelManager()
        config = ModelConfig(
            model_name="test-model",
            device="cpu",
            use_onnx=True,
            onnx_model_path="/models/onnx/model.onnx",
        )

        manager.get_models(config)
        metadata = manager.get_runtime_metadata()

        assert metadata["embedder_backend"] == "onnx"
        assert metadata["embedder_metadata"]["model_path"] == "/models/onnx/model.onnx"
        assert metadata["embedder_metadata"]["providers"] == ["CPUExecutionProvider"]

    @patch("tag_extractor.model_manager.SentenceTransformer")
    @patch("tag_extractor.model_manager.KeyBERT")
    @patch("tag_extractor.model_manager.Tagger")
    def test_runtime_metadata_reports_sentence_transformer_backend(
        self, mock_tagger, mock_keybert, mock_sentence_transformer
    ):
        """Ensure runtime metadata exposes SentenceTransformer backend details."""
        mock_embedder = MagicMock()
        mock_embedder.get_sentence_embedding_dimension.return_value = 384
        mock_kb = MagicMock()
        mock_ja_tagger = MagicMock()

        mock_sentence_transformer.return_value = mock_embedder
        mock_keybert.return_value = mock_kb
        mock_tagger.return_value = mock_ja_tagger

        manager = ModelManager()
        config = ModelConfig(model_name="test-model", device="cpu", use_onnx=False)

        manager.get_models(config)
        metadata = manager.get_runtime_metadata()

        assert metadata["embedder_backend"] == "sentence_transformer"
        assert metadata["embedder_metadata"]["model_name"] == "test-model"
        assert metadata["embedder_metadata"]["device"] == "cpu"
        assert metadata["embedder_metadata"]["embedding_dimension"] == 384

    def test_get_models_handles_none_models(self):
        """Missing ML libraries must surface as a diagnosable ModelLoadError,
        not a generic 'NoneType' object is not callable' TypeError."""
        from tag_generator.domain.errors import ModelLoadError

        manager = ModelManager()
        config = ModelConfig()

        # Simulate the actual bug: ML libraries are None (not available)
        with (
            patch("tag_extractor.model_manager.SentenceTransformer", None),
            patch("tag_extractor.model_manager.KeyBERT", None),
            patch("tag_extractor.model_manager.Tagger", None),
        ):
            with pytest.raises(ModelLoadError, match="Missing ML dependency"):
                manager.get_models(config)

    @patch("tag_extractor.model_manager.SentenceTransformer")
    @patch("tag_extractor.model_manager.KeyBERT")
    @patch("tag_extractor.model_manager.Tagger")
    def test_get_models_handles_loading_failure(self, mock_tagger, mock_keybert, mock_sentence_transformer):
        """Test model loading failure handling."""
        # Arrange - make model loading fail
        mock_sentence_transformer.side_effect = Exception("Model loading failed")

        manager = ModelManager()
        config = ModelConfig()

        # Act & Assert
        with pytest.raises(Exception, match="Model loading failed"):
            manager.get_models(config)

        # Models should not be marked as loaded
        assert not manager.is_loaded()

    def test_get_models_caches_loaded_models(self):
        """Test that models are cached and not reloaded unnecessarily."""
        with patch.object(ModelManager, "_load_models") as mock_load:
            manager = ModelManager()
            config = ModelConfig()

            # Set up mock models
            manager._embedder = MagicMock()
            manager._keybert = MagicMock()
            manager._ja_tagger = MagicMock()
            manager._config = config

            # Call get_models twice
            manager.get_models(config)
            manager.get_models(config)

            # _load_models should not be called since models are already loaded
            mock_load.assert_not_called()

    def test_get_models_reloads_on_config_change(self):
        """Test that models are reloaded when configuration changes."""
        with patch.object(ModelManager, "_load_models") as mock_load:
            manager = ModelManager()
            config1 = ModelConfig(model_name="model1")
            config2 = ModelConfig(model_name="model2")

            # Set up initial models
            manager._embedder = MagicMock()
            manager._keybert = MagicMock()
            manager._ja_tagger = MagicMock()
            manager._config = config1

            # Call with different config
            manager.get_models(config2)

            # _load_models should be called due to config change
            mock_load.assert_called_once_with(config2)

    @patch("pathlib.Path.open", new_callable=lambda: mock_open(read_data="TestWord\n"))
    @patch("nltk.corpus.stopwords.words")
    def test_get_stopwords_loads_successfully(self, mock_nltk_stopwords, mock_path_open):
        """Test successful stopwords loading combines file content with NLTK.

        Regression note: this used to `@patch("builtins.open")`, but
        `_load_stopwords` reads through `Path.open(...)`, which does not
        dispatch through the patched `builtins.open` name -- the mock never
        engaged and the assertions only happened to pass because they never
        checked the file-derived content, just that *some* NLTK words made it
        into en_stopwords. Patching `pathlib.Path.open` (the real call site)
        lets us assert the actual merged result.
        """
        mock_nltk_stopwords.return_value = ["the", "a", "an"]

        manager = ModelManager()

        ja_stopwords, en_stopwords = manager.get_stopwords()

        # ja stopwords come solely from the (mocked) file, case-preserved.
        assert ja_stopwords == {"TestWord"}
        # en stopwords merge the (mocked) file content, lower-cased, with NLTK words.
        assert en_stopwords == {"testword", "the", "a", "an"}

    @patch("pathlib.Path.open", side_effect=FileNotFoundError())
    @patch("nltk.corpus.stopwords.words", side_effect=Exception("NLTK not available"))
    def test_get_stopwords_handles_file_errors(self, mock_nltk_stopwords, mock_path_open):
        """Both stopword files missing and NLTK unavailable must fail closed to
        empty sets rather than raising or crashing.

        Regression note: `_load_stopwords` reads via `Path.open(...)`, not the
        `builtins.open` builtin -- `pathlib.Path.open` doesn't dispatch through
        the patched `builtins.open` name, so patching `builtins.open` here
        never actually engages and the real on-disk stopwords files (which
        exist in this repo) were read instead. That silently turned this into
        a happy-path test that didn't exercise the error handling it claims
        to cover -- patching `pathlib.Path.open` (the real call site) instead.
        """
        manager = ModelManager()

        ja_stopwords, en_stopwords = manager.get_stopwords()

        # Every source failed, so both sets must actually be empty -- not
        # just "some set object", which would pass even if a stray default
        # word leaked in from a partially-failed load path.
        assert ja_stopwords == set()
        assert en_stopwords == set()

    def test_clear_models_resets_state(self):
        """Test that clear_models resets all model state."""
        manager = ModelManager()

        # Set up some fake state
        manager._embedder = MagicMock()
        manager._keybert = MagicMock()
        manager._ja_tagger = MagicMock()
        manager._ja_stopwords = {"test"}
        manager._en_stopwords = {"test"}
        manager._config = ModelConfig()

        # Clear models
        manager.clear_models()

        # Assert everything is reset
        assert manager._embedder is None
        assert manager._keybert is None
        assert manager._ja_tagger is None
        assert manager._ja_stopwords is None
        assert manager._en_stopwords is None
        assert manager._config is None
        assert not manager.is_loaded()


class TestModelConfig:
    """Test cases for ModelConfig dataclass."""

    def test_default_config(self):
        """Test ModelConfig default values."""
        config = ModelConfig()
        assert config.model_name == "paraphrase-multilingual-MiniLM-L12-v2"
        assert config.device == "cpu"

    def test_custom_config(self):
        """Test ModelConfig with custom values."""
        config = ModelConfig(model_name="custom-model", device="cuda")
        assert config.model_name == "custom-model"
        assert config.device == "cuda"

    def test_config_equality(self):
        """Test ModelConfig equality comparison."""
        config1 = ModelConfig(model_name="test", device="cpu")
        config2 = ModelConfig(model_name="test", device="cpu")
        config3 = ModelConfig(model_name="different", device="cpu")

        assert config1 == config2
        assert config1 != config3
