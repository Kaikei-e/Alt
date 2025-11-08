#!/usr/bin/env python3
"""
Utility script to regenerate genre classifier weights from a labelled dataset.

The script consumes a JSON file with structure identical to
`tests/data/golden_classification.json` and emits a weights JSON compatible with
`recap_worker::classification::model::HybridModel`.
"""

from __future__ import annotations

import argparse
import json
from collections import Counter, defaultdict
from pathlib import Path

FEATURE_VOCAB = [
    "人工知能",
    "自動運転",
    "資金調達",
    "投資",
    "決算",
    "政策",
    "政府",
    "遺伝子",
    "医療",
    "量子",
    "サッカー",
    "音楽",
    "confidential computing",
    "cybersecurity",
    "transformer",
    "diplomacy",
    "treaty",
    "economy",
    "business",
]

EMBEDDING_DIM = 6

EMBED_LOOKUP = {
    "人工知能": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "自動運転": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "transformer": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "資金調達": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "投資": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "決算": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "economy": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "business": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "政策": [0.0, 0.0, 1.0, 0.0, 0.0, 0.0],
    "政府": [0.0, 0.0, 1.0, 0.0, 0.0, 0.0],
    "diplomacy": [0.0, 0.3, 0.8, 0.0, 0.0, 0.0],
    "treaty": [0.0, 0.3, 0.8, 0.0, 0.0, 0.0],
    "遺伝子": [0.0, 0.0, 0.0, 1.0, 0.0, 0.0],
    "医療": [0.0, 0.0, 0.0, 1.0, 0.0, 0.0],
    "量子": [0.4, 0.1, 0.0, 0.9, 0.0, 0.0],
    "サッカー": [0.0, 0.0, 0.0, 0.0, 1.0, 0.0],
    "音楽": [0.0, 0.0, 0.0, 0.0, 0.0, 1.0],
    "confidential computing": [0.8, 0.3, 0.0, 0.0, 0.0, 0.0],
    "cybersecurity": [0.8, 0.2, 0.0, 0.0, 0.0, 0.0],
}

GENRES = ["ai", "tech", "business", "politics", "health", "sports", "science", "entertainment", "world"]


def load_samples(path: Path) -> list[dict]:
    data = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(data, list):
        raise ValueError("dataset must be a JSON array")
    return data


def expand_tokens(tokens: list[str]) -> list[str]:
    expanded = []
    for token in tokens:
        lower = token.lower()
        expanded.append(lower)
        # simple stemming for english plural forms
        if lower.endswith("s") and len(lower) > 3:
            expanded.append(lower[:-1])
    return expanded


def build_feature_counts(samples: list[dict]) -> tuple[dict[str, Counter], Counter]:
    feature_counts: dict[str, Counter] = {genre: Counter() for genre in GENRES}
    genre_totals: Counter = Counter()
    for sample in samples:
        expected = [g.lower() for g in sample.get("expected_genres", [])]
        tokens = expand_tokens(sample.get("tokens", []))
        for genre in expected:
            if genre not in feature_counts:
                continue
            genre_totals[genre] += 1
            for token in tokens:
                if token in FEATURE_VOCAB:
                    feature_counts[genre][token] += 1
    return feature_counts, genre_totals


def infer_tokens(title: str, body: str) -> list[str]:
    text = f"{title} {body}".lower().replace("　", " ")
    return [tok for tok in text.split() if tok]


def enrich_samples(samples: list[dict]) -> None:
    for sample in samples:
        tokens = sample.get("tokens")
        if not tokens:
            tokens = infer_tokens(sample.get("title", ""), sample.get("body", ""))
        sample["tokens"] = tokens


def compute_weights(samples: list[dict]) -> dict:
    enrich_samples(samples)
    feature_counts, genre_totals = build_feature_counts(samples)

    tfidf_weights = []
    for genre in GENRES:
        total = max(1, genre_totals[genre])
        row = []
        for term in FEATURE_VOCAB:
            tf = feature_counts[genre][term] / total
            idf = 1.0 + (1.0 / (1 + feature_counts[genre][term]))
            row.append(round(tf * idf * 1.5, 3))
        tfidf_weights.append(row)

    embedding_weights = []
    for genre in GENRES:
        agg = [0.0] * EMBEDDING_DIM
        hits = 0
        for term in FEATURE_VOCAB:
            vec = EMBED_LOOKUP.get(term)
            if vec and feature_counts[genre][term] > 0:
                hits += 1
                for idx, value in enumerate(vec):
                    agg[idx] += value
        if hits:
            agg = [round(v / hits, 3) for v in agg]
        embedding_weights.append(agg)

    bias = [round(0.01 * (1 + genre_totals[g]), 3) for g in GENRES]

    return {
        "feature_dim": len(FEATURE_VOCAB),
        "embedding_dim": EMBEDDING_DIM,
        "genres": GENRES,
        "tfidf_weights": tfidf_weights,
        "embedding_weights": embedding_weights,
        "bias": bias,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="Regenerate genre classifier weights.")
    parser.add_argument("dataset", type=Path, help="Path to labelled dataset JSON")
    parser.add_argument("output", type=Path, help="Output weights JSON path")
    args = parser.parse_args()

    samples = load_samples(args.dataset)
    weights = compute_weights(samples)
    args.output.write_text(json.dumps(weights, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()

