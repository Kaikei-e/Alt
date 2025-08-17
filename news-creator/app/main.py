"""
News Creator Service with Authentication Integration
LLM-based content generation service with tenant isolation
"""

import os
import asyncio
import json
from typing import Dict, Any, List, Optional
from dataclasses import dataclass
import logging

# Import the shared authentication library  
import sys
sys.path.append('../../shared/auth-lib-python')

from alt_auth.client import AuthClient, AuthConfig, UserContext, require_auth
from fastapi import FastAPI, HTTPException, Depends, Request
from contextlib import asynccontextmanager
import aiohttp

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
        self.llm_service_url = os.getenv("LLM_SERVICE_URL", "http://localhost:11434")
        
        logger.info("Authenticated news creator service initialized",
                   extra={
                       "auth_service_url": self.auth_config.auth_service_url,
                       "service_name": self.auth_config.service_name,
                       "llm_service_url": self.llm_service_url
                   })

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
            generated_text = await self._call_llm_service(prompt, request.max_length)
            
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

    async def _call_llm_service(self, prompt: str, max_length: int) -> str:
        """Call the LLM service to generate content."""
        # TODO: Integrate with actual LLM service (Ollama, OpenAI, etc.)
        # This is a simplified mock implementation
        
        # Simulate LLM response
        mock_response = f"""
Title: Understanding {prompt.split('"')[1] if '"' in prompt else 'the Topic'}

Summary: A comprehensive overview of the topic, providing key insights and actionable information for readers.

Content: This is a generated article about the specified topic. The content has been tailored to meet the user's preferences and requirements. It provides valuable information while maintaining the requested style and tone. The article is structured to be informative and engaging, offering readers a clear understanding of the subject matter.

The content continues with detailed explanations, examples, and insights that would be valuable for the target audience. Each section builds upon the previous one, creating a cohesive and comprehensive piece that serves the user's specific needs.
"""
        
        # Simulate network delay
        await asyncio.sleep(0.1)
        
        return mock_response.strip()

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
                "generation_model": "mock-llm",
                "style": request.style,
                "max_requested_length": request.max_length
            }
        )

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