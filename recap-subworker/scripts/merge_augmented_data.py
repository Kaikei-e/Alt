"""
Merge Original and Augmented Training Data.

Combines original training data with validated augmented samples.
Handles deduplication and balances class distribution.

Usage:
    uv run python scripts/merge_augmented_data.py \
        --original data/training_data.csv \
        --augmented data/augmented_validated.csv \
        --output data/training_data_augmented.csv
"""

import argparse
import hashlib
import sys
from pathlib import Path

import pandas as pd


def compute_content_hash(text: str) -> str:
    """Compute hash of text content for deduplication."""
    # Normalize whitespace and compute hash
    normalized = " ".join(text.split()).lower()
    return hashlib.md5(normalized.encode(), usedforsecurity=False).hexdigest()


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Merge original and augmented training data")
    parser.add_argument(
        "--original",
        type=Path,
        default=Path("data/training_data.csv"),
        help="Original training data CSV",
    )
    parser.add_argument(
        "--augmented",
        type=Path,
        nargs="+",
        required=True,
        help="Augmented data CSV file(s)",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("data/training_data_augmented.csv"),
        help="Output merged CSV file",
    )
    parser.add_argument(
        "--max-samples-per-genre",
        type=int,
        default=None,
        help="Maximum samples per genre (for balancing)",
    )
    parser.add_argument(
        "--min-samples-per-genre",
        type=int,
        default=50,
        help="Minimum samples per genre (will pad with augmented if available)",
    )
    parser.add_argument(
        "--deduplicate",
        action="store_true",
        default=True,
        help="Remove duplicate content",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=42,
        help="Random seed for sampling",
    )
    return parser.parse_args()


def _load_augmented(paths: list[Path]) -> list[pd.DataFrame]:
    sys.stdout.write("\nLoading augmented data...\n")
    aug_dfs = []
    for path in paths:
        if path.exists():
            df = pd.read_csv(path)
            df["is_augmented"] = True
            if "augmentation_method" not in df.columns:
                df["augmentation_method"] = "unknown"
            aug_dfs.append(df)
            sys.stdout.write(f"  Loaded {len(df)} samples from {path}\n")
        else:
            sys.stdout.write(f"  WARNING: {path} not found\n")
    return aug_dfs


def _deduplicate_df(combined_df: pd.DataFrame) -> pd.DataFrame:
    sys.stdout.write("\nDeduplicating...\n")
    initial_count = len(combined_df)
    combined_df["content_hash"] = combined_df["content"].apply(compute_content_hash)
    # Keep first occurrence (prioritize original over augmented)
    combined_df = combined_df.sort_values("is_augmented")
    combined_df = combined_df.drop_duplicates(subset=["content_hash"], keep="first")
    combined_df = combined_df.drop(columns=["content_hash"])
    removed = initial_count - len(combined_df)
    sys.stdout.write(f"  Removed {removed} duplicates\n")
    return combined_df


def _balance_classes(
    combined_df: pd.DataFrame,
    max_samples_per_genre: int | None,
    min_samples_per_genre: int | None,
    seed: int,
) -> pd.DataFrame:
    sys.stdout.write("\nBalancing classes...\n")
    balanced_dfs = []
    for genre in combined_df["genre"].unique():
        genre_df = combined_df[combined_df["genre"] == genre]
        current_count = len(genre_df)

        if max_samples_per_genre and current_count > max_samples_per_genre:
            orig_samples = genre_df[~genre_df["is_augmented"]]
            aug_samples = genre_df[genre_df["is_augmented"]]

            if len(orig_samples) >= max_samples_per_genre:
                sampled = orig_samples.sample(n=max_samples_per_genre, random_state=seed)
            else:
                aug_needed = max_samples_per_genre - len(orig_samples)
                aug_sampled = (
                    aug_samples.sample(n=aug_needed, random_state=seed)
                    if len(aug_samples) > aug_needed
                    else aug_samples
                )
                sampled = pd.concat([orig_samples, aug_sampled])

            balanced_dfs.append(sampled)
            sys.stdout.write(f"  {genre}: {current_count} -> {len(sampled)} (downsampled)\n")
        elif min_samples_per_genre and current_count < min_samples_per_genre:
            balanced_dfs.append(genre_df)
            sys.stdout.write(
                f"  {genre}: {current_count} (below minimum {min_samples_per_genre})\n"
            )
        else:
            balanced_dfs.append(genre_df)

    return pd.concat(balanced_dfs, ignore_index=True)


def main() -> None:
    args = _parse_args()

    # Load original data
    sys.stdout.write(f"Loading original data from {args.original}...\n")
    orig_df = pd.read_csv(args.original)
    orig_df = orig_df.dropna(subset=["content", "genre"])
    orig_df["is_augmented"] = False
    orig_df["augmentation_method"] = "original"

    sys.stdout.write(f"Original samples: {len(orig_df)}\n")

    aug_dfs = _load_augmented(args.augmented)
    if not aug_dfs:
        sys.stdout.write("WARNING: No augmented data files found. Using only original data.\n")
        merged_df = orig_df[["content", "genre"]].copy()
        merged_df.to_csv(args.output, index=False)
        sys.stdout.write(f"Saved to {args.output}\n")
        return

    aug_df = pd.concat(aug_dfs, ignore_index=True)
    sys.stdout.write(f"Total augmented samples: {len(aug_df)}\n")

    orig_subset = orig_df[["content", "genre", "is_augmented", "augmentation_method"]].copy()
    aug_subset = aug_df[["content", "genre", "is_augmented", "augmentation_method"]].copy()
    combined_df = pd.concat([orig_subset, aug_subset], ignore_index=True)
    sys.stdout.write(f"\nCombined samples: {len(combined_df)}\n")

    if args.deduplicate:
        combined_df = _deduplicate_df(combined_df)

    sys.stdout.write("\nCurrent genre distribution:\n")
    for genre, count in combined_df["genre"].value_counts().items():
        orig_count = len(
            combined_df[(combined_df["genre"] == genre) & (~combined_df["is_augmented"])]
        )
        aug_count = len(
            combined_df[(combined_df["genre"] == genre) & (combined_df["is_augmented"])]
        )
        sys.stdout.write(f"  {genre}: {count} (orig: {orig_count}, aug: {aug_count})\n")

    if args.max_samples_per_genre or args.min_samples_per_genre:
        combined_df = _balance_classes(
            combined_df, args.max_samples_per_genre, args.min_samples_per_genre, args.seed
        )

    combined_df = combined_df.sample(frac=1, random_state=args.seed).reset_index(drop=True)
    final_df = combined_df[["content", "genre"]].copy()

    sys.stdout.write("\n=== Final Dataset ===\n")
    sys.stdout.write(f"Total samples: {len(final_df)}\n")
    sys.stdout.write(f"Original: {len(combined_df[~combined_df['is_augmented']])}\n")
    sys.stdout.write(f"Augmented: {len(combined_df[combined_df['is_augmented']])}\n")

    sys.stdout.write("\nFinal genre distribution:\n")
    for genre, count in final_df["genre"].value_counts().items():
        sys.stdout.write(f"  {genre}: {count}\n")

    final_df.to_csv(args.output, index=False)
    sys.stdout.write(f"\nSaved to {args.output}\n")

    detailed_output = args.output.with_suffix(".detailed.csv")
    combined_df.to_csv(detailed_output, index=False)
    sys.stdout.write(f"Saved detailed version to {detailed_output}\n")


if __name__ == "__main__":
    main()
