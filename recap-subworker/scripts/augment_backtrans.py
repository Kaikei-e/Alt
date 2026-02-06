"""
Backtranslation Data Augmentation for Genre Classification.

Uses Ollama/Gemma for backtranslation:
- Japanese -> English -> Japanese
- English -> Japanese -> English

Usage:
    uv run python scripts/augment_backtrans.py \
        --input data/training_data.csv \
        --output data/augmented_backtrans.csv \
        --ollama-url http://localhost:11434 \
        --model gemma3:4b \
        --target-genres health_medicine,games_esports,law_crime
"""

import argparse
import random
import re
import time
from pathlib import Path

import httpx
import pandas as pd


def detect_language(text: str) -> str:
    """Detect if text is primarily Japanese or English."""
    ja_pattern = re.compile(r"[\u3040-\u309F\u30A0-\u30FF\u4E00-\u9FFF]")
    ja_chars = len(ja_pattern.findall(text))
    total_chars = len(text.replace(" ", ""))

    if total_chars == 0:
        return "en"

    ja_ratio = ja_chars / total_chars
    return "ja" if ja_ratio > 0.3 else "en"


class OllamaBacktranslator:
    """Backtranslation using Ollama API."""

    def __init__(
        self,
        ollama_url: str = "http://localhost:11434",
        model: str = "gemma3:4b",
        timeout: float = 60.0,
        rate_limit_delay: float = 1.0,
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
                        "temperature": 0.7,
                        "top_p": 0.9,
                        "num_predict": 1024,
                    },
                },
            )
            response.raise_for_status()
            result = response.json()
            return result.get("response", "").strip()
        except Exception as e:
            print(f"Ollama API error: {e}")
            return None

    def translate_ja_to_en(self, text: str) -> str | None:
        """Translate Japanese to English."""
        prompt = f"""Translate the following Japanese text to English.
Output ONLY the English translation, nothing else.

Japanese text:
{text}

English translation:"""

        result = self._call_ollama(prompt)
        time.sleep(self.rate_limit_delay)
        return result

    def translate_en_to_ja(self, text: str) -> str | None:
        """Translate English to Japanese."""
        prompt = f"""Translate the following English text to Japanese.
Output ONLY the Japanese translation, nothing else.

English text:
{text}

Japanese translation:"""

        result = self._call_ollama(prompt)
        time.sleep(self.rate_limit_delay)
        return result

    def backtranslate(self, text: str, source_lang: str) -> str | None:
        """
        Perform backtranslation.

        Args:
            text: Original text
            source_lang: 'ja' or 'en'

        Returns:
            Backtranslated text or None if failed
        """
        if source_lang == "ja":
            # Japanese -> English -> Japanese
            intermediate = self.translate_ja_to_en(text)
            if not intermediate:
                return None
            return self.translate_en_to_ja(intermediate)
        else:
            # English -> Japanese -> English
            intermediate = self.translate_en_to_ja(text)
            if not intermediate:
                return None
            return self.translate_ja_to_en(intermediate)

    def close(self):
        """Close HTTP client."""
        self.client.close()


def validate_backtranslation(original: str, backtranslated: str) -> bool:
    """
    Basic validation of backtranslation quality.

    Checks:
    - Length ratio (0.5 - 2.0 of original)
    - Not identical to original
    - Minimum length
    """
    if not backtranslated or len(backtranslated.strip()) < 20:
        return False

    if backtranslated.strip() == original.strip():
        return False

    original_len = len(original)
    backtrans_len = len(backtranslated)

    if original_len == 0:
        return False

    length_ratio = backtrans_len / original_len

    # Accept if length is within 0.5x - 2.0x of original
    return 0.5 <= length_ratio <= 2.0


