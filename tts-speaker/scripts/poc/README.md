# Supertonic v3 PoC

Phase 0 of `~/.claude/plans/tts-https-github-com-supertone-inc-supe-virtual-sketch.md`.
**Goal: decide adoption by ear, before touching `tts_speaker/core/`.**

## Why this exists

`tts-speaker` currently runs Qwen3-TTS (ADR-000900). The previous engine swap
attempt (Fun-CosyVoice3, 2026-05-17 daily) reached RTF 0.91 on AIX but was
rejected on subjective audio quality. **The lesson encoded here: a human must
listen to Supertonic output against the same text as the Qwen samples before
the engine abstraction work begins.**

## Install (one-time, in the tts-speaker venv)

These deps are intentionally **not** added to `pyproject.toml` until Phase 2.
Install them ad-hoc for the probe so the production lockfile stays clean.

```bash
cd tts-speaker
uv pip install supertonic onnxruntime huggingface-hub
```

Note: the upstream README says **"GPU mode is not supported yet"**, so the
PoC and the eventual production adapter both run on CPU only. No
`onnxruntime-rocm` is needed for this phase. (The ROCm/MIGraphX fallback
stay in the plan as a Phase 2-or-later option for if/when upstream adds
GPU support.)

## Run order

```bash
cd tts-speaker

# 1. Smoke — one sentence, dumps the TTS class/instance surface so we know
#    how to inject ORT providers in Phase 2.
uv run python scripts/poc/supertonic_smoke.py

# 2. Sample matrix — every JA voice × total_steps ∈ {5, 8, 12}, with the
#    same text as voice-samples/manifest.json so you can A/B against the
#    Qwen WAVs side by side.
uv run python scripts/poc/supertonic_sample_matrix.py

# 3. Listen.
ls -1 ../voice-samples/sup-*.wav
ls -1 ../voice-samples/qwen-*.wav
```

## Outputs

| File | Owner |
|---|---|
| `voice-samples/sup-smoke.wav` | smoke script |
| `voice-samples/sup-<voice>-step<N>.wav` | matrix script |
| `voice-samples/supertonic-manifest.json` | matrix script (separate from qwen `manifest.json`) |

## Env knobs

- `SUP_TEXT` — Japanese text (matrix uses the Qwen sample text by default).
- `SUP_VOICE` (smoke) — voice name passed to `get_voice_style`; default
  `M4` (the upstream `example_pypi.py` choice).
- `SUP_VOICES` (matrix) — comma-separated voice list; default: probe
  `M1..M5,F1..F5` and use those that resolve.

The upstream lib's voice naming is not documented in the README; the smoke
script logs the surface of `TTS` and the `get_voice_style` signature so we
can update these defaults as soon as we know what JA-suitable voices ship.

## The gate

After listening, declare one of:

- **Adopt** → proceed to Phase 1 (`refactor(tts-speaker): introduce TTSEngine
  Port and QwenEngine Gateway`).
- **Reject** → delete `scripts/poc/` and the `sup-*` WAVs; we stay on Qwen
  and explore a different engine.

Do not skip the gate. CosyVoice 2026-05-17 had performance numbers and still
failed quality review — the same risk applies here.
