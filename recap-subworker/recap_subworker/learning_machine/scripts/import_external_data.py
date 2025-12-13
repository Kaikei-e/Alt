import argparse
import json
import logging
from pathlib import Path
from typing import Dict, Any, List

# Ensure transformers/datasets are installed
try:
    from datasets import load_dataset
except ImportError:
    print("Please install 'datasets' library: uv add datasets")
    exit(1)

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Livedoor News Mappings (Approximate)
# Source categories:
# "dokujo-tsushin" (Peachy / Women's interest) -> home_living / culture_arts / society_demographics ?
# "it-life-hack" -> consumer_tech
# "kaden-channel" -> consumer_tech / consumer_products
# "livedoor-homme" -> home_living / culture_arts
# "movie-enter" -> film_tv
# "peachy" -> home_living / fashion_beauty (if exists) -> let's check taxonomy
# "smax" -> consumer_tech (mobile)
# "sports-watch" -> sports
# "topic-news" -> society_general ?

# My 30 Genres (from memory/task):
# ai_data, climate_environment, consumer_products, consumer_tech, culture_arts, cybersecurity,
# diplomacy_security, economics_macro, education, energy_transition, film_tv, food_cuisine,
# games_esports, health_medicine, home_living, industry_logistics, internet_platforms,
# labor_workplace, law_crime, life_science, markets_finance, mobility_automotive, music_audio,
# politics_government, society_demographics, software_dev, space_astronomy, sports,
# startups_innovation, travel_places

LIVEDOOR_MAP = {
    "dokujo-tsushin": "home_living", # Often relationship/lifestyle
    "it-life-hack": "consumer_tech",
    "kaden-channel": "consumer_products",
    "livedoor-homme": "culture_arts", # Men's lifestyle
    "movie-enter": "film_tv",
    "peachy": "home_living", # Lifestyle
    "smax": "consumer_tech", # Mobile/Gadgets
    "sports-watch": "sports",
    "topic-news": "society_demographics" # Soft news/Social processing
}

import tarfile
import urllib.request
import gzip
import os

def import_livedoor(output_path: Path):
    logger.info("Downloading/Loading Livedoor News Corpus Manually...")

    url = "https://www.rondhuit.com/download/ldcc-20140209.tar.gz"
    cache_dir = Path("recap_subworker/learning_machine/data/cache")
    cache_dir.mkdir(parents=True, exist_ok=True)
    tar_path = cache_dir / "ldcc-20140209.tar.gz"

    if not tar_path.exists():
        logger.info(f"Downloading from {url}...")
        try:
            urllib.request.urlretrieve(url, tar_path)
        except Exception as e:
            logger.error(f"Download failed: {e}")
            return

    articles = []
    skipped = 0
    mapped = 0

    try:
        with tarfile.open(tar_path, "r:gz") as tar:
            for member in tar.getmembers():
                if not member.isfile():
                    continue
                if member.name.endswith("LICENSE.txt") or member.name.endswith("README.txt"):
                    continue

                # Path format: text/dokujo-tsushin/dokujo-tsushin-4778030.txt
                parts = member.name.split("/")
                if len(parts) < 3:
                    continue

                cat_name = parts[1] # e.g. dokujo-tsushin

                my_genre = LIVEDOOR_MAP.get(cat_name)
                if not my_genre:
                    skipped += 1
                    continue

                f = tar.extractfile(member)
                if f:
                    content_bytes = f.read()
                    try:
                        text_content = content_bytes.decode("utf-8")
                        lines = text_content.strip().split("\n")
                        # Format:
                        # URL
                        # Timestamp
                        # Title
                        # Body...
                        if len(lines) > 3:
                            url_line = lines[0]
                            time_line = lines[1]
                            title_line = lines[2]
                            body = "\n".join(lines[3:])

                            articles.append({
                                "source": "livedoor",
                                "lang": "ja",
                                "original_category": cat_name,
                                "label": my_genre,
                                "title": title_line,
                                "content": body,
                                "url": url_line,
                                "published_at": time_line
                            })
                            mapped += 1
                    except Exception as e:
                        logger.warning(f"Error reading file {member.name}: {e}")

    except Exception as e:
        logger.error(f"Error processing tar file: {e}")
        return

    logger.info(f"Livedoor: Mapped {mapped}, Skipped {skipped}")

    # Save append
    with open(output_path, "a", encoding="utf-8") as f:
        for a in articles:
            f.write(json.dumps(a, ensure_ascii=False) + "\n")

def import_ag_news(output_path: Path):
    """Import AG News dataset (English news classification)."""
    logger.info("Loading AG News dataset from HuggingFace...")
    try:
        dataset = load_dataset("ag_news", split="train")

        # AG News has 4 classes: World, Sports, Business, Sci/Tech
        # Map to our genres (approximate mapping)
        AG_NEWS_MAP = {
            0: "society_demographics",  # World
            1: "sports",  # Sports
            2: "economics_macro",  # Business
            3: "consumer_tech",  # Sci/Tech
        }

        articles = []
        for item in dataset:
            label_id = item["label"]
            mapped_genre = AG_NEWS_MAP.get(label_id, "society_demographics")
            articles.append({
                "source": "ag_news",
                "lang": "en",
                "original_label": label_id,
                "label": mapped_genre,
                "title": item.get("text", "").split("\n")[0] if "\n" in item.get("text", "") else "",
                "content": item.get("text", ""),
            })

        logger.info(f"AG News: Mapped {len(articles)} articles")

        # Append to file
        with open(output_path, "a", encoding="utf-8") as f:
            for a in articles:
                f.write(json.dumps(a, ensure_ascii=False) + "\n")
    except Exception as e:
        logger.error(f"Failed to import AG News: {e}")

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--include_ag_news", action="store_true", help="Include AG News English dataset")
    args = parser.parse_args()

    output_path = Path("recap_subworker/learning_machine/data/silver_external.jsonl")
    output_path.parent.mkdir(parents=True, exist_ok=True)

    # Clear file
    with open(output_path, "w") as f:
        pass

    import_livedoor(output_path)

    if args.include_ag_news:
        import_ag_news(output_path)

    logger.info(f"Saved external data to {output_path}")

if __name__ == "__main__":
    main()
