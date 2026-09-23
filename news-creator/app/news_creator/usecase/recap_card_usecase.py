"""Recap card generation usecase - generates structured cards from candidate items."""

from __future__ import annotations

import hashlib
import json
import logging
import time
from pathlib import Path
from typing import Any
from jinja2 import Template

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import (
    Card422Reason,
    CardGenerateRequest,
    CardGenerateResponse,
    CardGenerationMetadata,
    CardGenerationRejectedError,
    CardParseDetail,
    LLMGenerateResponse,
)
from news_creator.domain.prompt_boundary import (
    SYSTEM_BOUNDARY_INSTRUCTION,
    sanitize_untrusted_content,
    wrap_untrusted_content,
)
from news_creator.domain.prompts import wrap_gemma_prompt
from news_creator.port.cache_port import CachePort
from news_creator.port.llm_provider_port import LLMProviderPort
from news_creator.usecase.recap_card_parser import parse_card_output

logger = logging.getLogger(__name__)

PROMPTS_DIR = Path(__file__).resolve().parent.parent.parent / "prompts"
CARD_TEMPLATE_PATH = PROMPTS_DIR / "recap_card.jinja"


class RecapCardUsecase:
    """Usecase for generating structured recap cards from candidate items."""

    def __init__(
        self,
        config: NewsCreatorConfig,
        llm_provider: LLMProviderPort,
        cache: CachePort | None = None,
    ):
        """
        Initialize recap card usecase.

        Args:
            config: News creator configuration
            llm_provider: LLM provider port
            cache: Optional cache port for response caching
        """
        self.config = config
        self.llm_provider = llm_provider
        self.cache = cache
        self._template: Template | None = None

    def _get_template(self) -> Template:
        """Load and cache Jinja prompt template."""
        if self._template is None:
            if not CARD_TEMPLATE_PATH.exists():
                raise FileNotFoundError(
                    f"Recap card prompt template not found: {CARD_TEMPLATE_PATH}"
                )
            self._template = Template(CARD_TEMPLATE_PATH.read_text(encoding="utf-8"))
        return self._template

    def _generate_cache_key(self, request: CardGenerateRequest) -> str:
        """
        Generate cache key: prompt_version + sha256(canonical JSON of items sorted by n + revision_note).
        """
        sorted_items = sorted(request.items, key=lambda x: x.n)
        canonical_items = [
            {
                "feed_id": str(item.feed_id),
                "host": item.host,
                "lede": item.lede,
                "n": item.n,
                "pub_date": item.pub_date,
                "title": item.title,
                "url": item.url,
            }
            for item in sorted_items
        ]
        items_json = json.dumps(canonical_items, sort_keys=True, ensure_ascii=False)
        content_to_hash = items_json
        if request.revision_note:
            content_to_hash += f":{request.revision_note}"
        hash_hex = hashlib.sha256(content_to_hash.encode("utf-8")).hexdigest()
        return f"recap_card:{request.prompt_version}:{hash_hex}"

    async def _get_from_cache(self, cache_key: str) -> CardGenerateResponse | None:
        """Retrieve cached card response if available."""
        if self.cache is None:
            return None
        try:
            cached_json = await self.cache.get(cache_key)
            if not cached_json:
                return None
            cached_data = json.loads(cached_json)
            base_response = CardGenerateResponse.model_validate(cached_data)
            # Return with cache_hit=True
            return CardGenerateResponse(
                card=base_response.card,
                generation=CardGenerationMetadata(
                    model=base_response.generation.model,
                    prompt_version=base_response.generation.prompt_version,
                    cache_hit=True,
                    prompt_tokens=base_response.generation.prompt_tokens,
                    completion_tokens=base_response.generation.completion_tokens,
                    ms=base_response.generation.ms,
                    raw_text=base_response.generation.raw_text,
                ),
                ja_ratio=base_response.ja_ratio,
            )
        except Exception as exc:
            logger.warning(
                "Failed to retrieve cached card response",
                extra={"cache_key": cache_key, "error": str(exc)},
            )
            return None

    async def _save_to_cache(
        self, cache_key: str, response: CardGenerateResponse
    ) -> None:
        """Store generated card response in cache."""
        if self.cache is None:
            return
        try:
            await self.cache.set(cache_key, response.model_dump_json())
        except Exception as exc:
            logger.warning(
                "Failed to save card response to cache",
                extra={"cache_key": cache_key, "error": str(exc)},
            )

    def _build_prompt(
        self,
        request: CardGenerateRequest,
        reminder: str | None = None,
    ) -> str:
        """Render prompt template for card generation with turn separation and boundary protection."""
        template = self._get_template()
        system_rules = template.render(
            reminder=reminder,
        ).strip()
        system_prompt = f"{system_rules}\n\n{SYSTEM_BOUNDARY_INSTRUCTION}"

        lines: list[str] = []
        for item in request.items:
            safe_title = sanitize_untrusted_content(item.title)
            safe_host = sanitize_untrusted_content(item.host)
            safe_pub_date = (
                f", {sanitize_untrusted_content(item.pub_date)}"
                if item.pub_date
                else ""
            )
            safe_lede = sanitize_untrusted_content(item.lede)
            lines.append(
                f"[{item.n}] {safe_title} ({safe_host}{safe_pub_date})\n{safe_lede}"
            )

        items_body = "\n\n".join(lines)
        untrusted_sections = [f"### 入力アイテム\n{items_body}"]
        if request.revision_note:
            safe_note = sanitize_untrusted_content(request.revision_note)
            untrusted_sections.append(
                "### 前回の出力に対する修正指示\n"
                "前回の出力は以下の理由で棄却されました。指摘内容を確認し、問題点を修正したカードを出力してください:\n"
                f"{safe_note}"
            )

        untrusted_body = "\n\n".join(untrusted_sections)
        user_content = wrap_untrusted_content(untrusted_body)

        return wrap_gemma_prompt(user_prompt=user_content, system_prompt=system_prompt)

    async def _call_llm(
        self,
        prompt: str,
    ) -> tuple[str, str, int, int, int]:
        """
        Call LLM provider with configured parameters.

        Returns:
            (raw_text, model_name, prompt_tokens, completion_tokens, ms)
        """
        model = self.config.model_name
        start_time = time.perf_counter()
        llm_response = await self.llm_provider.generate(
            prompt=prompt,
            model=model,
            num_predict=self.config.llm.recap_card_max_new_tokens,
            options={
                "temperature": self.config.llm.recap_card_temperature,
                "num_predict": self.config.llm.recap_card_max_new_tokens,
            },
            priority="low",
        )
        elapsed_ms = int((time.perf_counter() - start_time) * 1000)

        if not isinstance(llm_response, LLMGenerateResponse):
            raise RuntimeError(
                f"Unexpected LLM response type: {type(llm_response).__name__}"
            )

        raw_text = llm_response.response or ""
        model_name = llm_response.model or model
        prompt_tokens = llm_response.prompt_eval_count or 0
        completion_tokens = llm_response.eval_count or 0
        ms = (
            int(llm_response.total_duration / 1_000_000)
            if llm_response.total_duration
            else elapsed_ms
        )

        logger.info(
            "Recap card LLM generation completed",
            extra={
                "model": model_name,
                "ms": ms,
                "prompt_tokens": prompt_tokens,
                "completion_tokens": completion_tokens,
            },
        )
        return raw_text, model_name, prompt_tokens, completion_tokens, ms

    def _build_reminder(
        self,
        reason: Card422Reason | None,
        detail: CardParseDetail | None,
        valid_refs: set[int],
    ) -> str:
        """Build targeted regeneration reminder based on parse failure detail."""
        base_reminder = (
            f"前回の出力は理由「{detail or reason}」で契約を満たしませんでした。\n"
            "以下の規則を必ず完全に守って出力してください:\n"
            "- 【見出し】は60文字以内の日本語であること\n"
            "- 【何が起きた】は2〜3文で、各文末に必ず入力にある出典番号 [n] を付けること\n"
            "- 【なぜ重要】は影響・結果の客観的事実がある時だけ1文（末尾に [n]）で記述し、無ければ「該当なし」とだけ書くこと\n"
            f"- 入力に存在する出典番号 {sorted(list(valid_refs))} 以外を使用しないこと\n"
            "- すべての入力アイテムが英語であっても要約カードは必ず自然な日本語で作成すること（製品名やサービス名などの固有名詞のみアルファベット表記を維持可）"
        )
        if detail == "sentence_count":
            return (
                base_reminder
                + "\n- 【何が起きた】は 2〜3 文にまとめること。出典が多い場合は 1 文に複数の番号を [1] [2] のように付けてよい"
            )
        if detail == "headline_too_long":
            return (
                base_reminder
                + "\n- 【見出し】は60文字以内の日本語であること。60文字を超えてはならない。"
            )
        if detail in ("missing_citation", "unknown_citation_format"):
            return (
                base_reminder
                + "\n- 出典番号は文末に [1] [2] の形で、複数のときは半角スペース区切り ([1] [2]) で並べ、・ および , や [1, 2] は使わないこと"
            )
        if detail == "sources_tag":
            return (
                base_reminder
                + "\n- 【出典】タグを省略せず、使用した出典番号を [1] [2] のように必ず記載すること"
            )
        if detail == "why_format":
            return (
                base_reminder
                + "\n- 【なぜ重要】は影響・結果の客観的事実がある時だけ1文（末尾に [n]）で記述し、無ければ「該当なし」とだけ書くこと"
            )
        if detail == "missing_tag":
            return (
                base_reminder
                + "\n- 【見出し】【何が起きた】【なぜ重要】【出典】の各セクションタグを必ず含めること"
            )

        return base_reminder

    async def generate_card(self, request: CardGenerateRequest) -> CardGenerateResponse:
        """
        Generate a Japanese summary card from candidate items.

        Flow:
        1. Check cache (prompt_version + items hash + revision_note)
        2. Attempt 1: Call LLM with prompt -> strip thinking blocks -> parse tags
        3. If parse fails: Attempt 2 (regenerate exactly once with strict reminder)
        4. If still fails: raise CardGenerationRejectedError(reason, attempts=2, raw_text)
        5. On success: save to cache and return response.
        """
        cache_key = self._generate_cache_key(request)
        cached = await self._get_from_cache(cache_key)
        if cached is not None:
            logger.info("Card generation cache hit", extra={"cache_key": cache_key})
            return cached

        valid_refs = {item.n for item in request.items}
        ja_threshold = self.config.llm.recap_ja_ratio_threshold
        source_texts = [
            text for item in request.items for text in (item.title, item.lede) if text
        ]

        # --- Attempt 1 ---
        prompt_1 = self._build_prompt(request)
        raw_text_1, model_1, p_tokens_1, c_tokens_1, ms_1 = await self._call_llm(
            prompt_1
        )

        parse_result_1 = parse_card_output(
            raw_text=raw_text_1,
            valid_refs=valid_refs,
            ja_ratio_threshold=ja_threshold,
            source_texts=source_texts,
        )

        if parse_result_1.success and parse_result_1.card is not None:
            if parse_result_1.measured_ratio is None:
                raise RuntimeError("Parser succeeded but measured_ratio is None")
            response = CardGenerateResponse(
                card=parse_result_1.card,
                generation=CardGenerationMetadata(
                    model=model_1,
                    prompt_version=request.prompt_version,
                    cache_hit=False,
                    prompt_tokens=p_tokens_1,
                    completion_tokens=c_tokens_1,
                    ms=ms_1,
                    raw_text=raw_text_1,
                ),
                ja_ratio=parse_result_1.measured_ratio,
            )
            await self._save_to_cache(cache_key, response)
            return response

        # --- Attempt 2 (regenerate exactly once) ---
        extra_1: dict[str, Any] = {
            "reason": parse_result_1.reason,
            "raw_text_len": len(raw_text_1),
            "job_id": str(request.job_id),
            "candidate_id": str(request.candidate_id),
        }
        if parse_result_1.detail is not None:
            extra_1["detail"] = parse_result_1.detail
        if (
            parse_result_1.reason == "language"
            and parse_result_1.measured_ratio is not None
        ):
            extra_1["measured_ratio"] = parse_result_1.measured_ratio

        logger.warning(
            "Card parsing failed on attempt 1, regenerating with strict reminder",
            extra=extra_1,
        )

        reminder = self._build_reminder(
            parse_result_1.reason,
            parse_result_1.detail,
            valid_refs,
        )

        prompt_2 = self._build_prompt(request, reminder=reminder)
        raw_text_2, model_2, p_tokens_2, c_tokens_2, ms_2 = await self._call_llm(
            prompt_2
        )

        parse_result_2 = parse_card_output(
            raw_text=raw_text_2,
            valid_refs=valid_refs,
            ja_ratio_threshold=ja_threshold,
            source_texts=source_texts,
        )

        if parse_result_2.success and parse_result_2.card is not None:
            if parse_result_2.measured_ratio is None:
                raise RuntimeError("Parser succeeded but measured_ratio is None")
            response = CardGenerateResponse(
                card=parse_result_2.card,
                generation=CardGenerationMetadata(
                    model=model_2,
                    prompt_version=request.prompt_version,
                    cache_hit=False,
                    prompt_tokens=p_tokens_1 + p_tokens_2,
                    completion_tokens=c_tokens_1 + c_tokens_2,
                    ms=ms_1 + ms_2,
                    raw_text=raw_text_2,
                ),
                ja_ratio=parse_result_2.measured_ratio,
            )
            await self._save_to_cache(cache_key, response)
            return response

        # Rejection after 2 attempts
        reason = parse_result_2.reason or "parse_failed"
        extra_2: dict[str, Any] = {
            "reason": reason,
            "attempts": 2,
            "raw_text_len": len(raw_text_2),
            "job_id": str(request.job_id),
            "candidate_id": str(request.candidate_id),
        }
        if parse_result_2.detail is not None:
            extra_2["detail"] = parse_result_2.detail
        if reason == "language" and parse_result_2.measured_ratio is not None:
            extra_2["measured_ratio"] = parse_result_2.measured_ratio

        logger.error(
            "Card generation failed after regeneration attempt",
            extra=extra_2,
        )
        raise CardGenerationRejectedError(
            reason=reason,
            attempts=2,
            raw_text=raw_text_2,
            detail=parse_result_2.detail,
            measured_ratio=parse_result_2.measured_ratio,
            character_counts=parse_result_2.character_counts,
        )
