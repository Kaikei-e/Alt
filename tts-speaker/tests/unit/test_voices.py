"""Tests for GET /v1/voices endpoint."""

from __future__ import annotations

import pytest
from httpx import AsyncClient


@pytest.mark.asyncio
async def test_voices_returns_list(client: AsyncClient):
    """Voices endpoint returns list of 5 Japanese voices."""
    resp = await client.get("/v1/voices")
    assert resp.status_code == 200
    body = resp.json()
    assert isinstance(body["voices"], list)
    assert len(body["voices"]) == 5


@pytest.mark.asyncio
async def test_voices_ids(client: AsyncClient):
    """Voices endpoint returns expected voice IDs."""
    resp = await client.get("/v1/voices")
    voice_ids = [v["id"] for v in resp.json()["voices"]]
    assert "jf_alpha" in voice_ids
    assert "jf_gongitsune" in voice_ids
    assert "jf_nezumi" in voice_ids
    assert "jf_tebukuro" in voice_ids
    assert "jm_kumo" in voice_ids


@pytest.mark.asyncio
async def test_voices_structure(client: AsyncClient):
    """Each voice has id, name, and gender fields."""
    resp = await client.get("/v1/voices")
    for voice in resp.json()["voices"]:
        assert "id" in voice
        assert "name" in voice
        assert "gender" in voice
