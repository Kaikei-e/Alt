"""
Authentication service integration for tag-generator.
Implements user-specific tag generation with tenant isolation.
Also runs background tag generation service for batch processing.
"""

import inspect
import os
import threading
from contextlib import asynccontextmanager
from dataclasses import dataclass
from functools import wraps
from typing import Any

import structlog

try:  # Prefer shared auth client when available
    from alt_auth.client import AuthClient, AuthConfig, UserContext, require_auth  # type: ignore

    _ALT_AUTH_AVAILABLE = True
except ModuleNotFoundError:
    _ALT_AUTH_AVAILABLE = False

    @dataclass
    class AuthConfig:
        auth_service_url: str
        service_name: str
        service_secret: str
        token_ttl: int = 3600

    @dataclass
    class UserContext:
        user_id: str = "anonymous"
        tenant_id: str = "public"
        roles: tuple[str, ...] = ()
        metadata: dict[str, Any] | None = None

    class AuthClient:
        """Fallback authentication client that performs no external validation."""

        def __init__(self, config: AuthConfig):
            self.config = config

        async def __aenter__(self) -> "AuthClient":
            return self

        async def __aexit__(self, exc_type, exc, tb) -> bool:
            return False

    def _default_user_context() -> UserContext:
        roles_env = os.getenv("DEFAULT_USER_ROLES", "")
        roles: tuple[str, ...] = tuple(role.strip() for role in roles_env.split(",") if role.strip())
        return UserContext(
            user_id=os.getenv("DEFAULT_USER_ID", "anonymous"),
            tenant_id=os.getenv("DEFAULT_TENANT_ID", "public"),
            roles=roles,
        )

    def require_auth(_client: AuthClient | None = None):
        """Fallback decorator that injects a default user context."""

        def decorator(func):
            if inspect.iscoroutinefunction(func):

                @wraps(func)
                async def async_wrapper(*args, **kwargs):
                    kwargs.setdefault("user_context", _default_user_context())
                    return await func(*args, **kwargs)

                return async_wrapper

            @wraps(func)
            def sync_wrapper(*args, **kwargs):
                kwargs.setdefault("user_context", _default_user_context())
                return func(*args, **kwargs)

            return sync_wrapper

        return decorator


from fastapi import FastAPI, HTTPException, Request
from pydantic import BaseModel

from tag_fetcher import fetch_tags_by_article_ids

logger = structlog.get_logger(__name__)

if not _ALT_AUTH_AVAILABLE:
    logger.info("alt_auth.client not found; using no-op authentication stubs")


@dataclass
class TagGenerationRequest:
    article_id: str
    title: str
    content: str
    metadata: dict[str, Any] | None = None


@dataclass
class TagResult:
    tag: str
    confidence: float
    category: str
    source: str = "ml_model"


def get_service_secret() -> str:
    """Get service secret from env var or file."""
    secret = os.getenv("SERVICE_SECRET", "")
    if not secret:
        secret_file = os.getenv("SERVICE_SECRET_FILE")
        if secret_file:
            try:
                with open(secret_file) as f:
                    secret = f.read().strip()
            except Exception as e:
                logger.error(f"Failed to read SERVICE_SECRET_FILE: {e}")
    return secret


