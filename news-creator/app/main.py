"""
News Creator Service with Authentication Integration
LLM-based content generation service with tenant isolation
"""

import os
import asyncio
import json
import logging
from dataclasses import dataclass
from typing import Dict, Any, List, Optional, Tuple, Union

# Import the shared authentication library
import sys
sys.path.append('../../shared/auth-lib-python')

from alt_auth.client import AuthClient, AuthConfig, UserContext, require_auth
from fastapi import FastAPI, HTTPException, Depends, Request
from pydantic import BaseModel, Field
from contextlib import asynccontextmanager
import aiohttp


SUMMARY_PROMPT_TEMPLATE = """<start_of_turn>user
You are an expert multilingual journalist specializing in Japanese news summarization. Your task is to analyze English articles and create comprehensive Japanese summaries that capture the essence while being culturally appropriate for Japanese audiences.

PURPOSE:
- Produce a “詳細寄りの要約” in Japanese that preserves concrete facts (5W1H), numbers, proper nouns, background, and implications without opinion.

STYLE:
- 新聞記事スタイル（常体ベース。「〜だ／である」。敬語は必要最小限）
- 客観・中立。推測や評価語は避ける
- 日本語として自然で流れる文。箇条書きは使わない

LENGTH:
- 最大1500字、目標は1200±300字（圧縮しすぎない）

KEEP THESE DETAILS (do not omit):
- 5W1H（誰・何・いつ・どこ・なぜ・どのように）
- 具体的な数値（少なくとも2点：金額、割合、日付、件数、比較など）
- 固有名詞（人名・組織名・制度名）。初出のみ英語表記を括弧で併記：例）米大統領ジョン・ドウ（John Doe）
- 経緯／背景（なぜ起きたか、過去との連続性）
- 影響・論点（当事者・市場・政策・世論への波及）
- 今後の見通し（次のマイルストーンや日程、未確定事項）
- 可能なら短い重要引用を1箇所だけ「」で（20字以内）

OUTPUT STRUCTURE:
- 段落1＝リード（最重要事実＋結論＋5W1Hを2–3文で）
- 段落2以降＝詳細（背景→影響→見通しの順で6–10文）
- ラベルや前置きは付けず、本文から開始すること

RULES:
- 数字は半角、単位は日本語。日付・時刻・金額・比率は具体表記
- 原文になかった情報は推測しない。不足は「未提示」と明記
- 複数ソースが混在する場合は重複を排し、食い違いは併記（媒体名のみ簡潔に）

ARTICLE TO SUMMARIZE:
---
{content}
---

Begin directly with the lead paragraph in Japanese, then continue the detailed body.
<end_of_turn>
<start_of_turn>model
"""


class SummarizeRequest(BaseModel):
    article_id: str = Field(min_length=1)
    content: str = Field(min_length=1)


class SummarizeResponse(BaseModel):
    success: bool
    article_id: str
    summary: str
    model: str
    prompt_tokens: Optional[int] = None
    completion_tokens: Optional[int] = None
    total_duration_ms: Optional[float] = None


class GenerateRequest(BaseModel):
    model: Optional[str] = None
    prompt: str = Field(min_length=1)
    stream: bool = False
    keep_alive: Optional[Union[int, str]] = None
    options: Dict[str, Any] = Field(default_factory=dict)

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

@dataclass
class NewsGenerationRequest:
    topic: str
    style: str = "news"  # news, blog, summary
    max_length: int = 500
    language: str = "en"
    metadata: Optional[Dict[str, Any]] = None

@dataclass
class GeneratedContent:
    content: str
    title: str
    summary: str
    confidence: float
    word_count: int
    language: str
    metadata: Dict[str, Any]

