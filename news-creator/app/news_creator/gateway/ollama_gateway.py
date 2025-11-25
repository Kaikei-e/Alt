"""Ollama Gateway - implements LLMProviderPort."""

import logging
from typing import Dict, Any, Optional, Union

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import LLMGenerateResponse
from news_creator.driver.ollama_driver import OllamaDriver
from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)


class OllamaGateway(LLMProviderPort):
    """Gateway for Ollama LLM service - Anti-Corruption Layer."""

    def __init__(self, config: NewsCreatorConfig):
        """Initialize Ollama gateway."""
        self.config = config
        self.driver = OllamaDriver(config)

    async def initialize(self) -> None:
        """Initialize the Ollama driver."""
        await self.driver.initialize()
        logger.info("Ollama gateway initialized")

    async def cleanup(self) -> None:
        """Cleanup Ollama driver resources."""
        await self.driver.cleanup()
        logger.info("Ollama gateway cleaned up")

    async def generate(
        self,
        prompt: str,
        *,
        model: Optional[str] = None,
        num_predict: Optional[int] = None,
        stream: bool = False,
        keep_alive: Optional[Union[int, str]] = None,
        format: Optional[str] = None,
        options: Optional[Dict[str, Any]] = None,
    ) -> LLMGenerateResponse:
        """
        Generate text using Ollama.

        Args:
            prompt: Input prompt
            model: Optional model name override
            num_predict: Optional max tokens override
            stream: Whether to stream response
            keep_alive: Keep-alive duration
            format: Optional output format (e.g., "json" for structured output)
            options: Additional generation options

        Returns:
            LLMGenerateResponse with generated text

        Raises:
            ValueError: If prompt is empty
            RuntimeError: If Ollama service fails
        """
        if not prompt or not prompt.strip():
            raise ValueError("prompt cannot be empty")

        # Build options from config and overrides
        llm_options = self.config.get_llm_options()
        if options:
            llm_options.update(options)

        # Apply num_predict override if provided
        if num_predict is not None:
            llm_options["num_predict"] = num_predict

        # Build payload for Ollama API
        payload: Dict[str, Any] = {
            "model": model or self.config.model_name,
            "prompt": prompt.strip(),
            "stream": stream,
            "keep_alive": keep_alive if keep_alive is not None else self.config.llm_keep_alive,
            "options": llm_options,
        }

        # Add format parameter if provided (Ollama structured output)
        if format is not None:
            payload["format"] = format
            logger.debug("Using structured output format", extra={"format": format})

        logger.debug(
            "Generating with Ollama",
            extra={
                "model": payload["model"],
                "prompt_length": len(prompt),
                "num_predict": llm_options.get("num_predict"),
            }
        )

        # Call driver
        response_data = await self.driver.generate(payload)

        # Validate response
        if "response" not in response_data:
            logger.error("Ollama response missing 'response' field", extra={"keys": list(response_data.keys())})
            raise RuntimeError("Invalid Ollama response format")

        # Map to domain model
        return LLMGenerateResponse(
            response=response_data.get("response", ""),
            model=response_data.get("model", payload["model"]),
            done=response_data.get("done"),
            done_reason=response_data.get("done_reason"),
            prompt_eval_count=response_data.get("prompt_eval_count"),
            eval_count=response_data.get("eval_count"),
            total_duration=response_data.get("total_duration"),
        )

    async def list_models(self) -> list[Dict[str, Any]]:
        """
        List available Ollama models.

        Returns:
            List of model dictionaries with name and metadata

        Raises:
            RuntimeError: If Ollama service fails
        """
        try:
            tags_response = await self.driver.list_tags()
            models = tags_response.get("models", [])
            logger.debug(f"Found {len(models)} models in Ollama", extra={"count": len(models)})
            return models
        except Exception as err:
            logger.error("Failed to list Ollama models", extra={"error": str(err)})
            raise RuntimeError(f"Failed to list Ollama models: {err}") from err