def main():
    parser = argparse.ArgumentParser(
        description="Backtranslation data augmentation for genre classification"
    )
    parser.add_argument(
        "--input",
        type=Path,
        default=Path("data/training_data.csv"),
        help="Input CSV file with 'content' and 'genre' columns",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("data/augmented_backtrans.csv"),
        help="Output CSV file",
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
        help="Ollama model to use for translation",
    )
    parser.add_argument(
        "--target-genres",
        type=str,
        required=True,
        help="Comma-separated list of genres to augment",
    )
    parser.add_argument(
        "--aug-factor",
        type=int,
        default=2,
        help="Augmentation factor (how many backtranslations per sample)",
    )
    parser.add_argument(
        "--max-samples",
        type=int,
        default=None,
        help="Maximum samples to augment per genre (for testing)",
    )
    parser.add_argument(
        "--rate-limit",
        type=float,
        default=1.0,
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

    # Load data
    print(f"Loading data from {args.input}...")
    df = pd.read_csv(args.input)
    df = df.dropna(subset=["content", "genre"])

    print(f"Total samples: {len(df)}")

    # Parse target genres
    target_genres = [g.strip() for g in args.target_genres.split(",")]
    print(f"Target genres: {target_genres}")

    # Print current counts
    print("\nCurrent genre distribution (target genres):")
    for genre in target_genres:
        count = len(df[df["genre"] == genre])
        print(f"  {genre}: {count}")

    # Initialize backtranslator
    print(f"\nInitializing Ollama backtranslator (model: {args.model})...")
    translator = OllamaBacktranslator(
        ollama_url=args.ollama_url,
        model=args.model,
        rate_limit_delay=args.rate_limit,
    )

    # Test connection
    print("Testing Ollama connection...")
    test_result = translator.translate_en_to_ja("Hello, this is a test.")
    if test_result:
        print(f"Connection OK. Test result: {test_result[:50]}...")
    else:
        print("WARNING: Ollama connection test failed. Check if Ollama is running.")
        translator.close()
        return

    # Augment target genres
    augmented_rows = []
    total_processed = 0
    total_failed = 0

    for genre in target_genres:
        genre_df = df[df["genre"] == genre]

        if len(genre_df) == 0:
            print(f"\nSkipping {genre}: no samples")
            continue

        print(f"\nProcessing {genre} ({len(genre_df)} samples)...")

        # Sample if max_samples specified
        if args.max_samples and len(genre_df) > args.max_samples:
            genre_df = genre_df.sample(n=args.max_samples, random_state=args.seed)
            print(f"  Sampled {args.max_samples} samples")

        processed = 0
        failed = 0

        for _, row in genre_df.iterrows():
            text = str(row["content"])
            lang = detect_language(text)

            # Skip very short texts
            if len(text) < 50:
                continue

            # Generate multiple backtranslations
            for _ in range(args.aug_factor):
                backtranslated = translator.backtranslate(text, lang)

                if backtranslated and validate_backtranslation(text, backtranslated):
                    augmented_rows.append({
                        "content": backtranslated,
                        "genre": genre,
                        "augmentation_method": "backtranslation",
                        "source_language": lang,
                        "source_genre": genre,
                    })
                    processed += 1
                else:
                    failed += 1

            total_processed += processed
            total_failed += failed

        print(f"  Generated: {processed}, Failed: {failed}")

    translator.close()

    # Save results
    if augmented_rows:
        aug_df = pd.DataFrame(augmented_rows)
        print(f"\nTotal augmented samples: {len(aug_df)}")
        print(f"Total failed: {total_failed}")

        aug_df.to_csv(args.output, index=False)
        print(f"Saved to {args.output}")

        # Print summary
        print("\nAugmented genre distribution:")
        for genre, count in aug_df["genre"].value_counts().items():
            print(f"  {genre}: {count}")
    else:
        print("\nNo samples were augmented.")
        pd.DataFrame(
            columns=["content", "genre", "augmentation_method", "source_language", "source_genre"]
        ).to_csv(args.output, index=False)


if __name__ == "__main__":
    main()
