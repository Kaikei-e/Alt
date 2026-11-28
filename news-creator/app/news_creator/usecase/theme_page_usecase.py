"""Theme page generation usecase - generates structured theme pages from source items."""

from __future__ import annotations

import hashlib
import json
import logging
import re
import time
from datetime import date
from pathlib import Path
from typing import Any

from jinja2 import Template

try:
    import json_repair
except ImportError:
    json_repair = None  # type: ignore

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.errors import CardGenerationInvariantError
from news_creator.domain.models import (
    THEME_MAX_KEY_POINTS,
    THEME_MAX_OPEN_QUESTIONS,
    THEME_MAX_OUTPUT_TEXT_CHARS,
    THEME_MAX_OUTPUT_TITLE_CHARS,
    THEME_MAX_TIMELINE,
    THEME_MAX_WHAT_CHANGED,
    CardGenerationRejectedError,
    LLMGenerateResponse,
    ThemeGenerateRequest,
    ThemeGenerateResponse,
    ThemeItemInput,
    ThemePageLLMOutput,
    ThemeSentence,
    ThemeTimelineEntry,
)
from news_creator.domain.prompt_boundary import (
    SYSTEM_BOUNDARY_INSTRUCTION,
    sanitize_untrusted_content,
    wrap_untrusted_content,
)
from news_creator.domain.prompts import _THINKING_BLOCK_RE, wrap_gemma_prompt
from news_creator.port.cache_port import CachePort
from news_creator.port.llm_provider_port import LLMProviderPort
from news_creator.usecase.recap_card_parser import split_sentences

logger = logging.getLogger(__name__)

PROMPTS_DIR = Path(__file__).resolve().parent.parent.parent / "prompts"
THEME_TEMPLATE_PATH = PROMPTS_DIR / "theme_page.jinja"
_ISO_DATE_RE = re.compile(r"^[0-9]{4}-[0-9]{2}-[0-9]{2}$")
_CACHE_KEY_PREFIX = "theme_page:v1"


def _is_strict_int(value: object) -> bool:
    return isinstance(value, int) and not isinstance(value, bool)


def _is_valid_iso_date(value: object) -> bool:
    if not isinstance(value, str) or not _ISO_DATE_RE.fullmatch(value):
        return False
    try:
        date.fromisoformat(value)
    except ValueError:
        return False
    return True


