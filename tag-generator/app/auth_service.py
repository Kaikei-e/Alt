"""
Authentication service integration for tag-generator.
Implements user-specific tag generation with tenant isolation.
"""

import os

# Import the shared authentication library
import sys
from dataclasses import dataclass
from typing import Any

import structlog

sys.path.append("../../shared/auth-lib-python")

from contextlib import asynccontextmanager

from alt_auth.client import AuthClient, AuthConfig, UserContext, require_auth  # type: ignore
from fastapi import FastAPI, HTTPException

logger = structlog.get_logger(__name__)


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


class AuthenticatedTagGeneratorService:
    """Enhanced tag generator service with authentication and tenant isolation."""

    def __init__(self):
        self.auth_config = AuthConfig(
            auth_service_url=os.getenv(
                "AUTH_SERVICE_URL",
                "http://auth-service.alt-auth.svc.cluster.local:8080",
            ),
            service_name="tag-generator",
            service_secret=os.getenv("SERVICE_SECRET", ""),
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


# FastAPI application with authentication
app = FastAPI(title="Tag Generator Service", version="1.0.0")

# Global service instance
tag_service = AuthenticatedTagGeneratorService()


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan management."""
    await tag_service.initialize()
    yield
    await tag_service.cleanup()


app.router.lifespan_context = lifespan


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


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8000)