class AuthenticatedTagGeneratorService:
    """Enhanced tag generator service with authentication and tenant isolation."""

    def __init__(self):
        self.auth_config = AuthConfig(
            auth_service_url=os.getenv(
                "AUTH_SERVICE_URL",
                "http://auth-service.alt-auth.svc.cluster.local:8080",
            ),
            service_name="tag-generator",
            service_secret=get_service_secret(),
            token_ttl=3600,
        )

        if not self.auth_config.service_secret:
            raise ValueError("SERVICE_SECRET environment variable is required")

        self.auth_client = None
        logger.info(
            "Authenticated tag generator service initialized",
            auth_service_url=self.auth_config.auth_service_url,
            service_name=self.auth_config.service_name,
        )

    async def initialize(self):
        """Initialize the authentication client."""
        self.auth_client = AuthClient(self.auth_config)
        await self.auth_client.__aenter__()
        logger.info("Authentication client initialized")

    async def cleanup(self):
        """Cleanup authentication client."""
        if self.auth_client:
            await self.auth_client.__aexit__(None, None, None)
            logger.info("Authentication client cleaned up")

    async def generate_personalized_tags(
        self, request: TagGenerationRequest, user_context: UserContext
    ) -> list[TagResult]:
        """Generate personalized tags for a user."""
        logger.info(
            "Generating personalized tags",
            user_id=user_context.user_id,
            tenant_id=user_context.tenant_id,
            article_id=request.article_id,
        )

        try:
            # Get user-specific tag preferences
            user_preferences = await self._get_user_tag_preferences(user_context.tenant_id, user_context.user_id)

            # Generate tags using ML model with user preferences
            tags = await self._generate_tags_with_preferences(request, user_preferences)

            # Save generated tags to user-specific storage
            await self._save_user_tags(user_context.tenant_id, user_context.user_id, request.article_id, tags)

            logger.info(
                "Personalized tags generated successfully",
                user_id=user_context.user_id,
                tenant_id=user_context.tenant_id,
                article_id=request.article_id,
                tag_count=len(tags),
            )

            return tags

        except Exception as e:
            logger.error(
                "Failed to generate personalized tags",
                user_id=user_context.user_id,
                tenant_id=user_context.tenant_id,
                article_id=request.article_id,
                error=str(e),
            )
            raise

    async def _get_user_tag_preferences(self, tenant_id: str, user_id: str) -> dict[str, Any]:
        """Get user's tag generation preferences."""
        # TODO: Implement database lookup for user preferences
        # This should be tenant-isolated
        logger.debug("Retrieving user tag preferences", tenant_id=tenant_id, user_id=user_id)

        # Default preferences for now
        return {
            "preferred_categories": ["technology", "science", "business"],
            "confidence_threshold": 0.6,
            "max_tags": 10,
            "language_preference": "en",
        }

    async def _generate_tags_with_preferences(
        self, request: TagGenerationRequest, user_preferences: dict[str, Any]
    ) -> list[TagResult]:
        """Generate tags using ML model with user preferences."""
        # TODO: Integrate with actual ML model
        # This is a simplified implementation for demonstration

        content_text = f"{request.title} {request.content}".lower()

        # Keyword-based tag generation (to be replaced with ML model)
        tag_keywords = {
            "technology": ["tech", "software", "programming", "ai", "machine learning"],
            "science": ["research", "study", "experiment", "analysis", "data"],
            "business": ["market", "company", "revenue", "strategy", "growth"],
            "health": ["health", "medical", "wellness", "fitness", "nutrition"],
            "environment": ["climate", "environment", "sustainability", "green", "eco"],
        }

        tags = []
        preferred_categories = user_preferences.get("preferred_categories", [])
        confidence_threshold = user_preferences.get("confidence_threshold", 0.6)
        max_tags = user_preferences.get("max_tags", 10)

        for category, keywords in tag_keywords.items():
            # Prioritize preferred categories
            category_boost = 0.2 if category in preferred_categories else 0.0

            for keyword in keywords:
                if keyword in content_text:
                    confidence = min(0.8 + category_boost, 1.0)
                    if confidence >= confidence_threshold:
                        tags.append(
                            TagResult(
                                tag=keyword,
                                confidence=confidence,
                                category=category,
                                source="ml_model_personalized",
                            )
                        )

        # Sort by confidence and limit to max_tags
        tags = sorted(tags, key=lambda x: x.confidence, reverse=True)[:max_tags]

        return tags

    async def _save_user_tags(self, tenant_id: str, user_id: str, article_id: str, tags: list[TagResult]) -> None:
        """Save generated tags to user-specific storage."""
        logger.debug(
            "Saving user tags",
            tenant_id=tenant_id,
            user_id=user_id,
            article_id=article_id,
            tag_count=len(tags),
        )

        # TODO: Implement database save with tenant isolation
        # This should save to a tenant-specific table/collection
        pass


# Global service instance
tag_service = AuthenticatedTagGeneratorService()

# Background tag generation service
_background_tag_service = None
_background_thread = None


