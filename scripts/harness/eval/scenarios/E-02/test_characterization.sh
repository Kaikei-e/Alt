#!/usr/bin/env bash
set -euo pipefail

WORKSPACE_DIR="${WORKSPACE_DIR:-$(pwd)}"
TARGET="$WORKSPACE_DIR/src/queue.py"

if [[ ! -f "$TARGET" ]]; then
  echo "FAIL: missing target file $TARGET" >&2
  exit 1
fi

python3 -B - "$TARGET" <<'PY'
import asyncio
import importlib.util
import sys

spec = importlib.util.spec_from_file_location("worker_queue_mod", sys.argv[1])
assert spec is not None and spec.loader is not None
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)


async def main() -> None:
    recorded: list[tuple[str, float, bool]] = []

    def record_queued_acquire(
        result: str, start_time: float, high_priority: bool
    ) -> tuple[float, int]:
        recorded.append((result, start_time, high_priority))
        return (0.25, 42)

    sem = mod.PrioritySemaphore(record_queued_acquire)

    loop = asyncio.get_running_loop()
    fut: asyncio.Future[str] = loop.create_future()
    fut.set_result("slot-ok")
    got = await sem.acquire(fut, 1.5, True)
    assert got == (0.25, 42), f"expected (0.25, 42), got {got!r}"
    assert recorded == [("slot-ok", 1.5, True)], f"unexpected calls: {recorded!r}"

    pending: asyncio.Future[str] = loop.create_future()
    task = asyncio.create_task(sem.acquire(pending, 2.0, False))
    await asyncio.sleep(0)
    task.cancel()
    try:
        await task
    except asyncio.CancelledError:
        pass
    else:
        raise AssertionError("expected CancelledError to propagate")
    assert pending.cancelled(), "expected underlying future to be cancelled"


asyncio.run(main())
PY
