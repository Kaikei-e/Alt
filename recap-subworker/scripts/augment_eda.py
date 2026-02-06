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
from pathlib import Path

import pandas as pd

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
    return "ja" if ja_ratio > 0.3 else "en"


class JapaneseAugmenter:
    """EDA augmenter for Japanese text using Sudachi."""

    def __init__(self):
        if not SUDACHI_AVAILABLE:
            raise ImportError("sudachipy is required for Japanese augmentation")
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
        if len(tokens) < 2:
            return text

        # Get content words (skip particles, punctuation)
        content_indices = [
            i for i, t in enumerate(tokens)
            if len(t) > 1 and not re.match(r"^[、。！？\s]+$", t)
        ]

        if not content_indices:
            return text

        random.shuffle(content_indices)
        replacements = 0

        for idx in content_indices[:n * 2]:  # Try more indices in case no synonyms
            if replacements >= n:
                break
            word = tokens[idx]
            synonyms = self.get_synonyms(word)
            if synonyms:
                tokens[idx] = random.choice(synonyms)
                replacements += 1

        return "".join(tokens)

    def random_insertion(self, text: str, n: int = 1) -> str:
        """Insert n random synonyms at random positions."""
        tokens = self.tokenize(text)
        if len(tokens) < 2:
            return text

        for _ in range(n):
            # Pick a random content word and find its synonym
            content_tokens = [t for t in tokens if len(t) > 1]
            if not content_tokens:
                break

            random_word = random.choice(content_tokens)
            synonyms = self.get_synonyms(random_word)

            if synonyms:
                insert_pos = random.randint(0, len(tokens))
                tokens.insert(insert_pos, random.choice(synonyms))

        return "".join(tokens)

    def random_swap(self, text: str, n: int = 1) -> str:
        """Swap n pairs of words randomly."""
        tokens = self.tokenize(text)
        if len(tokens) < 4:
            return text

        for _ in range(n):
            idx1, idx2 = random.sample(range(len(tokens)), 2)
            tokens[idx1], tokens[idx2] = tokens[idx2], tokens[idx1]

        return "".join(tokens)

    def random_deletion(self, text: str, p: float = 0.1) -> str:
        """Delete words with probability p."""
        tokens = self.tokenize(text)
        if len(tokens) < 3:
            return text

        # Keep at least half the tokens
        min_keep = max(2, len(tokens) // 2)
        new_tokens = [t for t in tokens if random.random() > p]

        if len(new_tokens) < min_keep:
            new_tokens = random.sample(tokens, min_keep)

        return "".join(new_tokens)


class EnglishAugmenter:
    """EDA augmenter for English text using WordNet."""

    def __init__(self):
        if not WORDNET_AVAILABLE:
            raise ImportError("nltk with wordnet is required for English augmentation")

    def tokenize(self, text: str) -> list[str]:
        """Simple whitespace tokenization."""
        return text.split()

    def get_synonyms(self, word: str) -> list[str]:
        """Get synonyms from WordNet."""
        synonyms = set()
        for syn in wordnet.synsets(word.lower()):
            for lemma in syn.lemmas():
                synonym = lemma.name().replace("_", " ")
                if synonym.lower() != word.lower():
                    synonyms.add(synonym)
        return list(synonyms)

    def synonym_replacement(self, text: str, n: int = 1) -> str:
        """Replace n random words with synonyms."""
        tokens = self.tokenize(text)
        if len(tokens) < 2:
            return text

        # Get content word indices (skip short words, punctuation)
        content_indices = [
            i for i, t in enumerate(tokens)
            if len(t) > 3 and t.isalpha()
        ]

        if not content_indices:
            return text

        random.shuffle(content_indices)
        replacements = 0

        for idx in content_indices[:n * 2]:
            if replacements >= n:
                break
            word = tokens[idx]
            synonyms = self.get_synonyms(word)
            if synonyms:
                # Preserve case
                replacement = random.choice(synonyms)
                if word[0].isupper():
                    replacement = replacement.capitalize()
                tokens[idx] = replacement
                replacements += 1

        return " ".join(tokens)

    def random_insertion(self, text: str, n: int = 1) -> str:
        """Insert n random synonyms at random positions."""
        tokens = self.tokenize(text)
        if len(tokens) < 2:
            return text

        for _ in range(n):
            content_tokens = [t for t in tokens if len(t) > 3 and t.isalpha()]
            if not content_tokens:
                break

            random_word = random.choice(content_tokens)
            synonyms = self.get_synonyms(random_word)

            if synonyms:
                insert_pos = random.randint(0, len(tokens))
                tokens.insert(insert_pos, random.choice(synonyms))

        return " ".join(tokens)

    def random_swap(self, text: str, n: int = 1) -> str:
        """Swap n pairs of words randomly."""
        tokens = self.tokenize(text)
        if len(tokens) < 4:
            return text

        for _ in range(n):
            idx1, idx2 = random.sample(range(len(tokens)), 2)
            tokens[idx1], tokens[idx2] = tokens[idx2], tokens[idx1]

        return " ".join(tokens)

    def random_deletion(self, text: str, p: float = 0.1) -> str:
        """Delete words with probability p."""
        tokens = self.tokenize(text)
        if len(tokens) < 3:
            return text

        min_keep = max(2, len(tokens) // 2)
        new_tokens = [t for t in tokens if random.random() > p]

        if len(new_tokens) < min_keep:
            new_tokens = random.sample(tokens, min_keep)

        return " ".join(new_tokens)


def augment_text(
    text: str,
    augmenter: JapaneseAugmenter | EnglishAugmenter,
    alpha_sr: float = 0.1,
    alpha_ri: float = 0.1,
    alpha_rs: float = 0.1,
    p_rd: float = 0.1,
    num_aug: int = 4,
) -> list[str]:
    """
    Apply EDA augmentations to generate multiple variants.

    Args:
        text: Original text
        augmenter: Language-specific augmenter
        alpha_sr: Percentage of words to replace with synonyms
        alpha_ri: Percentage of words to insert
        alpha_rs: Percentage of words to swap
        p_rd: Probability of word deletion
        num_aug: Number of augmented samples to generate

    Returns:
        List of augmented texts
    """
    tokens = augmenter.tokenize(text)
    n_words = len(tokens)

    # Calculate number of operations based on text length
    n_sr = max(1, int(alpha_sr * n_words))
    n_ri = max(1, int(alpha_ri * n_words))
    n_rs = max(1, int(alpha_rs * n_words))

    augmented = set()

    # Generate augmented samples
    for _ in range(num_aug * 3):  # Try more times to get unique samples
        if len(augmented) >= num_aug:
            break

        # Randomly choose augmentation type
        aug_type = random.choice(["sr", "ri", "rs", "rd"])

        if aug_type == "sr":
            aug_text = augmenter.synonym_replacement(text, n_sr)
        elif aug_type == "ri":
            aug_text = augmenter.random_insertion(text, n_ri)
        elif aug_type == "rs":
            aug_text = augmenter.random_swap(text, n_rs)
        else:  # rd
            aug_text = augmenter.random_deletion(text, p_rd)

        if aug_text != text and len(aug_text.strip()) > 10:
            augmented.add(aug_text)

    return list(augmented)[:num_aug]


def main():
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

    args = parser.parse_args()
    random.seed(args.seed)

    # Load data
    print(f"Loading data from {args.input}...")
    df = pd.read_csv(args.input)
    df = df.dropna(subset=["content", "genre"])

    print(f"Total samples: {len(df)}")

    # Identify minority genres
    genre_counts = df["genre"].value_counts()
    print("\nGenre distribution:")
    for genre, count in genre_counts.items():
        status = "MINORITY" if count < args.min_samples else ""
        print(f"  {genre}: {count} {status}")

    # Determine target genres
    if args.target_genres:
        target_genres = [g.strip() for g in args.target_genres.split(",")]
    else:
        target_genres = genre_counts[genre_counts < args.min_samples].index.tolist()

    print(f"\nTarget genres for augmentation: {target_genres}")

    # Initialize augmenters
    ja_augmenter = None
    en_augmenter = None

    if SUDACHI_AVAILABLE:
        ja_augmenter = JapaneseAugmenter()
        print("Japanese augmenter initialized (Sudachi)")
    else:
        print("WARNING: Sudachi not available, Japanese texts will be skipped")

    if WORDNET_AVAILABLE:
        en_augmenter = EnglishAugmenter()
        print("English augmenter initialized (WordNet)")
    else:
        print("WARNING: WordNet not available, English texts will be skipped")

    # Augment minority classes
    augmented_rows = []

    for genre in target_genres:
        genre_df = df[df["genre"] == genre]
        current_count = len(genre_df)

        if current_count == 0:
            print(f"\nSkipping {genre}: no samples")
            continue

        # Calculate how many samples to generate
        target_count = max(args.min_samples, current_count * args.aug_factor)
        samples_needed = target_count - current_count

        print(f"\nAugmenting {genre}: {current_count} -> {target_count} (need {samples_needed})")

        # Augment each sample
        aug_per_sample = max(1, samples_needed // current_count)
        generated = 0

        for _, row in genre_df.iterrows():
            if generated >= samples_needed:
                break

            text = row["content"]
            lang = detect_language(text)

            # Select appropriate augmenter
            if lang == "ja" and ja_augmenter:
                augmenter = ja_augmenter
            elif lang == "en" and en_augmenter:
                augmenter = en_augmenter
            else:
                continue

            # Generate augmented samples
            aug_texts = augment_text(
                text,
                augmenter,
                alpha_sr=args.alpha,
                alpha_ri=args.alpha,
                alpha_rs=args.alpha,
                p_rd=args.alpha,
                num_aug=aug_per_sample,
            )

            for aug_text in aug_texts:
                if generated >= samples_needed:
                    break
                augmented_rows.append({
                    "content": aug_text,
                    "genre": genre,
                    "augmentation_method": "eda",
                    "source_genre": genre,
                })
                generated += 1

        print(f"  Generated {generated} augmented samples")

    # Create augmented dataframe
    if augmented_rows:
        aug_df = pd.DataFrame(augmented_rows)
        print(f"\nTotal augmented samples: {len(aug_df)}")

        # Save augmented data
        aug_df.to_csv(args.output, index=False)
        print(f"Saved to {args.output}")

        # Print summary
        print("\nAugmented genre distribution:")
        for genre, count in aug_df["genre"].value_counts().items():
            print(f"  {genre}: {count}")
    else:
        print("\nNo samples were augmented.")
        # Create empty file with headers
        pd.DataFrame(columns=["content", "genre", "augmentation_method", "source_genre"]).to_csv(
            args.output, index=False
        )


if __name__ == "__main__":
    main()
