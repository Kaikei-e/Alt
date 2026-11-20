import json
import logging
import sys
from pathlib import Path

import yaml

# Add project root to path
current_dir = Path(__file__).resolve().parent
project_root = current_dir.parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.insert(0, str(project_root))

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


def main() -> None:
    # Source: The original "Golden Classification" JSON
    source_path = project_root / "data" / "golden_classification.json"
    output_path = project_root / "recap_subworker" / "learning_machine" / "data" / "gold_seed.jsonl"
    output_path.parent.mkdir(parents=True, exist_ok=True)

    if not source_path.exists():
        logger.error("Source file not found: %s", source_path)
        return

    with source_path.open(encoding="utf-8") as f:
        data = json.load(f)

    # Data is {"items": [...], "genres": [...]}
    items = data.get("items", [])
    genres_list = data.get("genres", [])

    # Save taxonomy to yaml
    # We want a simple list of IDs for label mapping
    genre_ids = [g["id"] for g in genres_list]

    taxonomy_path = (
        project_root / "recap_subworker" / "learning_machine" / "taxonomy" / "genres.yaml"
    )
    taxonomy_path.parent.mkdir(parents=True, exist_ok=True)

    with taxonomy_path.open("w", encoding="utf-8") as f:
        yaml.dump({"genres": genre_ids}, f, allow_unicode=True)
    logger.info("Saved taxonomy (%d genres) to %s", len(genre_ids), taxonomy_path)

    # Process items
    valid_items = []
    for item in items:
        # Expected format: {"text": "...", "expected_genres": ["..."]}
        # Flatten to single label if needed for initial Teacher training?
        # Teacher model (BERT) supports multi-label logic usually if we use BCEWithLogitsLoss.
        # But Phase 3 says "class-weight (imbalance)" which suggests Multi-Class or Multi-Label.
        # "Pseudo-labeling" often implies single label or distribution.
        # Let's keep "expected_genres" list.

        # Extract Japanese (or primary text)
        text_ja = item.get("content_ja") or item.get("text") or item.get("content")
        if text_ja:
            valid_items.append(
                {
                    "source": "golden_v1_ja",
                    "lang": "ja",
                    "title": item.get("title", ""),
                    "content": text_ja,
                    "labels": item.get("expected_genres", []),
                }
            )

        # Extract English if available
        text_en = item.get("content_en")
        if text_en:
            valid_items.append(
                {
                    "source": "golden_v1_en",
                    "lang": "en",
                    "title": item.get("title", "") + " (EN)",
                    "content": text_en,
                    "labels": item.get("expected_genres", []),
                }
            )

    with output_path.open("w", encoding="utf-8") as f:
        for item in valid_items:
            f.write(json.dumps(item, ensure_ascii=False) + "\n")

    logger.info("Built gold_seed.jsonl with %d items.", len(valid_items))


if __name__ == "__main__":
    main()
