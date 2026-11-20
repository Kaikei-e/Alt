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
import sys
import time
from pathlib import Path

import httpx
import pandas as pd

PROGRESS_LOG_INTERVAL = 10


def load_genre_definitions(golden_path: Path) -> dict:
    """Load genre definitions from golden_classification.json."""
    with golden_path.open() as f:
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
    ) -> None:
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
            if not isinstance(result, dict):
                sys.stdout.write(
                    f"Ollama API error: expected JSON object, got {type(result).__name__}\n"
                )
                return None
            return result.get("response", "").strip()
        except (httpx.HTTPError, ValueError) as e:
            sys.stdout.write(f"Ollama API error: {e}\n")
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

ジャンル: {genre_info["name_ja"]}
定義: {genre_info["definition_ja"]}
除外条件: {genre_info["exclusions_ja"]}

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

Genre: {genre_info["name_en"]}
Definition: {genre_info["definition_en"]}
Exclusions: {genre_info["exclusions_en"]}

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

    def close(self) -> None:
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

    return all(not re.search(pattern, text, re.IGNORECASE) for pattern in failure_patterns)


def _parse_args() -> argparse.Namespace:
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
    return parser.parse_args()


def _resolve_target_genres(raw_genres: str, genre_definitions: dict) -> list[str]:
    target_genres = [g.strip() for g in raw_genres.split(",")]
    valid_genres = []
    for genre in target_genres:
        if genre not in genre_definitions:
            sys.stdout.write(f"WARNING: Genre '{genre}' not found in golden_classification.json\n")
        else:
            valid_genres.append(genre)
    return valid_genres


def _generate_genre_samples(
    genre: str,
    genre_info: dict,
    generator: OllamaSyntheticGenerator,
    samples_per_genre: int,
    ja_ratio: float,
) -> list[dict[str, str]]:
    sys.stdout.write(f"\nGenerating samples for {genre} ({genre_info['name_en']})...\n")

    ja_samples_target = int(samples_per_genre * ja_ratio)
    en_samples_target = samples_per_genre - ja_samples_target
    generated_ja = 0
    generated_en = 0
    failed = 0
    max_attempts = samples_per_genre * 3
    samples: list[dict[str, str]] = []

    for _attempt in range(max_attempts):
        if generated_ja >= ja_samples_target and generated_en >= en_samples_target:
            break

        choose_ja = generated_ja < ja_samples_target and (
            generated_en >= en_samples_target or random.random() < ja_ratio  # noqa: S311 - non-cryptographic synthetic sample generation
        )
        if choose_ja:
            hint = random.choice(VARIATION_HINTS_JA)  # noqa: S311 - non-cryptographic synthetic sample generation
            text = generator.generate_japanese_sample(genre, genre_info, hint)
            lang = "ja"
            target_count = ja_samples_target
            current_count = generated_ja
        else:
            hint = random.choice(VARIATION_HINTS_EN)  # noqa: S311 - non-cryptographic synthetic sample generation
            text = generator.generate_english_sample(genre, genre_info, hint)
            lang = "en"
            target_count = en_samples_target
            current_count = generated_en

        if current_count >= target_count:
            continue

        if text is not None and validate_synthetic_sample(text):
            samples.append(
                {
                    "content": text,
                    "genre": genre,
                    "augmentation_method": "llm_synthetic",
                    "source_language": lang,
                    "source_genre": genre,
                }
            )
            if lang == "ja":
                generated_ja += 1
            else:
                generated_en += 1

            total = generated_ja + generated_en
            if total % PROGRESS_LOG_INTERVAL == 0:
                sys.stdout.write(f"  Progress: {total}/{samples_per_genre}\n")
        else:
            failed += 1

    sys.stdout.write(f"  Generated: JA={generated_ja}, EN={generated_en}, Failed={failed}\n")
    return samples


def _write_output(all_samples: list[dict[str, str]], output_path: Path) -> None:
    if all_samples:
        df = pd.DataFrame(all_samples)
        sys.stdout.write(f"\nTotal synthetic samples: {len(df)}\n")

        df.to_csv(output_path, index=False)
        sys.stdout.write(f"Saved to {output_path}\n")

        sys.stdout.write("\nSynthetic samples by genre:\n")
        for genre, count in df["genre"].value_counts().items():
            sys.stdout.write(f"  {genre}: {count}\n")

        sys.stdout.write("\nSynthetic samples by language:\n")
        for lang, count in df["source_language"].value_counts().items():
            sys.stdout.write(f"  {lang}: {count}\n")
    else:
        sys.stdout.write("\nNo samples were generated.\n")
        pd.DataFrame(
            columns=["content", "genre", "augmentation_method", "source_language", "source_genre"]
        ).to_csv(output_path, index=False)


def main() -> None:
    args = _parse_args()
    random.seed(args.seed)

    sys.stdout.write(f"Loading genre definitions from {args.golden_path}...\n")
    genre_definitions = load_genre_definitions(args.golden_path)
    sys.stdout.write(f"Loaded {len(genre_definitions)} genre definitions\n")

    target_genres = _resolve_target_genres(args.genres, genre_definitions)
    if not target_genres:
        sys.stdout.write("ERROR: No valid genres specified\n")
        return

    sys.stdout.write(f"Target genres: {target_genres}\n")
    sys.stdout.write(f"Samples per genre: {args.samples_per_genre}\n")
    sys.stdout.write(f"Japanese ratio: {args.ja_ratio}\n")

    sys.stdout.write(f"\nInitializing Ollama generator (model: {args.model})...\n")
    generator = OllamaSyntheticGenerator(
        ollama_url=args.ollama_url,
        model=args.model,
        rate_limit_delay=args.rate_limit,
    )

    sys.stdout.write("Testing Ollama connection...\n")
    test_result = generator._call_ollama("Say 'OK' if you can read this.")
    if test_result:
        sys.stdout.write(f"Connection OK. Test result: {test_result[:50]}...\n")
    else:
        sys.stdout.write("WARNING: Ollama connection test failed. Check if Ollama is running.\n")
        generator.close()
        return

    all_samples: list[dict[str, str]] = []
    for genre in target_genres:
        all_samples.extend(
            _generate_genre_samples(
                genre,
                genre_definitions[genre],
                generator,
                args.samples_per_genre,
                args.ja_ratio,
            )
        )

    generator.close()
    _write_output(all_samples, args.output)


if __name__ == "__main__":
    main()
