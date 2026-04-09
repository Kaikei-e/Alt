"""Unit tests for ContentStore — run-scoped article body cache."""

import pytest

from acolyte.gateway.memory_content_store import MemoryContentStore


@pytest.mark.asyncio
async def test_store_and_fetch() -> None:
    store = MemoryContentStore()
    await store.store("art-1", "Full article body text.")
    result = await store.fetch("art-1")
    assert result == "Full article body text."


@pytest.mark.asyncio
async def test_fetch_missing_returns_none() -> None:
    store = MemoryContentStore()
    result = await store.fetch("nonexistent")
    assert result is None


@pytest.mark.asyncio
async def test_store_overwrites() -> None:
    store = MemoryContentStore()
    await store.store("art-1", "First version.")
    await store.store("art-1", "Second version.")
    result = await store.fetch("art-1")
    assert result == "Second version."


@pytest.mark.asyncio
async def test_fetch_multiple() -> None:
    store = MemoryContentStore()
    await store.store("art-1", "Body one.")
    await store.store("art-2", "Body two.")
    ids = ["art-1", "art-2", "art-3"]
    results = await store.fetch_many(ids)
    assert results == {"art-1": "Body one.", "art-2": "Body two."}
