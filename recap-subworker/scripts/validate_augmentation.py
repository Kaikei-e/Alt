"""
Augmentation Quality Validation Script.

Validates augmented data quality using:
- Semantic similarity (E5 embeddings, threshold 0.7)
- Label consistency check (classifier confidence)
- Length ratio validation

Usage:
    uv run python scripts/validate_augmentation.py \
        --input data/augmented_eda.csv data/augmented_backtrans.csv \
        --output data/augmented_validated.csv \
        --min-similarity 0.7
"""

import argparse
import dataclasses
import sys
from pathlib import Path

import joblib
import numpy as np
import pandas as pd
from sklearn.metrics.pairwise import cosine_similarity

from recap_subworker.infra.config import get_settings
from recap_subworker.services.embedder import Embedder, EmbedderConfig


def load_embedder() -> Embedder:
    """Load the E5 embedder."""
    settings = get_settings()

    config = dataclasses.replace(
        EmbedderConfig.from_settings(settings),
        device="cuda",
        batch_size=32,
        cache_size=1000,
    )

    return Embedder(config)


def compute_embeddings(texts: list[str], embedder: Embedder) -> np.ndarray:
    """Compute E5 embeddings for texts."""
    # E5 expects "passage: " prefix
    prefixed = [f"passage: {t}" for t in texts]
    return embedder.encode(prefixed)


def validate_semantic_similarity(
    augmented_texts: list[str],
    original_texts: list[str],
    embedder: Embedder,
    min_similarity: float = 0.7,
    batch_size: int = 100,
) -> list[tuple[int, float]]:
    """
    Validate augmented texts against originals using semantic similarity.

    Returns:
        List of (index, similarity_score) for samples that pass validation
    """
    valid_samples = []

    # Process in batches to manage memory
    for batch_start in range(0, len(augmented_texts), batch_size):
        batch_end = min(batch_start + batch_size, len(augmented_texts))

        aug_batch = augmented_texts[batch_start:batch_end]
        orig_batch = original_texts[batch_start:batch_end]

        # Compute embeddings
        aug_embs = compute_embeddings(aug_batch, embedder)
        orig_embs = compute_embeddings(orig_batch, embedder)

        # Compute pairwise similarities
        for i, (aug_emb, orig_emb) in enumerate(zip(aug_embs, orig_embs, strict=True)):
            similarity = cosine_similarity(aug_emb.reshape(1, -1), orig_emb.reshape(1, -1))[0, 0]

            if similarity >= min_similarity:
                valid_samples.append((batch_start + i, float(similarity)))

    return valid_samples


def validate_length_ratio(
    augmented_texts: list[str],
    original_texts: list[str],
    min_ratio: float = 0.5,
    max_ratio: float = 2.0,
) -> list[tuple[int, float]]:
    """
    Validate length ratio between augmented and original texts.

    Returns:
        List of (index, ratio) for samples that pass validation
    """
    valid_samples = []

    for i, (aug, orig) in enumerate(zip(augmented_texts, original_texts, strict=True)):
        if len(orig) == 0:
            continue

        ratio = len(aug) / len(orig)

        if min_ratio <= ratio <= max_ratio:
            valid_samples.append((i, ratio))

    return valid_samples


