#!/bin/bash
exec text-generation-launcher \
  --model-id "${MODEL_ID}" \
  --port 9100 \
  --max-total-tokens 4096 \
  --max-batch-prefill-tokens 4096 \
  --quantize bitsandbytes-nf4