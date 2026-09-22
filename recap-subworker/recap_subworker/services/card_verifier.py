"""Card verification service for generated topic cards.

Implements quality verification gates:
- G3 Attribution similarity: sentence embedding vs cited item title+lede embeddings
- G4 Filler rule: detects speculation and filler phrases
- G5 Specificity: entity (proper nouns) and number density via Sudachi
- G6 Why-hint: detects result/effect/numerical cues in source evidence
"""

from __future__ import annotations

import re
from collections.abc import Sequence
from typing import Any, Literal
from uuid import UUID

import numpy as np
import structlog
from pydantic import BaseModel, Field

from .embed_service import (
    EmbedderError,
    EmbedResponse,
    EmbedService,
    UnresolvableDimensionError,
)

logger = structlog.get_logger(__name__)

# G4: Speculation / filler phrases
DEFAULT_FILLER_PHRASES: tuple[str, ...] = (
    "注目される",
    "注目を集める",
    "期待される",
    "期待が高まる",
    "見込まれる",
    "重要な一歩",
    "今後の動向",
    "と言える",
    "予想される",
    "可能性が高い",
)

# G6: Result, effect, and numerical cues for why-importance justification
WHY_HINT_CUES: tuple[str, ...] = (
    "%",
    "円",
    "ドル",
    "億",
    "万",
    "倍",
    "増加",
    "減少",
    "上昇",
    "下落",
    "発表",
    "決定",
    "買収",
    "提携",
    "停止",
    "開始",
    "削減",
)

NON_CONTENT_POS: frozenset[str] = frozenset({"助詞", "助動詞", "補助記号", "記号", "空白"})


class VerifySentenceInput(BaseModel):
    """Input sentence to verify."""

    idx: int = Field(..., description="0-based sentence index in the card")
    kind: Literal["what", "why"] = Field(..., description="Sentence kind: 'what' or 'why'")
    text: str = Field(..., description="Sentence text")
    refs: list[int] = Field(
        default_factory=list,
        description="1-based citation references [n] to cluster items",
    )


class VerifyItemInput(BaseModel):
    """Input source item cited by the card."""

    n: int = Field(..., description="1-based citation identifier")
    title: str = Field(..., description="Item headline or title")
    lede: str = Field(..., description="Item lede or description")


class VerifyThresholds(BaseModel):
    """Configurable thresholds for card verification."""

    attribution_cos: float = Field(
        default=0.55,
        ge=0.0,
        le=1.0,
        description="Minimum cosine similarity for citation attribution pass",
    )


class VerifyCardRequest(BaseModel):
    """Request payload for /v1/verify endpoint."""

    job_id: UUID = Field(..., description="Recap job UUID")
    card_id: UUID = Field(..., description="Topic card UUID")
    language: Literal["ja"] = Field(
        default="ja",
        description="Language code; currently only 'ja' is supported",
    )
    sentences: list[VerifySentenceInput] = Field(
        default_factory=list,
        description="Sentences to verify",
    )
    items: list[VerifyItemInput] = Field(
        default_factory=list,
        description="Source items cited by card sentences",
    )
    thresholds: VerifyThresholds = Field(
        default_factory=VerifyThresholds,
        description="Verification thresholds",
    )


class AttributionResult(BaseModel):
    """Result of citation attribution gate (G3)."""

    max_cos: float = Field(..., description="Max cosine similarity to cited item embeddings")
    best_n: int | None = Field(
        default=None,
        description="Citation identifier [n] with highest similarity, or null if no refs",
    )
    pass_: bool = Field(
        ...,
        serialization_alias="pass",
        description="Whether max_cos >= attribution_cos threshold",
    )

    model_config = {"populate_by_name": True}


class FillerResult(BaseModel):
    """Result of speculation/filler gate (G4)."""

    matched: list[str] = Field(
        default_factory=list,
        description="Speculation/filler phrases detected in sentence",
    )
    pass_: bool = Field(
        ...,
        serialization_alias="pass",
        description="Whether no filler phrases were matched",
    )

    model_config = {"populate_by_name": True}


class SpecificityResult(BaseModel):
    """Result of entity/number specificity gate (G5)."""

    proper_nouns: int = Field(..., description="Count of proper noun tokens")
    numbers: int = Field(..., description="Count of number tokens")
    tokens: int = Field(..., description="Count of content tokens")
    density: float = Field(
        ...,
        description="(proper_nouns + numbers) / max(tokens, 1)",
    )


class SentenceVerificationResult(BaseModel):
    """Verification results for an individual sentence."""

    idx: int = Field(..., description="Sentence index")
    attribution: AttributionResult = Field(..., description="Attribution gate results")
    filler: FillerResult = Field(..., description="Filler gate results")
    specificity: SpecificityResult = Field(..., description="Specificity gate results")