def validate_label_consistency(
    texts: list[str],
    expected_labels: list[str],
    min_confidence: float = 0.6,
) -> list[tuple[int, float]]:
    """
    Validate label consistency using the trained classifier.

    Returns:
        List of (index, confidence) for samples where classifier agrees
    """
    try:
        clf = joblib.load(Path("data/genre_classifier.joblib"))
        tfidf = joblib.load(Path("data/tfidf_vectorizer.joblib"))
        svd = joblib.load(Path("data/tfidf_svd.joblib"))
        scaler = joblib.load(Path("data/feature_scaler.joblib"))
    except FileNotFoundError:
        sys.stdout.write("WARNING: Classifier not found, skipping label consistency check\n")
        return [(i, 1.0) for i in range(len(texts))]

    valid_samples = []

    # Load embedder for combined features
    embedder = load_embedder()

    # Process in batches
    batch_size = 100
    for batch_start in range(0, len(texts), batch_size):
        batch_end = min(batch_start + batch_size, len(texts))

        batch_texts = texts[batch_start:batch_end]
        batch_labels = expected_labels[batch_start:batch_end]

        # Generate features (same as train.py)
        embs = compute_embeddings(batch_texts, embedder)
        tfidf_features = tfidf.transform(batch_texts)
        tfidf_svd = svd.transform(tfidf_features)

        combined = np.hstack((embs, tfidf_svd))
        combined_scaled = scaler.transform(combined)

        # Get predictions
        probs = clf.predict_proba(combined_scaled)
        predictions = clf.predict(combined_scaled)
        classes = clf.classes_

        for i, (pred, prob, expected) in enumerate(
            zip(predictions, probs, batch_labels, strict=True)
        ):
            global_idx = batch_start + i

            if pred == expected:
                # Get confidence for the expected class
                class_idx = list(classes).index(expected) if expected in classes else -1
                if class_idx >= 0:
                    confidence = prob[class_idx]
                    if confidence >= min_confidence:
                        valid_samples.append((global_idx, float(confidence)))

    return valid_samples


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Validate augmented data quality")
    parser.add_argument(
        "--input",
        type=Path,
        nargs="+",
        required=True,
        help="Input CSV files with augmented data",
    )
    parser.add_argument(
        "--original",
        type=Path,
        default=Path("data/training_data.csv"),
        help="Original training data CSV",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("data/augmented_validated.csv"),
        help="Output CSV with validated samples",
    )
    parser.add_argument(
        "--min-similarity",
        type=float,
        default=0.7,
        help="Minimum semantic similarity threshold",
    )
    parser.add_argument(
        "--min-confidence",
        type=float,
        default=0.6,
        help="Minimum classifier confidence for label consistency",
    )
    parser.add_argument(
        "--length-ratio-min",
        type=float,
        default=0.5,
        help="Minimum length ratio",
    )
    parser.add_argument(
        "--length-ratio-max",
        type=float,
        default=2.0,
        help="Maximum length ratio",
    )
    parser.add_argument(
        "--skip-similarity",
        action="store_true",
        help="Skip semantic similarity check (faster, for synthetic data)",
    )
    parser.add_argument(
        "--skip-label-check",
        action="store_true",
        help="Skip label consistency check",
    )
    return parser.parse_args()


def _load_augmented_data(paths: list[Path]) -> pd.DataFrame | None:
    sys.stdout.write("Loading augmented data...\n")
    aug_dfs = []
    for path in paths:
        if path.exists():
            df = pd.read_csv(path)
            df["source_file"] = path.name
            aug_dfs.append(df)
            sys.stdout.write(f"  Loaded {len(df)} samples from {path}\n")
        else:
            sys.stdout.write(f"  WARNING: {path} not found\n")

    if not aug_dfs:
        sys.stdout.write("ERROR: No augmented data files found\n")
        return None

    aug_df = pd.concat(aug_dfs, ignore_index=True)
    sys.stdout.write(f"Total augmented samples: {len(aug_df)}\n")
    return aug_df


def _validate_lengths(
    aug_df: pd.DataFrame,
    orig_by_genre: dict[object, list[str]],
    validation_mask: np.ndarray,
    min_ratio: float,
    max_ratio: float,
) -> None:
    sys.stdout.write("\n1. Validating length ratios...\n")
    valid_length = 0
    for i, row in aug_df.iterrows():
        genre = row["genre"]
        aug_text = row["content"]

        if orig_by_genre.get(genre):
            avg_orig_len = np.mean([len(t) for t in orig_by_genre[genre]])
            ratio = len(aug_text) / avg_orig_len
            if not (min_ratio <= ratio <= max_ratio):
                validation_mask[i] = False
            else:
                valid_length += 1
        else:
            valid_length += 1

    sys.stdout.write(f"  Length ratio valid: {valid_length}/{len(aug_df)}\n")


def _validate_similarities(
    aug_df: pd.DataFrame,
    orig_by_genre: dict[object, list[str]],
    validation_mask: np.ndarray,
    similarity_scores: np.ndarray,
    min_similarity: float,
) -> None:
    sys.stdout.write("\n2. Validating semantic similarity...\n")
    embedder = load_embedder()
    batch_size = 100
    valid_sim = 0
    batch_end = 0

    for batch_start in range(0, len(aug_df), batch_size):
        batch_end = min(batch_start + batch_size, len(aug_df))
        batch = aug_df.iloc[batch_start:batch_end]
        aug_texts = batch["content"].tolist()
        genres = batch["genre"].tolist()

        orig_refs = [
            orig_by_genre[genre][0] if orig_by_genre.get(genre) else aug_texts[genres.index(genre)]
            for genre in genres
        ]

        aug_embs = compute_embeddings(aug_texts, embedder)
        orig_embs = compute_embeddings(orig_refs, embedder)

        for i, (aug_emb, orig_emb) in enumerate(zip(aug_embs, orig_embs, strict=True)):
            global_idx = batch_start + i
            if not validation_mask[global_idx]:
                continue

            similarity = cosine_similarity(aug_emb.reshape(1, -1), orig_emb.reshape(1, -1))[0, 0]
            similarity_scores[global_idx] = similarity

            if similarity < min_similarity:
                validation_mask[global_idx] = False
            else:
                valid_sim += 1

        if batch_end % 500 == 0:
            sys.stdout.write(f"  Processed {batch_end}/{len(aug_df)}...\n")

    sys.stdout.write(
        f"  Semantic similarity valid: {valid_sim}/{sum(validation_mask[:batch_end])}\n"
    )


