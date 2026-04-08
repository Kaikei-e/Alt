"""Options Builder for Ollama LLM requests (Phase 2 refactoring).

This module extracts options building logic from OllamaGateway.generate()
following SOLID principles (Single Responsibility Principle).

Following Python 3.14 best practices:
- Protocol for structural typing
- Immutable data handling
"""

from __future__ import annotations

import logging
from typing import Any, Protocol, Union

from news_creator.config.llm_config import LLMConfig
from news_creator.config.model_routing_config import ModelRoutingConfig

logger = logging.getLogger(__name__)


class OptionsBuilderProtocol(Protocol):
    """Protocol for options building strategies."""

    def build(
        self,
        extra_options: dict[str, Any] | None = None,
        num_predict_override: int | None = None,
    ) -> dict[str, Any]:
        """Build LLM options dictionary."""
        ...


class OptionsBuilder:
    """Builds Ollama LLM options from config and overrides.

    Responsibilities:
    - Build base options from LLMConfig
    - Merge extra options (filtering num_ctx to prevent override)
    - Apply num_predict override

    This class extracts lines 166-183 from OllamaGateway.generate().
    """

    def __init__(self, llm_config: LLMConfig):
        """Initialize with LLM configuration.

        Args:
            llm_config: LLM configuration dataclass
        """
        self._config = llm_config

    def build(
        self,
        extra_options: dict[str, Any] | None = None,
        num_predict_override: int | None = None,
    ) -> dict[str, Any]:
        """Build LLM options dictionary.

        Args:
            extra_options: Additional options to merge (num_ctx will be filtered)
            num_predict_override: Explicit num_predict value (takes precedence)

        Returns:
            Dictionary of LLM options for Ollama API
        """
        # Start with base options from config
        options = self._config.get_options()

        # Merge extra options if provided
        if extra_options:
            # CRITICAL: Remove num_ctx from extra options to prevent override
            # num_ctx is fixed in Modelfile or set by config
            filtered_options = {
                k: v for k, v in extra_options.items() if k != "num_ctx"
            }
            options.update(filtered_options)

        # Apply num_predict override if provided (takes precedence over everything)
        if num_predict_override is not None:
            options["num_predict"] = num_predict_override

        return options


class KeepAliveResolver:
    """Resolves keep_alive value based on model and configuration.

    Responsibilities:
    - Return explicit keep_alive if provided
    - Return model-specific keep_alive (8K vs 60K)
    - Return default keep_alive for unknown models

    This class extracts lines 185-194 from OllamaGateway.generate().
    """

    def __init__(
        self,
        llm_config: LLMConfig,
        routing_config: ModelRoutingConfig,
    ):
        """Initialize with configuration.

        Args:
            llm_config: LLM configuration with keep_alive values
            routing_config: Model routing configuration with model names
        """
        self._llm_config = llm_config
        self._routing_config = routing_config

    def resolve(
        self,
        model: str,
        explicit_keep_alive: Union[int, str] | None = None,
    ) -> Union[int, str]:
        """Resolve keep_alive value for a model.

        Args:
            model: Model name to resolve keep_alive for
            explicit_keep_alive: Explicitly provided keep_alive (takes precedence)

        Returns:
            keep_alive value (int for seconds, str for duration like "24h")
        """
        if explicit_keep_alive is not None:
            return explicit_keep_alive

        # Use model-specific keep_alive
        return self._llm_config.get_keep_alive_for_model(
            model,
            self._routing_config.model_8k_name,
            self._routing_config.model_60k_name,
        )
