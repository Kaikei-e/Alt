from __future__ import annotations

import argparse
import logging
import sys
from typing import Any

from fastapi import Depends
from fastapi.responses import JSONResponse
import torch
import uvicorn

from irodori_openai_tts.app import app, require_auth
from irodori_openai_tts.config import get_settings


def _get_cuda_memory_dict() -> dict[str, Any]:
    dev = torch.cuda.current_device()
    device_free_bytes, device_total_bytes = torch.cuda.mem_get_info(dev)
    return {
        "device": dev,
        "allocated_bytes": torch.cuda.memory_allocated(dev),
        "reserved_bytes": torch.cuda.memory_reserved(dev),
        "max_allocated_bytes": torch.cuda.max_memory_allocated(dev),
        "max_reserved_bytes": torch.cuda.max_memory_reserved(dev),
        "device_free_bytes": device_free_bytes,
        "device_total_bytes": device_total_bytes,
    }


def register_internal_routes() -> None:
    @app.get("/internal/cuda-memory", dependencies=[Depends(require_auth)])
    def cuda_memory() -> Any:
        if not torch.cuda.is_available():
            return JSONResponse(status_code=503, content={"detail": "cuda unavailable"})
        return _get_cuda_memory_dict()

    @app.post("/internal/cuda-memory/reset-peak", dependencies=[Depends(require_auth)])
    def cuda_memory_reset_peak() -> Any:
        if not torch.cuda.is_available():
            return JSONResponse(status_code=503, content={"detail": "cuda unavailable"})
        dev = torch.cuda.current_device()
        torch.cuda.reset_peak_memory_stats(dev)
        return _get_cuda_memory_dict()


def main() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(levelname)s:%(name)s:%(message)s",
    )
    register_internal_routes()
    settings = get_settings()
    if not settings.api_key or not settings.api_key.strip():
        sys.exit("IRODORI_API_KEY must be set and non-empty")

    parser = argparse.ArgumentParser(description="Run the Irodori-TTS OpenAI-compatible API.")
    parser.add_argument("--host", default=settings.host)
    parser.add_argument("--port", type=int, default=settings.port)
    args = parser.parse_args()

    uvicorn.run(
        app,
        host=str(args.host),
        port=int(args.port),
    )


if __name__ == "__main__":
    main()
