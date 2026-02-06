"""Ollama gateway — implements LLMPort for G-Eval evaluation."""

import asyncio
import json
import re
from dataclasses import dataclass, field

import httpx
import structlog

from recap_evaluator.config import Settings

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
        return (self.coherence + self.consistency + self.fluency + self.relevance) / 4


@dataclass
class BatchGEvalResult:
    """Aggregated G-Eval results for multiple summaries."""

    results: list[GEvalResult] = field(default_factory=list)

    @property
    def count(self) -> int:
        return len(self.results)

    @property
    def success_count(self) -> int:
        return sum(1 for r in self.results if r.error is None)

    @property
    def avg_coherence(self) -> float:
        valid = [r.coherence for r in self.results if r.error is None]
        return sum(valid) / len(valid) if valid else 0.0

    @property
    def avg_consistency(self) -> float:
        valid = [r.consistency for r in self.results if r.error is None]
        return sum(valid) / len(valid) if valid else 0.0

    @property
    def avg_fluency(self) -> float:
        valid = [r.fluency for r in self.results if r.error is None]
        return sum(valid) / len(valid) if valid else 0.0

    @property
    def avg_relevance(self) -> float:
        valid = [r.relevance for r in self.results if r.error is None]
        return sum(valid) / len(valid) if valid else 0.0

    @property
    def avg_overall(self) -> float:
        valid = [r.average_score for r in self.results if r.error is None]
        return sum(valid) / len(valid) if valid else 0.0


class OllamaGateway:
    """Ollama HTTP client with connection pooling and concurrency control."""

    def __init__(self, client: httpx.AsyncClient, settings: Settings) -> None:
        self._client = client
        self._base_url = settings.ollama_url
        self._model = settings.ollama_model
        self._semaphore = asyncio.Semaphore(settings.ollama_concurrency)

    def _build_geval_prompt(self, source_articles: str, summary: str) -> str:
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
        try:
            json_match = re.search(r"\{[^{}]*\}", response_text, re.DOTALL)
            if not json_match:
                return GEvalResult(
                    coherence=0, consistency=0, fluency=0, relevance=0,
                    raw_response=response_text, error="No JSON found in response",
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
                coherence=0, consistency=0, fluency=0, relevance=0,
                raw_response=response_text, error=f"Failed to parse response: {e}",
            )

    async def evaluate_summary(
        self, source_articles: str, summary: str
    ) -> GEvalResult:
        prompt = self._build_geval_prompt(source_articles, summary)
        try:
            async with self._semaphore:
                response = await self._client.post(
                    f"{self._base_url}/api/generate",
                    json={
                        "model": self._model,
                        "prompt": prompt,
                        "stream": False,
                        "options": {"temperature": 0.1, "num_predict": 256},
                    },
                )
                response.raise_for_status()
                result_data = response.json()
                return self._parse_geval_response(result_data.get("response", ""))
        except httpx.HTTPStatusError as e:
            logger.error("Ollama HTTP error", status_code=e.response.status_code)
            return GEvalResult(
                coherence=0, consistency=0, fluency=0, relevance=0,
                error=f"HTTP error: {e.response.status_code}",
            )
        except httpx.TimeoutException:
            logger.error("Ollama request timeout")
            return GEvalResult(
                coherence=0, consistency=0, fluency=0, relevance=0,
                error="Request timeout",
            )
        except Exception as e:
            logger.error("Ollama request failed", error=str(e))
            return GEvalResult(
                coherence=0, consistency=0, fluency=0, relevance=0,
                error=str(e),
            )

    async def evaluate_batch(
        self, items: list[tuple[str, str]]
    ) -> BatchGEvalResult:
        async def _eval_one(item: tuple[str, str]) -> GEvalResult:
            return await self.evaluate_summary(item[0], item[1])

        results = await asyncio.gather(
            *[_eval_one(item) for item in items], return_exceptions=True
        )

        batch = BatchGEvalResult()
        for r in results:
            if isinstance(r, Exception):
                logger.warning("G-Eval item failed", error=str(r))
                batch.results.append(
                    GEvalResult(
                        coherence=0, consistency=0, fluency=0, relevance=0,
                        error=str(r),
                    )
                )
            else:
                batch.results.append(r)

        logger.info(
            "Batch G-Eval completed",
            total=batch.count,
            success=batch.success_count,
            avg_overall=batch.avg_overall,
        )
        return batch

    async def health_check(self) -> bool:
        try:
            response = await self._client.get(f"{self._base_url}/api/tags")
            response.raise_for_status()
            data = response.json()
            models = [m.get("name", "") for m in data.get("models", [])]
            if not any(self._model in m for m in models):
                logger.warning(
                    "Configured model not found",
                    model=self._model,
                    available_models=models,
                )
            return True
        except Exception as e:
            logger.error("Ollama health check failed", error=str(e))
            return False
