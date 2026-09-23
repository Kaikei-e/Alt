"""Card verification endpoint router."""

from __future__ import annotations

import asyncio

import structlog
from fastapi import APIRouter, Depends, HTTPException, status

from ...services.card_verifier import (
    BackendEmbeddingError,
    CardVerifierService,
    TokenizerUnavailableError,
    VerifyCardRequest,
    VerifyCardResponse,
)
from ...services.embed_service import EmbedderError
from ..deps import get_card_verifier_service_dep

logger = structlog.get_logger(__name__)

router = APIRouter()


@router.post("/verify", response_model=VerifyCardResponse)
async def verify_card_endpoint(
    request: VerifyCardRequest,
    service: CardVerifierService = Depends(get_card_verifier_service_dep),
) -> VerifyCardResponse:
    """Verify generated topic card sentences against cited source items.

    Evaluates:
    - G3 Attribution similarity: sentence embedding vs cited item title+lede embeddings
    - G4 Filler rule: detects speculation/filler phrases
    - G5 Specificity: entity (proper nouns) and number density via Sudachi
    - G6 Why-hint: detects result/effect/numerical cues in source evidence
    """
    try:
        return await asyncio.to_thread(service.verify_card, request=request)
    except TokenizerUnavailableError as exc:
        logger.error("tokenizer unavailable during card verification", error=str(exc))
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail={"reason": "Tokenizer service unavailable"},
        ) from exc
    except (BackendEmbeddingError, EmbedderError) as exc:
        logger.error("embedding backend failed during card verification", error=str(exc))
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail={"reason": "Embedding service unavailable"},
        ) from exc
    except Exception as exc:
        logger.error("card verification backend error", error=str(exc))
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail={"reason": "Card verification backend error"},
        ) from exc
