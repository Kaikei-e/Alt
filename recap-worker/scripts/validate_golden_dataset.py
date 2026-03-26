#!/usr/bin/env python3
"""
Golden Classification Dataset Validation Script

Validates the golden dataset against quality criteria:
1. Each genre has minimum 100 items
2. Difficulty distribution (target: 60% baseline, 25% boundary, 15% hard)
3. Parallel pairs >= 25%
4. JSON schema validity
5. No duplicate IDs
6. All required fields present
"""

import argparse
import json
import sys
from collections import Counter
from pathlib import Path


# Required fields for each item
REQUIRED_FIELDS = ["id", "expected_genres", "primary_genre", "difficulty", "language_pairing", "source"]

# Valid enum values
VALID_DIFFICULTIES = ["baseline", "boundary", "hard"]
VALID_LANGUAGE_PAIRINGS = ["ja_only", "en_only", "parallel", "none", "same_story"]
VALID_STYLES = ["headline", "lead", "long_form", None]
VALID_TERMINOLOGY_DENSITIES = ["low", "medium", "high", None]

# 30 expected genres
EXPECTED_GENRES = [
    "ai_data", "software_dev", "cybersecurity", "consumer_tech", "internet_platforms",
    "space_astronomy", "climate_environment", "energy_transition", "health_medicine",
    "life_science", "economics_macro", "markets_finance", "startups_innovation",
    "industry_logistics", "politics_government", "diplomacy_security", "law_crime",
    "education", "labor_workplace", "society_demographics", "culture_arts", "film_tv",
    "music_audio", "sports", "food_cuisine", "travel_places", "home_living",
    "games_esports", "mobility_automotive", "consumer_products"
]


