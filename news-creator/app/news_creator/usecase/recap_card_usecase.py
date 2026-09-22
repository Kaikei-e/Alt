"""Recap card generation usecase - generates structured cards from candidate items."""

from __future__ import annotations

import hashlib
import json
import logging
import time
from pathlib import Path
from jinja2 import Template

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import (
    CardGenerateRequest,
    CardGenerateResponse,
    CardGenerationMetadata,
    CardGenerationRejectedError,
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
            num_predict=self.config.llm.recap_card_num_predict,
            options={
                "temperature": self.config.llm.recap_card_temperature,
                "num_predict": self.config.llm.recap_card_num_predict,
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

        # --- Attempt 1 ---
        prompt_1 = self._build_prompt(request)
        raw_text_1, model_1, p_tokens_1, c_tokens_1, ms_1 = await self._call_llm(
            prompt_1
        )

        parse_result_1 = parse_card_output(
            raw_text=raw_text_1,
            valid_refs=valid_refs,
            ja_ratio_threshold=ja_threshold,
        )

        if parse_result_1.success and parse_result_1.card is not None:
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
            )
            await self._save_to_cache(cache_key, response)
            return response

        # --- Attempt 2 (regenerate exactly once) ---
        logger.warning(
            "Card parsing failed on attempt 1, regenerating with strict reminder",
            extra={
                "reason": parse_result_1.reason,
                "job_id": str(request.job_id),
                "candidate_id": str(request.candidate_id),
            },
        )

        reminder = (
            f"前回の出力は理由「{parse_result_1.reason}」で契約を満たしませんでした。\n"
            "以下の規則を必ず完全に守って出力してください:\n"
            "- 【見出し】は40文字以内の日本語であること\n"
            "- 【何が起きた】は2〜3文で、各文末に必ず入力にある出典番号 [n] を付けること\n"
            "- 【なぜ重要】は影響・結果の客観的事実がある時だけ1文（末尾に [n]）で記述し、無ければ「該当なし」とだけ書くこと\n"
            f"- 入力に存在する出典番号 {sorted(list(valid_refs))} 以外を使用しないこと\n"
            "- 出力は日本語のみとすること"
        )

        prompt_2 = self._build_prompt(request, reminder=reminder)
        raw_text_2, model_2, p_tokens_2, c_tokens_2, ms_2 = await self._call_llm(
            prompt_2
        )

        parse_result_2 = parse_card_output(
            raw_text=raw_text_2,
            valid_refs=valid_refs,
            ja_ratio_threshold=ja_threshold,
        )

        if parse_result_2.success and parse_result_2.card is not None:
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
            )
            await self._save_to_cache(cache_key, response)
            return response

        # Rejection after 2 attempts
        reason = parse_result_2.reason or "parse_failed"
        logger.error(
            "Card generation failed after regeneration attempt",
            extra={
                "reason": reason,
                "attempts": 2,
                "job_id": str(request.job_id),
                "candidate_id": str(request.candidate_id),
            },
        )
        raise CardGenerationRejectedError(
            reason=reason,
            attempts=2,
            raw_text=raw_text_2,
        )
