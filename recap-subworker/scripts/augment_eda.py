"""
Easy Data Augmentation (EDA) for Genre Classification.

Implements EDA techniques with Japanese and English support:
- Synonym Replacement (WordNet for English, Sudachi for Japanese)
- Random Insertion
- Random Swap
- Random Deletion

Usage:
    uv run python scripts/augment_eda.py \
        --input data/training_data.csv \
        --output data/augmented_eda.csv \
        --min-samples 100 \
        --aug-factor 2
"""

import argparse
import random
import re
import sys
from dataclasses import dataclass
from pathlib import Path

import pandas as pd

JA_RATIO_THRESHOLD = 0.3
MIN_TOKEN_LEN_FOR_SR = 2
MIN_TOKEN_LEN_FOR_SWAP = 4
MIN_TOKEN_LEN_FOR_DEL = 3
MIN_EN_WORD_LEN = 3
MIN_AUG_TEXT_LEN = 10


class MissingSudachiError(ImportError):
    """Raised when sudachipy is unavailable for Japanese augmentation."""

    def __init__(self) -> None:
        super().__init__("sudachipy is required for Japanese augmentation")


class MissingWordNetError(ImportError):
    """Raised when nltk wordnet is unavailable for English augmentation."""

    def __init__(self) -> None:
        super().__init__("nltk with wordnet is required for English augmentation")


@dataclass(frozen=True)
class AugmentationParams:
    """Parameters controlling EDA augmentation operations."""

    alpha_sr: float = 0.1
    alpha_ri: float = 0.1
    alpha_rs: float = 0.1
    p_rd: float = 0.1
    num_aug: int = 4


# Japanese tokenizer
try:
    from sudachipy import Dictionary, SplitMode

    SUDACHI_AVAILABLE = True
except ImportError:
    SUDACHI_AVAILABLE = False

# English synonym replacement
try:
    import nltk
    from nltk.corpus import wordnet

    try:
        wordnet.synsets("test")
    except LookupError:
        nltk.download("wordnet", quiet=True)
        nltk.download("omw-1.4", quiet=True)
    WORDNET_AVAILABLE = True
except ImportError:
    WORDNET_AVAILABLE = False


def detect_language(text: str) -> str:
    """Detect if text is primarily Japanese or English."""
    # Count Japanese characters (hiragana, katakana, kanji)
    ja_pattern = re.compile(r"[\u3040-\u309F\u30A0-\u30FF\u4E00-\u9FFF]")
    ja_chars = len(ja_pattern.findall(text))
    total_chars = len(text.replace(" ", ""))

    if total_chars == 0:
        return "en"

    ja_ratio = ja_chars / total_chars
    return "ja" if ja_ratio > JA_RATIO_THRESHOLD else "en"