class ThemePageUsecase:
    """Usecase for generating structured Japanese theme pages from numbered items."""

    def __init__(
        self,
        config: NewsCreatorConfig,
        llm_provider: LLMProviderPort,
        cache: CachePort | None = None,
    ) -> None:
        self.config = config
        self.llm_provider = llm_provider
        self.cache = cache
        self._template: Template | None = None

    def _get_template(self) -> Template:
        if self._template is None:
            if not THEME_TEMPLATE_PATH.exists():
                msg = f"Theme page prompt template not found: {THEME_TEMPLATE_PATH}"
                raise FileNotFoundError(msg)
            self._template = Template(THEME_TEMPLATE_PATH.read_text(encoding="utf-8"))
        return self._template

    def _generate_cache_key(self, request: ThemeGenerateRequest) -> str:
        sorted_items = sorted(request.items, key=lambda x: x.n)
        canonical = {
            "items": [
                {
                    "n": item.n,
                    "title": item.title,
                    "published_at": item.published_at,
                    "text": item.text,
                }
                for item in sorted_items
            ],
            "new_item_ns": sorted(set(request.new_item_ns)),
            "previous": (
                {
                    "title": request.previous.title,
                    "key_points": list(request.previous.key_points),
                    "open_questions": list(request.previous.open_questions),
                }
                if request.previous is not None
                else None
            ),
            "failure_notes": list(request.failure_notes),
            "user_steering": (
                {
                    "pinned_key_points": list(request.user_steering.pinned_key_points),
                    "dismissed_key_points": list(
                        request.user_steering.dismissed_key_points
                    ),
                    "user_questions": list(request.user_steering.user_questions),
                    "user_notes": list(request.user_steering.user_notes),
                }
                if request.user_steering is not None
                else None
            ),
        }
        digest = hashlib.sha256(
            json.dumps(canonical, sort_keys=True, ensure_ascii=False).encode("utf-8")
        ).hexdigest()
        return f"{_CACHE_KEY_PREFIX}:{digest}"

    async def _get_from_cache(self, cache_key: str) -> ThemeGenerateResponse | None:
        if self.cache is None:
            return None
        try:
            cached_json = await self.cache.get(cache_key)
            if not cached_json:
                return None
            return ThemeGenerateResponse.model_validate_json(cached_json)
        except (OSError, RuntimeError, ValueError, TypeError) as exc:
            logger.warning(
                "Failed to retrieve cached theme page response",
                extra={"cache_key": cache_key, "error": str(exc)},
            )
            return None

    async def _save_to_cache(
        self, cache_key: str, response: ThemeGenerateResponse
    ) -> None:
        if self.cache is None:
            return
        try:
            await self.cache.set(cache_key, response.model_dump_json())
        except (OSError, RuntimeError, ValueError, TypeError) as exc:
            logger.warning(
                "Failed to save theme page response to cache",
                extra={"cache_key": cache_key, "error": str(exc)},
            )

    @staticmethod
    def _format_user_steering(request: ThemeGenerateRequest) -> str | None:
        steering = request.user_steering
        if steering is None:
            return None
        groups: tuple[tuple[str, list[str]], ...] = (
            (
                "読者が重要とピン留めした要点（優先保持・出典裏づけ必須）:",
                steering.pinned_key_points,
            ),
            (
                "読者が不要と却下した要点（繰り返さないこと）:",
                steering.dismissed_key_points,
            ),
            (
                "読者が立てた問い（入力アイテムに根拠があれば回答または未解決として反映）:",
                steering.user_questions,
            ),
            (
                "読者の着眼点・抜粋メモ（観点の参考にし、主張は必ず入力アイテムの事実で裏づけること）:",
                steering.user_notes,
            ),
        )
        steering_lines: list[str] = []
        for header, entries in groups:
            if not entries:
                continue
            steering_lines.append(header)
            for entry in entries:
                single_line = " ".join(
                    sanitize_untrusted_content(entry).splitlines()
                ).strip()
                if single_line:
                    steering_lines.append(f"- {single_line}")
        if not steering_lines:
            return None
        return "### 読者の着眼点とキュレーション\n" + "\n".join(steering_lines)

    def _build_prompt(
        self,
        request: ThemeGenerateRequest,
        reminder: str | None = None,
    ) -> str:
        template = self._get_template()
        system_rules = template.render(reminder=reminder).strip()
        system_prompt = f"{system_rules}\n\n{SYSTEM_BOUNDARY_INSTRUCTION}"

        new_item_ns_set = set(request.new_item_ns)
        item_lines: list[str] = []
        for item in request.items:
            safe_title = sanitize_untrusted_content(item.title)
            safe_text = sanitize_untrusted_content(item.text)
            new_badge = " [新規]" if item.n in new_item_ns_set else ""
            date_part = (
                f" ({sanitize_untrusted_content(item.published_at)})"
                if item.published_at
                else ""
            )
            item_lines.append(
                f"[{item.n}]{new_badge}{date_part} {safe_title}\n{safe_text}"
            )

        sections = ["### 入力アイテム\n" + "\n\n".join(item_lines)]

        if request.previous is not None:
            prev_lines = [
                f"タイトル: {sanitize_untrusted_content(request.previous.title)}",
                "要点:",
            ]
            if request.previous.key_points:
                prev_lines.extend(
                    f"- {sanitize_untrusted_content(kp)}"
                    for kp in request.previous.key_points
                )
            else:
                prev_lines.append("- なし")
            prev_lines.append("未解決の問い:")
            if request.previous.open_questions:
                prev_lines.extend(
                    f"- {sanitize_untrusted_content(oq)}"
                    for oq in request.previous.open_questions
                )
            else:
                prev_lines.append("- なし")
            sections.append("### 前回の版\n" + "\n".join(prev_lines))

        steering_section = self._format_user_steering(request)
        if steering_section:
            sections.append(steering_section)

        if request.failure_notes:
            note_lines = [
                f"- {sanitize_untrusted_content(note)}"
                for note in request.failure_notes
            ]
            sections.append("### 前回の出力に対する修正指示\n" + "\n".join(note_lines))

        user_content = wrap_untrusted_content("\n\n".join(sections))
        return wrap_gemma_prompt(user_prompt=user_content, system_prompt=system_prompt)

    async def _call_llm(self, prompt: str) -> tuple[str, str]:
        model = self.config.model_name
        max_tokens = self.config.llm.recap_theme_max_new_tokens
        temperature = self.config.llm.recap_card_temperature
        json_schema = ThemePageLLMOutput.model_json_schema()

        start_time = time.perf_counter()
        llm_response = await self.llm_provider.generate(
            prompt=prompt,
            model=model,
            num_predict=max_tokens,
            format=json_schema,
            options={
                "temperature": temperature,
                "num_predict": max_tokens,
            },
            priority="low",
        )
        elapsed_ms = int((time.perf_counter() - start_time) * 1000)

        if not isinstance(llm_response, LLMGenerateResponse):
            msg = f"Unexpected LLM response type: {type(llm_response).__name__}"
            raise CardGenerationInvariantError(msg)

        raw_text = llm_response.response or ""
        model_name = llm_response.model or model
        logger.info(
            "Theme page LLM generation completed",
            extra={
                "model": model_name,
                "ms": elapsed_ms,
                "prompt_tokens": llm_response.prompt_eval_count or 0,
                "completion_tokens": llm_response.eval_count or 0,
            },
        )
        return raw_text, model_name

    @staticmethod
    def _parse_json_object(raw_text: str) -> dict[str, Any]:
        cleaned = _THINKING_BLOCK_RE.sub("", raw_text).strip()
        cleaned = re.sub(r"<think>.*?</think>", "", cleaned, flags=re.DOTALL).strip()
        if not cleaned:
            raise ValueError("empty_output")

        try:
            parsed = json.loads(cleaned)
        except json.JSONDecodeError as exc:
            if json_repair is None:
                raise ValueError("parse_failed") from exc
            try:
                repaired = json_repair.repair_json(cleaned)
                parsed = json.loads(repaired)
            except Exception as repair_exc:
                raise ValueError("parse_failed") from repair_exc

        if not isinstance(parsed, dict):
            raise ValueError("parse_failed")  # noqa: TRY004 - caller handles ValueError('parse_failed') for retry
        return parsed

    @staticmethod
    def _clean_sentence_text(raw_text: object) -> str | None:
        if not isinstance(raw_text, str):
            return None
        text = raw_text.strip()
        if not text or len(text) > THEME_MAX_OUTPUT_TEXT_CHARS:
            return None
        if len(split_sentences(text)) != 1:
            return None
        return text

    @staticmethod
    def _validate_refs(raw_refs: object, valid_refs: set[int]) -> list[int] | None:
        if not isinstance(raw_refs, list) or not raw_refs:
            return None
        deduped: list[int] = []
        seen: set[int] = set()
        for ref in raw_refs:
            if not _is_strict_int(ref) or ref not in valid_refs:
                return None
            if ref not in seen:
                seen.add(ref)
                deduped.append(ref)
        return deduped

    def _post_process(
        self,
        raw_text: str,
        model_name: str,
        request: ThemeGenerateRequest,
    ) -> tuple[ThemeGenerateResponse | None, str]:
        try:
            payload = self._parse_json_object(raw_text)
        except ValueError as exc:
            return None, str(exc)

        raw_title = payload.get("title")
        if not isinstance(raw_title, str):
            return None, "parse_failed"
        title = raw_title.strip()
        if not title or len(title) > THEME_MAX_OUTPUT_TITLE_CHARS:
            return None, "parse_failed"

        items_by_n: dict[int, ThemeItemInput] = {item.n: item for item in request.items}
        valid_refs = set(items_by_n.keys())
        new_item_ns_set = set(request.new_item_ns)

        key_points = self._filter_sentence_list(
            payload.get("key_points"),
            valid_refs=valid_refs,
            cap=THEME_MAX_KEY_POINTS,
        )
        if not key_points:
            return None, "empty_output"

        timeline = self._filter_timeline(
            payload.get("timeline"),
            items_by_n=items_by_n,
            valid_refs=valid_refs,
            cap=THEME_MAX_TIMELINE,
        )

        if request.previous is None:
            what_changed: list[ThemeSentence] = []
        else:
            what_changed = self._filter_sentence_list(
                payload.get("what_changed"),
                valid_refs=valid_refs,
                cap=THEME_MAX_WHAT_CHANGED,
                required_any_refs=new_item_ns_set,
            )

        open_questions = self._filter_sentence_list(
            payload.get("open_questions"),
            valid_refs=valid_refs,
            cap=THEME_MAX_OPEN_QUESTIONS,
        )

        return (
            ThemeGenerateResponse(
                title=title,
                key_points=key_points,
                timeline=timeline,
                what_changed=what_changed,
                open_questions=open_questions,
                model=model_name,
            ),
            "",
        )

    def _filter_sentence_list(
        self,
        raw_entries: object,
        *,
        valid_refs: set[int],
        cap: int,
        required_any_refs: set[int] | None = None,
    ) -> list[ThemeSentence]:
        if not isinstance(raw_entries, list):
            return []
        accepted: list[ThemeSentence] = []
        for entry in raw_entries:
            if not isinstance(entry, dict):
                continue
            text = self._clean_sentence_text(entry.get("text"))
            if text is None:
                continue
            refs = self._validate_refs(entry.get("refs"), valid_refs)
            if refs is None:
                continue
            if required_any_refs is not None and not any(
                r in required_any_refs for r in refs
            ):
                continue
            accepted.append(ThemeSentence(text=text, refs=refs))
            if len(accepted) >= cap:
                break
        return accepted

    def _filter_timeline(
        self,
        raw_entries: object,
        *,
        items_by_n: dict[int, ThemeItemInput],
        valid_refs: set[int],
        cap: int,
    ) -> list[ThemeTimelineEntry]:
        if not isinstance(raw_entries, list):
            return []
        accepted: list[ThemeTimelineEntry] = []
        for entry in raw_entries:
            if not isinstance(entry, dict):
                continue
            entry_date = entry.get("date")
            if not _is_valid_iso_date(entry_date):
                continue
            text = self._clean_sentence_text(entry.get("text"))
            if text is None:
                continue
            refs = self._validate_refs(entry.get("refs"), valid_refs)
            if refs is None:
                continue
            if not isinstance(entry_date, str):
                continue
            if not any(
                items_by_n[r].published_at == entry_date
                or entry_date in items_by_n[r].text
                for r in refs
            ):
                continue
            accepted.append(ThemeTimelineEntry(date=entry_date, text=text, refs=refs))
            if len(accepted) >= cap:
                break
        return accepted

    @staticmethod
    def _build_retry_reminder(reason: str, valid_refs: set[int]) -> str:
        return (
            f"前回の出力は理由「{reason}」で検証を通過しませんでした。\n"
            f"- title は1〜60文字の日本語にしてください。\n"
            f"- key_points は1件以上、各文200文字以内の1文で記述してください。\n"
            f"- すべての文の refs には入力にある番号 {sorted(valid_refs)} のみを含めてください。"
        )

    async def generate_theme_page(
        self, request: ThemeGenerateRequest
    ) -> ThemeGenerateResponse:
        """Generate a validated theme page response with at most one retry."""
        cache_key = self._generate_cache_key(request)
        cached = await self._get_from_cache(cache_key)
        if cached is not None:
            logger.info(
                "Theme page generation cache hit", extra={"cache_key": cache_key}
            )
            return cached

        valid_refs = {item.n for item in request.items}

        # Attempt 1
        prompt_1 = self._build_prompt(request)
        raw_1, model_1 = await self._call_llm(prompt_1)
        parsed_1, reason_1 = self._post_process(raw_1, model_1, request)
        if parsed_1 is not None:
            await self._save_to_cache(cache_key, parsed_1)
            return parsed_1

        logger.warning(
            "Theme page parsing/post-processing failed on attempt 1, retrying once",
            extra={
                "reason": reason_1,
                "raw_text_len": len(raw_1),
                "job_id": str(request.job_id),
                "theme_id": str(request.theme_id),
            },
        )

        # Attempt 2
        reminder = self._build_retry_reminder(reason_1, valid_refs)
        prompt_2 = self._build_prompt(request, reminder=reminder)
        raw_2, model_2 = await self._call_llm(prompt_2)
        parsed_2, reason_2 = self._post_process(raw_2, model_2, request)
        if parsed_2 is not None:
            await self._save_to_cache(cache_key, parsed_2)
            return parsed_2

        rejection_reason = (
            "empty_output" if reason_2 == "empty_output" else "parse_failed"
        )
        logger.error(
            "Theme page generation failed after regeneration attempt",
            extra={
                "reason": rejection_reason,
                "attempts": 2,
                "raw_text_len": len(raw_2),
                "job_id": str(request.job_id),
                "theme_id": str(request.theme_id),
            },
        )
        raise CardGenerationRejectedError(
            reason=rejection_reason,
            attempts=2,
            raw_text=raw_2,
        )
