"""Learning client port: Protocol for communicating with recap-worker learning endpoint."""

from __future__ import annotations

from typing import Any, Protocol, runtime_checkable


@runtime_checkable
class LearningClientPort(Protocol):
    """Port for sending genre learning payloads to recap-worker."""

    async def send_learning_payload(
        self, payload: dict[str, Any]
    ) -> dict[str, Any]:
        """Send a learning payload and return the response.

        Args:
            payload: Genre learning data to send.

        Returns:
            Response dict from the learning endpoint.

        Raises:
            RuntimeError: If the request fails.
        """
        ...

    async def close(self) -> None:
        """Release HTTP client resources."""
        ...
