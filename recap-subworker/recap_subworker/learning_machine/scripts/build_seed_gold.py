import json
import logging
from pathlib import Path
import sys

# Add project root to path
current_dir = Path(__file__).resolve().parent
project_root = current_dir.parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.insert(0, str(project_root))

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

def main():
    # Source: The original "Golden Classification" JSON
    source_path = project_root / "data" / "golden_classification.json"
    output_path = project_root / "recap_subworker" / "learning_machine" / "data" / "gold_seed.jsonl"
    output_path.parent.mkdir(parents=True, exist_ok=True)

    if not source_path.exists():
        logger.error(f"Source file not found: {source_path}")
        return

    with open(source_path, "r", encoding="utf-8") as f:
        data = json.load(f)

    # Data is {"items": [...], "genres": [...]}
    items = data.get("items", [])
    genres_list = data.get("genres", [])

    # Save taxonomy to yaml
    # We want a simple list of IDs for label mapping
    genre_ids = [g["id"] for g in genres_list]

    taxonomy_path = project_root / "recap_subworker" / "learning_machine" / "taxonomy" / "genres.yaml"
    taxonomy_path.parent.mkdir(parents=True, exist_ok=True)
    import yaml
    with open(taxonomy_path, "w", encoding="utf-8") as f:
        yaml.dump({"genres": genre_ids}, f, allow_unicode=True)
    logger.info(f"Saved taxonomy ({len(genre_ids)} genres) to {taxonomy_path}")

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
            valid_items.append({
                "source": "golden_v1_ja",
                "lang": "ja",
                "title": item.get("title", ""),
                "content": text_ja,
                "labels": item.get("expected_genres", [])
            })

        # Extract English if available
        text_en = item.get("content_en")
        if text_en:
            valid_items.append({
                "source": "golden_v1_en",
                "lang": "en",
                "title": item.get("title", "") + " (EN)",
                "content": text_en,
                "labels": item.get("expected_genres", [])
            })

    with open(output_path, "w", encoding="utf-8") as f:
        for item in valid_items:
            f.write(json.dumps(item, ensure_ascii=False) + "\n")

    logger.info(f"Built gold_seed.jsonl with {len(valid_items)} items.")

if __name__ == "__main__":
    main()
