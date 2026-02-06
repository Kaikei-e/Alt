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
from pathlib import Path

import pandas as pd


def compute_content_hash(text: str) -> str:
    """Compute hash of text content for deduplication."""
    # Normalize whitespace and compute hash
    normalized = " ".join(text.split()).lower()
    return hashlib.md5(normalized.encode()).hexdigest()


def main():
    parser = argparse.ArgumentParser(
        description="Merge original and augmented training data"
    )
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

    args = parser.parse_args()

    # Load original data
    print(f"Loading original data from {args.original}...")
    orig_df = pd.read_csv(args.original)
    orig_df = orig_df.dropna(subset=["content", "genre"])
    orig_df["is_augmented"] = False
    orig_df["augmentation_method"] = "original"

    print(f"Original samples: {len(orig_df)}")

    # Load augmented data
    print("\nLoading augmented data...")
    aug_dfs = []
    for path in args.augmented:
        if path.exists():
            df = pd.read_csv(path)
            df["is_augmented"] = True
            if "augmentation_method" not in df.columns:
                df["augmentation_method"] = "unknown"
            aug_dfs.append(df)
            print(f"  Loaded {len(df)} samples from {path}")
        else:
            print(f"  WARNING: {path} not found")

    if not aug_dfs:
        print("WARNING: No augmented data files found. Using only original data.")
        merged_df = orig_df[["content", "genre"]].copy()
        merged_df.to_csv(args.output, index=False)
        print(f"Saved to {args.output}")
        return

    aug_df = pd.concat(aug_dfs, ignore_index=True)
    print(f"Total augmented samples: {len(aug_df)}")

    # Combine original and augmented
    # Keep only essential columns
    orig_subset = orig_df[["content", "genre", "is_augmented", "augmentation_method"]].copy()
    aug_subset = aug_df[["content", "genre", "is_augmented", "augmentation_method"]].copy()

    combined_df = pd.concat([orig_subset, aug_subset], ignore_index=True)
    print(f"\nCombined samples: {len(combined_df)}")

    # Deduplication
    if args.deduplicate:
        print("\nDeduplicating...")
        initial_count = len(combined_df)

        # Compute hashes
        combined_df["content_hash"] = combined_df["content"].apply(compute_content_hash)

        # Keep first occurrence (prioritize original over augmented)
        # Sort so originals come first
        combined_df = combined_df.sort_values("is_augmented")
        combined_df = combined_df.drop_duplicates(subset=["content_hash"], keep="first")
        combined_df = combined_df.drop(columns=["content_hash"])

        removed = initial_count - len(combined_df)
        print(f"  Removed {removed} duplicates")

    # Print current distribution
    print("\nCurrent genre distribution:")
    for genre, count in combined_df["genre"].value_counts().items():
        orig_count = len(combined_df[(combined_df["genre"] == genre) & (~combined_df["is_augmented"])])
        aug_count = len(combined_df[(combined_df["genre"] == genre) & (combined_df["is_augmented"])])
        print(f"  {genre}: {count} (orig: {orig_count}, aug: {aug_count})")

    # Balance classes if specified
    if args.max_samples_per_genre or args.min_samples_per_genre:
        print("\nBalancing classes...")

        balanced_dfs = []
        for genre in combined_df["genre"].unique():
            genre_df = combined_df[combined_df["genre"] == genre]
            current_count = len(genre_df)

            if args.max_samples_per_genre and current_count > args.max_samples_per_genre:
                # Downsample, prioritizing original samples
                orig_samples = genre_df[~genre_df["is_augmented"]]
                aug_samples = genre_df[genre_df["is_augmented"]]

                if len(orig_samples) >= args.max_samples_per_genre:
                    # Only use originals
                    sampled = orig_samples.sample(
                        n=args.max_samples_per_genre,
                        random_state=args.seed
                    )
                else:
                    # Use all originals + sample from augmented
                    aug_needed = args.max_samples_per_genre - len(orig_samples)
                    if len(aug_samples) > aug_needed:
                        aug_sampled = aug_samples.sample(n=aug_needed, random_state=args.seed)
                    else:
                        aug_sampled = aug_samples
                    sampled = pd.concat([orig_samples, aug_sampled])

                balanced_dfs.append(sampled)
                print(f"  {genre}: {current_count} -> {len(sampled)} (downsampled)")

            elif args.min_samples_per_genre and current_count < args.min_samples_per_genre:
                # Not enough samples, use all available
                balanced_dfs.append(genre_df)
                print(f"  {genre}: {current_count} (below minimum {args.min_samples_per_genre})")

            else:
                balanced_dfs.append(genre_df)

        combined_df = pd.concat(balanced_dfs, ignore_index=True)

    # Shuffle
    combined_df = combined_df.sample(frac=1, random_state=args.seed).reset_index(drop=True)

    # Final output (keep only content and genre for training)
    final_df = combined_df[["content", "genre"]].copy()

    print(f"\n=== Final Dataset ===")
    print(f"Total samples: {len(final_df)}")
    print(f"Original: {len(combined_df[~combined_df['is_augmented']])}")
    print(f"Augmented: {len(combined_df[combined_df['is_augmented']])}")

    print("\nFinal genre distribution:")
    for genre, count in final_df["genre"].value_counts().items():
        print(f"  {genre}: {count}")

    # Save
    final_df.to_csv(args.output, index=False)
    print(f"\nSaved to {args.output}")

    # Also save detailed version with augmentation metadata
    detailed_output = args.output.with_suffix(".detailed.csv")
    combined_df.to_csv(detailed_output, index=False)
    print(f"Saved detailed version to {detailed_output}")


if __name__ == "__main__":
    main()
