"""
Global pytest configuration and fixtures for the tag-generator service.
Provides mocks for ML models and NLTK dependencies to enable testing without
external downloads.
"""

import os
from unittest.mock import MagicMock, mock_open, patch

import pytest


@pytest.fixture(scope="session", autouse=True)
def mock_ml_models():
    """Mock all ML model classes to avoid requiring actual model downloads in tests."""

    # Create mock classes that behave like the real ones
    mock_sentence_transformer = MagicMock()
    mock_sentence_transformer.encode.return_value = [[0.1, 0.2, 0.3]]  # Mock embeddings

    mock_keybert = MagicMock()
    mock_keybert.extract_keywords.return_value = [
        ("machine learning", 0.8),
        ("ai", 0.6),
    ]

    mock_tagger = MagicMock()
    mock_tagger.parse.return_value = [
        MagicMock(surface="機械", pos="名詞-普通名詞-一般"),
        MagicMock(surface="学習", pos="名詞-普通名詞-サ変可能"),
    ]

    # Mock the model classes themselves
    with (
        patch(
            "tag_extractor.extract.SentenceTransformer",
            return_value=mock_sentence_transformer,
        ),
        patch("tag_extractor.extract.KeyBERT", return_value=mock_keybert),
        patch("tag_extractor.extract.Tagger", return_value=mock_tagger),
        patch(
            "tag_extractor.model_manager.SentenceTransformer",
            return_value=mock_sentence_transformer,
        ),
        patch("tag_extractor.model_manager.KeyBERT", return_value=mock_keybert),
        patch("tag_extractor.model_manager.Tagger", return_value=mock_tagger),
    ):
        yield {
            "sentence_transformer": mock_sentence_transformer,
            "keybert": mock_keybert,
            "tagger": mock_tagger,
        }


@pytest.fixture(scope="session", autouse=True)
def mock_nltk_data():
    """Mock NLTK data to avoid requiring downloads."""

    # Mock stopwords
    mock_stopwords = {
        "english": [
            "the",
            "a",
            "an",
            "is",
            "are",
            "was",
            "were",
            "be",
            "been",
            "being",
            "have",
            "has",
            "had",
            "do",
            "does",
            "did",
            "will",
            "would",
            "should",
            "could",
            "may",
            "might",
            "must",
            "can",
            "to",
            "of",
            "in",
            "for",
            "on",
            "at",
            "by",
            "with",
            "from",
            "as",
            "but",
            "or",
            "and",
            "not",
            "this",
            "that",
            "these",
            "those",
        ]
    }

    # Create a mock stopwords corpus that doesn't try to load data
    mock_stopwords_corpus = MagicMock()
    mock_stopwords_corpus.words.side_effect = lambda lang="english": mock_stopwords.get(lang, [])

    # Mock punkt tokenizer
    def mock_sent_tokenize(text, language="english"):
        return text.split(". ") if ". " in text else [text]

    with (
        patch("nltk.corpus.stopwords", mock_stopwords_corpus),
        patch("nltk.tokenize.sent_tokenize", side_effect=mock_sent_tokenize),
        patch("nltk.download", return_value=True),
    ):
        yield {"stopwords": mock_stopwords, "sent_tokenize": mock_sent_tokenize}


@pytest.fixture(scope="session", autouse=True)
def mock_stopword_files():
    """Mock the stopwords files to avoid file system dependencies."""

    ja_stopwords_content = """の
に
は
を
が
で
と
から
まで
より
について
によって
における
"""

    en_stopwords_content = """the
a
an
is
are
was
were
be
been
being
have
has
had
"""

    def mock_open_file(filepath, *args, **kwargs):
        if "stopwords_ja.txt" in filepath:
            return mock_open(read_data=ja_stopwords_content)(*args, **kwargs)
        elif "stopwords_en.txt" in filepath:
            return mock_open(read_data=en_stopwords_content)(*args, **kwargs)
        else:
            # Fallback to original open for other files
            return open(filepath, *args, **kwargs)

    with patch("builtins.open", side_effect=mock_open_file):
        yield


@pytest.fixture(scope="function")
def mock_database_connection():
    """Mock database connections for testing without DB dependencies."""
    mock_conn = MagicMock()
    mock_cursor = MagicMock()
    mock_cursor.fetchall.return_value = []
    mock_cursor.fetchone.return_value = None
    mock_cursor.rowcount = 0
    mock_conn.cursor.return_value.__enter__.return_value = mock_cursor

    with patch("psycopg2.connect", return_value=mock_conn):
        yield mock_conn


@pytest.fixture(scope="function")
def mock_model_manager():
    """Provide a mock model manager for testing."""
    mock_manager = MagicMock()

    # Mock stopwords
    ja_stopwords = {"の", "に", "は", "を", "が", "で", "と"}
    en_stopwords = {"the", "a", "an", "is", "are", "was", "were", "be"}
    mock_manager.get_stopwords.return_value = (ja_stopwords, en_stopwords)

    # Mock models
    mock_embedder = MagicMock()
    mock_embedder.encode.return_value = [[0.1, 0.2, 0.3]]

    mock_keybert = MagicMock()
    mock_keybert.extract_keywords.return_value = [("test", 0.8)]

    mock_tagger = MagicMock()
    mock_tagger.parse.return_value = []

    mock_manager.get_models.return_value = (mock_embedder, mock_keybert, mock_tagger)
    mock_manager.is_loaded.return_value = True

    return mock_manager


@pytest.fixture(scope="function")
def mock_language_detection():
    """Mock language detection to avoid external dependencies."""
    with patch("langdetect.detect") as mock_detect:
        mock_detect.return_value = "en"  # Default to English
        yield mock_detect


@pytest.fixture(scope="function")
def sample_article_data():
    """Provide sample article data for testing."""
    return {
        "title": "Machine Learning Tutorial",
        "content": "This is a comprehensive tutorial about machine learning algorithms and neural networks.",
        "url": "https://example.com/ml-tutorial",
    }


@pytest.fixture(scope="function")
def japanese_article_data():
    """Provide sample Japanese article data for testing."""
    return {
        "title": "機械学習の基礎",
        "content": "機械学習は人工知能の一分野で、コンピュータがデータから学習する技術です。",
        "url": "https://example.com/ml-basics-ja",
    }


# Configure logging for tests
@pytest.fixture(scope="session", autouse=True)
def configure_test_logging():
    """Configure logging for tests to avoid noise in test output."""
    import logging

    import structlog

    # Reduce log level for tests
    logging.getLogger().setLevel(logging.WARNING)

    # Configure structlog for testing
    structlog.configure(
        processors=[
            structlog.stdlib.filter_by_level,
            structlog.stdlib.add_logger_name,
            structlog.stdlib.add_log_level,
            structlog.testing.LogCapture(),
        ],
        wrapper_class=structlog.stdlib.BoundLogger,
        logger_factory=structlog.testing.CapturingLoggerFactory(),
        cache_logger_on_first_use=True,
    )

    yield


# Environment setup
@pytest.fixture(scope="session", autouse=True)
def test_environment():
    """Set up test environment variables."""
    os.environ["SERVICE_NAME"] = "tag-generator-test"
    os.environ["LOG_LEVEL"] = "WARNING"
    yield
    # Cleanup
    if "SERVICE_NAME" in os.environ:
        del os.environ["SERVICE_NAME"]
    if "LOG_LEVEL" in os.environ:
        del os.environ["LOG_LEVEL"]
