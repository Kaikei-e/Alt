"""Model warmup service for preloading models with keep_alive."""

import asyncio
import logging
from typing import Optional

from news_creator.config.config import NewsCreatorConfig
from news_creator.driver.ollama_driver import OllamaDriver

logger = logging.getLogger(__name__)


class ModelWarmupService:
    """Service for warming up Ollama models with keep_alive."""

    def __init__(self, config: NewsCreatorConfig, driver: OllamaDriver):
        """
        Initialize model warmup service.

        Args:
            config: News Creator configuration
            driver: Ollama driver instance
        """
        self.config = config
        self.driver = driver

    async def warmup_models(self) -> None:
        """
        Warm up models based on configuration.

        - 2-model mode: Warm up 16K model only (80K is loaded on-demand)
        - Single model mode: Warm up default model
        """
        if not self.config.warmup_enabled:
            logger.info("Model warmup is disabled")
            return

        # Use longer keep_alive to ensure 8K model stays in GPU memory
        # Default is 30 minutes, but we use 24h to match entrypoint.sh
        keep_alive_minutes = max(self.config.warmup_keep_alive_minutes, 1440)  # At least 24h
        keep_alive_str = f"{keep_alive_minutes}m"

        try:
            models_to_warmup = []
            if self.config.model_routing_enabled:
                # RTX 4060最適化: 16Kモデルのみをウォームアップ（80Kはオンデマンドでロード）
                # models_to_warmup = [
                #     self.config.model_8k_name,  # 8kモデルは使用しない
                # ]
                models_to_warmup = [
                    self.config.model_16k_name,
                ]
                logger.info(
                    f"Warming up 16K model only (80K will be loaded on-demand when needed)"
                )
            else:
                # Single model mode: warm up default model
                models_to_warmup = [self.config.model_name]

            if len(models_to_warmup) > 0:
                logger.info(
                    f"Warming up {len(models_to_warmup)} model(s): {', '.join(models_to_warmup)} "
                    f"(keep_alive: {keep_alive_str})"
                )

            # Warm up models in parallel
            warmup_tasks = [
                self._warmup_single_model(model, keep_alive_str)
                for model in models_to_warmup
            ]
            results = await asyncio.gather(*warmup_tasks, return_exceptions=True)

            # Log results (errors only)
            for model, result in zip(models_to_warmup, results):
                if isinstance(result, Exception):
                    logger.warning(
                        f"Failed to warm up model {model}: {result}",
                        exc_info=result,
                    )

        except Exception as e:
            logger.error(
                f"Error during model warmup: {e}. Continuing without warmup.",
                exc_info=True,
            )

    async def _warmup_single_model(
        self, model_name: str, keep_alive: str
    ) -> None:
        """
        Warm up a single model.

        Args:
            model_name: Name of the model to warm up
            keep_alive: Keep-alive duration string (e.g., "30m")
        """
        try:
            # Use a simple ping message to warm up the model
            payload = {
                "model": model_name,
                "prompt": "ping",
                "stream": False,
                "keep_alive": keep_alive,
                "options": {
                    "num_predict": 1,  # Minimal generation
                },
            }

            logger.debug(f"Warming up model {model_name}...")
            response = await self.driver.generate(payload)

            if "response" in response:
                logger.debug(f"Model {model_name} warmed up (keep_alive: {keep_alive})")
            else:
                logger.warning(
                    f"Model {model_name} warmup response missing 'response' field",
                    extra={"response_keys": list(response.keys())},
                )

        except Exception as e:
            logger.error(
                f"Failed to warm up model {model_name}: {e}",
                exc_info=True,
            )
            raise