class EmbeddingInfo(BaseModel):
    """Embedding model provenance metadata."""

    model: str = Field(..., description="Model identifier")
    identity: str = Field(..., description="Canonical model identity")


class VerifyCardResponse(BaseModel):
    """Response payload for /v1/verify endpoint."""

    sentences: list[SentenceVerificationResult] = Field(
        default_factory=list,
        description="Verification results per sentence",
    )
    why_hint_present: bool = Field(
        ...,
        description="Whether cited items contain result/effect/number cues for why statement",
    )
    embedding: EmbeddingInfo = Field(
        ...,
        description="Embedding model metadata",
    )


class TokenizerUnavailableError(Exception):
    """Raised when Sudachi dictionary or tokenizer fails to load."""


class BackendEmbeddingError(EmbedderError):
    """Raised when the embedding provider fails or is unreachable."""


def match_fillers(text: str, phrases: Sequence[str] = DEFAULT_FILLER_PHRASES) -> list[str]:
    """Find all speculation/filler phrases that appear in the given text.

    Returns the matched phrases in order of appearance or occurrence in the phrase list.
    """
    if not text:
        return []
    return [phrase for phrase in phrases if phrase in text]


def check_why_hint_present(
    sentences: Sequence[VerifySentenceInput],
    items: Sequence[VerifyItemInput],
) -> bool:
    """Check if cited source items contain result/effect/number cues.

    Attribution cue detection for why statements:
    Target items are the union of refs of kind="why" sentences,
    or all items if there are no refs or no why sentences.
    Cues are digits, currency/unit symbols (%, 円, ドル, 億, 万, 倍),
    or change/action keywords (増加, 減少, 上昇, 下落, 発表, 決定, 買収, 提携, 停止, 開始, 削減).
    """
    why_refs: set[int] = set()
    for sentence in sentences:
        if sentence.kind == "why":
            why_refs.update(sentence.refs)

    target_items = [item for item in items if item.n in why_refs] if why_refs else list(items)

    for item in target_items:
        combined_text = f"{item.title} {item.lede}"
        if re.search(r"\d", combined_text):
            return True
        if any(cue in combined_text for cue in WHY_HINT_CUES):
            return True

    return False


def calculate_specificity(
    text: str,
    tokenizer_obj: Any,
    split_mode: Any,
) -> tuple[int, int, int, float]:
    """Calculate entity and number specificity density using Sudachi.

    Rules:
    - proper_nouns: tokens whose part-of-speech starts with 名詞,固有名詞
    - numbers: tokens with 名詞,数詞 (plus digit regex fallback)
    - tokens: content tokens (excluding particles, auxiliaries, symbols, whitespace)
    - density: (proper_nouns + numbers) / max(tokens, 1)

    Returns:
        (proper_nouns, numbers, content_tokens, density)
    """
    if not text or not text.strip():
        return 0, 0, 0, 0.0

    if tokenizer_obj is None:
        raise TokenizerUnavailableError("Sudachi tokenizer instance is required; failing closed.")

    tokens = tokenizer_obj.tokenize(text, split_mode)
    proper_nouns = 0
    numbers = 0
    content_tokens = 0

    for t in tokens:
        surface = t.surface()
        if not surface.strip():
            continue
        pos = t.part_of_speech()
        is_proper = len(pos) >= 2 and pos[0] == "名詞" and pos[1] == "固有名詞"
        is_num_pos = len(pos) >= 2 and pos[0] == "名詞" and pos[1] == "数詞"
        has_digit = bool(re.search(r"\d", surface))
        is_num = is_num_pos or has_digit

        if is_proper:
            proper_nouns += 1
            content_tokens += 1
        elif is_num:
            numbers += 1
            content_tokens += 1
        else:
            if pos[0] not in NON_CONTENT_POS:
                content_tokens += 1

    density = (proper_nouns + numbers) / max(content_tokens, 1)
    return proper_nouns, numbers, content_tokens, round(float(density), 4)


def compute_attribution(
    sentence_vec: np.ndarray,
    item_vec_map: dict[int, np.ndarray],
    refs: Sequence[int],
    threshold: float,
) -> tuple[float, int | None, bool]:
    """Compute attribution max cosine similarity for cited items.

    attribution.max_cos = max cosine between sentence embedding and embeddings of
    `title + " — " + lede` of the items the sentence cites (refs).
    pass = max_cos >= thresholds.attribution_cos.
    If refs is empty or no cited items found: max_cos = 0.0, best_n = None, pass = False.
    """
    if not refs:
        return 0.0, None, False

    valid_refs = [n for n in refs if n in item_vec_map]
    if not valid_refs:
        return 0.0, None, False

    max_cos = -1.0
    best_n: int | None = None

    for n in valid_refs:
        item_vec = item_vec_map[n]
        cos = float(np.dot(sentence_vec, item_vec))
        if cos > max_cos:
            max_cos = cos
            best_n = n

    max_cos = max(-1.0, min(1.0, max_cos))
    pass_attr = bool(max_cos >= threshold)
    return round(max_cos, 4), best_n, pass_attr


