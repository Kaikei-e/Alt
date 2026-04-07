"""Usecase for structured query planning for Augur Conversational RAG.

Uses Ollama structured outputs (JSON Schema via GBNF grammar) to produce a
QueryPlan from user query + conversation history.

Key design decisions based on research (Ollama structured output best practices):
- reasoning field BEFORE decision fields (+8pp accuracy, DSdev 2025)
- Few-shot examples included (near-zero accuracy without them, RANLP 2025)
- Schema described in prompt (model cannot see the format parameter)
- Temperature = 0 for maximum schema adherence
- num_predict = 512 (room for reasoning field)
- Options use config.get_llm_options() base to prevent model reload (PM-008)
"""

import json
import logging
import time
from datetime import datetime, timezone

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import (
    PlanQueryRequest,
    PlanQueryResponse,
    QueryPlan,
)
from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)

PLAN_QUERY_PROMPT = """You are a query planner for a news retrieval system. Produce a JSON retrieval plan.

Current Date: {current_date}

Output JSON with these fields IN ORDER:
1. reasoning: Your step-by-step thinking about the query
2. resolved_query: A self-contained search query (no pronouns, must contain the topic)
3. search_queries: 3-5 topically relevant search queries in Japanese AND English. NEVER output dates alone.
4. intent: One of causal_explanation, temporal, synthesis, comparison, fact_check, topic_deep_dive, general
5. retrieval_policy: global_only or article_only (default: global_only)
6. answer_format: causal_analysis, summary, list, detail, or comparison (default: summary)
7. should_clarify: ALMOST ALWAYS false. Only true for bare phrases like "もっと詳しく" with NO context.
8. topic_entities: Key entities from the query

Intent selection guide:
- "原因", "なぜ", "要因", "真因", "root cause", "why did", "caused by" → causal_explanation
- "最近", "今週", "動向", "latest", "recent" → temporal (UNLESS combined with causal keywords like 原因/なぜ, then use causal_explanation)
- "比較", "違い", "vs", "difference" → comparison
- "そもそも", "とは何", "全体像", "overview" → synthesis
- "詳しく", "deep dive", "技術的" → topic_deep_dive

<example>
Query: 最近のEV市場の動向は？
{{"reasoning": "User asks about recent EV market trends. Clear standalone query about electric vehicles. Temporal because of '最近の'. No causal keyword. No conversation history, no coreference.", "resolved_query": "最近のEV市場の動向と成長トレンド", "search_queries": ["EV市場 動向 2026", "electric vehicle market trends", "EV 販売台数 成長率", "電気自動車 市場規模"], "intent": "temporal", "retrieval_policy": "global_only", "answer_format": "summary", "should_clarify": false, "topic_entities": ["EV", "電気自動車"]}}
</example>

<example>
Query: 最近の石油危機の原因は？
{{"reasoning": "User asks about the CAUSE of the recent oil crisis. Both temporal ('最近') and causal ('原因') cues present. Per intent guide, causal keywords override temporal — this is causal_explanation, not temporal. The user wants to understand WHY the crisis happened, not just WHEN.", "resolved_query": "最近の石油危機が発生した原因と背景", "search_queries": ["石油危機 原因 2026", "oil crisis causes geopolitical", "原油価格 高騰 要因", "Iran oil supply disruption reasons"], "intent": "causal_explanation", "retrieval_policy": "global_only", "answer_format": "causal_analysis", "should_clarify": false, "topic_entities": ["石油危機", "原油", "イラン"]}}
</example>

<example>
Conversation:
user: AIチップの最新動向は？
assistant: NVIDIAのBlackwellが発表され推論性能が向上しました。

Query: それについてもっと詳しく教えて
{{"reasoning": "Follow-up query. 'それ' refers to NVIDIA Blackwell from the previous answer. Resolve coreference: the user wants more detail about NVIDIA Blackwell architecture. This is NOT ambiguous because conversation history provides clear referent.", "resolved_query": "NVIDIA Blackwellアーキテクチャの技術的詳細と推論性能", "search_queries": ["NVIDIA Blackwell architecture details", "Blackwell 推論性能 仕様", "NVIDIA B200 GPU specs"], "intent": "topic_deep_dive", "retrieval_policy": "global_only", "answer_format": "detail", "should_clarify": false, "topic_entities": ["NVIDIA", "Blackwell"]}}
</example>

{context_section}Query: {query}
"""


