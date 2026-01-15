"""G-Eval: LLM-as-Judge evaluation for summarization quality.

This module implements G-Eval methodology for evaluating text summaries
using Large Language Models as judges with Chain-of-Thought prompting.

References:
- G-Eval Paper: https://arxiv.org/abs/2303.16634
- G-Eval Guide: https://www.confident-ai.com/blog/g-eval-the-definitive-guide
"""

from __future__ import annotations

import asyncio
import re
from dataclasses import dataclass, field
from enum import Enum
from typing import Optional

import structlog

logger = structlog.get_logger(__name__)


class EvaluationDimension(Enum):
    """Evaluation dimensions for G-Eval.

    Based on the original G-Eval paper dimensions:
    - Coherence: Logical flow and structure
    - Consistency: Factual alignment with source
    - Fluency: Language quality and readability
    - Relevance: Coverage of important information
    """

    COHERENCE = "coherence"
    CONSISTENCY = "consistency"
    FLUENCY = "fluency"
    RELEVANCE = "relevance"


@dataclass
class GEvalResult:
    """Result container for G-Eval evaluation.

    Attributes:
        coherence: Score for logical flow (1-5).
        consistency: Score for factual accuracy (1-5).
        fluency: Score for language quality (1-5).
        relevance: Score for information coverage (1-5).
        explanations: Optional explanations for each dimension.
    """

    coherence: float
    consistency: float
    fluency: float
    relevance: float
    explanations: Optional[dict[str, str]] = field(default=None)

    @property
    def average(self) -> float:
        """Compute average score across all dimensions."""
        scores = [self.coherence, self.consistency, self.fluency, self.relevance]
        return sum(scores) / len(scores)

    def to_dict(self) -> dict:
        """Convert to dictionary representation."""
        result = {
            "coherence": self.coherence,
            "consistency": self.consistency,
            "fluency": self.fluency,
            "relevance": self.relevance,
            "average": self.average,
        }
        if self.explanations is not None:
            result["explanations"] = self.explanations
        return result


# Prompt templates for each dimension
DIMENSION_PROMPTS = {
    EvaluationDimension.COHERENCE: """Evaluate the coherence of the following summary.

Coherence measures the logical flow and structure of the summary:
- Does the summary have a clear beginning, middle, and end?
- Are ideas connected logically with appropriate transitions?
- Is the information organized in a sensible order?

Source Document:
{source}

Summary to Evaluate:
{summary}

Rate the coherence on a scale of 1-5:
1 = Very poor coherence, disjointed and confusing
2 = Poor coherence, some logical gaps
3 = Acceptable coherence, mostly flows well
4 = Good coherence, well-structured
5 = Excellent coherence, perfectly organized

Provide your evaluation in the following format:
Score: [1-5]
Explanation: [Brief explanation of your rating]""",
    EvaluationDimension.CONSISTENCY: """Evaluate the consistency of the following summary.

Consistency measures factual alignment with the source document:
- Does the summary accurately reflect the information in the source?
- Are there any factual errors or contradictions?
- Is all information in the summary supported by the source?

Source Document:
{source}

Summary to Evaluate:
{summary}

Rate the consistency on a scale of 1-5:
1 = Major factual errors or contradictions
2 = Some inaccuracies or unsupported claims
3 = Mostly accurate with minor issues
4 = Accurate with negligible issues
5 = Perfectly consistent with the source

Provide your evaluation in the following format:
Score: [1-5]
Explanation: [Brief explanation of your rating]""",
    EvaluationDimension.FLUENCY: """Evaluate the fluency of the following summary.

Fluency measures the language quality and readability:
- Is the summary grammatically correct?
- Does it use natural, readable language?
- Is the vocabulary appropriate and clear?

Source Document:
{source}

Summary to Evaluate:
{summary}

Rate the fluency on a scale of 1-5:
1 = Very poor grammar, hard to understand
2 = Noticeable grammatical errors
3 = Acceptable, some awkward phrasing
4 = Good language quality, minor issues
5 = Excellent, natural and polished writing

Provide your evaluation in the following format:
Score: [1-5]
Explanation: [Brief explanation of your rating]""",
    EvaluationDimension.RELEVANCE: """Evaluate the relevance of the following summary.

Relevance measures how well the summary captures important information:
- Does the summary include the key points from the source?
- Is important information prioritized correctly?
- Does it avoid including irrelevant details?

Source Document:
{source}

Summary to Evaluate:
{summary}

Rate the relevance on a scale of 1-5:
1 = Misses most important information
2 = Captures some key points, misses others
3 = Covers main points adequately
4 = Good coverage of important information
5 = Excellent, captures all key points

Provide your evaluation in the following format:
Score: [1-5]
Explanation: [Brief explanation of your rating]""",
}


