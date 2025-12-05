import numpy as np
from typing import Dict, List, Optional
import structlog
from .embedder import Embedder

logger = structlog.get_logger(__name__)

# Initial genre descriptions for zero-shot prototype generation
GENRE_DESCRIPTIONS = {
    "ai": "Artificial Intelligence, machine learning, deep learning, LLMs, generative AI",
    "tech": "Technology news, gadgets, software, hardware, internet services",
    "business": "Business news, markets, companies, startups, economy",
    "politics": "Politics, government, elections, policy, legislation",
    "health": "Health, medicine, medical research, wellness, diseases",
    "sports": "Sports news, games, athletes, teams, matches",
    "science": "Science, research, space, physics, biology, chemistry",
    "entertainment": "Entertainment, movies, music, celebrities, arts",
    "world": "World news, international relations, global events",
    "security": "Cybersecurity, hacking, vulnerabilities, information security",
    "product": "New products, product launches, consumer goods",
    "design": "Design, UI/UX, art, creative industry",
    "culture": "Culture, society, trends, lifestyle",
    "environment": "Environment, climate change, sustainability, nature",
    "lifestyle": "Lifestyle, living, food, travel, fashion",
    "art_culture": "Art, culture, exhibitions, creative works",
    "developer_insights": "Programming, software development, coding, engineering",
    "pro_it_media": "Professional IT media, enterprise tech, industry news",
    "consumer_tech": "Consumer electronics, smartphones, personal computers",
    "global_politics": "International politics, diplomacy, geopolitics",
    "environment_policy": "Environmental policy, green energy, climate regulations",
    "society_justice": "Social issues, justice, human rights, law",
    "travel_lifestyle": "Travel, tourism, destinations, experiences",
    "security_policy": "National security, defense, military, security treaties",
    "business_finance": "Finance, investing, stock market, banking",
    "ai_research": "AI research papers, algorithms, theoretical AI",
    "ai_policy": "AI regulation, ethics, compliance, safety",
    "games_puzzles": "Video games, gaming industry, puzzles, esports",
    "other": "Miscellaneous, uncategorized content"
}

class CoarseClassifier:
    """
    Coarse genre classifier using E5 embeddings and prototype vectors.
    """

    def __init__(self, embedder: Embedder):
        self.embedder = embedder
        self.prototypes: Dict[str, np.ndarray] = {}
        self.initialized = False

    def initialize_prototypes(self):
        """
        Initialize genre prototypes by embedding descriptions.
        This is a 'zero-shot' initialization. ideally we would load centroids from a dataset.
        """
        logger.info("Initializing coarse classifier prototypes")

        genres = list(GENRE_DESCRIPTIONS.keys())
        # Add "query: " prefix for E5 asymmetric tasks (though prototypes are kinda symmetric to documents)
        # Docs say: "for symmetric tasks... query: prefix is generally recommended"
        # We will use "query: " for both prototypes and input text to be safe/consistent.
        texts = [f"query: {GENRE_DESCRIPTIONS[g]}" for g in genres]

        embeddings = self.embedder.encode(texts)

        for genre, embedding in zip(genres, embeddings):
            self.prototypes[genre] = embedding

        self.initialized = True
        logger.info("Prototypes initialized", count=len(self.prototypes))

    def predict_coarse(self, text: str, threshold: float = 0.0) -> Dict[str, float]:
        """
        Predict genre scores for input text.

        Args:
            text: Input text
            threshold: Minimum score threshold (if we were doing hard filtering, but here we return all)

        Returns:
            Dictionary of genre scores (dot product/cosine similarity)
        """
        if not self.initialized:
            self.initialize_prototypes()

        # Using "query: " prefix for input as well
        # Truncate text to avoid overly long inputs if necessary, E5 handles 512
        input_text = f"query: {text[:2000]}"
        embedding = self.embedder.encode([input_text])[0]

        scores = {}
        for genre, prototype in self.prototypes.items():
            # Embeddings are normalized, so dot product is cosine similarity
            score = float(np.dot(embedding, prototype))
            scores[genre] = score

        return scores