def validate_dataset(data: dict, verbose: bool = True) -> tuple[bool, list[str], list[str]]:
    """
    Validate the golden dataset.

    Returns:
        (is_valid, errors, warnings)
    """
    errors = []
    warnings = []

    # Check schema version
    schema_version = data.get("schema_version", "unknown")
    if verbose:
        print(f"Schema version: {schema_version}")

    if schema_version not in ["2.1", "2.2"]:
        warnings.append(f"Unknown schema version: {schema_version}")

    # Get items
    items = data.get("items", [])
    if not items:
        errors.append("No items found in dataset")
        return False, errors, warnings

    if verbose:
        print(f"Total items: {len(items)}")

    # Check for duplicate IDs
    ids = [item.get("id", "") for item in items]
    id_counts = Counter(ids)
    duplicates = [id_ for id_, count in id_counts.items() if count > 1]
    if duplicates:
        errors.append(f"Duplicate IDs found: {duplicates[:10]}{'...' if len(duplicates) > 10 else ''}")

    # Validate each item
    genre_counts = Counter()
    difficulty_counts = Counter()
    language_pairing_counts = Counter()
    style_counts = Counter()

    for idx, item in enumerate(items):
        item_id = item.get("id", f"item_{idx}")

        # Check required fields
        for field in REQUIRED_FIELDS:
            if field not in item:
                errors.append(f"Item '{item_id}' missing required field: {field}")

        # Check content fields
        has_ja = item.get("content_ja") and item["content_ja"].strip()
        has_en = item.get("content_en") and item["content_en"].strip()
        has_content = item.get("content") and item["content"].strip()

        if not has_ja and not has_en and not has_content:
            errors.append(f"Item '{item_id}' has no content (content_ja, content_en, or content)")

        # Validate difficulty
        difficulty = item.get("difficulty", "baseline")
        if difficulty not in VALID_DIFFICULTIES:
            errors.append(f"Item '{item_id}' has invalid difficulty: {difficulty}")
        difficulty_counts[difficulty] += 1

        # Validate language_pairing
        pairing = item.get("language_pairing", "none")
        if pairing not in VALID_LANGUAGE_PAIRINGS:
            errors.append(f"Item '{item_id}' has invalid language_pairing: {pairing}")
        language_pairing_counts[pairing] += 1

        # Validate primary_genre
        primary_genre = item.get("primary_genre", "")
        if primary_genre and primary_genre not in EXPECTED_GENRES:
            warnings.append(f"Item '{item_id}' has unexpected primary_genre: {primary_genre}")
        genre_counts[primary_genre] += 1

        # Validate expected_genres
        expected_genres = item.get("expected_genres", [])
        if not expected_genres:
            warnings.append(f"Item '{item_id}' has empty expected_genres")
        for genre in expected_genres:
            if genre not in EXPECTED_GENRES:
                warnings.append(f"Item '{item_id}' has unexpected genre in expected_genres: {genre}")

        # Validate optional fields
        style = item.get("style")
        if style is not None and style not in VALID_STYLES:
            warnings.append(f"Item '{item_id}' has unexpected style: {style}")
        style_counts[style] += 1

        terminology_density = item.get("terminology_density")
        if terminology_density is not None and terminology_density not in VALID_TERMINOLOGY_DENSITIES:
            warnings.append(f"Item '{item_id}' has unexpected terminology_density: {terminology_density}")

        # Validate boundary_pair for boundary items
        if difficulty == "boundary":
            boundary_pair = item.get("boundary_pair", [])
            if not boundary_pair or len(boundary_pair) != 2:
                warnings.append(f"Boundary item '{item_id}' missing or invalid boundary_pair")

        # Validate secondary_genres for hard items
        if difficulty == "hard":
            secondary = item.get("secondary_genres", [])
            if not secondary:
                warnings.append(f"Hard item '{item_id}' missing secondary_genres")

    # Check genre distribution (minimum 100 per genre)
    if verbose:
        print("\n=== Genre Distribution ===")
    for genre in EXPECTED_GENRES:
        count = genre_counts.get(genre, 0)
        if verbose:
            print(f"  {genre}: {count}")
        if count < 100:
            errors.append(f"Genre '{genre}' has only {count} items (minimum: 100)")
        elif count < 120:
            warnings.append(f"Genre '{genre}' has only {count} items (target: 120)")

    # Check difficulty distribution
    total = len(items)
    baseline_pct = difficulty_counts.get("baseline", 0) / total * 100 if total > 0 else 0
    boundary_pct = difficulty_counts.get("boundary", 0) / total * 100 if total > 0 else 0
    hard_pct = difficulty_counts.get("hard", 0) / total * 100 if total > 0 else 0

    if verbose:
        print("\n=== Difficulty Distribution ===")
        print(f"  baseline: {difficulty_counts.get('baseline', 0)} ({baseline_pct:.1f}%)")
        print(f"  boundary: {difficulty_counts.get('boundary', 0)} ({boundary_pct:.1f}%)")
        print(f"  hard: {difficulty_counts.get('hard', 0)} ({hard_pct:.1f}%)")

    # Target distribution warnings
    if baseline_pct < 55:
        warnings.append(f"Baseline ratio {baseline_pct:.1f}% is below target (60%)")
    if boundary_pct < 20:
        warnings.append(f"Boundary ratio {boundary_pct:.1f}% is below target (25%)")
    if hard_pct < 10:
        warnings.append(f"Hard ratio {hard_pct:.1f}% is below target (15%)")

    # Check parallel pair ratio
    parallel_count = language_pairing_counts.get("parallel", 0)
    parallel_pct = parallel_count / total * 100 if total > 0 else 0

    if verbose:
        print("\n=== Language Pairing Distribution ===")
        for pairing, count in sorted(language_pairing_counts.items()):
            pct = count / total * 100 if total > 0 else 0
            print(f"  {pairing}: {count} ({pct:.1f}%)")

    if parallel_pct < 25:
        warnings.append(f"Parallel pair ratio {parallel_pct:.1f}% is below target (25%)")

    # Summary
    is_valid = len(errors) == 0

    if verbose:
        print("\n=== Validation Summary ===")
        print(f"Total items: {total}")
        print(f"Unique genres: {len(genre_counts)}")
        print(f"Errors: {len(errors)}")
        print(f"Warnings: {len(warnings)}")

        if errors:
            print("\nErrors:")
            for error in errors[:20]:  # Limit output
                print(f"  - {error}")
            if len(errors) > 20:
                print(f"  ... and {len(errors) - 20} more errors")

        if warnings:
            print("\nWarnings:")
            for warning in warnings[:20]:  # Limit output
                print(f"  - {warning}")
            if len(warnings) > 20:
                print(f"  ... and {len(warnings) - 20} more warnings")

        print(f"\nValidation {'PASSED' if is_valid else 'FAILED'}")

    return is_valid, errors, warnings


def main():
    parser = argparse.ArgumentParser(description="Validate Golden Classification Dataset")
    parser.add_argument(
        "input",
        nargs="?",
        default="recap-worker/recap-worker/tests/data/golden_classification.json",
        help="Input golden dataset path"
    )
    parser.add_argument(
        "-q", "--quiet",
        action="store_true",
        help="Quiet mode (only print errors and warnings)"
    )

    args = parser.parse_args()

    input_path = Path(args.input)
    if not input_path.is_absolute():
        candidates = [
            Path(args.input),
            Path(__file__).parent.parent / args.input,
            Path.cwd() / args.input,
        ]
        for candidate in candidates:
            if candidate.exists():
                input_path = candidate
                break

    if not input_path.exists():
        print(f"Error: Input file not found: {input_path}")
        return 1

    print(f"Validating: {input_path}")

    with open(input_path, "r", encoding="utf-8") as f:
        data = json.load(f)

    is_valid, errors, warnings = validate_dataset(data, verbose=not args.quiet)

    return 0 if is_valid else 1


if __name__ == "__main__":
    sys.exit(main())
