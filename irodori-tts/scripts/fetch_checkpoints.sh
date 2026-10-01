#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${IRODORI_MODELS_HOST_PATH:-}" ]]; then
  echo "Error: IRODORI_MODELS_HOST_PATH environment variable is required." >&2
  exit 1
fi

ROOT_DIR="${IRODORI_MODELS_HOST_PATH}"
HF_HUB_PKG="huggingface_hub==1.22.0"

echo "==> Fetching Irodori-TTS checkpoints to ${ROOT_DIR}..."
mkdir -p "${ROOT_DIR}/v4.1-Small" "${ROOT_DIR}/v4.1-Small-MF" "${ROOT_DIR}/v4.1-Small-Quantized" "${ROOT_DIR}/codec"

echo "--> Downloading v4.1-Small..."
uv tool run --from "${HF_HUB_PKG}" hf download \
  Aratako/Irodori-TTS-v4.1-Small \
  --revision 2b28324dc263ed5e6638b3cf3dd94c82ead07b4b \
  --local-dir "${ROOT_DIR}/v4.1-Small" \
  --include "model.safetensors" \
  --include "tokenizer/*"

echo "--> Downloading v4.1-Small-MF..."
uv tool run --from "${HF_HUB_PKG}" hf download \
  Aratako/Irodori-TTS-v4.1-Small-MF \
  --revision ccc78f5d480b6e51b69b2d5042a14c4da04fea6e \
  --local-dir "${ROOT_DIR}/v4.1-Small-MF" \
  --include "model.safetensors" \
  --include "tokenizer/*"

echo "--> Downloading v4.1-Small-Quantized..."
uv tool run --from "${HF_HUB_PKG}" hf download \
  Aratako/Irodori-TTS-v4.1-Small-Quantized \
  --revision ef04e6c3ba56138ae23e86a2eabc004f76990e37 \
  --local-dir "${ROOT_DIR}/v4.1-Small-Quantized" \
  --include "int8-weight-only/model.safetensors" \
  --include "tokenizer/*"

echo "--> Downloading codec..."
uv tool run --from "${HF_HUB_PKG}" hf download \
  Aratako/Semantic-DACVAE-Japanese-32dim \
  --revision 47376ee24834d7a05a48ebabfe3cde29b3c5e214 \
  --local-dir "${ROOT_DIR}/codec" \
  --include "weights.pth"

echo "==> Done fetching checkpoints."
