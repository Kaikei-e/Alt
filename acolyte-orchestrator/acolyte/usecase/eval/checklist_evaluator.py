"""Checklist evaluator — rule-based quality checks, no LLM required.

Evaluates Task Fulfillment, Coverage, and Presentation using
deterministic heuristics. Part of multi-protocol eval framework.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field

from acolyte.domain.meta_patterns import META_PATTERNS

MIN_SECTION_LENGTH = 200

_MIN_SIGNIFICANT_WORD_LENGTH = 2
_TOPIC_WORD_MATCH_RATIO_THRESHOLD = 0.5
_MIN_SECTIONS_FOR_DUPLICATION_CHECK = 2
_SECTION_DUPLICATION_OVERLAP_THRESHOLD = 0.3
_MIN_BIGRAM_CHARS = 2


@dataclass(frozen=True)
class ChecklistItem:
    name: str
    passed: bool
    detail: str = ""


@dataclass
class ChecklistResult:
    items: list[ChecklistItem] = field(default_factory=list)

    @property
    def score(self) -> float:
        if not self.items:
            return 0.0
        return sum(1 for c in self.items if c.passed) / len(self.items)


class ChecklistEvaluator:
    """Rule-based report quality evaluator."""

    def check_task_fulfillment(
        self,
        scope: dict,
        outline: list[dict],
        sections: dict[str, str],
    ) -> list[ChecklistItem]:
        items: list[ChecklistItem] = []
        topic = scope.get("topic", "")

        # Check if topic keywords appear in generated content
        all_text = " ".join(sections.values()).lower()
        topic_words = [w.strip().lower() for w in topic.split() if len(w.strip()) > _MIN_SIGNIFICANT_WORD_LENGTH]
        if topic_words:
            matched = sum(1 for w in topic_words if w in all_text)
            ratio = matched / len(topic_words)
            items.append(
                ChecklistItem(
                    name="topic_in_content",
                    passed=ratio >= _TOPIC_WORD_MATCH_RATIO_THRESHOLD,
                    detail=f"{matched}/{len(topic_words)} topic words found",
                )
            )

        # Check all outline sections have corresponding generated sections
        for section in outline:
            key = section.get("key", "")
            items.append(
                ChecklistItem(
                    name=f"section_generated:{key}",
                    passed=key in sections and len(sections.get(key, "")) > 0,
                    detail=f"Section '{key}' exists" if key in sections else f"Section '{key}' missing",
                )
            )

        return items

    def check_coverage(
        self,
        outline: list[dict],
        sections: dict[str, str],
    ) -> list[ChecklistItem]:
        items: list[ChecklistItem] = []

        for section in outline:
            key = section.get("key", "")
            body = sections.get(key, "")

            items.append(
                ChecklistItem(
                    name=f"section_present:{key}",
                    passed=bool(body),
                    detail=f"length={len(body)}" if body else "missing",
                )
            )

            if body:
                items.append(
                    ChecklistItem(
                        name=f"section_length:{key}",
                        passed=len(body) >= MIN_SECTION_LENGTH,
                        detail=f"length={len(body)}, min={MIN_SECTION_LENGTH}",
                    )
                )

        return items

    def check_presentation(
        self,
        sections: dict[str, str],
    ) -> list[ChecklistItem]:
        items: list[ChecklistItem] = []
        all_text = "\n".join(sections.values())

        # Check for meta-statements
        found_meta = []
        for pattern in META_PATTERNS:
            if re.search(re.escape(pattern), all_text, re.IGNORECASE):
                found_meta.append(pattern)

        items.append(
            ChecklistItem(
                name="no_meta_statements",
                passed=len(found_meta) == 0,
                detail=f"Found: {found_meta}" if found_meta else "Clean",
            )
        )

        # Check for section duplication (bigram overlap between sections)
        section_bodies = list(sections.values())
        if len(section_bodies) >= _MIN_SECTIONS_FOR_DUPLICATION_CHECK:
            max_overlap = 0.0
            for i in range(len(section_bodies)):
                for j in range(i + 1, len(section_bodies)):
                    overlap = _bigram_overlap(section_bodies[i], section_bodies[j])
                    max_overlap = max(max_overlap, overlap)
            items.append(
                ChecklistItem(
                    name="low_section_duplication",
                    passed=max_overlap < _SECTION_DUPLICATION_OVERLAP_THRESHOLD,
                    detail=f"max_bigram_overlap={max_overlap:.2f}",
                )
            )

        return items

    def evaluate(
        self,
        scope: dict,
        outline: list[dict],
        sections: dict[str, str],
    ) -> ChecklistResult:
        items: list[ChecklistItem] = []
        items.extend(self.check_task_fulfillment(scope, outline, sections))
        items.extend(self.check_coverage(outline, sections))
        items.extend(self.check_presentation(sections))
        return ChecklistResult(items=items)


def _bigram_overlap(text_a: str, text_b: str) -> float:
    """Jaccard similarity of character bigrams between two texts."""
    if len(text_a) < _MIN_BIGRAM_CHARS or len(text_b) < _MIN_BIGRAM_CHARS:
        return 0.0
    bigrams_a = {text_a[i : i + 2] for i in range(len(text_a) - 1)}
    bigrams_b = {text_b[i : i + 2] for i in range(len(text_b) - 1)}
    intersection = bigrams_a & bigrams_b
    union = bigrams_a | bigrams_b
    return len(intersection) / len(union) if union else 0.0
