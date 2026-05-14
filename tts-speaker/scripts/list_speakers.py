"""Print the supported speakers for Qwen3-TTS-12Hz-0.6B-CustomVoice without synthesizing.

Loads the model once and dumps `get_supported_speakers()` / `get_supported_languages()`.
"""

from __future__ import annotations

import json
import os
import sys

import torch

from qwen_tts import Qwen3TTSModel  # type: ignore[import-untyped]

MODEL_ID = os.environ.get(
    "TTS_QWEN_MODEL_ID", "Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice"
)


def main() -> int:
    device_map = "cpu" if os.environ.get("TTS_FORCE_CPU") == "1" else (
        "cuda:0" if torch.cuda.is_available() else "cpu"
    )
    dtype = getattr(torch, os.environ.get("TTS_QWEN_DTYPE", "bfloat16"))
    print(f"loading {MODEL_ID} (device={device_map}, dtype={dtype})...", file=sys.stderr)
    model = Qwen3TTSModel.from_pretrained(
        MODEL_ID, device_map=device_map, dtype=dtype, attn_implementation="sdpa",
    )

    speakers_fn = getattr(model, "get_supported_speakers", None)
    languages_fn = getattr(model, "get_supported_languages", None)
    speakers = speakers_fn() if speakers_fn else None
    languages = languages_fn() if languages_fn else None

    payload = {
        "model_id": MODEL_ID,
        "languages": list(languages) if languages is not None else None,
        "speakers": (
            speakers
            if isinstance(speakers, dict)
            else (list(speakers) if speakers is not None else None)
        ),
    }
    print(json.dumps(payload, ensure_ascii=False, indent=2, default=str))
    return 0


if __name__ == "__main__":
    sys.exit(main())
