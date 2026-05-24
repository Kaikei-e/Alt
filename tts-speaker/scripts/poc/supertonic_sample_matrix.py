"""Supertonic v3 sample matrix — every probed voice × the same JA paragraph.

Phase 0 of the Supertonic adoption plan. Run **after** `supertonic_smoke.py`
succeeded and the smoke WAV sounded reasonable.

For each voice that `get_voice_style` accepts, synthesize the same Japanese
test sentence (matching `voice-samples/manifest.json` from the Qwen samples
for direct A/B), write to `voice-samples/sup-<voice>.wav`, and record
RTF + wall + sample rate into `voice-samples/supertonic-manifest.json`
(separate file so the Qwen manifest stays untouched).

The upstream `supertonic` Python lib is CPU-only as of the current release
("GPU mode is not supported yet" — README), so this matrix is intentionally
single-pass. The `total_steps` knob mentioned in the upstream README is not
exposed by `TTS.__init__` or `synthesize` as of v1.3.1; it can be re-added
if a future release surfaces it.

Run from `tts-speaker/`:

    uv run python scripts/poc/supertonic_sample_matrix.py

Env knobs
=========
- `SUP_VOICES` — comma-separated voice name list (default: probe a small
  grid M1..M5/F1..F5 and use what resolves).
- `SUP_TEXT` — default matches `voice-samples/manifest.json`.
"""

from __future__ import annotations

import json
import logging
import os
import sys
import time
from pathlib import Path
from typing import Any

LOG = logging.getLogger("supertonic-matrix")

REPO_ROOT = Path(__file__).resolve().parents[3]
OUT_DIR = REPO_ROOT / "voice-samples"
MANIFEST = OUT_DIR / "supertonic-manifest.json"

# Same text as voice-samples/manifest.json so users can A/B against qwen-*.wav.
DEFAULT_TEXT = "今日は5月15日、東京は穏やかな晴れです。AltというRSSアプリの新機能をご紹介します。"

VOICE_PROBES = ("M1", "M2", "M3", "M4", "M5", "F1", "F2", "F3", "F4", "F5")
SAMPLE_RATE = 44100  # README: "44.1kHz 16-bit WAV files"


def _import_supertonic() -> tuple[Any, Any]:
    try:
        import supertonic  # type: ignore[import-not-found]
        from supertonic import TTS  # type: ignore[import-not-found]
    except ImportError as err:
        LOG.error(
            "supertonic not installed. Run: pip install supertonic onnxruntime huggingface-hub"
        )
        raise SystemExit(2) from err
    return supertonic, TTS


def _ort_providers() -> list[str]:
    try:
        import onnxruntime as ort
    except ImportError:
        return []
    return sorted(ort.get_available_providers())


def _resolve_voices(tts: Any) -> list[str]:
    env = os.environ.get("SUP_VOICES", "").strip()
    if env:
        requested = [v.strip() for v in env.split(",") if v.strip()]
        ok: list[str] = []
        for name in requested:
            try:
                _ = tts.get_voice_style(voice_name=name)
            except Exception as err:  # noqa: BLE001
                LOG.warning("SUP_VOICES name %r rejected: %s", name, err)
                continue
            ok.append(name)
        return ok
    discovered: list[str] = []
    for name in VOICE_PROBES:
        try:
            _ = tts.get_voice_style(voice_name=name)
        except Exception as err:  # noqa: BLE001
            LOG.debug("probe %r failed: %s", name, err)
            continue
        discovered.append(name)
    LOG.info("Probed voices that resolved: %s", discovered)
    return discovered


def main() -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )
    OUT_DIR.mkdir(parents=True, exist_ok=True)

    supertonic, TTS = _import_supertonic()
    providers = _ort_providers()
    LOG.info(
        "supertonic=%s providers=%s",
        getattr(supertonic, "__version__", "unknown"),
        providers,
    )

    text = os.environ.get("SUP_TEXT", DEFAULT_TEXT)

    LOG.info("Constructing TTS(auto_download=True)")
    tts = TTS(auto_download=True)

    voices = _resolve_voices(tts)
    if not voices:
        LOG.error("No voices resolved — nothing to render")
        return 1
    LOG.info("Will render %d voice(s): %s", len(voices), voices)
    LOG.info("Test text: %s", text)

    import numpy as np

    samples: list[dict[str, Any]] = []
    for voice in voices:
        out_file = OUT_DIR / f"sup-{voice}.wav"
        style = tts.get_voice_style(voice_name=voice)
        t = time.monotonic()
        try:
            wav, duration = tts.synthesize(text, voice_style=style)
        except Exception:  # noqa: BLE001
            LOG.exception("synthesize failed (voice=%s)", voice)
            continue
        wall = time.monotonic() - t
        tts.save_audio(wav, str(out_file))

        audio = np.asarray(wav)
        n_samples = int(audio.shape[-1]) if audio.ndim else int(audio.size)
        audio_seconds = n_samples / SAMPLE_RATE
        reported = (
            float(np.asarray(duration).flatten()[0]) if duration is not None else float("nan")
        )
        rtf = wall / audio_seconds if audio_seconds else float("nan")
        LOG.info(
            "  %s samples=%d audio_dur=%.2fs reported=%.2f wall=%.2fs rtf=%.2f",
            out_file.name,
            n_samples,
            audio_seconds,
            reported,
            wall,
            rtf,
        )
        samples.append(
            {
                "voice": voice,
                "file": str(out_file.relative_to(REPO_ROOT)),
                "sample_rate": SAMPLE_RATE,
                "samples": n_samples,
                "duration_seconds": audio_seconds,
                "reported_duration": reported,
                "wall_seconds": wall,
                "rtf": rtf,
            }
        )

    MANIFEST.write_text(
        json.dumps(
            {
                "language": "Japanese",
                "text": text,
                "providers": providers,
                "supertonic_version": getattr(supertonic, "__version__", "unknown"),
                "sample_rate": SAMPLE_RATE,
                "samples": samples,
            },
            ensure_ascii=False,
            indent=2,
        ),
        encoding="utf-8",
    )
    LOG.info("Wrote manifest %s", MANIFEST)
    LOG.info("=== Generated %d sample(s) ===", len(samples))
    LOG.info("A/B listen against voice-samples/qwen-*.wav before approving Phase 1.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
