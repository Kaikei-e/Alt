"""
LLM-based Synthetic Data Generation for Genre Classification.

Generates synthetic training samples using LLM for zero-shot or low-sample genres.
Uses genre definitions from golden_classification.json.

Usage:
    uv run python scripts/augment_llm_synthetic.py \
        --genres space_astronomy,climate_environment,life_science \
        --samples-per-genre 100 \
        --output data/augmented_synthetic.csv \
        --ollama-url http://localhost:11434 \
        --model gemma3:4b
"""

import argparse
import json
import random
import re
import time
from pathlib import Path

import httpx
import pandas as pd


def load_genre_definitions(golden_path: Path) -> dict:
    """Load genre definitions from golden_classification.json."""
    with open(golden_path) as f:
        data = json.load(f)

    genres = {}
    for genre in data.get("genres", []):
        genres[genre["id"]] = {
            "name_ja": genre.get("name_ja", ""),
            "name_en": genre.get("name_en", ""),
            "definition_ja": genre.get("definition_ja", ""),
            "definition_en": genre.get("definition_en", ""),
            "exclusions_ja": genre.get("exclusions_ja", ""),
            "exclusions_en": genre.get("exclusions_en", ""),
        }

    return genres


class OllamaSyntheticGenerator:
    """Generate synthetic training data using Ollama."""

    def __init__(
        self,
        ollama_url: str = "http://localhost:11434",
        model: str = "gemma3:4b",
        timeout: float = 120.0,
        rate_limit_delay: float = 2.0,
    ):
        self.ollama_url = ollama_url.rstrip("/")
        self.model = model
        self.timeout = timeout
        self.rate_limit_delay = rate_limit_delay
        self.client = httpx.Client(timeout=timeout)

    def _call_ollama(self, prompt: str) -> str | None:
        """Call Ollama API for generation."""
        try:
            response = self.client.post(
                f"{self.ollama_url}/api/generate",
                json={
                    "model": self.model,
                    "prompt": prompt,
                    "stream": False,
                    "options": {
                        "temperature": 0.9,
                        "top_p": 0.95,
                        "num_predict": 2048,
                    },
                },
            )
            response.raise_for_status()
            result = response.json()
            return result.get("response", "").strip()
        except Exception as e:
            print(f"Ollama API error: {e}")
            return None

    def generate_japanese_sample(
        self,
        genre_id: str,
        genre_info: dict,
        variation_hint: str = "",
    ) -> str | None:
        """Generate a synthetic Japanese sample for a genre."""
        prompt = f"""あなたはニュース記事のコンテンツライターです。
以下のジャンルに該当する、リアルなニュース記事の本文を生成してください。

ジャンル: {genre_info['name_ja']}
定義: {genre_info['definition_ja']}
除外条件: {genre_info['exclusions_ja']}

要件:
- 200〜500文字程度の日本語記事
- 実際のニュース記事のような文体
- 具体的な数字や事例を含める
- {variation_hint}
- 記事本文のみを出力（タイトルや見出しは不要）

記事本文:"""

        result = self._call_ollama(prompt)
        time.sleep(self.rate_limit_delay)
        return result

    def generate_english_sample(
        self,
        genre_id: str,
        genre_info: dict,
        variation_hint: str = "",
    ) -> str | None:
        """Generate a synthetic English sample for a genre."""
        prompt = f"""You are a news content writer.
Generate a realistic news article body for the following genre.

Genre: {genre_info['name_en']}
Definition: {genre_info['definition_en']}
Exclusions: {genre_info['exclusions_en']}

Requirements:
- 150-400 words in English
- Professional news article style
- Include specific numbers or examples
- {variation_hint}
- Output only the article body (no title or headline)

Article body:"""

        result = self._call_ollama(prompt)
        time.sleep(self.rate_limit_delay)
        return result

    def close(self):
        """Close HTTP client."""
        self.client.close()


# Variation hints to increase diversity
VARIATION_HINTS_JA = [
    "最新の技術動向に焦点を当てる",
    "企業の取り組みを紹介する",
    "国際的な動向を含める",
    "研究成果や調査結果を引用する",
    "専門家のコメントを含める",
    "社会への影響を考察する",
    "将来の展望について言及する",
    "具体的な事例やケーススタディを含める",
    "統計データを引用する",
    "業界団体や政府の発表を含める",
]

VARIATION_HINTS_EN = [
    "Focus on recent technological developments",
    "Highlight corporate initiatives",
    "Include international perspectives",
    "Cite research findings or studies",
    "Include expert commentary",
    "Discuss societal implications",
    "Mention future outlook",
    "Include specific case studies or examples",
    "Reference statistical data",
    "Include industry or government announcements",
]


def validate_synthetic_sample(text: str, min_length: int = 100) -> bool:
    """Validate a synthetic sample."""
    if not text:
        return False

    text = text.strip()

    # Check minimum length
    if len(text) < min_length:
        return False

    # Check for common failure patterns
    failure_patterns = [
        r"^(記事本文|Article body|Here is|以下は)",
        r"(申し訳|I cannot|I can't|I'm sorry)",
        r"^(Title|タイトル|Headline|見出し):",
    ]

    for pattern in failure_patterns:
        if re.search(pattern, text, re.IGNORECASE):
            return False

    return True


