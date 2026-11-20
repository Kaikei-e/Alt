"""Topic coherence evaluation using C_V and NPMI metrics.

This module provides topic coherence evaluation for cluster quality assessment.
C_V coherence has been shown to correlate best with human judgments.

References:
- C_V Coherence: https://towardsdatascience.com/cᵥ-topic-coherence-explained-fc70e2a85227/
- Gensim CoherenceModel: https://radimrehurek.com/gensim/models/coherencemodel.html
"""

from __future__ import annotations

import re
from collections import Counter
from dataclasses import dataclass, field
from enum import Enum
from typing import Literal

import structlog

try:
    from gensim.corpora import Dictionary
    from gensim.models.coherencemodel import CoherenceModel

    GENSIM_AVAILABLE = True
except ImportError:
    CoherenceModel = None
    Dictionary = None
    GENSIM_AVAILABLE = False

try:
    from janome.tokenizer import Tokenizer as JanomeTokenizer
except ImportError:
    JanomeTokenizer = None

logger = structlog.get_logger(__name__)
_MIN_ENGLISH_WORD_EXCLUDED_LEN = 2
_MIN_TOPIC_WORDS = 2

# Simple English stopwords list
ENGLISH_STOPWORDS = {
    "a",
    "an",
    "and",
    "are",
    "as",
    "at",
    "be",
    "by",
    "for",
    "from",
    "has",
    "he",
    "in",
    "is",
    "it",
    "its",
    "of",
    "on",
    "that",
    "the",
    "to",
    "was",
    "were",
    "will",
    "with",
    "this",
    "they",
    "their",
    "have",
    "had",
    "been",
    "would",
    "could",
    "should",
    "can",
    "may",
    "might",
    "must",
    "shall",
    "do",
    "does",
    "did",
    "or",
    "but",
    "not",
    "no",
    "so",
    "if",
    "than",
    "then",
    "too",
    "very",
    "just",
    "also",
    "more",
    "most",
    "some",
    "any",
    "all",
    "each",
    "every",
    "other",
    "such",
    "only",
    "own",
    "same",
    "both",
    "few",
    "many",
    "much",
}


class CoherenceType(Enum):
    """Topic coherence measure types.

    - C_V: Best correlation with human judgments (recommended)
    - C_NPMI: Normalized Pointwise Mutual Information
    - C_UCI: UCI coherence measure
    - U_MASS: UMass coherence (intrinsic, doesn't need reference corpus)
    """

    C_V = "c_v"
    C_NPMI = "c_npmi"
    C_UCI = "c_uci"
    U_MASS = "u_mass"


@dataclass
class CoherenceResult:
    """Result container for topic coherence evaluation.

    Attributes:
        overall_coherence: Weighted average coherence across clusters.
        coherence_type: Type of coherence measure used.
        per_cluster_coherence: Coherence score for each cluster.
        num_clusters: Total number of clusters evaluated.
        num_documents: Total number of documents in corpus.
    """

    overall_coherence: float
    coherence_type: str
    per_cluster_coherence: dict[int, float]
    num_clusters: int | None = field(default=None)
    num_documents: int | None = field(default=None)

    def to_dict(self) -> dict:
        """Convert to dictionary representation."""
        result = {
            "overall_coherence": self.overall_coherence,
            "coherence_type": self.coherence_type,
            "per_cluster_coherence": self.per_cluster_coherence,
        }
        if self.num_clusters is not None:
            result["num_clusters"] = self.num_clusters
        if self.num_documents is not None:
            result["num_documents"] = self.num_documents
        return result


