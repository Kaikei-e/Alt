"""Minimal reproduction of SSE stream forwarding with contextlib.aclosing."""

import asyncio
from collections.abc import AsyncGenerator
from contextlib import aclosing


async def forward_stream_chunks(
    original_stream: AsyncGenerator[str],
    data_queue: asyncio.Queue[tuple[str, str] | None],
) -> None:
    try:
        async with aclosing(original_stream) as stream:
            async for chunk in stream:
                await data_queue.put(("data", chunk))
    except Exception:
        await data_queue.put(("error", "summary generation failed"))
    finally:
        # Always emit terminal sentinel so the SSE response finishes cleanly.
        await data_queue.put(None)