def _validate_labels(
    aug_df: pd.DataFrame,
    validation_mask: np.ndarray,
    confidence_scores: np.ndarray,
    min_confidence: float,
) -> None:
    sys.stdout.write("\n3. Validating label consistency...\n")
    valid_indices = np.where(validation_mask)[0]
    if len(valid_indices) == 0:
        return

    valid_texts = aug_df.iloc[valid_indices]["content"].tolist()
    valid_labels = aug_df.iloc[valid_indices]["genre"].tolist()
    consistent = validate_label_consistency(
        valid_texts, valid_labels, min_confidence=min_confidence
    )

    consistent_map = dict(consistent)
    for i, global_idx in enumerate(valid_indices):
        if i not in consistent_map:
            validation_mask[global_idx] = False
        else:
            confidence_scores[global_idx] = consistent_map[i]

    sys.stdout.write(f"  Label consistency valid: {len(consistent)}/{len(valid_indices)}\n")


def _report_and_save(
    aug_df: pd.DataFrame,
    validation_mask: np.ndarray,
    similarity_scores: np.ndarray,
    confidence_scores: np.ndarray,
    output_path: Path,
) -> None:
    validated_df = aug_df[validation_mask].copy()
    validated_df["similarity_score"] = similarity_scores[validation_mask]
    validated_df["confidence_score"] = confidence_scores[validation_mask]

    sys.stdout.write("\n=== Validation Summary ===\n")
    sys.stdout.write(f"Input samples: {len(aug_df)}\n")
    sys.stdout.write(f"Validated samples: {len(validated_df)}\n")
    sys.stdout.write(f"Validation rate: {len(validated_df) / len(aug_df) * 100:.1f}%\n")

    sys.stdout.write("\nValidated samples by genre:\n")
    for genre, count in validated_df["genre"].value_counts().items():
        orig_count = len(aug_df[aug_df["genre"] == genre])
        rate = count / orig_count * 100 if orig_count > 0 else 0
        sys.stdout.write(f"  {genre}: {count}/{orig_count} ({rate:.1f}%)\n")

    if "augmentation_method" in validated_df.columns:
        sys.stdout.write("\nValidated samples by augmentation method:\n")
        for method, count in validated_df["augmentation_method"].value_counts().items():
            orig_count = len(aug_df[aug_df["augmentation_method"] == method])
            rate = count / orig_count * 100 if orig_count > 0 else 0
            sys.stdout.write(f"  {method}: {count}/{orig_count} ({rate:.1f}%)\n")

    validated_df.to_csv(output_path, index=False)
    sys.stdout.write(f"\nSaved validated data to {output_path}\n")


def main() -> None:
    args = _parse_args()
    aug_df = _load_augmented_data(args.input)
    if aug_df is None:
        return

    sys.stdout.write(f"Loading original data from {args.original}...\n")
    orig_df = pd.read_csv(args.original)
    orig_df = orig_df.dropna(subset=["content", "genre"])
    sys.stdout.write(f"Original samples: {len(orig_df)}\n")

    orig_by_genre = {genre: group["content"].tolist() for genre, group in orig_df.groupby("genre")}
    validation_mask = np.ones(len(aug_df), dtype=bool)
    similarity_scores = np.zeros(len(aug_df))
    confidence_scores = np.zeros(len(aug_df))

    _validate_lengths(
        aug_df, orig_by_genre, validation_mask, args.length_ratio_min, args.length_ratio_max
    )

    if not args.skip_similarity:
        _validate_similarities(
            aug_df, orig_by_genre, validation_mask, similarity_scores, args.min_similarity
        )
    else:
        sys.stdout.write("\n2. Skipping semantic similarity check\n")
        similarity_scores[:] = 1.0

    if not args.skip_label_check:
        _validate_labels(aug_df, validation_mask, confidence_scores, args.min_confidence)
    else:
        sys.stdout.write("\n3. Skipping label consistency check\n")
        confidence_scores[:] = 1.0

    _report_and_save(aug_df, validation_mask, similarity_scores, confidence_scores, args.output)


if __name__ == "__main__":
    main()
