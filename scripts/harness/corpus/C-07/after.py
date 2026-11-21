"""BUG: manual aclose() inside finally aborts before emitting the SSE terminal sentinel."""

import asyncio
from collections.abc import AsyncGenerator


async def forward_stream_chunks(
    original_stream: AsyncGenerator[str],
    data_queue: asyncio.Queue[tuple[str, str] | None],
) -> None:
    try:
        async for chunk in original_stream:
            await data_queue.put(("data", chunk))
    except Exception:
        await data_queue.put(("error", "summary generation failed"))
    finally:
        # BUG: if aclose() raises, the terminal sentinel is never queued and SSE hangs.
        await original_stream.aclose()
        await data_queue.put(None)
