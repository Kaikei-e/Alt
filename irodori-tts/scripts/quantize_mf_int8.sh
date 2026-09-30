#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${IRODORI_MODELS_HOST_PATH:-}" ]]; then
  echo "Error: IRODORI_MODELS_HOST_PATH environment variable is required." >&2
  exit 1
fi

MODELS_DIR="${IRODORI_MODELS_HOST_PATH}"

if [[ ! -d "${MODELS_DIR}" ]]; then
  echo "Error: models directory '${MODELS_DIR}' does not exist." >&2
  exit 1
fi

OUTPUT_FILE="${MODELS_DIR}/v4.1-Small-MF-int8/model.safetensors"
if [[ -f "${OUTPUT_FILE}" ]]; then
  echo "Output file '${OUTPUT_FILE}' already exists."
  exit 0
fi

# The models directory must be writable by uid 1000 for this step.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

echo "==> Quantizing MeanFlow checkpoint to int8 using CPU..."
docker compose -f "${REPO_ROOT}/compose/compose.yaml" -p alt --profile tts run --rm --no-deps \
  -v "${MODELS_DIR}:/models:rw" \
  irodori-tts \
  python /opt/irodori-tts/quantize_checkpoint.py \
  /models/v4.1-Small-MF/model.safetensors \
  --output /models/v4.1-Small-MF-int8/model.safetensors \
  --quantization int8-weight-only \
  --profile core \
  --device cpu

echo "==> Quantization complete: ${OUTPUT_FILE}"