class TopicCoherenceEvaluator:
    """Topic coherence evaluator using C_V/NPMI metrics.

    Evaluates the quality of topic clusters by measuring how coherent
    the top words in each cluster are with respect to a reference corpus.

    Example:
        >>> evaluator = TopicCoherenceEvaluator()
        >>> result = evaluator.compute_coherence(
        ...     clusters={0: ["AI text 1", "AI text 2"], 1: ["Finance text"]},
        ...     texts=["AI text 1", "AI text 2", "Finance text"],
        ... )
        >>> print(f"Overall coherence: {result.overall_coherence:.3f}")
    """

    def __init__(
        self,
        coherence_type: CoherenceType = CoherenceType.C_V,
        top_n_words: int = 10,
    ) -> None:
        """Initialize the coherence evaluator.

        Args:
            coherence_type: Type of coherence measure to use.
            top_n_words: Number of top words to use per topic.
        """
        if not GENSIM_AVAILABLE:
            logger.warning(
                "gensim not available",
                hint="Install with: pip install gensim",
            )

        self.coherence_type = coherence_type
        self.top_n_words = top_n_words

    def _tokenize(
        self,
        text: str,
        lang: Literal["ja", "en"] = "en",
    ) -> list[str]:
        """Tokenize text into words.

        Args:
            text: Text to tokenize.
            lang: Language code.

        Returns:
            List of tokens.
        """
        if lang == "ja":
            # For Japanese, use simple character-based tokenization
            # or integrate with existing tokenizer
            if JanomeTokenizer is not None:
                tokenizer = JanomeTokenizer()
                return [
                    token.surface.lower()
                    for token in tokenizer.tokenize(text)
                    if len(token.surface) > 1  # Skip single chars
                ]
            # Fallback: simple splitting
            return [w for w in re.findall(r"\w+", text.lower()) if len(w) > 1]
        # English tokenization
        words = re.findall(r"\b[a-zA-Z]+\b", text.lower())
        # Remove stopwords
        return [
            w
            for w in words
            if w not in ENGLISH_STOPWORDS and len(w) > _MIN_ENGLISH_WORD_EXCLUDED_LEN
        ]

    def _extract_topic_words(
        self,
        cluster_texts: list[str],
        top_n: int = 10,
        lang: Literal["ja", "en"] = "en",
    ) -> list[str]:
        """Extract top representative words from cluster texts.

        Args:
            cluster_texts: List of texts in the cluster.
            top_n: Number of top words to extract.
            lang: Language code.

        Returns:
            List of top words.
        """
        all_tokens = []
        for text in cluster_texts:
            tokens = self._tokenize(text, lang=lang)
            all_tokens.extend(tokens)

        # Count word frequencies
        word_counts = Counter(all_tokens)
        return [word for word, _ in word_counts.most_common(top_n)]

    def compute_coherence(
        self,
        clusters: dict[int, list[str]],
        texts: list[str],
        lang: Literal["ja", "en"] = "en",
        min_cluster_size: int = 1,
    ) -> CoherenceResult:
        """Compute topic coherence for clusters.

        Args:
            clusters: Dictionary mapping cluster ID to list of texts.
            texts: Full corpus of texts for reference.
            lang: Language code.
            min_cluster_size: Minimum cluster size to evaluate.

        Returns:
            CoherenceResult with coherence scores.

        Raises:
            ValueError: If clusters or texts is empty.
        """
        if not clusters:
            msg = "clusters cannot be empty"
            raise ValueError(msg)
        if not texts:
            msg = "texts cannot be empty"
            raise ValueError(msg)

        if not GENSIM_AVAILABLE:
            msg = "gensim is not installed. Install with: pip install gensim"
            raise RuntimeError(msg)

        logger.info(
            "Computing topic coherence",
            num_clusters=len(clusters),
            num_texts=len(texts),
            coherence_type=self.coherence_type.value,
        )

        if Dictionary is None or CoherenceModel is None:
            msg = "gensim is not installed. Install with: pip install gensim"
            raise RuntimeError(msg)

        # Tokenize all texts for the reference corpus
        tokenized_texts = [self._tokenize(text, lang=lang) for text in texts]

        # Build dictionary from corpus
        dictionary = Dictionary(tokenized_texts)

        # Filter extreme values
        dictionary.filter_extremes(no_below=2, no_above=0.9)

        per_cluster_coherence: dict[int, float] = {}
        cluster_sizes: dict[int, int] = {}

        for cluster_id, cluster_texts in clusters.items():
            if len(cluster_texts) < min_cluster_size:
                logger.debug(
                    "Skipping small cluster",
                    cluster_id=cluster_id,
                    size=len(cluster_texts),
                    min_size=min_cluster_size,
                )
                continue

            # Extract topic words for this cluster
            topic_words = self._extract_topic_words(
                cluster_texts, top_n=self.top_n_words, lang=lang
            )

            if len(topic_words) < _MIN_TOPIC_WORDS:
                logger.warning(
                    "Insufficient topic words",
                    cluster_id=cluster_id,
                    num_words=len(topic_words),
                )
                per_cluster_coherence[cluster_id] = 0.0
                cluster_sizes[cluster_id] = len(cluster_texts)
                continue

            try:
                # Compute coherence for this cluster's topic
                cm = CoherenceModel(
                    topics=[topic_words],
                    texts=tokenized_texts,
                    dictionary=dictionary,
                    coherence=self.coherence_type.value,
                )
                coherence = cm.get_coherence()
                per_cluster_coherence[cluster_id] = coherence
                cluster_sizes[cluster_id] = len(cluster_texts)

                logger.debug(
                    "Cluster coherence computed",
                    cluster_id=cluster_id,
                    coherence=f"{coherence:.4f}",
                    num_docs=len(cluster_texts),
                )

            except Exception as e:  # noqa: BLE001 - per-cluster coherence fallback; records 0.0 on gensim errors
                logger.warning(
                    "Error computing cluster coherence",
                    cluster_id=cluster_id,
                    error=str(e),
                )
                per_cluster_coherence[cluster_id] = 0.0
                cluster_sizes[cluster_id] = len(cluster_texts)

        # Compute weighted average coherence
        if per_cluster_coherence:
            total_weight = sum(cluster_sizes.values())
            overall_coherence = (
                sum(
                    coherence * cluster_sizes[cid]
                    for cid, coherence in per_cluster_coherence.items()
                )
                / total_weight
                if total_weight > 0
                else 0.0
            )
        else:
            overall_coherence = 0.0

        logger.info(
            "Topic coherence computed",
            overall_coherence=f"{overall_coherence:.4f}",
            num_clusters_evaluated=len(per_cluster_coherence),
        )

        return CoherenceResult(
            overall_coherence=overall_coherence,
            coherence_type=self.coherence_type.value,
            per_cluster_coherence=per_cluster_coherence,
            num_clusters=len(per_cluster_coherence),
            num_documents=len(texts),
        )

    def compute_coherence_from_topics(
        self,
        topics: list[list[str]],
        texts: list[str],
        lang: Literal["ja", "en"] = "en",
    ) -> CoherenceResult:
        """Compute coherence directly from topic word lists.

        This is useful when topic words are already extracted
        (e.g., from a topic model like LDA or BERTopic).

        Args:
            topics: List of topic word lists.
            texts: Full corpus of texts for reference.
            lang: Language code.

        Returns:
            CoherenceResult with coherence scores.
        """
        if not topics:
            msg = "topics cannot be empty"
            raise ValueError(msg)
        if not texts:
            msg = "texts cannot be empty"
            raise ValueError(msg)

        if not GENSIM_AVAILABLE:
            msg = "gensim is not installed. Install with: pip install gensim"
            raise RuntimeError(msg)

        if Dictionary is None or CoherenceModel is None:
            msg = "gensim is not installed. Install with: pip install gensim"
            raise RuntimeError(msg)

        # Tokenize all texts
        tokenized_texts = [self._tokenize(text, lang=lang) for text in texts]

        # Build dictionary
        dictionary = Dictionary(tokenized_texts)
        dictionary.filter_extremes(no_below=2, no_above=0.9)

        per_cluster_coherence: dict[int, float] = {}

        for i, topic_words in enumerate(topics):
            if len(topic_words) < _MIN_TOPIC_WORDS:
                per_cluster_coherence[i] = 0.0
                continue

            try:
                cm = CoherenceModel(
                    topics=[topic_words],
                    texts=tokenized_texts,
                    dictionary=dictionary,
                    coherence=self.coherence_type.value,
                )
                per_cluster_coherence[i] = cm.get_coherence()
            except Exception as e:  # noqa: BLE001 - per-topic coherence fallback; records 0.0 on gensim errors
                logger.warning("Error computing coherence", topic_id=i, error=str(e))
                per_cluster_coherence[i] = 0.0

        # Simple average for topics (no cluster sizes)
        overall_coherence = (
            sum(per_cluster_coherence.values()) / len(per_cluster_coherence)
            if per_cluster_coherence
            else 0.0
        )

        return CoherenceResult(
            overall_coherence=overall_coherence,
            coherence_type=self.coherence_type.value,
            per_cluster_coherence=per_cluster_coherence,
            num_clusters=len(topics),
            num_documents=len(texts),
        )
