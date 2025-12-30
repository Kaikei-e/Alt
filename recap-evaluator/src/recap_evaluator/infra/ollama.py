"""Ollama client for G-Eval summarization evaluation."""

import json
import re
from dataclasses import dataclass, field
from typing import Any

import httpx
import structlog

from recap_evaluator.config import settings

logger = structlog.get_logger()


@dataclass
class GEvalResult:
    """G-Eval evaluation result for a single summary."""

    coherence: float
    consistency: float
    fluency: float
    relevance: float
    reasoning: str = ""
    raw_response: str = ""
    error: str | None = None

    @property
    def average_score(self) -> float:
        """Calculate average score across all dimensions."""
        return (self.coherence + self.consistency + self.fluency + self.relevance) / 4

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "coherence": self.coherence,
            "consistency": self.consistency,
            "fluency": self.fluency,
            "relevance": self.relevance,
            "average": self.average_score,
            "reasoning": self.reasoning,
            "error": self.error,
        }


@dataclass
class BatchGEvalResult:
    """Aggregated G-Eval results for multiple summaries."""

    results: list[GEvalResult] = field(default_factory=list)

    @property
    def count(self) -> int:
        """Number of evaluations."""
        return len(self.results)

    @property
    def success_count(self) -> int:
        """Number of successful evaluations."""
        return sum(1 for r in self.results if r.error is None)

    @property
    def avg_coherence(self) -> float:
        """Average coherence score."""
        valid = [r.coherence for r in self.results if r.error is None]
        return sum(valid) / len(valid) if valid else 0.0

    @property
    def avg_consistency(self) -> float:
        """Average consistency score."""
        valid = [r.consistency for r in self.results if r.error is None]
        return sum(valid) / len(valid) if valid else 0.0

    @property
    def avg_fluency(self) -> float:
        """Average fluency score."""
        valid = [r.fluency for r in self.results if r.error is None]
        return sum(valid) / len(valid) if valid else 0.0

    @property
    def avg_relevance(self) -> float:
        """Average relevance score."""
        valid = [r.relevance for r in self.results if r.error is None]
        return sum(valid) / len(valid) if valid else 0.0

    @property
    def avg_overall(self) -> float:
        """Average overall score."""
        valid = [r.average_score for r in self.results if r.error is None]
        return sum(valid) / len(valid) if valid else 0.0

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "count": self.count,
            "success_count": self.success_count,
            "avg_coherence": round(self.avg_coherence, 3),
            "avg_consistency": round(self.avg_consistency, 3),
            "avg_fluency": round(self.avg_fluency, 3),
            "avg_relevance": round(self.avg_relevance, 3),
            "avg_overall": round(self.avg_overall, 3),
        }


