"""XML DSL parser for LLM structured output.

3-layer pipeline:
  1. Pre-cleaner: strip thought blocks, extract XML tag block from prose
  2. Tolerant parser: ET.fromstring with repair for common LLM output issues
  3. Normalizers: convert parsed XML to dicts matching Pydantic model shapes

Usage:
    result = await generate_xml_validated(
        llm, prompt, MyModel, root_tag="plan", normalizer=normalize_plan_output,
    )
"""

from __future__ import annotations

import re
import xml.etree.ElementTree as ET
from typing import TYPE_CHECKING, Any

import structlog
from pydantic import TypeAdapter

if TYPE_CHECKING:
    from collections.abc import Callable

    from pydantic import BaseModel

    from acolyte.port.llm_provider import LLMProviderPort, LLMResponse

logger = structlog.get_logger(__name__)

_TRUNCATION_RATIO = 0.95
_BUDGET_INCREASE = 1.25

# Gemma4 thought block patterns
_THINK_RE = re.compile(r"<think>.*?</think>\s*", re.DOTALL)
_CHANNEL_RE = re.compile(r"<\|channel>thought\n.*?<channel\|>\s*", re.DOTALL)

# Valid claim types for SectionPlanner
_VALID_CLAIM_TYPES = {"factual", "statistical", "comparative", "synthesis"}


class XmlParseError(ValueError):
    """Raised when XML parsing fails after repair attempts."""


# ---------------------------------------------------------------------------
# Layer 1: Pre-cleaner
# ---------------------------------------------------------------------------


def strip_gemma_thought_block(text: str) -> str:
    """Strip Gemma4 thought blocks (<think>...</think> and channel format)."""
    text = _THINK_RE.sub("", text)
    text = _CHANNEL_RE.sub("", text)
    return text.strip()


def extract_tag_block(text: str, root_tag: str) -> str | None:
    """Extract the first <root_tag>...</root_tag> block from text.

    Returns None if opening or closing tag is missing (truncation indicator).
    """
    open_tag = f"<{root_tag}>"
    close_tag = f"</{root_tag}>"
    start = text.find(open_tag)
    if start == -1:
        # Try with attributes (e.g., <plan type="...">)
        match = re.search(rf"<{re.escape(root_tag)}[\s>]", text)
        if match:
            start = match.start()
        else:
            return None
    end = text.find(close_tag, start)
    if end == -1:
        return None
    return text[start : end + len(close_tag)]


# ---------------------------------------------------------------------------
# Layer 2: Tolerant parser
# ---------------------------------------------------------------------------


def _repair_xml(text: str) -> str:
    """Apply lightweight repairs for common LLM XML issues."""
    # Replace bare & (not already escaped) with &amp;
    text = re.sub(r"&(?!amp;|lt;|gt;|quot;|apos;|#)", "&amp;", text)
    return text


def parse_xmlish_block(text: str, root_tag: str) -> ET.Element:
    """Parse XML block from LLM output with pre-cleaning and repair.

    Raises XmlParseError if parsing fails after repair attempts.
    """
    cleaned = strip_gemma_thought_block(text)
    block = extract_tag_block(cleaned, root_tag)
    if block is None:
        raise XmlParseError(f"No <{root_tag}>...</{root_tag}> block found in output")

    # Try strict parse first
    try:
        return ET.fromstring(block)  # noqa: S314
    except ET.ParseError:
        pass

    # Apply repairs and retry
    repaired = _repair_xml(block)
    try:
        element = ET.fromstring(repaired)  # noqa: S314
        logger.info("xml_parse_repaired", root_tag=root_tag)
        return element
    except ET.ParseError as exc:
        raise XmlParseError(f"XML parse failed after repair: {exc}") from exc


# ---------------------------------------------------------------------------
# Layer 3: Normalizers
# ---------------------------------------------------------------------------


def _text(element: ET.Element | None, default: str = "") -> str:
    """Safely extract text from an element."""
    if element is None:
        return default
    return (element.text or "").strip()


def _texts(parent: ET.Element, tag: str) -> list[str]:
    """Collect text from all child elements with the given tag, skipping empties."""
    return [t for el in parent.findall(tag) if (t := _text(el))]


def _bool(text: str) -> bool:
    """Coerce string to bool."""
    return text.strip().lower() in ("true", "1", "yes")


def normalize_plan_output(root: ET.Element) -> dict:
    """Convert <plan> XML to QueryExpansionOutput-shaped dict."""
    reasoning = _text(root.find("reasoning"))
    queries: dict[str, list[str]] = {}
    for section in root.findall("section"):
        key = _text(section.find("key"))
        if key:
            queries[key] = _texts(section, "query")
    return {"reasoning": reasoning, "queries": queries}


def normalize_critic_output(root: ET.Element) -> dict:
    """Convert <critic> XML to critic result dict."""
    reasoning = _text(root.find("reasoning"))
    verdict = _text(root.find("verdict"), "revise")
    if verdict not in ("accept", "revise"):
        verdict = "revise"

    revise_sections = _texts(root, "revise_section")

    feedback: dict[str, str] = {}
    for fb in root.findall("feedback"):
        section = _text(fb.find("section"))
        message = _text(fb.find("message"))
        if section and message:
            feedback[section] = message

    return {
        "reasoning": reasoning,
        "verdict": verdict,
        "revise_sections": revise_sections,
        "feedback": feedback,
    }