class CardVerifierService:
    """Service verifying topic card sentences against source items."""

    def __init__(
        self,
        embed_service: EmbedService,
        filler_phrases: Sequence[str] = DEFAULT_FILLER_PHRASES,
    ) -> None:
        self.embed_service = embed_service
        self.filler_phrases = list(filler_phrases)
        try:
            from sudachipy import dictionary, tokenizer

            self._tokenizer = dictionary.Dictionary().create()
            self._split_mode = tokenizer.Tokenizer.SplitMode.C
        except Exception as exc:
            logger.error("sudachi tokenizer initialization failed", error=str(exc))
            raise TokenizerUnavailableError(
                f"Sudachi dictionary or tokenizer unavailable: {exc}"
            ) from exc

    def verify_card(self, request: VerifyCardRequest) -> VerifyCardResponse:
        """Verify card sentences against cited items.

        Batches all sentence texts and item texts into a single embedding call.
        Raises BackendEmbeddingError on embedding failure (mapped to 502 in router).
        """
        try:
            model_ident = self.embed_service.resolve_model_identity()
        except Exception as exc:
            logger.error("failed to resolve embedder model identity", error=str(exc))
            raise BackendEmbeddingError(f"Embedding backend identity error: {exc}") from exc

        sentence_texts = [s.text for s in request.sentences]
        item_texts = [f"{item.title} — {item.lede}" for item in request.items]
        all_texts = sentence_texts + item_texts

        if all_texts:
            try:
                embed_res = self.embed_service.embed(all_texts, normalize=True)
            except Exception as exc:
                logger.error("embedding backend failed during card verification", error=str(exc))
                raise BackendEmbeddingError(f"Embedding backend failed: {exc}") from exc
        else:
            dim = getattr(self.embed_service.embedder, "dim", None) or getattr(
                self.embed_service.embedder, "dimension", None
            )
            if dim is None or int(dim) <= 0:
                raise UnresolvableDimensionError(
                    "Embedder backend did not report embedding dimension."
                )
            embed_res = EmbedResponse(
                model=model_ident,
                dim=int(dim),
                embeddings=[],
            )

        n_sentences = len(sentence_texts)
        sentence_vecs = [
            np.array(vec, dtype=np.float32) for vec in embed_res.embeddings[:n_sentences]
        ]
        item_vecs = [np.array(vec, dtype=np.float32) for vec in embed_res.embeddings[n_sentences:]]
        item_vec_map = {item.n: item_vecs[i] for i, item in enumerate(request.items)}

        sentence_results: list[SentenceVerificationResult] = []
        tok = self._tokenizer
        mode = self._split_mode

        for idx, sentence in enumerate(request.sentences):
            # G3: Attribution
            sent_vec = sentence_vecs[idx]
            max_cos, best_n, attr_pass = compute_attribution(
                sentence_vec=sent_vec,
                item_vec_map=item_vec_map,
                refs=sentence.refs,
                threshold=request.thresholds.attribution_cos,
            )
            attribution = AttributionResult(
                max_cos=max_cos,
                best_n=best_n,
                pass_=attr_pass,
            )

            # G4: Filler
            matched_fillers = match_fillers(sentence.text, self.filler_phrases)
            filler = FillerResult(
                matched=matched_fillers,
                pass_=len(matched_fillers) == 0,
            )

            # G5: Specificity
            proper_nouns, numbers, tokens, density = calculate_specificity(
                sentence.text,
                tokenizer_obj=tok,
                split_mode=mode,
            )
            specificity = SpecificityResult(
                proper_nouns=proper_nouns,
                numbers=numbers,
                tokens=tokens,
                density=density,
            )

            sentence_results.append(
                SentenceVerificationResult(
                    idx=sentence.idx,
                    attribution=attribution,
                    filler=filler,
                    specificity=specificity,
                )
            )

        # G6: Why-hint presence
        why_hint_present = check_why_hint_present(request.sentences, request.items)

        model_ident = self.embed_service.resolve_model_identity()
        embedding_info = EmbeddingInfo(
            model=embed_res.model or model_ident,
            identity=model_ident,
        )

        return VerifyCardResponse(
            sentences=sentence_results,
            why_hint_present=why_hint_present,
            embedding=embedding_info,
        )
