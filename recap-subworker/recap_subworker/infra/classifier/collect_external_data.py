
import os
import tarfile
import urllib.request
import pandas as pd
from pathlib import Path
from datasets import load_dataset

DATA_DIR = Path("data")
LIVEDOOR_URL = "https://www.rondhuit.com/download/ldcc-20140209.tar.gz"
LIVEDOOR_DIR = DATA_DIR / "text"
EXTERNAL_OUTPUT_PATH = DATA_DIR / "external_data.csv"

# Livedoor Mapping
# Categories:
# topic-news, livedoor-homme, k-tai-watch (not in ldcc?),
# dokujo-tsushin, it-life-hack, kaden-channel, movie-enter, peachy, smax, sports-watch
LIVEDOOR_MAP = {
    'it-life-hack': 'consumer_tech', # IT, Tech hacks
    'kaden-channel': 'consumer_tech', # Appliances -> Consumer Tech or Home Living? Let's go Tech/Products.
    'smax': 'consumer_tech', # Smartphones -> Consumer Tech
    'movie-enter': 'film_tv', # Movies -> Film & TV
    'sports-watch': 'sports', # Sports -> Sports
    'dokujo-tsushin': 'home_living', # Lifestyle (Women) -> Home & Living or Society? Home/Living seems safer for soft news.
    'peachy': 'home_living', # Lifestyle -> Home & Living
    'livedoor-homme': 'home_living', # Lifestyle (Men) -> Home & Living
    'topic-news': 'politics_government', # News -> Politics/Gov (Often mixed, but heavily newsy)
}

# AG News Mapping
# Class Index: 1-World, 2-Sports, 3-Business, 4-Sci/Tech
AG_NEWS_MAP = {
    1: 'politics_government', # World -> Politics/Gov (Closest fit for World News)
    2: 'sports', # Sports -> Sports
    3: 'markets_finance', # Business -> Markets & Finance (or Business/Economy)
    4: 'consumer_tech', # Sci/Tech -> Consumer Tech (dominant) or Science. Let's pick Consumer Tech as it's more common.
}

def download_livedoor():
    if not LIVEDOOR_DIR.exists():
        print("Downloading Livedoor News Corpus...")
        tar_path = DATA_DIR / "ldcc.tar.gz"
        urllib.request.urlretrieve(LIVEDOOR_URL, tar_path)
        print("Extracting...")
        with tarfile.open(tar_path, "r:gz") as tar:
            tar.extractall(path=DATA_DIR)
        os.remove(tar_path)
    else:
        print("Livedoor data already exists.")

def process_livedoor():
    rows = []
    if not LIVEDOOR_DIR.exists():
        return rows

    for cat_dir in LIVEDOOR_DIR.iterdir():
        if not cat_dir.is_dir():
            continue
        category = cat_dir.name
        genre = LIVEDOOR_MAP.get(category)
        if not genre:
            continue

        for file_path in cat_dir.glob("*.txt"):
            if file_path.name == "LICENSE.txt":
                continue
            try:
                with open(file_path, "r", encoding="utf-8") as f:
                    lines = f.readlines()
                    # Skip header lines (url, time) - usually first 2 lines
                    if len(lines) > 2:
                        content = "".join(lines[2:]).strip()
                        if content:
                            rows.append({'content': content, 'genre': genre})
            except Exception as e:
                print(f"Error reading {file_path}: {e}")

    print(f"Processed {len(rows)} Livedoor articles.")
    return rows

def process_ag_news():
    print("Loading AG News...")
    try:
        dataset = load_dataset("ag_news", split="train") # 120k samples
        rows = []
        # Downsample strictly to avoid overwhelming the dataset
        # Take e.g. 500 per class
        shuffled = dataset.shuffle(seed=42)

        counts = {g: 0 for g in AG_NEWS_MAP.values()}
        limit = 500

        for item in shuffled:
            label = item['label'] + 1 # labels are 0-3, map expects 1-4
            genre = AG_NEWS_MAP.get(label)
            if not genre:
                continue

            if counts[genre] < limit:
                counts[genre] += 1
                content = item['text']
                rows.append({'content': content, 'genre': genre})

            if all(c >= limit for c in counts.values()):
                break

        print(f"Processed {len(rows)} AG News articles.")
        return rows
    except Exception as e:
        print(f"Error processing AG News: {e}")
        return []

def main():
    DATA_DIR.mkdir(exist_ok=True)

    # 1. Livedoor
    download_livedoor()
    livedoor_rows = process_livedoor()

    # 2. AG News
    ag_rows = process_ag_news()

    # Combined
    all_rows = livedoor_rows + ag_rows
    df = pd.DataFrame(all_rows)

    if not df.empty:
        print(f"Saving {len(df)} external samples to {EXTERNAL_OUTPUT_PATH}")
        df.to_csv(EXTERNAL_OUTPUT_PATH, index=False)
        print(df['genre'].value_counts())
    else:
        print("No data collected.")

if __name__ == "__main__":
    main()
