"""Port interfaces for external dependencies."""

from news_creator.port.llm_provider_port import LLMProviderPort
from news_creator.port.auth_port import AuthPort, UserContext
from news_creator.port.user_preferences_port import UserPreferencesPort

__all__ = ["LLMProviderPort", "AuthPort", "UserContext", "UserPreferencesPort"]