class PlanQueryUsecase:
    """Plans retrieval strategy for a user query using LLM structured output."""

    PLANNING_MODEL = "gemma4-e4b-12k"

    def __init__(self, config: NewsCreatorConfig, llm_provider: LLMProviderPort):
        self.config = config
        self.llm_provider = llm_provider

    async def plan_query(self, request: PlanQueryRequest) -> PlanQueryResponse:
        start = time.monotonic()

        try:
            plan = await self._plan_with_llm(request)
        except Exception as e:
            import traceback
            logger.warning("plan_query_llm_failed", extra={
                "error": str(e),
                "error_type": type(e).__name__,
                "traceback": traceback.format_exc(),
                "query": request.query,
            })
            plan = self._fallback_plan(request.query)

        elapsed_ms = (time.monotonic() - start) * 1000

        return PlanQueryResponse(
            plan=plan,
            original_query=request.query,
            model=self.PLANNING_MODEL,
            processing_time_ms=round(elapsed_ms, 2),
        )

    async def _plan_with_llm(self, request: PlanQueryRequest) -> QueryPlan:
        prompt = self._build_prompt(request)

        # Use /api/chat with thinking enabled for better intent classification.
        # Do NOT use the 'format' parameter — Ollama #10929: thinking + format = empty content.
        # The prompt + few-shot examples guide the model to produce valid JSON.
        # Thinking mode adds ~25s latency but dramatically improves intent accuracy.
        # The streaming layer sends progress events to keep the client connection alive.
        payload = {
            "model": self.PLANNING_MODEL,
            "messages": [
                {"role": "user", "content": prompt},
            ],
            "stream": False,
            "options": {
                "temperature": 0,
                "num_predict": 2048,
            },
        }

        response = await self.llm_provider.chat_generate(payload)
        content = response.get("message", {}).get("content", "")

        try:
            json_str = self._extract_json(content)
            parsed = json.loads(json_str)
            return QueryPlan(**parsed)
        except (json.JSONDecodeError, KeyError, Exception) as e:
            logger.warning("plan_query_parse_failed", extra={
                "error": str(e),
                "raw_content": content[:300] if content else "(empty)",
            })
            return self._fallback_plan(request.query)

    def _build_prompt(self, request: PlanQueryRequest) -> str:
        context_section = ""

        if request.conversation_history:
            lines = []
            for msg in request.conversation_history[-6:]:
                content = msg.content[:300] + "..." if len(msg.content) > 300 else msg.content
                lines.append(f"{msg.role}: {content}")
            context_section += "Conversation:\n" + "\n".join(lines) + "\n\n"

        if request.article_id and request.article_title:
            context_section += f"Article scope: {request.article_title} [id: {request.article_id}]\n\n"

        current_date = datetime.now(timezone.utc).strftime("%Y-%m-%d")

        return PLAN_QUERY_PROMPT.format(
            current_date=current_date,
            context_section=context_section,
            query=request.query,
        )

    @staticmethod
    def _extract_json(text: str) -> str:
        """Extract JSON object from LLM response that may contain markdown fences or preamble."""
        # Strip markdown code fences
        if "```json" in text:
            text = text.split("```json", 1)[1]
            if "```" in text:
                text = text.split("```", 1)[0]
        elif "```" in text:
            text = text.split("```", 1)[1]
            if "```" in text:
                text = text.split("```", 1)[0]
        # Find the first { and last }
        start = text.find("{")
        end = text.rfind("}")
        if start != -1 and end != -1 and end > start:
            return text[start:end + 1]
        return text.strip()

    @staticmethod
    def _fallback_plan(query: str) -> QueryPlan:
        return QueryPlan(
            reasoning="Fallback: LLM planning failed, using original query directly.",
            resolved_query=query,
            search_queries=[query],
            intent="general",
            retrieval_policy="global_only",
            answer_format="summary",
            should_clarify=False,
            topic_entities=[],
        )