class AuthenticatedNewsCreatorService:
    """News creator service with authentication and tenant isolation."""

    def __init__(self):
        self.auth_config = AuthConfig(
            auth_service_url=os.getenv("AUTH_SERVICE_URL", "http://auth-service.alt-auth.svc.cluster.local:8080"),
            service_name="news-creator",
            service_secret=os.getenv("SERVICE_SECRET", ""),
            token_ttl=3600
        )

        if not self.auth_config.service_secret:
            raise ValueError("SERVICE_SECRET environment variable is required")

        self.auth_client = None
        self.session: Optional[aiohttp.ClientSession] = None

        self.llm_service_url = os.getenv("LLM_SERVICE_URL", "http://localhost:11434")
        self.model_name = os.getenv("LLM_MODEL", "gemma3:4b")
        self.keep_alive = int(os.getenv("LLM_KEEP_ALIVE_SECONDS", "-1"))
        self.llm_timeout_seconds = int(os.getenv("LLM_TIMEOUT_SECONDS", "60"))
        self.summary_num_predict = int(os.getenv("SUMMARY_NUM_PREDICT", "500"))

        def _env_float(name: str, default: float) -> float:
            try:
                return float(os.getenv(name, default))
            except ValueError:
                logger.warning("Invalid float for %s. Using default %s", name, default)
                return default

        def _env_int(name: str, default: int) -> int:
            try:
                return int(os.getenv(name, default))
            except ValueError:
                logger.warning("Invalid int for %s. Using default %s", name, default)
                return default

        self.default_llm_options: Dict[str, Any] = {
            "temperature": _env_float("LLM_TEMPERATURE", 0.0),
            "top_p": _env_float("LLM_TOP_P", 0.9),
            "num_predict": _env_int("LLM_NUM_PREDICT", 500),
            "repeat_penalty": _env_float("LLM_REPEAT_PENALTY", 1.0),
            "num_ctx": _env_int("LLM_NUM_CTX", 8192),
            "stop": [token.strip() for token in os.getenv(
                "LLM_STOP_TOKENS",
                "<|user|>,<|system|>"
            ).split(",") if token.strip()],
        }

        if not self.default_llm_options["stop"]:
            self.default_llm_options["stop"] = ["<|user|>", "<|system|>"]

        logger.info("Authenticated news creator service initialized",
                   extra={
                       "auth_service_url": self.auth_config.auth_service_url,
                       "service_name": self.auth_config.service_name,
                       "llm_service_url": self.llm_service_url,
                       "model": self.model_name
                   })

    async def initialize(self):
        """Initialize the authentication client."""
        self.auth_client = AuthClient(self.auth_config)
        await self.auth_client.__aenter__()
        timeout = aiohttp.ClientTimeout(total=self.llm_timeout_seconds)
        self.session = aiohttp.ClientSession(timeout=timeout)
        logger.info("Authentication client initialized")

    async def cleanup(self):
        """Cleanup authentication client."""
        if self.session and not self.session.closed:
            await self.session.close()
        if self.auth_client:
            await self.auth_client.__aexit__(None, None, None)
        logger.info("Authentication client cleaned up")

    async def generate_personalized_content(
        self,
        request: NewsGenerationRequest,
        user_context: UserContext
    ) -> GeneratedContent:
        """Generate personalized content for a user."""
        logger.info("Generating personalized content",
                   extra={
                       "user_id": user_context.user_id,
                       "tenant_id": user_context.tenant_id,
                       "topic": request.topic,
                       "style": request.style
                   })

        try:
            # Get user-specific content preferences
            user_preferences = await self._get_user_content_preferences(
                user_context.tenant_id,
                user_context.user_id
            )

            # Generate content using LLM with user preferences
            content = await self._generate_content_with_preferences(
                request,
                user_preferences
            )

            # Save generated content to user-specific storage
            await self._save_user_content(
                user_context.tenant_id,
                user_context.user_id,
                request.topic,
                content
            )

            logger.info("Personalized content generated successfully",
                       extra={
                           "user_id": user_context.user_id,
                           "tenant_id": user_context.tenant_id,
                           "topic": request.topic,
                           "word_count": content.word_count
                       })

            return content

        except Exception as e:
            logger.error("Failed to generate personalized content",
                        extra={
                            "user_id": user_context.user_id,
                            "tenant_id": user_context.tenant_id,
                            "topic": request.topic,
                            "error": str(e)
                        })
            raise

    async def _get_user_content_preferences(
        self,
        tenant_id: str,
        user_id: str
    ) -> Dict[str, Any]:
        """Get user's content generation preferences."""
        logger.debug("Retrieving user content preferences",
                    extra={"tenant_id": tenant_id, "user_id": user_id})

        # TODO: Implement database lookup for user preferences
        # This should be tenant-isolated
        return {
            "preferred_style": "professional",
            "preferred_length": "medium",
            "tone": "informative",
            "language": "en",
            "complexity_level": "intermediate"
        }

    async def _generate_content_with_preferences(
        self,
        request: NewsGenerationRequest,
        user_preferences: Dict[str, Any]
    ) -> GeneratedContent:
        """Generate content using LLM with user preferences."""
        try:
            # Build prompt based on user preferences
            prompt = self._build_personalized_prompt(request, user_preferences)

            # Call LLM service (simplified implementation)
            llm_response = await self._call_llm_service(prompt, request.max_length)
            generated_text = llm_response.get("response", "")

            # Parse and structure the response
            content = self._parse_generated_content(generated_text, request)

            return content

        except Exception as e:
            logger.error("Content generation failed", extra={"error": str(e)})
            raise

    def _build_personalized_prompt(
        self,
        request: NewsGenerationRequest,
        user_preferences: Dict[str, Any]
    ) -> str:
        """Build a personalized prompt for the LLM."""
        style = user_preferences.get("preferred_style", "professional")
        tone = user_preferences.get("tone", "informative")
        complexity = user_preferences.get("complexity_level", "intermediate")

        prompt = f"""
Generate a {request.style} article about "{request.topic}" with the following specifications:
- Style: {style}
- Tone: {tone}
- Complexity level: {complexity}
- Maximum length: {request.max_length} words
- Language: {request.language}

The content should be engaging, accurate, and appropriate for the specified style and tone.
Please include a clear title and a brief summary.

Topic: {request.topic}
"""
        return prompt.strip()

    async def _call_llm_service(
        self,
        prompt: str,
        max_length: int,
        *,
        num_predict_override: Optional[int] = None,
        model_override: Optional[str] = None,
        stream_override: Optional[bool] = None,
        keep_alive_override: Optional[Union[int, str]] = None,
        options_override: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """Call the locally hosted Gemma (Ollama) service with the provided prompt."""
        if not prompt or not prompt.strip():
            raise ValueError("Prompt cannot be empty")

        if self.session is None or self.session.closed:
            timeout = aiohttp.ClientTimeout(total=self.llm_timeout_seconds)
            self.session = aiohttp.ClientSession(timeout=timeout)

        # Derive num_predict from request length if override not provided.
        options = dict(self.default_llm_options)
        if "stop" in options and isinstance(options["stop"], list):
            options["stop"] = list(options["stop"])

        if options_override:
            for key, value in options_override.items():
                if key == "stop" and isinstance(value, list):
                    options[key] = [str(token) for token in value]
                else:
                    options[key] = value

        effective_max = max_length or options.get("num_predict", 500)
        if num_predict_override is not None:
            options["num_predict"] = num_predict_override
        else:
            # Rough heuristic: assume 1 word ≈ 1.33 tokens
            estimated_tokens = int(effective_max * 1.33)
            options["num_predict"] = min(
                max(options.get("num_predict", 500), estimated_tokens),
                options.get("num_ctx", 8192)
            )

        payload = {
            "model": model_override or self.model_name,
            "prompt": prompt,
            "stream": False if stream_override is None else stream_override,
            "keep_alive": self.keep_alive if keep_alive_override is None else keep_alive_override,
            "options": options,
        }

        url = f"{self.llm_service_url.rstrip('/')}/api/generate"
        logger.debug("Sending prompt to LLM", extra={"url": url, "model": self.model_name})

        async with self.session.post(url, json=payload) as response:
            text_body = await response.text()
            if response.status != 200:
                logger.error(
                    "LLM service returned non-200 response",
                    extra={
                        "status": response.status,
                        "body": text_body[:500],
                        "model": self.model_name,
                    },
                )
                raise RuntimeError(f"LLM service error: {response.status}")

        try:
            parsed = json.loads(text_body)
        except json.JSONDecodeError as err:
            logger.error("Failed to decode LLM response", extra={"error": str(err)})
            raise RuntimeError("Failed to decode LLM response") from err

        if "response" not in parsed:
            logger.error("LLM response missing 'response' field", extra={"keys": list(parsed.keys())})
            raise RuntimeError("Unexpected LLM response format")

        return parsed

    def _parse_generated_content(
        self,
        generated_text: str,
        request: NewsGenerationRequest
    ) -> GeneratedContent:
        """Parse the generated text into structured content."""
        lines = generated_text.split('\n')

        title = ""
        summary = ""
        content = ""

        current_section = None

        for line in lines:
            line = line.strip()
            if line.startswith("Title:"):
                title = line.replace("Title:", "").strip()
                current_section = "title"
            elif line.startswith("Summary:"):
                summary = line.replace("Summary:", "").strip()
                current_section = "summary"
            elif line.startswith("Content:"):
                content = line.replace("Content:", "").strip()
                current_section = "content"
            elif line and current_section == "content":
                content += " " + line

        word_count = len(content.split()) if content else 0

        return GeneratedContent(
            content=content,
            title=title or f"Article about {request.topic}",
            summary=summary or "A generated article summary",
            confidence=0.85,  # Mock confidence score
            word_count=word_count,
            language=request.language,
            metadata={
                "generation_model": self.model_name,
                "style": request.style,
                "max_requested_length": request.max_length
            }
        )

    async def generate_summary(self, article_id: str, content: str) -> Tuple[str, Dict[str, Any]]:
        """Generate a Japanese summary for a full article body."""
        if not article_id.strip():
            raise ValueError("article_id cannot be empty")
        if not content or not content.strip():
            raise ValueError("content cannot be empty")

        prompt = SUMMARY_PROMPT_TEMPLATE.format(content=content.strip())

        llm_response = await self._call_llm_service(
            prompt,
            max_length=len(content.split()),
            num_predict_override=self.summary_num_predict,
        )

        raw_summary = llm_response.get("response", "")
        cleaned_summary = self._clean_summary_text(raw_summary)

        if not cleaned_summary:
            raise RuntimeError("LLM returned an empty summary")

        metadata = {
            "model": llm_response.get("model", self.model_name),
            "prompt_tokens": llm_response.get("prompt_eval_count"),
            "completion_tokens": llm_response.get("eval_count"),
            "total_duration_ms": self._nanoseconds_to_milliseconds(
                llm_response.get("total_duration")
            ),
        }

        logger.info(
            "Summary generated successfully",
            extra={
                "article_id": article_id,
                "summary_length": len(cleaned_summary),
                "model": metadata["model"],
            },
        )

        # Enforce 1500 character max as per prompt guidance.
        truncated_summary = cleaned_summary[:1500]

        return truncated_summary, metadata

    async def forward_generate_request(self, request: GenerateRequest) -> Dict[str, Any]:
        """Process raw Ollama-compatible generate requests."""
        prompt = request.prompt.strip()
        if not prompt:
            raise ValueError("prompt cannot be empty")

        options_override = dict(request.options or {})
        num_predict_override: Optional[int] = None
        if "num_predict" in options_override:
            raw_num_predict = options_override.pop("num_predict")
            try:
                num_predict_override = int(raw_num_predict)
            except (TypeError, ValueError):
                logger.warning(
                    "Invalid num_predict override provided; falling back to heuristic",
                    extra={"value": raw_num_predict},
                )
                num_predict_override = None

        llm_response = await self._call_llm_service(
            prompt=prompt,
            max_length=len(prompt.split()),
            num_predict_override=num_predict_override,
            model_override=request.model or self.model_name,
            stream_override=request.stream,
            keep_alive_override=request.keep_alive,
            options_override=options_override,
        )

        llm_response.setdefault("model", request.model or self.model_name)
        llm_response.setdefault("done", True)
        llm_response.setdefault("done_reason", llm_response.get("done_reason", "stop"))

        return llm_response

    @staticmethod
    def _nanoseconds_to_milliseconds(value: Optional[int]) -> Optional[float]:
        if value is None:
            return None
        try:
            return value / 1_000_000
        except TypeError:
            return None

    @staticmethod
    def _clean_summary_text(content: str) -> str:
        if not content:
            return ""

        cleaned = (
            content.replace("<|system|>", "")
            .replace("<|user|>", "")
            .replace("<|assistant|>", "")
        )

        lines = cleaned.splitlines()
        final_lines: List[str] = []

        for line in lines:
            stripped = line.strip()
            if not stripped:
                continue
            if stripped.startswith("---") or stripped.startswith("**"):
                continue
            if stripped.lower().startswith("summary:") or "要約:" in stripped:
                stripped = stripped.replace("Summary:", "").replace("要約:", "").strip()
                if not stripped:
                    continue
            final_lines.append(stripped)

        return " ".join(final_lines).strip()

    async def _save_user_content(
        self,
        tenant_id: str,
        user_id: str,
        topic: str,
        content: GeneratedContent
    ) -> None:
        """Save generated content to user-specific storage."""
        logger.debug("Saving user content",
                    extra={
                        "tenant_id": tenant_id,
                        "user_id": user_id,
                        "topic": topic,
                        "word_count": content.word_count
                    })

        # TODO: Implement database save with tenant isolation
        # This should save to a tenant-specific table/collection
        pass

# FastAPI application with authentication
app = FastAPI(title="News Creator Service", version="1.0.0")

# Global service instance
news_service = AuthenticatedNewsCreatorService()

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan management."""
    await news_service.initialize()
    yield
    await news_service.cleanup()

app.router.lifespan_context = lifespan


@app.post("/api/generate")
async def ollama_generate_endpoint(request: GenerateRequest) -> Dict[str, Any]:
    try:
        return await news_service.forward_generate_request(request)
    except ValueError as exc:
        logger.warning("Invalid /api/generate payload", extra={"error": str(exc)})
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except RuntimeError as exc:
        logger.error(
            "LLM generate request failed",
            extra={"error": str(exc)},
        )
        raise HTTPException(status_code=502, detail=str(exc)) from exc
    except Exception as exc:  # pragma: no cover - defensive fallback
        logger.exception("Unhandled error in /api/generate")
        raise HTTPException(status_code=500, detail="Internal server error") from exc


@app.post("/api/v1/summarize", response_model=SummarizeResponse)
async def summarize_endpoint(request: SummarizeRequest) -> SummarizeResponse:
    """Generate a Japanese summary using locally hosted Gemma (Ollama)."""
    try:
        summary, metadata = await news_service.generate_summary(
            article_id=request.article_id,
            content=request.content,
        )

        return SummarizeResponse(
            success=True,
            article_id=request.article_id,
            summary=summary,
            model=metadata.get("model", news_service.model_name),
            prompt_tokens=metadata.get("prompt_tokens"),
            completion_tokens=metadata.get("completion_tokens"),
            total_duration_ms=metadata.get("total_duration_ms"),
        )

    except ValueError as exc:
        logger.warning("Invalid summarize request", extra={"error": str(exc)})
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except RuntimeError as exc:
        logger.error(
            "Failed to generate summary",
            extra={"error": str(exc), "article_id": request.article_id},
        )
        raise HTTPException(status_code=502, detail=str(exc)) from exc
    except Exception as exc:  # pragma: no cover - defensive fallback
        logger.exception(
            "Unexpected error while generating summary",
            extra={"article_id": request.article_id},
        )
        raise HTTPException(status_code=500, detail="Internal server error") from exc

@app.post("/api/v1/generate-content")
@require_auth(news_service.auth_client)
async def generate_content_endpoint(
    request: NewsGenerationRequest,
    user_context: UserContext
) -> Dict[str, Any]:
    """Generate content with user authentication."""
    try:
        content = await news_service.generate_personalized_content(request, user_context)

        return {
            "success": True,
            "content": {
                "title": content.title,
                "summary": content.summary,
                "content": content.content,
                "word_count": content.word_count,
                "confidence": content.confidence,
                "language": content.language,
                "metadata": content.metadata
            },
            "user_id": user_context.user_id,
            "tenant_id": user_context.tenant_id,
            "topic": request.topic,
            "timestamp": "2025-01-01T00:00:00Z"  # TODO: Use actual timestamp
        }

    except Exception as e:
        logger.error("Content generation endpoint failed",
                    extra={
                        "user_id": user_context.user_id,
                        "topic": request.topic,
                        "error": str(e)
                    })
        raise HTTPException(status_code=500, detail=f"Content generation failed: {str(e)}")

@app.get("/health")
async def health_check():
    """Health check endpoint."""
    return {"status": "healthy", "service": "news-creator"}

@app.get("/api/v1/user-preferences")
@require_auth(news_service.auth_client)
async def get_user_content_preferences(user_context: UserContext) -> Dict[str, Any]:
    """Get user's content generation preferences."""
    preferences = await news_service._get_user_content_preferences(
        user_context.tenant_id,
        user_context.user_id
    )

    return {
        "user_id": user_context.user_id,
        "tenant_id": user_context.tenant_id,
        "preferences": preferences
    }

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8001)
