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
from pathlib import Path

import numpy as np
import pandas as pd
from sklearn.metrics.pairwise import cosine_similarity


def load_embedder():
    """Load the E5 embedder."""
    from recap_subworker.services.embedder import Embedder, EmbedderConfig
    from recap_subworker.infra.config import get_settings

    settings = get_settings()

    config = EmbedderConfig(
        model_id=settings.model_id,
        backend=settings.model_backend,
        device="cuda",
        distill_model_id=settings.distill_model_id,
        batch_size=32,
        cache_size=1000,
    )

    return Embedder(config)


def compute_embeddings(texts: list[str], embedder) -> np.ndarray:
    """Compute E5 embeddings for texts."""
    # E5 expects "passage: " prefix
    prefixed = [f"passage: {t}" for t in texts]
    return embedder.encode(prefixed)


def validate_semantic_similarity(
    augmented_texts: list[str],
    original_texts: list[str],
    embedder,
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
        for i, (aug_emb, orig_emb) in enumerate(zip(aug_embs, orig_embs)):
            similarity = cosine_similarity(
                aug_emb.reshape(1, -1),
                orig_emb.reshape(1, -1)
            )[0, 0]

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

    for i, (aug, orig) in enumerate(zip(augmented_texts, original_texts)):
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
        import joblib

        clf = joblib.load(Path("data/genre_classifier.joblib"))
        tfidf = joblib.load(Path("data/tfidf_vectorizer.joblib"))
        svd = joblib.load(Path("data/tfidf_svd.joblib"))
        scaler = joblib.load(Path("data/feature_scaler.joblib"))
    except FileNotFoundError:
        print("WARNING: Classifier not found, skipping label consistency check")
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

        for i, (pred, prob, expected) in enumerate(zip(predictions, probs, batch_labels)):
            global_idx = batch_start + i

            if pred == expected:
                # Get confidence for the expected class
                class_idx = list(classes).index(expected) if expected in classes else -1
                if class_idx >= 0:
                    confidence = prob[class_idx]
                    if confidence >= min_confidence:
                        valid_samples.append((global_idx, float(confidence)))

    return valid_samples


def main():
    parser = argparse.ArgumentParser(
        description="Validate augmented data quality"
    )
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

    args = parser.parse_args()

    # Load augmented data
    print("Loading augmented data...")
    aug_dfs = []
    for path in args.input:
        if path.exists():
            df = pd.read_csv(path)
            df["source_file"] = path.name
            aug_dfs.append(df)
            print(f"  Loaded {len(df)} samples from {path}")
        else:
            print(f"  WARNING: {path} not found")

    if not aug_dfs:
        print("ERROR: No augmented data files found")
        return

    aug_df = pd.concat(aug_dfs, ignore_index=True)
    print(f"Total augmented samples: {len(aug_df)}")

    # Load original data for similarity comparison
    print(f"Loading original data from {args.original}...")
    orig_df = pd.read_csv(args.original)
    orig_df = orig_df.dropna(subset=["content", "genre"])
    print(f"Original samples: {len(orig_df)}")

    # Create lookup for original samples by genre
    orig_by_genre = {}
    for genre, group in orig_df.groupby("genre"):
        orig_by_genre[genre] = group["content"].tolist()

    # Track validation results
    validation_mask = np.ones(len(aug_df), dtype=bool)
    similarity_scores = np.zeros(len(aug_df))
    confidence_scores = np.zeros(len(aug_df))

    # 1. Length ratio validation
    print("\n1. Validating length ratios...")
    valid_length = 0
    for i, row in aug_df.iterrows():
        genre = row["genre"]
        aug_text = row["content"]

        if genre in orig_by_genre and orig_by_genre[genre]:
            # Compare to average original length
            avg_orig_len = np.mean([len(t) for t in orig_by_genre[genre]])
            ratio = len(aug_text) / avg_orig_len

            if not (args.length_ratio_min <= ratio <= args.length_ratio_max):
                validation_mask[i] = False
            else:
                valid_length += 1
        else:
            # No original samples to compare, pass by default
            valid_length += 1

    print(f"  Length ratio valid: {valid_length}/{len(aug_df)}")

    # 2. Semantic similarity validation
    if not args.skip_similarity:
        print("\n2. Validating semantic similarity...")
        embedder = load_embedder()

        # For each augmented sample, compare to a random original from same genre
        batch_size = 100
        valid_sim = 0

        for batch_start in range(0, len(aug_df), batch_size):
            batch_end = min(batch_start + batch_size, len(aug_df))
            batch = aug_df.iloc[batch_start:batch_end]

            aug_texts = batch["content"].tolist()
            genres = batch["genre"].tolist()

            # Get reference originals
            orig_refs = []
            for genre in genres:
                if genre in orig_by_genre and orig_by_genre[genre]:
                    orig_refs.append(orig_by_genre[genre][0])  # First sample as reference
                else:
                    orig_refs.append(aug_texts[genres.index(genre)])  # Self reference

            # Compute similarities
            aug_embs = compute_embeddings(aug_texts, embedder)
            orig_embs = compute_embeddings(orig_refs, embedder)

            for i, (aug_emb, orig_emb) in enumerate(zip(aug_embs, orig_embs)):
                global_idx = batch_start + i

                if not validation_mask[global_idx]:
                    continue

                similarity = cosine_similarity(
                    aug_emb.reshape(1, -1),
                    orig_emb.reshape(1, -1)
                )[0, 0]

                similarity_scores[global_idx] = similarity

                if similarity < args.min_similarity:
                    validation_mask[global_idx] = False
                else:
                    valid_sim += 1

            if batch_end % 500 == 0:
                print(f"  Processed {batch_end}/{len(aug_df)}...")

        print(f"  Semantic similarity valid: {valid_sim}/{sum(validation_mask[:batch_end])}")
    else:
        print("\n2. Skipping semantic similarity check")
        similarity_scores[:] = 1.0

    # 3. Label consistency validation
    if not args.skip_label_check:
        print("\n3. Validating label consistency...")

        # Only validate samples that passed previous checks
        valid_indices = np.where(validation_mask)[0]

        if len(valid_indices) > 0:
            valid_texts = aug_df.iloc[valid_indices]["content"].tolist()
            valid_labels = aug_df.iloc[valid_indices]["genre"].tolist()

            consistent = validate_label_consistency(
                valid_texts,
                valid_labels,
                min_confidence=args.min_confidence,
            )

            # Update mask based on consistency results
            consistent_set = set(idx for idx, _ in consistent)
            for i, global_idx in enumerate(valid_indices):
                if i not in consistent_set:
                    validation_mask[global_idx] = False
                else:
                    # Find confidence for this index
                    for idx, conf in consistent:
                        if idx == i:
                            confidence_scores[global_idx] = conf
                            break

            print(f"  Label consistency valid: {len(consistent)}/{len(valid_indices)}")
    else:
        print("\n3. Skipping label consistency check")
        confidence_scores[:] = 1.0

    # Create validated dataframe
    validated_df = aug_df[validation_mask].copy()
    validated_df["similarity_score"] = similarity_scores[validation_mask]
    validated_df["confidence_score"] = confidence_scores[validation_mask]

    print(f"\n=== Validation Summary ===")
    print(f"Input samples: {len(aug_df)}")
    print(f"Validated samples: {len(validated_df)}")
    print(f"Validation rate: {len(validated_df)/len(aug_df)*100:.1f}%")

    # Print by genre
    print("\nValidated samples by genre:")
    for genre, count in validated_df["genre"].value_counts().items():
        orig_count = len(aug_df[aug_df["genre"] == genre])
        rate = count / orig_count * 100 if orig_count > 0 else 0
        print(f"  {genre}: {count}/{orig_count} ({rate:.1f}%)")

    # Print by augmentation method
    if "augmentation_method" in validated_df.columns:
        print("\nValidated samples by augmentation method:")
        for method, count in validated_df["augmentation_method"].value_counts().items():
            orig_count = len(aug_df[aug_df["augmentation_method"] == method])
            rate = count / orig_count * 100 if orig_count > 0 else 0
            print(f"  {method}: {count}/{orig_count} ({rate:.1f}%)")

    # Save validated data
    validated_df.to_csv(args.output, index=False)
    print(f"\nSaved validated data to {args.output}")


if __name__ == "__main__":
    main()
