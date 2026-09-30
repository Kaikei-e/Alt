"""Driver for interacting with Irodori-TTS-Server via HTTP."""

from typing import Any

import httpx


class IrodoriHttpDriver:
    """Sends raw HTTP requests to Irodori TTS server."""

    def __init__(self, client: httpx.AsyncClient, api_key: str) -> None:
        self._client = client
        self._api_key = api_key

    async def post_speech(self, payload: dict[str, Any]) -> httpx.Response:
        """POST /v1/audio/speech with Bearer authentication."""
        raise NotImplementedError
