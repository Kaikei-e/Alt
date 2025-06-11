#!/usr/bin/env bash
# -------- ユーザー設定 --------
PORT=9100
MODEL_ID="microsoft/Phi-4-mini-4k-instruct-awq"
CUDA_FRAC=0.70            # 3B なので 70 % でも余裕
TOK_TOTAL=8192            # 総トークン上限（3B に現実的）
TOK_INPUT=3072            # 入力トークン (<= PREFILL)
PREFILL=3072              # ウォームアップ時に使う最大トークン
# -----------------------------

export DISABLE_CUSTOM_KERNELS=true

exec text-generation-launcher \
  --model-id "$MODEL_ID" \
  --port "$PORT" \
  --quantize awq \
  --speculate 0 \
  --disable-custom-kernels \
  --cuda-memory-fraction "$CUDA_FRAC" \
  --max-input-tokens  "$TOK_INPUT" \
  --max-total-tokens  "$TOK_TOTAL" \
  --max-batch-prefill-tokens "$PREFILL" \
  --max-batch-total-tokens 8192
