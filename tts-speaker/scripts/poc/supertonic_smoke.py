"""Supertonic v3 smoke probe.

Phase 0 of the Supertonic adoption plan
(`~/.claude/plans/tts-https-github-com-supertone-inc-supe-virtual-sketch.md`).

Purpose
=======
1. Confirm `supertonic` installs and constructs in this environment.
2. Dump the public surface of the `TTS` class + `get_voice_style` so the
   Phase 2 adapter (`core/engines/supertonic.py`) is built against the real
   API, not a guess.
3. Synthesize one short Japanese sentence and write it to
   `voice-samples/sup-smoke.wav` so a human can listen before we touch the
   production pipeline (CosyVoice 2026-05-17 lesson: subjective audio gate
   first, performance numbers second).

The upstream README documents `pip install supertonic` and the API:

    from supertonic import TTS
    tts = TTS(auto_download=True)              # downloads ~260MB on first run
    style = tts.get_voice_style(voice_name="M4")
    wav, duration = tts.synthesize(text, voice_style=style)
    tts.save_audio(wav, "out.wav")

The README also notes "GPU mode is not supported yet", so this script does
not attempt to inject ONNX Runtime providers — it only reports which ones
are available so we know the deployment ceiling.

Run from `tts-speaker/`:

    uv pip install supertonic onnxruntime huggingface-hub
    uv run python scripts/poc/supertonic_smoke.py

Env knobs
=========
- `SUP_VOICE` — voice name passed to `get_voice_style` (default `M4`, the
  one used by upstream `example_pypi.py`). The script also probes a small
  set of likely Japanese-suitable voice names and reports which succeed.
- `SUP_TEXT` — Japanese smoke sentence (default below).
"""

from __future__ import annotations

import inspect
import json
import logging
import os
import sys
import time
from pathlib import Path
from typing import Any

LOG = logging.getLogger("supertonic-smoke")

REPO_ROOT = Path(__file__).resolve().parents[3]
OUT_DIR = REPO_ROOT / "voice-samples"

DEFAULT_TEXT = (
    "今日は5月24日、東京は穏やかな晴れです。AltというRSSアプリにSupertonicの音声を試しています。"
)

# Likely-existing voice names to probe. The upstream example uses "M4".
# v3 README lists ~31 languages; voice naming is undocumented so we
# brute-force a small grid and log which ones succeed.
VOICE_PROBES = ("M1", "M2", "M3", "M4", "M5", "F1", "F2", "F3", "F4", "F5")


def _report_ort() -> None:
    try:
        import onnxruntime as ort
    except ImportError:
        LOG.warning("onnxruntime not importable — supertonic should still install its own copy")
        return
    LOG.info("ORT available providers: %s", sorted(ort.get_available_providers()))


def _dump_surface(obj: Any, label: str) -> dict[str, Any]:
    attrs = sorted(a for a in dir(obj) if not a.startswith("_"))
    info: dict[str, Any] = {"label": label, "attrs": attrs}
    for method in ("synthesize", "save_audio", "get_voice_style", "list_voices"):
        fn = getattr(obj, method, None)
        if fn is None or not callable(fn):
            continue
        try:
            info[f"signature.{method}"] = str(inspect.signature(fn))
        except (TypeError, ValueError) as err:
            info[f"signature.{method}"] = f"<unavailable: {err}>"
    return info


def _probe_voices(tts: Any) -> list[str]:
    ok: list[str] = []
    for name in VOICE_PROBES:
        try:
            _ = tts.get_voice_style(voice_name=name)
        except Exception as err:  # noqa: BLE001 — probe must not abort
            LOG.debug("voice %r failed: %s", name, err)
            continue
        ok.append(name)
    LOG.info("Voices that resolved: %s", ok)
    return ok


def main() -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    _report_ort()

    try:
        import supertonic  # type: ignore[import-not-found]
        from supertonic import TTS  # type: ignore[import-not-found]
    except ImportError as err:
        LOG.error(
            "supertonic not installed. Run: pip install supertonic onnxruntime huggingface-hub"
        )
        raise SystemExit(2) from err

    LOG.info("supertonic version: %s", getattr(supertonic, "__version__", "unknown"))

    LOG.info("Constructing TTS(auto_download=True) — first run downloads ~260MB")
    t0 = time.monotonic()
    tts = TTS(auto_download=True)
    LOG.info("TTS constructed in %.2fs", time.monotonic() - t0)

    surface_cls = _dump_surface(TTS, "TTS class")
    surface_obj = _dump_surface(tts, "TTS instance")
    LOG.info("Class surface: %s", json.dumps(surface_cls, ensure_ascii=False, indent=2))
    LOG.info("Instance surface: %s", json.dumps(surface_obj, ensure_ascii=False, indent=2))

    resolved = _probe_voices(tts)

    voice = os.environ.get("SUP_VOICE", "").strip() or (resolved[0] if resolved else "M4")
    LOG.info("Using voice=%r", voice)
    style = tts.get_voice_style(voice_name=voice)
    LOG.info("get_voice_style → %r (type=%s)", style, type(style).__name__)

    text = os.environ.get("SUP_TEXT", DEFAULT_TEXT)
    LOG.info("Synthesizing %d chars", len(text))
    t_synth = time.monotonic()
    wav, duration = tts.synthesize(text, voice_style=style)
    synth_wall = time.monotonic() - t_synth

    out_file = OUT_DIR / "sup-smoke.wav"
    tts.save_audio(wav, str(out_file))

    import numpy as np

    audio = np.asarray(wav)
    # README says sample rate is 44.1 kHz; derive audio duration from shape.
    samples = int(audio.shape[-1]) if audio.ndim else int(audio.size)
    sample_rate = 44100
    audio_seconds = samples / sample_rate
    reported = float(np.asarray(duration).flatten()[0]) if duration is not None else float("nan")
    rtf = synth_wall / audio_seconds if audio_seconds else float("nan")
    LOG.info(
        "Wrote %s (shape=%s, samples=%d, audio_dur=%.2fs, reported_dur=%.2f, wall=%.2fs, rtf=%.2f)",
        out_file.relative_to(REPO_ROOT),
        audio.shape,
        samples,
        audio_seconds,
        reported,
        synth_wall,
        rtf,
    )
    LOG.info("=== Smoke OK. Listen to %s and decide ===", out_file)
    return 0


if __name__ == "__main__":
    sys.exit(main())
