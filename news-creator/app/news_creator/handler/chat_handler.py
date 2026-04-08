"""Chat proxy handler — routes Ollama /api/chat through HybridPrioritySemaphore.

Ask Augur (rag-orchestrator) sends streaming chat requests directly to the Ollama
backend, bypassing the priority semaphore. This proxy ensures Ask Augur requests
acquire a HIGH PRIORITY slot before reaching Ollama, preventing GPU starvation
by batch summarization jobs.
"""

import json
import logging
from typing import Any, Dict, List, Optional

from fastapi import APIRouter, HTTPException
from fastapi.responses import JSONResponse, StreamingResponse
from pydantic import BaseModel, Field

from news_creator.gateway.hybrid_priority_semaphore import QueueFullError
from news_creator.gateway.ollama_gateway import OllamaGateway
from news_creator.utils.context_logger import (
    clear_context,
    set_ai_pipeline,
    set_processing_stage,
)

logger = logging.getLogger(__name__)


class ChatMessage(BaseModel):
    role: str
    content: str


class ChatRequest(BaseModel):
    model: Optional[str] = None
    messages: List[ChatMessage] = Field(min_length=1)
    stream: bool = True
    keep_alive: Optional[Any] = None
    format: Optional[Any] = None
    think: Optional[Any] = None
    options: Optional[Dict[str, Any]] = None


def create_chat_router(gateway: OllamaGateway) -> APIRouter:
    """Create chat proxy router with dependency injection."""
    router = APIRouter()

    @router.post("/api/chat")
    async def chat_endpoint(request: ChatRequest):
        """Proxy Ollama /api/chat through HybridPrioritySemaphore with HIGH priority."""
        set_ai_pipeline("chat-proxy")
        set_processing_stage("handler")

        try:
            if not request.stream:
                # Non-streaming chat (used by morning letter)
                payload: Dict[str, Any] = {
                    "messages": [
                        {"role": m.role, "content": m.content} for m in request.messages
                    ],
                }
                if request.model:
                    payload["model"] = request.model
                if request.keep_alive is not None:
                    payload["keep_alive"] = request.keep_alive
                if request.format is not None:
                    payload["format"] = request.format
                if request.think is not None:
                    payload["think"] = request.think
                if request.options is not None:
                    payload["options"] = request.options

                result = await gateway.chat_generate(payload=payload)
                return JSONResponse(content=result)

            # Build payload preserving all Ollama fields
            payload: Dict[str, Any] = {
                "messages": [
                    {"role": m.role, "content": m.content} for m in request.messages
                ],
                "stream": True,
            }
            if request.model:
                payload["model"] = request.model
            if request.keep_alive is not None:
                payload["keep_alive"] = request.keep_alive
            if request.format is not None:
                payload["format"] = request.format
            if request.think is not None:
                payload["think"] = request.think
            if request.options is not None:
                payload["options"] = request.options

            stream_iter = await gateway.chat_stream(payload=payload)

            async def ndjson_generator():
                async for chunk in stream_iter:
                    yield json.dumps(chunk, ensure_ascii=False) + "\n"

            return StreamingResponse(
                ndjson_generator(),
                media_type="application/x-ndjson",
                headers={
                    "Cache-Control": "no-cache",
                    "Connection": "keep-alive",
                    "X-Accel-Buffering": "no",
                },
            )

        except QueueFullError:
            logger.warning("Chat proxy: queue full, returning 429")
            return JSONResponse(
                status_code=429,
                content={"error": "queue full"},
                headers={"Retry-After": "30"},
            )

        except HTTPException:
            raise

        except RuntimeError as exc:
            logger.error("Chat proxy: upstream error", extra={"error": str(exc)})
            raise HTTPException(
                status_code=502, detail="Upstream service error"
            ) from exc

        except Exception as exc:
            logger.exception("Chat proxy: unexpected error")
            raise HTTPException(
                status_code=500, detail="Internal server error"
            ) from exc

        finally:
            clear_context()

    return router
