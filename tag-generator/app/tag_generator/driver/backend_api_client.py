"""Connect-RPC client for alt-backend's BackendInternalService.

Uses the Connect protocol (HTTP/1.1 + JSON) for service-to-service communication.
"""

from __future__ import annotations

import os
from typing import Any

import httpx
import structlog

logger = structlog.get_logger(__name__)

# Connect-RPC service path
_SERVICE_PATH = "services.backend.v1.BackendInternalService"


class BackendAPIClient:
    """HTTP client for alt-backend's internal Connect-RPC API."""

    def __init__(self, base_url: str, service_token: str) -> None:
        self.base_url = base_url.rstrip("/")
        self.service_token = service_token
        self.client = httpx.Client(timeout=30.0)
        logger.info("BackendAPIClient initialized", base_url=self.base_url)

    def call(self, method: str, payload: dict[str, Any]) -> dict[str, Any]:
        """Call a Connect-RPC unary method.

        Args:
            method: RPC method name (e.g., "ListUntaggedArticles")
            payload: JSON request body

        Returns:
            JSON response body as dict
        """
        url = f"{self.base_url}/{_SERVICE_PATH}/{method}"
        headers: dict[str, str] = {
            "Content-Type": "application/json",
        }
        if self.service_token:
            headers["X-Service-Token"] = self.service_token

        resp = self.client.post(url, json=payload, headers=headers)
        resp.raise_for_status()
        return resp.json()

    def close(self) -> None:
        """Close the underlying HTTP client."""
        self.client.close()

    @classmethod
    def from_env(cls) -> BackendAPIClient | None:
        """Create a client from environment variables.

        Returns None if BACKEND_API_URL is not set.
        """
        base_url = os.getenv("BACKEND_API_URL", "")
        if not base_url:
            return None

        service_token = ""
        token_file = os.getenv("SERVICE_TOKEN_FILE", "")
        if token_file:
            try:
                with open(token_file) as f:
                    service_token = f.read().strip()
            except OSError:
                pass
        if not service_token:
            service_token = os.getenv("SERVICE_TOKEN", "")

        return cls(base_url, service_token)
