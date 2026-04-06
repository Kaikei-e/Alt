"""Usecase for structured query planning for Augur Conversational RAG.

Uses Ollama structured outputs (JSON Schema) to produce a deterministic
QueryPlan from user query + conversation history. This replaces the
rag-orchestrator's rule-based ConversationPlanner and query expansion.
"""

import json
import logging
import time
from datetime import datetime, timezone
from typing import Optional, List

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import (
    ConversationMessage,
    PlanQueryRequest,
    PlanQueryResponse,
    QueryPlan,
)
from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)

PLAN_QUERY_PROMPT_TEMPLATE = """<task>
You are a query planner for a news knowledge retrieval system.
Analyze the user query and produce a structured retrieval plan.
</task>

<rules>
- Current Date: {current_date}
- CRITICAL: should_clarify MUST be false for almost all queries. Set true ONLY for bare ambiguous phrases like "もっと詳しく" or "tell me more" with NO conversation history. If there is conversation history, resolve the reference and set should_clarify=false.
- resolved_query MUST be a complete, self-contained search query. Never return empty string or single characters.
- If the query already contains a clear topic, use it directly as resolved_query.
- Generate 3-5 search_queries: include Japanese AND English variations, synonyms, and related terms.
- intent: choose exactly one of: causal_explanation, temporal, synthesis, comparison, fact_check, topic_deep_dive, general
- retrieval_policy: choose exactly one of: global_only, article_only, tool_only, no_retrieval. Default is global_only.
- answer_format: choose exactly one of: causal_analysis, summary, list, detail, comparison, fact_check. Default is summary.
</rules>

{context_section}

<query>{query}</query>
"""

CONVERSATION_SECTION_TEMPLATE = """<conversation>
{history}
</conversation>
"""

ARTICLE_SECTION_TEMPLATE = """<article_scope>
Article: {title} [id: {article_id}]
</article_scope>
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
            logger.warning("plan_query_llm_failed", extra={"error": str(e), "query": request.query})
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
        json_schema = QueryPlan.model_json_schema()

        response = await self.llm_provider.generate(
            prompt,
            model=self.PLANNING_MODEL,
            num_predict=256,
            format=json_schema,
            options={
                "temperature": 0.2,
                "repeat_penalty": 1.1,
            },
            priority=request.priority,
        )

        try:
            parsed = json.loads(response.response)
            return QueryPlan(**parsed)
        except (json.JSONDecodeError, Exception) as e:
            logger.warning("plan_query_parse_failed", extra={
                "error": str(e),
                "raw_response": response.response[:200],
            })
            return self._fallback_plan(request.query)

    def _build_prompt(self, request: PlanQueryRequest) -> str:
        context_parts = []

        if request.conversation_history:
            history_lines = []
            for msg in request.conversation_history[-6:]:
                content = msg.content[:300] + "..." if len(msg.content) > 300 else msg.content
                history_lines.append(f"{msg.role}: {content}")
            context_parts.append(
                CONVERSATION_SECTION_TEMPLATE.format(history="\n".join(history_lines))
            )

        if request.article_id and request.article_title:
            context_parts.append(
                ARTICLE_SECTION_TEMPLATE.format(
                    title=request.article_title,
                    article_id=request.article_id,
                )
            )

        if request.last_answer_scope:
            context_parts.append(f"<last_answer_scope>{request.last_answer_scope}</last_answer_scope>\n")

        current_date = datetime.now(timezone.utc).strftime("%Y-%m-%d")

        return PLAN_QUERY_PROMPT_TEMPLATE.format(
            current_date=current_date,
            context_section="".join(context_parts),
            query=request.query,
        )

    @staticmethod
    def _fallback_plan(query: str) -> QueryPlan:
        return QueryPlan(
            resolved_query=query,
            search_queries=[query],
            intent="general",
            retrieval_policy="global_only",
            answer_format="summary",
            should_clarify=False,
            topic_entities=[],
        )
