"""Deterministic quality scorers for recap summaries."""

import re
from collections import Counter
from typing import Dict, List, Set, Tuple

from news_creator.domain.models import RecapSummary


# Axes where lower is better (inverted for "improved" check)
LOWER_IS_BETTER_AXES = frozenset({"redundancy"})

# Regex for [n] reference markers in bullets
_REF_MARKER_RE = re.compile(r"\[(\d+)\]")

# Regex for numeric values (dates, percentages, currencies, plain numbers)
_NUMERIC_RE = re.compile(
    r"\d{4}[-/年]\d{1,2}[-/月]"  # dates
    r"|\d+[%％]"  # percentages
    r"|\d+[億万千]"  # Japanese large numbers
    r"|[\$€£¥]\s?\d"  # currency symbols
    r"|\d+(?:\.\d+)?(?:ドル|円|ユーロ|ポンド)"  # currency words
    r"|\d{2,}",  # plain numbers ≥ 2 digits
)

# Heuristic patterns for 4-element structure detection
_STRUCTURE_PATTERNS: List[Tuple[str, re.Pattern]] = [
    # who/what: proper nouns (katakana sequences, ASCII names, 社/氏)
    ("who_what", re.compile(r"[ァ-ヶー]{3,}|[A-Z][a-zA-Z]+|.{1,10}[社氏]")),
    # action: verb-like endings
    (
        "action",
        re.compile(
            r"(?:した|される|発表|買収|開始|導入|開発|提供|発売|実施|公開|統合|改善|向上|引き上げ)"
        ),
    ),
    # background: context markers
    (
        "background",
        re.compile(
            r"(?:背景|経緯|これまで|従来|過去|以前|に伴い|を受けて|に対して|一方で|として)"
        ),
    ),
    # impact/outlook: forward-looking or consequence markers
    (
        "impact",
        re.compile(
            r"(?:見込み|予定|目指す|狙う|今後|将来|影響|結果|効果|期待|可能性|展望|視野)"
        ),
    ),
]