def main():
    parser = argparse.ArgumentParser(
        description="LLM-based synthetic data generation for genre classification"
    )
    parser.add_argument(
        "--genres",
        type=str,
        required=True,
        help="Comma-separated list of genre IDs to generate samples for",
    )
    parser.add_argument(
        "--samples-per-genre",
        type=int,
        default=100,
        help="Number of samples to generate per genre",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("data/augmented_synthetic.csv"),
        help="Output CSV file",
    )
    parser.add_argument(
        "--golden-path",
        type=Path,
        default=Path("data/golden_classification.json"),
        help="Path to golden_classification.json",
    )
    parser.add_argument(
        "--ollama-url",
        type=str,
        default="http://localhost:11434",
        help="Ollama API URL",
    )
    parser.add_argument(
        "--model",
        type=str,
        default="gemma3:4b",
        help="Ollama model to use",
    )
    parser.add_argument(
        "--ja-ratio",
        type=float,
        default=0.6,
        help="Ratio of Japanese samples (0.0 - 1.0)",
    )
    parser.add_argument(
        "--rate-limit",
        type=float,
        default=2.0,
        help="Delay between API calls in seconds",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=42,
        help="Random seed",
    )

    args = parser.parse_args()
    random.seed(args.seed)

    # Load genre definitions
    print(f"Loading genre definitions from {args.golden_path}...")
    genre_definitions = load_genre_definitions(args.golden_path)
    print(f"Loaded {len(genre_definitions)} genre definitions")

    # Parse target genres
    target_genres = [g.strip() for g in args.genres.split(",")]

    # Validate genres exist
    for genre in target_genres:
        if genre not in genre_definitions:
            print(f"WARNING: Genre '{genre}' not found in golden_classification.json")
            target_genres.remove(genre)

    if not target_genres:
        print("ERROR: No valid genres specified")
        return

    print(f"Target genres: {target_genres}")
    print(f"Samples per genre: {args.samples_per_genre}")
    print(f"Japanese ratio: {args.ja_ratio}")

    # Initialize generator
    print(f"\nInitializing Ollama generator (model: {args.model})...")
    generator = OllamaSyntheticGenerator(
        ollama_url=args.ollama_url,
        model=args.model,
        rate_limit_delay=args.rate_limit,
    )

    # Test connection
    print("Testing Ollama connection...")
    test_result = generator._call_ollama("Say 'OK' if you can read this.")
    if test_result:
        print(f"Connection OK. Test result: {test_result[:50]}...")
    else:
        print("WARNING: Ollama connection test failed. Check if Ollama is running.")
        generator.close()
        return

    # Generate samples
    all_samples = []

    for genre in target_genres:
        genre_info = genre_definitions[genre]
        print(f"\nGenerating samples for {genre} ({genre_info['name_en']})...")

        ja_samples_target = int(args.samples_per_genre * args.ja_ratio)
        en_samples_target = args.samples_per_genre - ja_samples_target

        generated_ja = 0
        generated_en = 0
        failed = 0
        max_attempts = args.samples_per_genre * 3  # Allow some failures

        for attempt in range(max_attempts):
            if generated_ja >= ja_samples_target and generated_en >= en_samples_target:
                break

            # Decide language for this attempt
            if generated_ja < ja_samples_target and (
                generated_en >= en_samples_target or random.random() < args.ja_ratio
            ):
                # Generate Japanese
                hint = random.choice(VARIATION_HINTS_JA)
                text = generator.generate_japanese_sample(genre, genre_info, hint)
                lang = "ja"
                target_count = ja_samples_target
                current_count = generated_ja
            else:
                # Generate English
                hint = random.choice(VARIATION_HINTS_EN)
                text = generator.generate_english_sample(genre, genre_info, hint)
                lang = "en"
                target_count = en_samples_target
                current_count = generated_en

            if current_count >= target_count:
                continue

            if validate_synthetic_sample(text):
                all_samples.append({
                    "content": text,
                    "genre": genre,
                    "augmentation_method": "llm_synthetic",
                    "source_language": lang,
                    "source_genre": genre,
                })

                if lang == "ja":
                    generated_ja += 1
                else:
                    generated_en += 1

                total = generated_ja + generated_en
                if total % 10 == 0:
                    print(f"  Progress: {total}/{args.samples_per_genre}")
            else:
                failed += 1

        print(f"  Generated: JA={generated_ja}, EN={generated_en}, Failed={failed}")

    generator.close()

    # Save results
    if all_samples:
        df = pd.DataFrame(all_samples)
        print(f"\nTotal synthetic samples: {len(df)}")

        df.to_csv(args.output, index=False)
        print(f"Saved to {args.output}")

        # Print summary
        print("\nSynthetic samples by genre:")
        for genre, count in df["genre"].value_counts().items():
            print(f"  {genre}: {count}")

        print("\nSynthetic samples by language:")
        for lang, count in df["source_language"].value_counts().items():
            print(f"  {lang}: {count}")
    else:
        print("\nNo samples were generated.")
        pd.DataFrame(
            columns=["content", "genre", "augmentation_method", "source_language", "source_genre"]
        ).to_csv(args.output, index=False)


if __name__ == "__main__":
    main()