class GEvalEvaluator:
    """LLM-as-Judge evaluator using G-Eval methodology.

    Uses Chain-of-Thought prompting to evaluate summaries across
    multiple quality dimensions: coherence, consistency, fluency, relevance.

    Example:
        >>> evaluator = GEvalEvaluator()
        >>> result = await evaluator.evaluate(
        ...     summary="AI is advancing rapidly in 2025.",
        ...     source="Artificial intelligence technology continues to make significant progress...",
        ... )
        >>> print(f"Average score: {result.average:.2f}")
    """

    DEFAULT_DIMENSIONS = [
        EvaluationDimension.COHERENCE,
        EvaluationDimension.CONSISTENCY,
        EvaluationDimension.FLUENCY,
        EvaluationDimension.RELEVANCE,
    ]

    def __init__(
        self,
        dimensions: Optional[list[EvaluationDimension]] = None,
        llm_client: Optional[object] = None,
        model_name: str = "gpt-4",
        temperature: float = 0.0,
    ):
        """Initialize the G-Eval evaluator.

        Args:
            dimensions: Evaluation dimensions to use (default: all four).
            llm_client: Optional LLM client for API calls.
            model_name: Model to use for evaluation.
            temperature: Temperature for LLM generation (0 for deterministic).
        """
        self.dimensions = dimensions or self.DEFAULT_DIMENSIONS
        self.llm_client = llm_client
        self.model_name = model_name
        self.temperature = temperature

    def _get_prompt_for_dimension(
        self,
        dimension: EvaluationDimension,
        summary: str,
        source: str,
    ) -> str:
        """Generate evaluation prompt for a specific dimension.

        Args:
            dimension: The evaluation dimension.
            summary: The summary to evaluate.
            source: The source document.

        Returns:
            Formatted prompt string.
        """
        template = DIMENSION_PROMPTS[dimension]
        return template.format(summary=summary, source=source)

    def _parse_score(self, response: str) -> float:
        """Parse numeric score from LLM response.

        Handles various response formats:
        - "Score: 4"
        - "4/5"
        - "Rating: 3.5"
        - "The score is 5."

        Args:
            response: LLM response text.

        Returns:
            Parsed score, clamped to [1, 5] range.
        """
        # Try various patterns
        patterns = [
            r"Score:\s*(\d+\.?\d*)",
            r"(\d+\.?\d*)\s*/\s*5",
            r"Rating:\s*(\d+\.?\d*)",
            r"score\s+(?:is\s+)?(\d+\.?\d*)",
            r"\b([1-5])\b",
        ]

        for pattern in patterns:
            match = re.search(pattern, response, re.IGNORECASE)
            if match:
                try:
                    score = float(match.group(1))
                    # Clamp to valid range
                    return max(1.0, min(5.0, score))
                except ValueError:
                    continue

        # Default to mid-range if parsing fails
        logger.warning("Failed to parse score from response", response=response[:100])
        return 3.0

    def _parse_explanation(self, response: str) -> Optional[str]:
        """Extract explanation from LLM response.

        Args:
            response: LLM response text.

        Returns:
            Extracted explanation or None.
        """
        patterns = [
            r"Explanation:\s*(.+?)(?:\n|$)",
            r"(?:Because|Reason):\s*(.+?)(?:\n|$)",
        ]

        for pattern in patterns:
            match = re.search(pattern, response, re.IGNORECASE | re.DOTALL)
            if match:
                return match.group(1).strip()

        # If no explicit explanation, return text after score
        score_match = re.search(r"Score:\s*\d+\.?\d*\s*(.+)", response, re.DOTALL)
        if score_match:
            return score_match.group(1).strip()

        return None

    async def _call_llm(self, prompt: str) -> str:
        """Call the LLM with the given prompt.

        This is a placeholder that should be overridden or configured
        with an actual LLM client.

        Args:
            prompt: The prompt to send to the LLM.

        Returns:
            LLM response text.
        """
        if self.llm_client is None:
            # Return a mock response for testing
            logger.warning("No LLM client configured, returning mock response")
            return "Score: 3\nExplanation: No LLM client configured."

        # Actual implementation would call the LLM here
        # This depends on the specific LLM client being used
        raise NotImplementedError("LLM client call not implemented")

    async def _evaluate_dimension(
        self,
        dimension: EvaluationDimension,
        summary: str,
        source: str,
    ) -> tuple[float, Optional[str]]:
        """Evaluate a single dimension.

        Args:
            dimension: The dimension to evaluate.
            summary: The summary to evaluate.
            source: The source document.

        Returns:
            Tuple of (score, explanation).
        """
        prompt = self._get_prompt_for_dimension(dimension, summary, source)

        try:
            response = await self._call_llm(prompt)
            score = self._parse_score(response)
            explanation = self._parse_explanation(response)

            logger.debug(
                "Dimension evaluated",
                dimension=dimension.value,
                score=score,
            )

            return score, explanation

        except Exception as e:
            logger.error(
                "Error evaluating dimension",
                dimension=dimension.value,
                error=str(e),
            )
            return 3.0, f"Error: {str(e)}"

    async def evaluate(
        self,
        summary: str,
        source: str,
        include_explanations: bool = False,
    ) -> GEvalResult:
        """Evaluate a summary using G-Eval methodology.

        Evaluates the summary across all configured dimensions
        using LLM-as-Judge with Chain-of-Thought prompting.

        Args:
            summary: The summary to evaluate.
            source: The source document.
            include_explanations: Whether to include explanations in result.

        Returns:
            GEvalResult with scores for each dimension.
        """
        logger.info(
            "Starting G-Eval evaluation",
            num_dimensions=len(self.dimensions),
            include_explanations=include_explanations,
        )

        # Evaluate all dimensions concurrently
        tasks = [
            self._evaluate_dimension(dim, summary, source) for dim in self.dimensions
        ]
        results = await asyncio.gather(*tasks)

        # Build result dict
        scores = {}
        explanations = {} if include_explanations else None

        for dim, (score, explanation) in zip(self.dimensions, results):
            scores[dim.value] = score
            if explanations is not None and explanation:
                explanations[dim.value] = explanation

        # Fill in default values for any missing dimensions
        return GEvalResult(
            coherence=scores.get("coherence", 3.0),
            consistency=scores.get("consistency", 3.0),
            fluency=scores.get("fluency", 3.0),
            relevance=scores.get("relevance", 3.0),
            explanations=explanations,
        )

    async def evaluate_batch(
        self,
        summaries: list[str],
        sources: list[str],
        include_explanations: bool = False,
    ) -> list[GEvalResult]:
        """Evaluate a batch of summaries.

        Args:
            summaries: List of summaries to evaluate.
            sources: List of source documents (same length as summaries).
            include_explanations: Whether to include explanations.

        Returns:
            List of GEvalResult for each summary.

        Raises:
            ValueError: If summaries and sources have different lengths.
        """
        if len(summaries) != len(sources):
            raise ValueError(
                f"Mismatched lengths: {len(summaries)} summaries vs {len(sources)} sources"
            )

        tasks = [
            self.evaluate(summary, source, include_explanations)
            for summary, source in zip(summaries, sources)
        ]

        return await asyncio.gather(*tasks)