def normalize_section_plan_output(root: ET.Element) -> dict:
    """Convert <section_plan> XML to ClaimPlannerOutput-shaped dict."""
    reasoning = _text(root.find("reasoning"))
    claims: list[dict] = []

    for claim_el in root.findall("claim"):
        claim_text = _text(claim_el.find("text"))
        if not claim_text:
            logger.warning("Dropping claim with missing <text>")
            continue

        claim_type = _text(claim_el.find("claim_type"), "factual")
        if claim_type not in _VALID_CLAIM_TYPES:
            logger.warning("Unknown claim_type, defaulting to factual", claim_type=claim_type)
            claim_type = "factual"

        must_cite_text = _text(claim_el.find("must_cite"), "true")

        claims.append(
            {
                "claim": claim_text,
                "claim_type": claim_type,
                "evidence_ids": _texts(claim_el, "evidence_id"),
                "supporting_quotes": _texts(claim_el, "supporting_quote"),
                "numeric_facts": _texts(claim_el, "numeric_fact"),
                "must_cite": _bool(must_cite_text),
            }
        )

    return {"reasoning": reasoning, "claims": claims}


# Valid data types for FactNormalizer
_VALID_DATA_TYPES = {"statistic", "date", "quote", "trend", "comparison"}

# Valid confidence bands
_VALID_CONFIDENCE_BANDS = {"low", "medium", "high"}

_CONFIDENCE_SCORES: dict[str, float] = {"low": 0.3, "medium": 0.6, "high": 0.9}


def confidence_to_score(band: str) -> float:
    """Map confidence band to numeric score for downstream ranking."""
    return _CONFIDENCE_SCORES.get(band, 0.3)


def normalize_fact_output(root: ET.Element) -> dict:
    """Convert <facts> XML to FactNormalizerOutput-shaped dict."""
    fact = root.find("fact")
    if fact is None:
        raise XmlParseError("No <fact> element in <facts> block")

    claim = _text(fact.find("claim"))
    if not claim:
        raise XmlParseError("Empty <claim> in <fact>")

    confidence_text = _text(fact.find("confidence"), "medium")
    if confidence_text not in _VALID_CONFIDENCE_BANDS:
        confidence_text = "low"

    data_type = _text(fact.find("data_type"), "quote")
    if data_type not in _VALID_DATA_TYPES:
        data_type = "quote"

    return {"claim": claim, "confidence": confidence_text, "data_type": data_type}


# ---------------------------------------------------------------------------
# Orchestrator
# ---------------------------------------------------------------------------


async def generate_xml_validated[T: "BaseModel"](
    llm: LLMProviderPort,
    prompt: str,
    model_cls: type[T],
    root_tag: str,
    normalizer: Callable[[ET.Element], dict],
    *,
    retries: int = 1,
    fallback: T | None = None,
    **llm_kwargs: Any,
) -> T:
    """Generate LLM output, parse as XML DSL, validate with Pydantic.

    Unlike generate_validated(), this does NOT inject format= into llm_kwargs.
    The LLM generates free text with XML tags, parsed by our 3-layer pipeline.
    """
    adapter = TypeAdapter(model_cls)

    last_error: Exception | None = None
    budget_increased = False
    response: LLMResponse | None = None

    for attempt in range(1 + retries):
        try:
            response = await llm.generate(prompt, **llm_kwargs)
            element = parse_xmlish_block(response.text, root_tag)
            parsed = normalizer(element)
            result = adapter.validate_python(parsed)
            logger.info("xml_parse_success", root_tag=root_tag, attempt=attempt + 1)
            return result
        except (XmlParseError, ET.ParseError) as exc:
            last_error = exc
            # Truncation detection: closing tag missing + near budget limit
            num_predict = llm_kwargs.get("num_predict")
            if (
                not budget_increased
                and isinstance(num_predict, int)
                and num_predict > 0
                and response is not None
                and response.completion_tokens >= num_predict * _TRUNCATION_RATIO
            ):
                increased = int(num_predict * _BUDGET_INCREASE)
                logger.warning(
                    "XML truncated at num_predict limit, increasing budget",
                    attempt=attempt + 1,
                    old_budget=num_predict,
                    new_budget=increased,
                )
                llm_kwargs["num_predict"] = increased
                budget_increased = True
            else:
                logger.warning(
                    "XML parse failed",
                    attempt=attempt + 1,
                    max_attempts=1 + retries,
                    error=str(exc),
                )
        except Exception as exc:
            last_error = exc
            logger.warning(
                "XML validation failed",
                attempt=attempt + 1,
                max_attempts=1 + retries,
                error=str(exc),
            )

    if fallback is not None:
        logger.warning(
            "xml_parse_exhausted",
            root_tag=root_tag,
            attempts=1 + retries,
            error=str(last_error),
        )
        return fallback

    raise ValueError(f"XML parse/validation failed after {1 + retries} attempts: {last_error}")
