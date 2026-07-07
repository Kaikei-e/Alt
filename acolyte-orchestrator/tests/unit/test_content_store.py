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


@pytest.mark.asyncio
async def test_store_evicts_oldest_when_over_capacity() -> None:
    """The store is process-global (main.py:86), so it must cap growth —
    otherwise article bodies accumulate across every run forever."""
    store = MemoryContentStore(max_size=2)
    await store.store("art-1", "Body one.")
    await store.store("art-2", "Body two.")
    await store.store("art-3", "Body three.")  # exceeds capacity -> evict least recently used

    assert await store.fetch("art-1") is None
    assert await store.fetch("art-2") == "Body two."
    assert await store.fetch("art-3") == "Body three."


@pytest.mark.asyncio
async def test_store_lru_access_refreshes_recency() -> None:
    """Fetching an entry must protect it from eviction as the least-recently-used."""
    store = MemoryContentStore(max_size=2)
    await store.store("art-1", "Body one.")
    await store.store("art-2", "Body two.")
    await store.fetch("art-1")  # refresh recency of art-1
    await store.store("art-3", "Body three.")  # should evict art-2, not art-1

    assert await store.fetch("art-1") == "Body one."
    assert await store.fetch("art-2") is None
    assert await store.fetch("art-3") == "Body three."


@pytest.mark.asyncio
async def test_default_construction_is_bounded() -> None:
    """Default construction (no explicit max_size) must still cap growth."""
    store = MemoryContentStore()
    for i in range(3000):
        await store.store(f"art-{i}", f"body-{i}")
    assert len(store) < 3000
