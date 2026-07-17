"""Generate Qwen3-TTS-12Hz-0.6B-CustomVoice voice samples for subjective review.

Outputs WAV files to ../voice-samples/ at the repo top:

    voice-samples/qwen-<speaker>.wav

Run from `tts-speaker/`:

    uv run python scripts/generate_voice_samples.py                 # all 9 speakers
    SPEAKERS=ono_anna,serena uv run python scripts/generate_voice_samples.py  # subset
    INSTRUCT_OVERRIDE="ゆっくり朗読調で" uv run python scripts/generate_voice_samples.py

Pass ``TTS_FORCE_CPU=1`` on machines without ROCm/CUDA. The first run downloads
~2 GB of weights from HuggingFace into ``HF_HOME``; subsequent runs are cached.
"""

from __future__ import annotations

import json
import logging
import os
import sys
import time
from pathlib import Path

import soundfile as sf
import torch

from qwen_tts import Qwen3TTSModel  # type: ignore[import-untyped]

LOG = logging.getLogger("voice-samples")

REPO_ROOT = Path(__file__).resolve().parents[2]
OUT_DIR = REPO_ROOT / "voice-samples"

MODEL_ID = os.environ.get(
    "TTS_QWEN_MODEL_ID", "Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice"
)
DTYPE_NAME = os.environ.get("TTS_QWEN_DTYPE", "float32")
ATTN_IMPL = os.environ.get("TTS_QWEN_ATTN", "sdpa")
LANGUAGE = "Japanese"

DEFAULT_INSTRUCT = "落ち着いた声で、自然なペースで日本語で読み上げてください"
INSTRUCT = os.environ.get("INSTRUCT_OVERRIDE", DEFAULT_INSTRUCT)

# Test sentence: mixed kana / kanji / numerals / punctuation; covers prosody.
TEST_TEXT = (
    "今日は5月15日、東京は穏やかな晴れです。"
    "AltというRSSアプリの新機能をご紹介します。"
)


def _resolve_torch_dtype(name: str) -> object:
    """Map a validated dtype name to a torch.dtype (no getattr on free strings)."""
    mapping = {
        "bfloat16": torch.bfloat16,
        "float16": torch.float16,
        "float32": torch.float32,
    }
    try:
        return mapping[name]
    except KeyError as err:
        raise ValueError(
            f"unsupported TTS_QWEN_DTYPE {name!r}; expected one of {sorted(mapping)}"
        ) from err


def _device_map() -> str:
    if os.environ.get("TTS_FORCE_CPU") == "1":
        return "cpu"
    if torch.cuda.is_available():
        return "cuda:0"
    return "cpu"


def _all_speakers(model: object) -> list[str]:
    get = getattr(model, "get_supported_speakers", None)
    if get is None:
        raise RuntimeError("Model has no get_supported_speakers()")
    raw = get()
    if isinstance(raw, dict):
        out: list[str] = []
        for value in raw.values():
            out.extend(value if isinstance(value, list) else [value])
        return [str(s) for s in out]
    return [str(s) for s in raw]


def _selected_speakers(model: object) -> list[str]:
    env = os.environ.get("SPEAKERS")
    if env:
        return [s.strip() for s in env.split(",") if s.strip()]
    return _all_speakers(model)


def main() -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    LOG.info("Output directory: %s", OUT_DIR)

    device_map = _device_map()
    dtype = _resolve_torch_dtype(DTYPE_NAME)
    LOG.info(
        "Loading %s (device_map=%s, dtype=%s, attn=%s)...",
        MODEL_ID,
        device_map,
        DTYPE_NAME,
        ATTN_IMPL,
    )
    t0 = time.monotonic()
    model = Qwen3TTSModel.from_pretrained(
        MODEL_ID,
        device_map=device_map,
        dtype=dtype,
        attn_implementation=ATTN_IMPL,
    )
    LOG.info("Model loaded in %.1fs", time.monotonic() - t0)

    speakers = _selected_speakers(model)
    LOG.info("Generating samples for speakers: %s", speakers)
    LOG.info("Text: %s", TEST_TEXT)
    LOG.info("Instruct: %s", INSTRUCT)

    summary: list[dict[str, object]] = []
    for speaker in speakers:
        out_file = OUT_DIR / f"qwen-{speaker}.wav"
        LOG.info("→ speaker=%s", speaker)
        t_speaker = time.monotonic()
        try:
            wavs, sr = model.generate_custom_voice(  # type: ignore[attr-defined]
                text=TEST_TEXT,
                language=LANGUAGE,
                speaker=speaker,
                instruct=INSTRUCT,
            )
        except Exception:  # noqa: BLE001 — per-speaker skip so the matrix still finishes
            LOG.exception("speaker=%s failed; skipping", speaker)
            summary.append(
                {
                    "speaker": speaker,
                    "file": None,
                    "error": "synthesis failed",
                }
            )
            continue
        audio = wavs[0]
        sf.write(out_file, audio, sr, format="WAV", subtype="FLOAT")
        duration = float(len(audio)) / float(sr)
        elapsed = time.monotonic() - t_speaker
        rtf = elapsed / duration if duration else float("nan")
        LOG.info(
            "  saved %s (sr=%d, audio_dur=%.2fs, wall=%.1fs, rtf=%.2f)",
            out_file.name,
            sr,
            duration,
            elapsed,
            rtf,
        )
        summary.append(
            {
                "speaker": speaker,
                "file": str(out_file.relative_to(REPO_ROOT)),
                "sample_rate": int(sr),
                "duration_seconds": duration,
                "wall_seconds": elapsed,
                "rtf": rtf,
            }
        )

    manifest = OUT_DIR / "manifest.json"
    manifest.write_text(
        json.dumps(
            {
                "model_id": MODEL_ID,
                "language": LANGUAGE,
                "text": TEST_TEXT,
                "instruct": INSTRUCT,
                "device": device_map,
                "dtype": DTYPE_NAME,
                "samples": summary,
            },
            ensure_ascii=False,
            indent=2,
        ),
        encoding="utf-8",
    )
    LOG.info("Wrote manifest %s", manifest)
    LOG.info("=== Generated %d samples in %s ===", len(summary), OUT_DIR)
    return 0


if __name__ == "__main__":
    sys.exit(main())
