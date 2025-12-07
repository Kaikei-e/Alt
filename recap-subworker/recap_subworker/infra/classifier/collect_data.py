
import asyncio
import pandas as pd
from pathlib import Path
import json
from collections import Counter

# Define paths
DATA_DIR = Path("data")
ALT_EXPORT_PATH = DATA_DIR / "alt_export.csv"
GOLDEN_PATH = DATA_DIR / "golden_classification.json"
OUTPUT_PATH = DATA_DIR / "training_data.csv"

# Tag Mapping Strategy (Granular 30 Genres)
TAG_TO_GENRE = {
    # 1. AI & Data (ai_data)
    'ai': 'ai_data', 'llm': 'ai_data', 'chatgpt': 'ai_data', 'openai': 'ai_data',
    'machine learning': 'ai_data', 'generative ai': 'ai_data', 'nvidia': 'ai_data',

    # 2. Software Development (software_dev)
    'python': 'software_dev', 'rust': 'software_dev', 'go': 'software_dev', 'java': 'software_dev',
    'javascript': 'software_dev', 'js': 'software_dev', 'code': 'software_dev', 'script': 'software_dev',
    'programming': 'software_dev', 'github': 'software_dev', 'docker': 'software_dev',
    '開発': 'software_dev', '技術': 'software_dev', '実装': 'software_dev', 'プログラミング': 'software_dev',
    'aws': 'software_dev', 'cloud': 'software_dev', 'linux': 'software_dev', 'ubuntu': 'software_dev',

    # 3. Cybersecurity (cybersecurity)
    'security': 'cybersecurity', 'hacker': 'cybersecurity', 'vulnerability': 'cybersecurity',
    'malware': 'cybersecurity', 'breach': 'cybersecurity', 'authentication': 'cybersecurity',

    # 4. Consumer Tech (consumer_tech)
    'android': 'consumer_tech', 'iphone': 'consumer_tech', 'smartphone': 'consumer_tech',
    'pixel': 'consumer_tech', 'galaxy': 'consumer_tech', 'ipad': 'consumer_tech', 'tablet': 'consumer_tech',
    'macbook': 'consumer_tech', 'windows': 'consumer_tech', 'pc': 'consumer_tech', 'laptop': 'consumer_tech',
    'device': 'consumer_tech', 'hardware': 'consumer_tech', 'gadget': 'consumer_tech',
    'apple': 'consumer_tech', 'samsung': 'consumer_tech', 'google': 'consumer_tech', # Broad but often consumer

    # 5. Internet Platforms (internet_platforms)
    'social media': 'internet_platforms', 'facebook': 'internet_platforms', 'twitter': 'internet_platforms',
    'x': 'internet_platforms', 'instagram': 'internet_platforms', 'tiktok': 'internet_platforms',
    'youtube': 'internet_platforms', 'app store': 'internet_platforms', 'browser': 'internet_platforms',

    # 6. Space & Astronomy (space_astronomy)
    'space': 'space_astronomy', 'nasa': 'space_astronomy', 'spacex': 'space_astronomy',
    'astronomy': 'space_astronomy', 'moon': 'space_astronomy', 'mars': 'space_astronomy',

    # 7. Climate & Environment (climate_environment)
    'climate': 'climate_environment', 'environment': 'climate_environment', 'global warming': 'climate_environment',
    'carbon': 'climate_environment', 'emission': 'climate_environment', 'plastic': 'climate_environment',

    # 8. Energy Transition (energy_transition)
    'energy': 'energy_transition', 'solar': 'energy_transition', 'wind': 'energy_transition',
    'battery': 'energy_transition', 'nuclear': 'energy_transition', 'hydrogen': 'energy_transition',
    'renewable': 'energy_transition',

    # 9. Healthcare & Medicine (health_medicine)
    'medicine': 'health_medicine', 'doctor': 'health_medicine', 'hospital': 'health_medicine',
    'health': 'health_medicine', # Broad, but fits here
    'covid': 'health_medicine', 'virus': 'health_medicine', 'vaccine': 'health_medicine',
    'mental health': 'health_medicine',

    # 10. Life Science (life_science)
    'biology': 'life_science', 'genetics': 'life_science', 'dna': 'life_science',
    'biotech': 'life_science', 'research': 'life_science', 'science': 'life_science', # 'science' is broad, maybe skip?

    # 11. Macroeconomics (economics_macro)
    'economy': 'economics_macro', 'inflation': 'economics_macro', 'gdp': 'economics_macro',
    'interest rate': 'economics_macro', 'employment': 'economics_macro', 'recession': 'economics_macro',

    # 12. Markets & Finance (markets_finance)
    'stock': 'markets_finance', 'market': 'markets_finance', 'investing': 'markets_finance',
    'finance': 'markets_finance', 'crypto': 'markets_finance', 'bitcoin': 'markets_finance',
    'bank': 'markets_finance', 'earnings': 'markets_finance', 'nasdaq': 'markets_finance',

    # 13. Startups & Innovation (startups_innovation)
    'startup': 'startups_innovation', 'venture capital': 'startups_innovation', 'funding': 'startups_innovation',
    'innovation': 'startups_innovation', 'entrepreneur': 'startups_innovation', 'founder': 'startups_innovation',

    # 14. Industry & Logistics (industry_logistics)
    'industry': 'industry_logistics', 'supply chain': 'industry_logistics', 'logistics': 'industry_logistics',
    'manufacturing': 'industry_logistics', 'factory': 'industry_logistics', 'production': 'industry_logistics',

    # 15. Politics & Government (politics_government)
    'politics': 'politics_government', 'government': 'politics_government', 'election': 'politics_government',
    'vote': 'politics_government', 'parliament': 'politics_government', 'congress': 'politics_government',
    'democrat': 'politics_government', 'republican': 'politics_government', 'senate': 'politics_government',
    'prime minister': 'politics_government', 'president': 'politics_government', 'biden': 'politics_government',
    'trump': 'politics_government', 'campaign': 'politics_government', 'policy': 'politics_government',

    # 16. Diplomacy & Security (diplomacy_security)
    'diplomacy': 'diplomacy_security', 'war': 'diplomacy_security', 'military': 'diplomacy_security',
    'defense': 'diplomacy_security', 'nato': 'diplomacy_security', 'geopolitics': 'diplomacy_security',

    # 17. Law & Crime (law_crime)
    'law': 'law_crime', 'court': 'law_crime', 'legal': 'law_crime', 'lawsuit': 'law_crime',
    'judge': 'law_crime', 'crime': 'law_crime', 'police': 'law_crime', 'scam': 'law_crime',

    # 18. Education (education)
    'education': 'education', 'school': 'education', 'university': 'education', 'student': 'education',
    'teacher': 'education', 'learning': 'education',

    # 19. Labor & Workplace (labor_workplace)
    'work': 'labor_workplace', 'job': 'labor_workplace', 'career': 'labor_workplace', 'hiring': 'labor_workplace',
    'workplace': 'labor_workplace', 'remote work': 'labor_workplace', 'salary': 'labor_workplace',

    # 20. Society & Demographics (society_demographics)
    'society': 'society_demographics', 'population': 'society_demographics', 'migration': 'society_demographics',
    'gender': 'society_demographics', 'welfare': 'society_demographics',

    # 21. Culture & Arts (culture_arts)
    'art': 'culture_arts', 'culture': 'culture_arts', 'museum': 'culture_arts', 'exhibition': 'culture_arts',
    'heritage': 'culture_arts',

    # 22. Film & TV (film_tv)
    'movie': 'film_tv', 'film': 'film_tv', 'cinema': 'film_tv', 'tv': 'film_tv', 'drama': 'film_tv',
    'netflix': 'film_tv', 'disney': 'film_tv', 'series': 'film_tv', 'actor': 'film_tv', 'hollywood': 'film_tv',
    '映画': 'film_tv',

    # 23. Music & Audio (music_audio)
    'music': 'music_audio', 'song': 'music_audio', 'concert': 'music_audio', 'spotify': 'music_audio',
    'band': 'music_audio', 'singer': 'music_audio', 'audio': 'music_audio', 'podcast': 'music_audio',

    # 24. Sports (sports)
    'sports': 'sports', 'football': 'sports', 'soccer': 'sports', 'baseball': 'sports',
    'basketball': 'sports', 'nba': 'sports', 'nfl': 'sports', 'tennis': 'sports',
    'olympics': 'sports', 'athlete': 'sports', 'stadium': 'sports', 'golf': 'sports',
    '野球': 'sports', 'ゴルフ': 'sports',

    # 25. Food & Cuisine (food_cuisine)
    'food': 'food_cuisine', 'drink': 'food_cuisine', 'restaurant': 'food_cuisine', 'cooking': 'food_cuisine',
    'recipe': 'food_cuisine', 'beer': 'food_cuisine', 'wine': 'food_cuisine',

    # 26. Travel & Places (travel_places)
    'travel': 'travel_places', 'tourism': 'travel_places', 'hotel': 'travel_places', 'airline': 'travel_places',
    'flight': 'travel_places', 'vacation': 'travel_places', 'resort': 'travel_places',

    # 27. Home & Living (home_living)
    'home': 'home_living', 'house': 'home_living', 'interior': 'home_living', 'furniture': 'home_living',
    'garden': 'home_living', 'diy': 'home_living', 'lifehack': 'home_living',

    # 28. Games & Esports (games_esports)
    'game': 'games_esports', 'games': 'games_esports', 'gaming': 'games_esports', 'esports': 'games_esports',
    'nintendo': 'games_esports', 'sony': 'games_esports', 'playstation': 'games_esports', 'xbox': 'games_esports',
    'steam': 'games_esports', 'ps5': 'games_esports', 'anime': 'games_esports', 'manga': 'games_esports',
    'comics': 'games_esports', 'アニメ': 'games_esports', '漫画': 'games_esports', # Often crossover, but close enough

    # 29. Mobility & Automotive (mobility_automotive)
    'car': 'mobility_automotive', 'auto': 'mobility_automotive', 'ev': 'mobility_automotive',
    'tesla': 'mobility_automotive', 'toyota': 'mobility_automotive', 'transport': 'mobility_automotive',
    'train': 'mobility_automotive', 'bus': 'mobility_automotive', 'mobility': 'mobility_automotive',

    # 30. Consumer & Products (consumer_products)
    'shopping': 'consumer_products', 'retail': 'consumer_products', 'product': 'consumer_products',
    'brand': 'consumer_products', 'sale': 'consumer_products', 'amazon': 'consumer_products',
    'price': 'consumer_products', 'discount': 'consumer_products', # Context dependent, but often consumer
}