class OllamaClient:
    """Client for Ollama API to run G-Eval evaluations."""

    def __init__(
        self,
        base_url: str | None = None,
        model: str | None = None,
        timeout: int | None = None,
    ) -> None:
        self.base_url = base_url or settings.ollama_url
        self.model = model or settings.ollama_model
        self.timeout = timeout or settings.ollama_timeout

    def _build_geval_prompt(self, source_articles: str, summary: str) -> str:
        """Build the G-Eval prompt for summarization evaluation."""
        # Truncate source articles if too long (token limit consideration)
        max_source_chars = 4000
        if len(source_articles) > max_source_chars:
            source_articles = source_articles[:max_source_chars] + "..."

        return f"""以下の要約を4つの観点で1-5点で評価してください。
必ずJSON形式のみで回答してください。説明文は不要です。

フォーマット:
{{"coherence": X, "consistency": X, "fluency": X, "relevance": X, "reasoning": "評価理由を1-2文で"}}

評価基準:
1. Coherence (論理性, 1-5): 要約は論理的で構造化されているか？
2. Consistency (整合性, 1-5): 要約の事実は元記事に裏付けられているか？
3. Fluency (流暢さ, 1-5): 文法的に正しく読みやすいか？
4. Relevance (関連性, 1-5): 最も重要な情報をカバーしているか？

元記事:
{source_articles}

要約:
{summary}

JSON評価結果:"""

    def _parse_geval_response(self, response_text: str) -> GEvalResult:
        """Parse the G-Eval response from Ollama."""
        try:
            # Try to find JSON in the response
            json_match = re.search(r"\{[^{}]*\}", response_text, re.DOTALL)
            if not json_match:
                return GEvalResult(
                    coherence=0,
                    consistency=0,
                    fluency=0,
                    relevance=0,
                    raw_response=response_text,
                    error="No JSON found in response",
                )

            data = json.loads(json_match.group())

            return GEvalResult(
                coherence=float(data.get("coherence", 0)),
                consistency=float(data.get("consistency", 0)),
                fluency=float(data.get("fluency", 0)),
                relevance=float(data.get("relevance", 0)),
                reasoning=str(data.get("reasoning", "")),
                raw_response=response_text,
            )
        except (json.JSONDecodeError, ValueError, KeyError) as e:
            return GEvalResult(
                coherence=0,
                consistency=0,
                fluency=0,
                relevance=0,
                raw_response=response_text,
                error=f"Failed to parse response: {e}",
            )

    async def evaluate_summary(
        self,
        source_articles: str,
        summary: str,
    ) -> GEvalResult:
        """Evaluate a single summary using G-Eval."""
        prompt = self._build_geval_prompt(source_articles, summary)

        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                response = await client.post(
                    f"{self.base_url}/api/generate",
                    json={
                        "model": self.model,
                        "prompt": prompt,
                        "stream": False,
                        "options": {
                            "temperature": 0.1,  # Low temperature for consistency
                            "num_predict": 256,  # Limit response length
                        },
                    },
                )
                response.raise_for_status()

                result_data = response.json()
                response_text = result_data.get("response", "")

                return self._parse_geval_response(response_text)

        except httpx.HTTPStatusError as e:
            logger.error("Ollama HTTP error", status_code=e.response.status_code)
            return GEvalResult(
                coherence=0,
                consistency=0,
                fluency=0,
                relevance=0,
                error=f"HTTP error: {e.response.status_code}",
            )
        except httpx.TimeoutException:
            logger.error("Ollama request timeout")
            return GEvalResult(
                coherence=0,
                consistency=0,
                fluency=0,
                relevance=0,
                error="Request timeout",
            )
        except Exception as e:
            logger.error("Ollama request failed", error=str(e))
            return GEvalResult(
                coherence=0,
                consistency=0,
                fluency=0,
                relevance=0,
                error=str(e),
            )

    async def evaluate_batch(
        self,
        items: list[tuple[str, str]],  # List of (source_articles, summary) tuples
    ) -> BatchGEvalResult:
        """Evaluate multiple summaries."""
        batch_result = BatchGEvalResult()

        for source_articles, summary in items:
            result = await self.evaluate_summary(source_articles, summary)
            batch_result.results.append(result)

            if result.error:
                logger.warning("G-Eval evaluation failed", error=result.error)

        logger.info(
            "Batch G-Eval completed",
            total=batch_result.count,
            success=batch_result.success_count,
            avg_overall=batch_result.avg_overall,
        )

        return batch_result

    async def health_check(self) -> bool:
        """Check if Ollama is available and the model is loaded."""
        try:
            async with httpx.AsyncClient(timeout=10) as client:
                response = await client.get(f"{self.base_url}/api/tags")
                response.raise_for_status()

                data = response.json()
                models = [m.get("name", "") for m in data.get("models", [])]

                # Check if our model is available
                model_available = any(self.model in m for m in models)

                if not model_available:
                    logger.warning(
                        "Configured model not found",
                        model=self.model,
                        available_models=models,
                    )

                return True
        except Exception as e:
            logger.error("Ollama health check failed", error=str(e))
            return False


# Singleton instance
ollama_client = OllamaClient()