def _run_background_tag_generation():
    """Run tag generation service in background thread."""
    global _background_tag_service
    try:
        import asyncio

        from tag_generator.config import TagGeneratorConfig
        from tag_generator.service import TagGeneratorService
        from tag_generator.stream_consumer import ConsumerConfig, StreamConsumer
        from tag_generator.stream_event_handler import TagGeneratorEventHandler

        logger.info("Starting background tag generation service")
        config = TagGeneratorConfig()
        _background_tag_service = TagGeneratorService(config)

        # Initialize Redis Streams consumer for on-the-fly tag generation (ADR-168)
        consumer_config = ConsumerConfig.from_env()
        if consumer_config.enabled:
            logger.info(
                "initializing_redis_streams_consumer",
                stream=consumer_config.stream_key,
                group=consumer_config.group_name,
                consumer=consumer_config.consumer_name,
            )
            event_handler = TagGeneratorEventHandler(_background_tag_service)
            consumer = StreamConsumer(consumer_config, event_handler)

            # Inject stream_consumer into event_handler for reply functionality
            event_handler.stream_consumer = consumer

            # Run consumer in separate thread
            def run_consumer() -> None:
                try:
                    asyncio.run(consumer.start())
                except Exception as e:
                    logger.error("Consumer thread error", error=str(e))

            consumer_thread = threading.Thread(
                target=run_consumer,
                daemon=True,
                name="redis-streams-consumer",
            )
            consumer_thread.start()
            logger.info("redis_streams_consumer_started")
        else:
            logger.info("redis_streams_consumer_disabled")

        # Run batch processing service (blocking)
        _background_tag_service.run_service()
    except Exception as e:
        logger.error("Background tag generation service failed", error=str(e), exc_info=True)
        # Don't raise - allow API server to continue running


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan management."""
    global _background_thread, tag_service

    logger.info("FastAPI lifespan: startup phase")

    # Initialize API service
    await tag_service.initialize()
    logger.info("API service initialized")

    # Start background tag generation service in a separate thread
    logger.info("Starting background tag generation service thread")
    _background_thread = threading.Thread(
        target=_run_background_tag_generation, daemon=True, name="tag-generation-service"
    )
    _background_thread.start()
    logger.info("Background tag generation service thread started")

    yield

    # Cleanup
    logger.info("FastAPI lifespan: shutdown phase")
    await tag_service.cleanup()
    # Note: Background thread will be terminated when main process exits
    logger.info("Tag generator service shutting down")


# FastAPI application with authentication
app = FastAPI(title="Tag Generator Service", version="1.0.0", lifespan=lifespan)


@app.post("/api/v1/generate-tags")
@require_auth(tag_service.auth_client)
async def generate_tags_endpoint(request: TagGenerationRequest, user_context: UserContext) -> dict[str, Any]:
    """Generate tags for an article with user authentication."""
    try:
        tags = await tag_service.generate_personalized_tags(request, user_context)

        return {
            "success": True,
            "tags": [
                {
                    "tag": tag.tag,
                    "confidence": tag.confidence,
                    "category": tag.category,
                    "source": tag.source,
                }
                for tag in tags
            ],
            "user_id": user_context.user_id,
            "tenant_id": user_context.tenant_id,
            "article_id": request.article_id,
            "timestamp": "2025-01-01T00:00:00Z",  # TODO: Use actual timestamp
        }

    except Exception as e:
        logger.error(
            "Tag generation endpoint failed",
            user_id=user_context.user_id,
            article_id=request.article_id,
            error=str(e),
        )
        raise HTTPException(status_code=500, detail=f"Tag generation failed: {str(e)}") from e


@app.get("/health")
async def health_check():
    """Health check endpoint."""
    return {"status": "healthy", "service": "tag-generator"}


@app.get("/api/v1/user-preferences")
@require_auth(tag_service.auth_client)
async def get_user_preferences(user_context: UserContext) -> dict[str, Any]:
    """Get user's tag generation preferences."""
    preferences = await tag_service._get_user_tag_preferences(user_context.tenant_id, user_context.user_id)

    return {
        "user_id": user_context.user_id,
        "tenant_id": user_context.tenant_id,
        "preferences": preferences,
    }


def verify_service_token(request: Request) -> None:
    """Verify X-Service-Token header for service-to-service authentication."""
    service_token = request.headers.get("X-Service-Token")
    expected_token = get_service_secret()

    if not expected_token:
        logger.warning("SERVICE_SECRET not configured, rejecting service token authentication")
        raise HTTPException(status_code=500, detail="Service authentication not configured")

    if not service_token:
        logger.warning("Missing X-Service-Token header")
        raise HTTPException(status_code=401, detail="Missing X-Service-Token header")

    if service_token != expected_token:
        logger.warning("Invalid service token provided")
        raise HTTPException(status_code=403, detail="Invalid service token")


class BatchTagsRequest(BaseModel):
    """Request model for batch tag fetching."""

    article_ids: list[str]


@app.post("/api/v1/tags/batch")
async def fetch_tags_batch(
    request: Request,
    body: BatchTagsRequest,
) -> dict[str, Any]:
    """
    Batch fetch tags for multiple articles by their IDs.
    Service-to-service endpoint (requires X-Service-Token header).
    """
    # Verify service token
    verify_service_token(request)

    article_ids = body.article_ids

    if not article_ids:
        raise HTTPException(status_code=400, detail="article_ids must be a non-empty list")

    if len(article_ids) > 1000:
        raise HTTPException(status_code=400, detail="Too many article_ids (max 1000)")

    try:
        logger.info("Fetching tags in batch", article_count=len(article_ids))
        tags_by_article = fetch_tags_by_article_ids(article_ids)

        return {
            "success": True,
            "tags": tags_by_article,
        }

    except Exception as e:
        logger.error("Batch tag fetch failed", error=str(e), article_count=len(article_ids))
        raise HTTPException(status_code=500, detail=f"Failed to fetch tags: {str(e)}") from e


if __name__ == "__main__":
    import uvicorn

    port = int(os.getenv("PORT", "9400"))
    uvicorn.run(app, host="0.0.0.0", port=port)
