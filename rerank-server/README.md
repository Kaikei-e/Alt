# Rerank Server

Cross-encoder reranking service for rag-orchestrator. Runs as the `rerank-local`
container (`compose/rag.yaml`) on CPU with a dynamic int8-quantized (avx2) ONNX
model; the `torch` backend is kept for local experiments on MPS/CUDA.

> **Note:** This server does not include authentication. It is designed to run on a private network and should **not** be exposed directly to the public internet. Deploy behind a reverse proxy with authentication if public access is required.

## Requirements

- Python 3.14+

## Installation

```bash
uv sync --group onnx   # container / ONNX backend
pip install -r requirements.txt   # torch backend only
```

## Running

```bash
# Development
uvicorn rerank_server:app --host 0.0.0.0 --port 8080

# Or directly
python rerank_server.py
```

## Tests

```bash
uv sync            # installs the dev group (pytest, httpx2)
uv run pytest tests/
uv run pyrefly check .
```

The suite never enters the app's lifespan, so no model weights are downloaded.

## Configuration

| Variable | Default | Notes |
| --- | --- | --- |
| `RERANK_MODEL` | `BAAI/bge-reranker-v2-m3` | Hugging Face repo id. See "Models" below. |
| `RERANK_BACKEND` | `torch` | `onnx` in the container. |
| `RERANK_MODEL_CACHE_ROOT` | `/models` | Exports land in `<root>/<model-name>-onnx`. |
| `RERANK_MODEL_DIR` | derived | Overrides the derived path outright. |
| `RERANK_BATCH_SIZE` | `16` | Batch handed to `predict()`. |
| `RERANK_CHUNK_SIZE` | `= RERANK_BATCH_SIZE` | Pairs per `predict()` call; also the deadline-check granularity. |
| `RERANK_MAX_LENGTH` | `512` | Token truncation. |
| `RERANK_SERVER_TIMEOUT` | `10` | Total budget for one request, lock wait included. Must stay **below** rag-orchestrator's `RERANK_TIMEOUT` (12s in `compose/rag.yaml`). |
| `RERANK_CANDIDATE_BUDGET_MS` | `250` | Per-candidate cost used to derive the bound below. |
| `RERANK_MAX_CANDIDATES` | derived (`40`) | `RERANK_SERVER_TIMEOUT / RERANK_CANDIDATE_BUDGET_MS`. Larger lists are rejected with 422 rather than timing out. |
| `RERANK_MAX_CANDIDATE_LENGTH` | `4000` | Characters per candidate. |

Per-candidate cost varies with passage length: ADR-000951 measured ~52ms on Ask
Augur production traffic (10 candidates ≈ 520ms) but ~250ms on ~500-token
passages that saturate `RERANK_MAX_LENGTH` (15 candidates, p95 3.8s). The
default budget uses the long-passage rate, since any request may be that case.
The same ADR records 30 long passages at 14-17s — cost grows faster than
linearly past one batch, which the per-chunk deadline check absorbs at runtime.

Changing `RERANK_MODEL` derives a new cache directory, so the previous model's
export is never served by mistake. rag-orchestrator's own `RERANK_MODEL` must be
changed to match, or its requests are rejected with 422.

## Models

`RERANK_MODEL` accepts any cross-encoder repo id. These five were checked
against the pinned export toolchain:

| Model | Arch | License | JQaRA |
| --- | --- | --- | --- |
| `BAAI/bge-reranker-v2-m3` (default) | XLM-RoBERTa | Apache-2.0 | 0.673 |
| `hotchpotch/japanese-bge-reranker-v2-m3-v1` | XLM-RoBERTa | MIT | 0.6918 |
| `hotchpotch/japanese-reranker-base-v2` | ModernBERT | MIT | 0.7845 |
| `hotchpotch/japanese-reranker-xsmall-v2` | ModernBERT | MIT | 0.7845 |
| `cl-nagoya/ruri-v3-reranker-310m` | ModernBERT | Apache-2.0 | 0.8688 |

`ruri-v3-reranker-310m` scores highest but ships no pre-exported ONNX, so its
first boot always pays the local export.

Anything else loads with a `known_good=False` startup log line.

## Endpoints

### POST /v1/rerank

Rerank candidates based on query relevance.

```bash
curl -X POST http://localhost:8080/v1/rerank \
  -H "Content-Type: application/json" \
  -d '{"query": "machine learning", "candidates": ["deep learning", "cooking recipes", "neural networks"]}'
```

Response:
```json
{
  "results": [
    {"index": 0, "score": 0.95},
    {"index": 2, "score": 0.85},
    {"index": 1, "score": 0.1}
  ],
  "model": "BAAI/bge-reranker-v2-m3",
  "processing_time_ms": 123.45
}
```

`model` is optional in the request; when present it must equal the server's
`RERANK_MODEL`. Status codes: 422 (oversized list, unknown model), 503 (model
still loading), 504 (budget exhausted).

### GET /health

```bash
curl http://localhost:8080/health
```

Returns 503 with `"status": "loading"` until the model is ready.

```json
{
  "status": "ok",
  "device": "cpu",
  "model": "BAAI/bge-reranker-v2-m3"
}
```

## Systemd Service (Optional)

Create `/etc/systemd/system/rerank-server.service`:

```ini
[Unit]
Description=Rerank Server
After=network.target

[Service]
Type=simple
User=youruser
WorkingDirectory=/path/to/rerank-server
ExecStart=/usr/bin/python3 -m uvicorn rerank_server:app --host 0.0.0.0 --port 8080
Restart=always

[Install]
WantedBy=multi-user.target
```

Then:
```bash
sudo systemctl daemon-reload
sudo systemctl enable rerank-server
sudo systemctl start rerank-server
```
