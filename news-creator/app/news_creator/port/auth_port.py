"""Port interface for authentication service."""

from abc import ABC, abstractmethod
from typing import Tuple


try:
    from alt_auth.client import UserContext  # type: ignore
except ModuleNotFoundError:
    from dataclasses import dataclass
    from typing import Dict, Any

    @dataclass
    class UserContext:
        """Fallback UserContext when alt_auth is not available."""
        user_id: str = "anonymous"
        tenant_id: str = "public"
        roles: Tuple[str, ...] = ()
        metadata: Dict[str, Any] | None = None


class AuthPort(ABC):
    """Abstract interface for authentication operations."""

    @abstractmethod
    async def initialize(self) -> None:
        """Initialize the authentication client."""
        pass

    @abstractmethod
    async def cleanup(self) -> None:
        """Cleanup authentication client resources."""
        pass