def decide_genre(tags_list):
    """
    Decide genre based on a list of tags.
    Returns (genre, confidence_score) or (None, 0).
    """
    if not tags_list:
        return None, 0.0

    votes = []
    for tag in tags_list:
        if not isinstance(tag, str):
            continue
        mapped = TAG_TO_GENRE.get(tag.lower())
        if mapped:
            votes.append(mapped)

    if not votes:
        return None, 0.0

    counts = Counter(votes)
    top_genre, top_count = counts.most_common(1)[0]
    total_votes = len(votes)

    # Confidence: simple majority ratio
    confidence = top_count / total_votes

    # Threshold: At least 50% agreement and at least 1 mapped tag (which is true if votes > 0)
    # If mixed (e.g. 1 tech, 1 business), confidence is 0.5. We might drop these or pick the more specific one?
    # For now, let's require > 0.5 to be sure, or accept 0.5 if total_votes == 1?
    # If 1 vote, conf is 1.0.
    if confidence >= 0.5:
        return top_genre, confidence

    return None, confidence

def collect_data():
    dfs = []

    # 1. Load Raw Export from Alt DB (Content + Tags)
    if ALT_EXPORT_PATH.exists():
        print(f"Loading exported raw data from {ALT_EXPORT_PATH}...")
        try:
            raw_df = pd.read_csv(ALT_EXPORT_PATH)
            print(f"Loaded {len(raw_df)} raw rows (content-tag pairs).")

            # Group tags by content
            # This aggregates all tags for the same article content
            grouped = raw_df.groupby('content')['tag_name'].apply(list).reset_index()
            print(f"Unique articles: {len(grouped)}")

            # Apply voting
            results = grouped['tag_name'].apply(decide_genre)
            grouped['genre'] = [r[0] for r in results]
            grouped['confidence'] = [r[1] for r in results]

            # Filter
            clean_df = grouped.dropna(subset=['genre'])
            print(f"Articles with resolved genre: {len(clean_df)}")

            dfs.append(clean_df[['content', 'genre']])

        except Exception as e:
            print(f"Error processing alt export: {e}")
            import traceback
            traceback.print_exc()

    # 2. Load Golden Set
    if GOLDEN_PATH.exists():
        print(f"Loading golden set from {GOLDEN_PATH}...")
        try:
            with open(GOLDEN_PATH) as f:
                golden = json.load(f)

            golden_rows = []
            for item in golden['items']:
                genre = item.get('primary_genre')
                if not genre:
                    continue
                if 'content_ja' in item and item['content_ja']:
                    golden_rows.append({'content': item['content_ja'], 'genre': genre})
                if 'content_en' in item and item['content_en']:
                    golden_rows.append({'content': item['content_en'], 'genre': genre})

            golden_df = pd.DataFrame(golden_rows)
            print(f"Loaded {len(golden_df)} samples from golden set.")
            dfs.append(golden_df)
        except Exception as e:
            print(f"Error loading golden set: {e}")

    # 3. Load External Data (Livedoor / AG News)
    EXTERNAL_PATH = DATA_DIR / "external_data.csv"
    if EXTERNAL_PATH.exists():
        print(f"Loading external data from {EXTERNAL_PATH}...")
        try:
            ext_df = pd.read_csv(EXTERNAL_PATH)
            print(f"Loaded {len(ext_df)} external samples.")
            dfs.append(ext_df[['content', 'genre']])
        except Exception as e:
            print(f"Error loading external data: {e}")

    if not dfs:
        print("No data collected!")
        return

    # 4. Combine
    full_df = pd.concat(dfs, ignore_index=True)
    full_df = full_df[full_df['content'].str.len() > 50]

    print("Combined Distribution:")
    print(full_df['genre'].value_counts())

    # 5. Balance
    # If 'consumer_tech' is huge, downsample it.
    min_samples = 20 # Lowered to keep more genres (e.g. ai_data ~47)
    max_samples = 2000

    balanced_dfs = []
    for genre, group in full_df.groupby('genre'):
        if len(group) < min_samples:
            print(f"Dropping genre '{genre}' (count {len(group)} < {min_samples})")
            continue

        if len(group) > max_samples:
            group = group.sample(n=max_samples, random_state=42)

        balanced_dfs.append(group)

    final_df = pd.concat(balanced_dfs, ignore_index=True)

    # 6. Save
    print(f"Saving {len(final_df)} samples to {OUTPUT_PATH}")
    final_df.to_csv(OUTPUT_PATH, index=False)
    print("Final Distribution:")
if __name__ == "__main__":
    collect_data()
