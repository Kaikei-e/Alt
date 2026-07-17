"""Shared meta-statement patterns for critic and checklist evaluators.

Kept in domain so critic_node and checklist_evaluator cannot drift apart.
"""

from __future__ import annotations

META_PATTERNS: list[str] = [
    "情報が不足",
    "トピックが明示されて",
    "一般的な知識",
    "データを提供してください",
    "具体的な情報がありません",
    "I don't have",
    "As an AI",
    "I cannot provide",
    "As a language model",
]