class RecapQualityEvaluator:
    """Evaluate recap summary quality using deterministic heuristics."""

    def evaluate_source_grounding(self, summary: RecapSummary) -> float:
        """Score reference integrity: bullet [n] markers vs references list.

        Checks:
        - All [n] markers in bullets have matching reference entries
        - All reference entries are cited by at least one bullet
        - No dangling markers, no unused references

        Returns 0.0 (no grounding) to 1.0 (perfect alignment).
        """
        # Extract all [n] markers from bullets
        cited_ids: Set[int] = set()
        for bullet in summary.bullets:
            for m in _REF_MARKER_RE.finditer(bullet):
                cited_ids.add(int(m.group(1)))

        refs = summary.references or []
        ref_ids: Set[int] = {r.id for r in refs}

        # No markers and no refs → grounding absent
        if not cited_ids and not ref_ids:
            return 0.0

        # Markers present but no references list → broken
        if cited_ids and not ref_ids:
            return 0.0

        # Calculate alignment
        cited_ids & ref_ids
        dangling = cited_ids - ref_ids  # markers with no ref
        unused = ref_ids - cited_ids  # refs never cited

        total_items = len(cited_ids | ref_ids)
        if total_items == 0:
            return 0.0

        # Penalize both dangling markers and unused references
        errors = len(dangling) + len(unused)
        score = max(0.0, 1.0 - errors / total_items)
        return score

    def evaluate_redundancy(self, summary: RecapSummary) -> float:
        """Score inter-bullet redundancy via bigram overlap.

        Returns 0.0 (no redundancy, best) to 1.0 (all bullets identical, worst).
        """
        bullets = summary.bullets
        if len(bullets) <= 1:
            return 0.0

        # Extract bigrams for each bullet
        bullet_bigrams: List[Counter] = []
        for bullet in bullets:
            # Tokenize by character bigrams (effective for Japanese)
            chars = re.sub(r"\s+", "", bullet)  # remove whitespace
            bigrams = Counter(chars[i : i + 2] for i in range(len(chars) - 1))
            bullet_bigrams.append(bigrams)

        # Compute pairwise Jaccard similarity
        pair_count = 0
        total_sim = 0.0
        for i in range(len(bullet_bigrams)):
            for j in range(i + 1, len(bullet_bigrams)):
                a, b = bullet_bigrams[i], bullet_bigrams[j]
                intersection = sum((a & b).values())
                union = sum((a | b).values())
                if union > 0:
                    total_sim += intersection / union
                pair_count += 1

        return total_sim / pair_count if pair_count > 0 else 0.0

    def evaluate_readability(self, summary: RecapSummary) -> float:
        """Score readability: bullet length and proper sentence endings.

        Ideal bullet: 400-1200 characters, ends with Japanese period or verb.
        Returns 0.0 (unreadable) to 1.0 (all bullets ideal).
        """
        if not summary.bullets:
            return 0.0

        scores: List[float] = []
        for bullet in summary.bullets:
            bullet_len = len(bullet)
            # Length score: 0.0 outside range, 1.0 in sweet spot
            if 400 <= bullet_len <= 1200:
                length_score = 1.0
            elif 200 <= bullet_len < 400:
                length_score = (bullet_len - 200) / 200  # linear ramp
            elif 1200 < bullet_len <= 1600:
                length_score = (1600 - bullet_len) / 400  # linear decay
            else:
                length_score = 0.0

            # Sentence ending score
            stripped = bullet.rstrip()
            # Remove trailing reference markers like [1]
            stripped = re.sub(r"\s*\[\d+\]\s*$", "", stripped).rstrip()
            good_endings = ("。", "た", "る", "い", "だ", "す", "ない", "ある")
            ending_score = 1.0 if stripped.endswith(good_endings) else 0.3

            scores.append(length_score * 0.7 + ending_score * 0.3)

        return sum(scores) / len(scores)

    def evaluate_structure(self, summary: RecapSummary) -> float:
        """Score 4-element structure presence per bullet.

        Elements: who/what, action, background, impact/outlook.
        Returns 0.0 (no structure) to 1.0 (all 4 elements in every bullet).
        """
        if not summary.bullets:
            return 0.0

        scores: List[float] = []
        for bullet in summary.bullets:
            elements_found = 0
            for _name, pattern in _STRUCTURE_PATTERNS:
                if pattern.search(bullet):
                    elements_found += 1
            scores.append(elements_found / len(_STRUCTURE_PATTERNS))

        return sum(scores) / len(scores)

    def evaluate_entity_density(self, summary: RecapSummary) -> float:
        """Score density of named entities and numeric values per bullet.

        Entities: katakana sequences (≥3 chars), ASCII proper nouns, numbers.
        Returns 0.0 (no entities) to 1.0 (rich in entities).
        """
        if not summary.bullets:
            return 0.0

        scores: List[float] = []
        for bullet in summary.bullets:
            entity_count = 0

            # Katakana sequences (company/service names)
            entity_count += len(re.findall(r"[ァ-ヶー]{3,}", bullet))

            # ASCII proper nouns (capitalized words)
            entity_count += len(re.findall(r"[A-Z][a-zA-Z]{2,}", bullet))

            # Numeric values
            entity_count += len(_NUMERIC_RE.findall(bullet))

            # Normalize: 0 entities → 0.0, ≥5 entities → 1.0
            scores.append(min(1.0, entity_count / 5))

        return sum(scores) / len(scores)

    def evaluate_all(self, summary: RecapSummary) -> Dict[str, float]:
        """Run all evaluators and return per-axis scores."""
        return {
            "source_grounding": self.evaluate_source_grounding(summary),
            "redundancy": self.evaluate_redundancy(summary),
            "readability": self.evaluate_readability(summary),
            "structure": self.evaluate_structure(summary),
            "entity_density": self.evaluate_entity_density(summary),
        }
