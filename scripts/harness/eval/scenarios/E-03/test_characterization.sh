#!/usr/bin/env bash
set -euo pipefail

WORKSPACE_DIR="${WORKSPACE_DIR:-$(pwd)}"
TARGET="$WORKSPACE_DIR/src/stream.py"

if [[ ! -f "$TARGET" ]]; then
  echo "FAIL: missing target file $TARGET" >&2
  exit 1
fi

python3 -B - "$TARGET" <<'PY'
import asyncio
import importlib.util
import sys

spec = importlib.util.spec_from_file_location("stream_handler_mod", sys.argv[1])
assert spec is not None and spec.loader is not None
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)


class FailingCloseStream:
    def __init__(self, chunks: list[str], fail_close: bool) -> None:
        self._chunks = list(chunks)
        self._fail_close = fail_close
        self.closed = False

    def __aiter__(self):
        return self

    async def __anext__(self) -> str:
        if not self._chunks:
            raise StopAsyncIteration
        return self._chunks.pop(0)

    async def aclose(self) -> None:
        self.closed = True
        if self._fail_close:
            raise RuntimeError("upstream close failure")


async def drain(q: asyncio.Queue[tuple[str, str] | None]) -> list[tuple[str, str] | None]:
    items: list[tuple[str, str] | None] = []
    while not q.empty():
        items.append(await q.get())
    return items


async def main() -> None:
    q1: asyncio.Queue[tuple[str, str] | None] = asyncio.Queue()
    s1 = FailingCloseStream(["hello", "world"], fail_close=False)
    await mod.forward_stream_chunks(s1, q1)
    assert s1.closed, "stream must be closed"
    got1 = await drain(q1)
    assert got1 == [("data", "hello"), ("data", "world"), None], f"unexpected normal events: {got1!r}"

    q2: asyncio.Queue[tuple[str, str] | None] = asyncio.Queue()
    s2 = FailingCloseStream(["part-1"], fail_close=True)
    await mod.forward_stream_chunks(s2, q2)
    assert s2.closed, "stream close must be attempted"
    got2 = await drain(q2)
    assert got2[-1] is None, f"terminal sentinel None missing when aclose() raises: {got2!r}"
    assert ("error", "summary generation failed") in got2, f"expected error event on close failure: {got2!r}"


asyncio.run(main())
PY