class JapaneseAugmenter:
    """EDA augmenter for Japanese text using Sudachi."""

    def __init__(self) -> None:
        if not SUDACHI_AVAILABLE:
            raise MissingSudachiError
        self.tokenizer = Dictionary().create()
        self.mode = SplitMode.C  # Longest unit for better synonym matching

    def tokenize(self, text: str) -> list[str]:
        """Tokenize Japanese text."""
        morphemes = self.tokenizer.tokenize(text, self.mode)
        return [m.surface() for m in morphemes]

    def get_synonyms(self, word: str) -> list[str]:
        """Get synonyms for a Japanese word using Sudachi's synonym groups."""
        morphemes = self.tokenizer.tokenize(word, self.mode)
        if not morphemes:
            return []

        synonyms = []
        for m in morphemes:
            # Use normalized form and reading for synonym finding
            normalized = m.normalized_form()
            if normalized != word and len(normalized) > 1:
                synonyms.append(normalized)

            # Use synonym group IDs if available
            synonym_ids = m.synonym_group_ids()
            if synonym_ids:
                # In real implementation, you'd look up synonyms by group ID
                # For now, we use normalized form as a simple replacement
                pass

        return list(set(synonyms))

    def synonym_replacement(self, text: str, n: int = 1) -> str:
        """Replace n random words with synonyms."""
        tokens = self.tokenize(text)
        if len(tokens) < MIN_TOKEN_LEN_FOR_SR:
            return text

        # Get content words (skip particles, punctuation)
        content_indices = [
            i for i, t in enumerate(tokens) if len(t) > 1 and not re.match(r"^[、。！？\s]+$", t)
        ]

        if not content_indices:
            return text

        random.shuffle(content_indices)
        replacements = 0

        for idx in content_indices[: n * 2]:  # Try more indices in case no synonyms
            if replacements >= n:
                break
            word = tokens[idx]
            synonyms = self.get_synonyms(word)
            if synonyms:
                tokens[idx] = random.choice(synonyms)  # noqa: S311 - non-cryptographic data augmentation sampling
                replacements += 1

        return "".join(tokens)

    def random_insertion(self, text: str, n: int = 1) -> str:
        """Insert n random synonyms at random positions."""
        tokens = self.tokenize(text)
        if len(tokens) < MIN_TOKEN_LEN_FOR_SR:
            return text

        for _ in range(n):
            # Pick a random content word and find its synonym
            content_tokens = [t for t in tokens if len(t) > 1]
            if not content_tokens:
                break

            random_word = random.choice(content_tokens)  # noqa: S311 - non-cryptographic data augmentation sampling
            synonyms = self.get_synonyms(random_word)

            if synonyms:
                insert_pos = random.randint(0, len(tokens))  # noqa: S311 - non-cryptographic data augmentation sampling
                tokens.insert(insert_pos, random.choice(synonyms))  # noqa: S311 - non-cryptographic data augmentation sampling

        return "".join(tokens)

    def random_swap(self, text: str, n: int = 1) -> str:
        """Swap n pairs of words randomly."""
        tokens = self.tokenize(text)
        if len(tokens) < MIN_TOKEN_LEN_FOR_SWAP:
            return text

        for _ in range(n):
            idx1, idx2 = random.sample(range(len(tokens)), 2)
            tokens[idx1], tokens[idx2] = tokens[idx2], tokens[idx1]

        return "".join(tokens)

    def random_deletion(self, text: str, p: float = 0.1) -> str:
        """Delete words with probability p."""
        tokens = self.tokenize(text)
        if len(tokens) < MIN_TOKEN_LEN_FOR_DEL:
            return text

        # Keep at least half the tokens
        min_keep = max(2, len(tokens) // 2)
        new_tokens = [t for t in tokens if random.random() > p]  # noqa: S311 - non-cryptographic data augmentation sampling

        if len(new_tokens) < min_keep:
            new_tokens = random.sample(tokens, min_keep)

        return "".join(new_tokens)


class EnglishAugmenter:
    """EDA augmenter for English text using WordNet."""

    def __init__(self) -> None:
        if not WORDNET_AVAILABLE:
            raise MissingWordNetError

    def tokenize(self, text: str) -> list[str]:
        """Simple whitespace tokenization."""
        return text.split()

    def get_synonyms(self, word: str) -> list[str]:
        """Get synonyms from WordNet."""
        synonyms = set()
        for syn in wordnet.synsets(word.lower()):
            if syn is None:
                continue
            lemmas = syn.lemmas()
            if lemmas is None:
                continue
            for lemma in lemmas:
                synonym = lemma.name().replace("_", " ")
                if synonym.lower() != word.lower():
                    synonyms.add(synonym)
        return list(synonyms)

    def synonym_replacement(self, text: str, n: int = 1) -> str:
        """Replace n random words with synonyms."""
        tokens = self.tokenize(text)
        if len(tokens) < MIN_TOKEN_LEN_FOR_SR:
            return text

        # Get content word indices (skip short words, punctuation)
        content_indices = [
            i for i, t in enumerate(tokens) if len(t) > MIN_EN_WORD_LEN and t.isalpha()
        ]

        if not content_indices:
            return text

        random.shuffle(content_indices)
        replacements = 0

        for idx in content_indices[: n * 2]:
            if replacements >= n:
                break
            word = tokens[idx]
            synonyms = self.get_synonyms(word)
            if synonyms:
                # Preserve case
                replacement = random.choice(synonyms)  # noqa: S311 - non-cryptographic data augmentation sampling
                if word[0].isupper():
                    replacement = replacement.capitalize()
                tokens[idx] = replacement
                replacements += 1

        return " ".join(tokens)

    def random_insertion(self, text: str, n: int = 1) -> str:
        """Insert n random synonyms at random positions."""
        tokens = self.tokenize(text)
        if len(tokens) < MIN_TOKEN_LEN_FOR_SR:
            return text

        for _ in range(n):
            content_tokens = [t for t in tokens if len(t) > MIN_EN_WORD_LEN and t.isalpha()]
            if not content_tokens:
                break

            random_word = random.choice(content_tokens)  # noqa: S311 - non-cryptographic data augmentation sampling
            synonyms = self.get_synonyms(random_word)

            if synonyms:
                insert_pos = random.randint(0, len(tokens))  # noqa: S311 - non-cryptographic data augmentation sampling
                tokens.insert(insert_pos, random.choice(synonyms))  # noqa: S311 - non-cryptographic data augmentation sampling

        return " ".join(tokens)

    def random_swap(self, text: str, n: int = 1) -> str:
        """Swap n pairs of words randomly."""
        tokens = self.tokenize(text)
        if len(tokens) < MIN_TOKEN_LEN_FOR_SWAP:
            return text

        for _ in range(n):
            idx1, idx2 = random.sample(range(len(tokens)), 2)
            tokens[idx1], tokens[idx2] = tokens[idx2], tokens[idx1]

        return " ".join(tokens)

    def random_deletion(self, text: str, p: float = 0.1) -> str:
        """Delete words with probability p."""
        tokens = self.tokenize(text)
        if len(tokens) < MIN_TOKEN_LEN_FOR_DEL:
            return text

        min_keep = max(2, len(tokens) // 2)
        new_tokens = [t for t in tokens if random.random() > p]  # noqa: S311 - non-cryptographic data augmentation sampling

        if len(new_tokens) < min_keep:
            new_tokens = random.sample(tokens, min_keep)

        return " ".join(new_tokens)


def augment_text(
    text: str,
    augmenter: JapaneseAugmenter | EnglishAugmenter,
    params: AugmentationParams | None = None,
) -> list[str]:
    """
    Apply EDA augmentations to generate multiple variants.

    Args:
        text: Original text
        augmenter: Language-specific augmenter
        params: Augmentation hyperparameters

    Returns:
        List of augmented texts
    """
    cfg = params or AugmentationParams()
    tokens = augmenter.tokenize(text)
    n_words = len(tokens)

    # Calculate number of operations based on text length
    n_sr = max(1, int(cfg.alpha_sr * n_words))
    n_ri = max(1, int(cfg.alpha_ri * n_words))
    n_rs = max(1, int(cfg.alpha_rs * n_words))

    augmented: set[str] = set()

    # Generate augmented samples
    for _ in range(cfg.num_aug * 3):  # Try more times to get unique samples
        if len(augmented) >= cfg.num_aug:
            break

        # Randomly choose augmentation type
        aug_type = random.choice(["sr", "ri", "rs", "rd"])  # noqa: S311 - non-cryptographic data augmentation sampling

        if aug_type == "sr":
            aug_text = augmenter.synonym_replacement(text, n_sr)
        elif aug_type == "ri":
            aug_text = augmenter.random_insertion(text, n_ri)
        elif aug_type == "rs":
            aug_text = augmenter.random_swap(text, n_rs)
        else:  # rd
            aug_text = augmenter.random_deletion(text, cfg.p_rd)

        if aug_text != text and len(aug_text.strip()) > MIN_AUG_TEXT_LEN:
            augmented.add(aug_text)

    return list(augmented)[: cfg.num_aug]


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="EDA (Easy Data Augmentation) for genre classification"
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
        default=Path("data/augmented_eda.csv"),
        help="Output CSV file",
    )
    parser.add_argument(
        "--min-samples",
        type=int,
        default=100,
        help="Target minimum samples per genre (will augment genres below this)",
    )
    parser.add_argument(
        "--aug-factor",
        type=int,
        default=2,
        help="Augmentation factor for minority classes",
    )
    parser.add_argument(
        "--target-genres",
        type=str,
        default=None,
        help="Comma-separated list of genres to augment (default: auto-detect minority)",
    )
    parser.add_argument(
        "--alpha",
        type=float,
        default=0.1,
        help="EDA alpha parameter (percentage of words to modify)",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=42,
        help="Random seed for reproducibility",
    )
    return parser.parse_args()


def _init_augmenters() -> tuple[JapaneseAugmenter | None, EnglishAugmenter | None]:
    ja_augmenter = None
    en_augmenter = None

    if SUDACHI_AVAILABLE:
        ja_augmenter = JapaneseAugmenter()
        sys.stdout.write("Japanese augmenter initialized (Sudachi)\n")
    else:
        sys.stdout.write("WARNING: Sudachi not available, Japanese texts will be skipped\n")

    if WORDNET_AVAILABLE:
        en_augmenter = EnglishAugmenter()
        sys.stdout.write("English augmenter initialized (WordNet)\n")
    else:
        sys.stdout.write("WARNING: WordNet not available, English texts will be skipped\n")

    return ja_augmenter, en_augmenter


def _augment_target_genres(
    df: pd.DataFrame,
    target_genres: list[str],
    ja_augmenter: JapaneseAugmenter | None,
    en_augmenter: EnglishAugmenter | None,
    args: argparse.Namespace,
) -> list[dict[str, str]]:
    augmented_rows: list[dict[str, str]] = []

    for genre in target_genres:
        genre_df = df[df["genre"] == genre]
        current_count = len(genre_df)

        if current_count == 0:
            sys.stdout.write(f"\nSkipping {genre}: no samples\n")
            continue

        target_count = max(args.min_samples, current_count * args.aug_factor)
        samples_needed = target_count - current_count
        sys.stdout.write(
            f"\nAugmenting {genre}: {current_count} -> {target_count} (need {samples_needed})\n"
        )

        aug_per_sample = max(1, samples_needed // current_count)
        params = AugmentationParams(
            alpha_sr=args.alpha,
            alpha_ri=args.alpha,
            alpha_rs=args.alpha,
            p_rd=args.alpha,
            num_aug=aug_per_sample,
        )
        generated = 0

        for _, row in genre_df.iterrows():
            if generated >= samples_needed:
                break

            text = row["content"]
            lang = detect_language(text)
            if lang == "ja" and ja_augmenter:
                augmenter: JapaneseAugmenter | EnglishAugmenter = ja_augmenter
            elif lang == "en" and en_augmenter:
                augmenter = en_augmenter
            else:
                continue

            for aug_text in augment_text(text, augmenter, params):
                if generated >= samples_needed:
                    break
                augmented_rows.append(
                    {
                        "content": aug_text,
                        "genre": genre,
                        "augmentation_method": "eda",
                        "source_genre": genre,
                    }
                )
                generated += 1

        sys.stdout.write(f"  Generated {generated} augmented samples\n")

    return augmented_rows


def _write_output(augmented_rows: list[dict[str, str]], output_path: Path) -> None:
    if augmented_rows:
        aug_df = pd.DataFrame(augmented_rows)
        sys.stdout.write(f"\nTotal augmented samples: {len(aug_df)}\n")
        aug_df.to_csv(output_path, index=False)
        sys.stdout.write(f"Saved to {output_path}\n")

        sys.stdout.write("\nAugmented genre distribution:\n")
        for genre, count in aug_df["genre"].value_counts().items():
            sys.stdout.write(f"  {genre}: {count}\n")
    else:
        sys.stdout.write("\nNo samples were augmented.\n")
        pd.DataFrame(columns=["content", "genre", "augmentation_method", "source_genre"]).to_csv(
            output_path, index=False
        )


def main() -> None:
    args = _parse_args()
    random.seed(args.seed)

    sys.stdout.write(f"Loading data from {args.input}...\n")
    df = pd.read_csv(args.input)
    df = df.dropna(subset=["content", "genre"])
    sys.stdout.write(f"Total samples: {len(df)}\n")

    genre_counts = df["genre"].value_counts()
    sys.stdout.write("\nGenre distribution:\n")
    for genre, count in genre_counts.items():
        status = "MINORITY" if count < args.min_samples else ""
        sys.stdout.write(f"  {genre}: {count} {status}\n")

    if args.target_genres:
        target_genres = [g.strip() for g in args.target_genres.split(",")]
    else:
        target_genres = genre_counts[genre_counts < args.min_samples].index.tolist()

    sys.stdout.write(f"\nTarget genres for augmentation: {target_genres}\n")

    ja_augmenter, en_augmenter = _init_augmenters()
    augmented_rows = _augment_target_genres(df, target_genres, ja_augmenter, en_augmenter, args)
    _write_output(augmented_rows, args.output)


if __name__ == "__main__":
    main()
