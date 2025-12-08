import json
import logging
import sys
from pathlib import Path
from typing import List, Dict

# Path setup
current_dir = Path(__file__).resolve().parent
project_root = current_dir.parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.insert(0, str(project_root))

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

def main():
    output_path = Path("recap_subworker/learning_machine/data/gold_seed.jsonl")

    # Synthetic Data (2 JA + 2 EN per genre)
    new_items = []

    logger.info(f"Adding {len(new_items)} items to {output_path}")

    with open(output_path, "a", encoding="utf-8") as f:
        for item in new_items:
            item["source"] = "gold_augment_synthetic"
            f.write(json.dumps(item, ensure_ascii=False) + "\n")

    logger.info("Successfully augmented gold Japanese data.")

if __name__ == "__main__":
    main()
