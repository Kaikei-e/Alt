#!/usr/bin/env bash
set -euo pipefail

# WHY: registration uses a separate writable container because the service mounts /voices read-only; the directory must be readable by uid 1000.

if [[ $# -ne 2 ]]; then
  echo "Usage: $0 <reference.wav> <voice-id>" >&2
  exit 1
fi

REF_WAV="$1"
VOICE_ID="$2"

if [[ -z "${IRODORI_MODELS_HOST_PATH:-}" ]]; then
  echo "Error: IRODORI_MODELS_HOST_PATH environment variable is required." >&2
  exit 1
fi

if [[ ! -d "${IRODORI_MODELS_HOST_PATH}" ]]; then
  echo "Error: models directory '${IRODORI_MODELS_HOST_PATH}' does not exist." >&2
  exit 1
fi

if [[ -z "${IRODORI_VOICES_HOST_PATH:-}" ]]; then
  echo "Error: IRODORI_VOICES_HOST_PATH environment variable is required." >&2
  exit 1
fi

if [[ ! "${VOICE_ID}" =~ ^[A-Za-z0-9_-]+$ ]]; then
  echo "Error: voice-id '${VOICE_ID}' must match ^[A-Za-z0-9_-]+$" >&2
  exit 1
fi

if [[ ! -f "${REF_WAV}" ]]; then
  echo "Error: reference file '${REF_WAV}' does not exist." >&2
  exit 1
fi

VOICES_DIR="${IRODORI_VOICES_HOST_PATH}"
mkdir -p "${VOICES_DIR}"

REF_WAV_DIR="$(cd "$(dirname "${REF_WAV}")" && pwd)"
REF_WAV_BASE="$(basename "${REF_WAV}")"
REF_WAV_ABS="${REF_WAV_DIR}/${REF_WAV_BASE}"

if ! docker image inspect alt-irodori-tts:local >/dev/null 2>&1; then
  echo "alt-irodori-tts:local not found; build it with: docker compose -f compose/compose.yaml -p alt --profile tts build irodori-tts" >&2
  exit 1
fi

docker run --rm \
  -v "${IRODORI_MODELS_HOST_PATH}:/models:ro" \
  -v "${VOICES_DIR}:/voices" \
  -v "${REF_WAV_ABS}:/in/ref.wav:ro" \
  -e IRODORI_CODEC_REPO=/models/codec/weights.pth \
  -e HF_HUB_OFFLINE=1 \
  alt-irodori-tts:local \
  python -m alt_irodori.encode_latent /in/ref.wav "/voices/${VOICE_ID}.pt" --device cpu
