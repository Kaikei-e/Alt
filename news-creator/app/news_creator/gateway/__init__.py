"""Gateway layer - Anti-Corruption Layer for external services."""

from news_creator.gateway.distributing_gateway import DistributingGateway
from news_creator.gateway.ollama_gateway import OllamaGateway

__all__ = ["DistributingGateway", "OllamaGateway"]
