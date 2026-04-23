"""Build a 30-class training CSV from the canonical golden set + silver teachers.

ADR-000835 stage 2: the existing ``data/training_data.csv`` only carries 17
genres because ``collect_data.py``'s ``TAG_TO_GENRE`` mapping never covered the
13 missing canonical genres. The golden set at
``data/golden_classification.json`` is already 30-class balanced (120 items per
genre, 3600 total) under ``taxonomy_version=genre-fixed-30-v1`` — use it as the
primary source and top up with silver-teacher pseudo-labels.

Output: ``data/training_data_30class.csv`` with columns ``content, genre``.
"""

from __future__ import annotations

import argparse
import json
from collections import Counter
from pathlib import Path

import pandas as pd


def _load_golden(path: Path) -> list[dict]:
    payload = json.loads(path.read_text())
    return list(payload.get("items", []))


def _load_silver(path: Path) -> list[dict]:
    rows: list[dict] = []
    with path.open() as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            rows.append(json.loads(line))
    return rows


def _golden_to_pairs(items: list[dict], min_content_chars: int) -> list[tuple[str, str]]:
    pairs: list[tuple[str, str]] = []
    for item in items:
        genre = item.get("primary_genre")
        if not genre:
            continue
        for key in ("content_ja", "content_en"):
            text = item.get(key) or ""
            if len(text) >= min_content_chars:
                pairs.append((text, str(genre)))
    return pairs


def _silver_to_pairs(rows: list[dict], min_content_chars: int) -> list[tuple[str, str]]:
    pairs: list[tuple[str, str]] = []
    for row in rows:
        genre = row.get("label") or row.get("genre")
        text = row.get("content") or row.get("text") or ""
        if genre and len(text) >= min_content_chars:
            pairs.append((str(text), str(genre)))
    return pairs


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--repo-root",
        type=Path,
        default=Path(__file__).resolve().parent.parent,
        help="recap-subworker repo root",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=None,
        help="output CSV (default: <repo-root>/data/training_data_30class.csv)",
    )
    parser.add_argument(
        "--min-content-chars",
        type=int,
        default=80,
        help="drop rows whose content is shorter than this (chars)",
    )
    parser.add_argument(
        "--include-silver-v0",
        action="store_true",
        help=(
            "include silver_teacher_v0.jsonl — adds volume but is heavily "
            "consumer_tech-skewed, may pull class prior"
        ),
    )
    args = parser.parse_args()

    root: Path = args.repo_root
    golden_path = root / "data" / "golden_classification.json"
    silver_ja = root / "recap_subworker/learning_machine/data/silver_teacher_v0_ja.jsonl"
    silver_en = root / "recap_subworker/learning_machine/data/silver_teacher_v0_en.jsonl"
    silver_v0 = root / "recap_subworker/learning_machine/data/silver_teacher_v0.jsonl"

    pairs: list[tuple[str, str]] = []
    if golden_path.is_file():
        gold_pairs = _golden_to_pairs(_load_golden(golden_path), args.min_content_chars)
        print(f"golden: {len(gold_pairs)} rows")
        pairs.extend(gold_pairs)
    else:
        raise FileNotFoundError(f"golden_classification.json not found at {golden_path}")

    for silver_path in (silver_ja, silver_en):
        if silver_path.is_file():
            sp = _silver_to_pairs(_load_silver(silver_path), args.min_content_chars)
            print(f"{silver_path.name}: {len(sp)} rows")
            pairs.extend(sp)

    if args.include_silver_v0 and silver_v0.is_file():
        sp = _silver_to_pairs(_load_silver(silver_v0), args.min_content_chars)
        print(f"{silver_v0.name}: {len(sp)} rows")
        pairs.extend(sp)

    df = pd.DataFrame(pairs, columns=["content", "genre"]).dropna(
        subset=["content", "genre"]
    )
    before = len(df)
    df = df.drop_duplicates(subset=["content", "genre"])
    print(f"dropped {before - len(df)} exact duplicates")

    counts = Counter(df["genre"])
    print(f"\ntotal rows: {len(df)}  unique genres: {len(counts)}")
    for genre, count in counts.most_common():
        print(f"  {genre}: {count}")

    missing = sorted(
        {
            "ai_data", "software_dev", "cybersecurity", "consumer_tech",
            "internet_platforms", "space_astronomy", "climate_environment",
            "energy_transition", "health_medicine", "life_science",
            "economics_macro", "markets_finance", "startups_innovation",
            "industry_logistics", "politics_government", "diplomacy_security",
            "law_crime", "education", "labor_workplace", "society_demographics",
            "culture_arts", "film_tv", "music_audio", "sports", "food_cuisine",
            "travel_places", "home_living", "games_esports", "mobility_automotive",
            "consumer_products",
        }
        - set(counts.keys())
    )
    if missing:
        print(f"\nWARNING: missing canonical genres: {missing}")

    output = args.output or (root / "data" / "training_data_30class.csv")
    output.parent.mkdir(parents=True, exist_ok=True)
    df.to_csv(output, index=False)
    print(f"\nwrote {output} ({len(df)} rows)")


if __name__ == "__main__":
    main()
