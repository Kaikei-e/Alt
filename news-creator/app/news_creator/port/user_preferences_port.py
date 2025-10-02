"""Port interface for user preferences repository."""

from abc import ABC, abstractmethod
from typing import Dict, Any


class UserPreferencesPort(ABC):
    """Abstract interface for user preferences operations."""

    @abstractmethod
    async def get_user_preferences(
        self,
        tenant_id: str,
        user_id: str
    ) -> Dict[str, Any]:
        """
        Get user's content generation preferences.

        Args:
            tenant_id: Tenant identifier for isolation
            user_id: User identifier

        Returns:
            Dictionary containing user preferences

        Raises:
            ValueError: If tenant_id or user_id is invalid
        """
        pass

    @abstractmethod
    async def save_user_content(
        self,
        tenant_id: str,
        user_id: str,
        topic: str,
        content_data: Dict[str, Any]
    ) -> None:
        """
        Save generated content to user-specific storage.

        Args:
            tenant_id: Tenant identifier for isolation
            user_id: User identifier
            topic: Content topic
            content_data: Generated content data to save

        Raises:
            ValueError: If parameters are invalid
        """
        pass
