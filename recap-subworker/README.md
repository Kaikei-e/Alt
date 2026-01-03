# recap-subworker

Alt の Recap Worker パイプラインから委譲される記事コーパスをクラスタリングし、Gemma 3 4B が消費するエビデンス JSON を生成する FastAPI サービスです。

## Runtime

- **Entry point**: Gunicorn + `uvicorn.workers.UvicornWorker` (see `recap_subworker/infra/gunicorn_conf.py`). Workers recycle automatically after ~400 requests with jitter to avoid thundering herds.
- **Pipeline execution**: `PipelineTaskRunner` fans out evidence runs to a dedicated `ProcessPoolExecutor` so a wedged clustering task cannot block the HTTP loop.
- **Backpressure**: `RunManager` enforces `RECAP_SUBWORKER_MAX_BACKGROUND_RUNS` (default 2) and a hard timeout (`RECAP_SUBWORKER_RUN_EXECUTION_TIMEOUT_SECONDS`) per genre. When the backlog exceeds `RECAP_SUBWORKER_QUEUE_WARNING_THRESHOLD` a warning is logged.

## Local tips

```bash
# Build + run via Docker
docker compose --profile recap up recap-subworker

# Direct gunicorn launch (virtualenv already synced via uv)
uv run gunicorn -c recap_subworker/infra/gunicorn_conf.py recap_subworker.app.main:create_app
```

## デバイス設定

EmbeddingモデルとClassificationモデルで異なるデバイスを使用できます。

### 分離設定（推奨）

```bash
# Embedding（クラスタリング）にGPU、Classification（分類）にCPU
export RECAP_SUBWORKER_DEVICE=cuda
export RECAP_SUBWORKER_CLASSIFICATION_DEVICE=cpu
```

### 単一設定（後方互換）

```bash
# 両方にCPUを使用（デフォルト）
export RECAP_SUBWORKER_DEVICE=cpu
```

`RECAP_SUBWORKER_CLASSIFICATION_DEVICE` 未設定時は `RECAP_SUBWORKER_DEVICE` の値を継承します。

### 推奨構成

| GPU VRAM | Ollama併用 | 推奨設定 |
|----------|-----------|----------|
| 8GB以上 | なし | 両方 `cuda` |
| 8GB | あり (~5GB使用) | embedding: `cuda`, classification: `cpu` |
| 4GB未満 | - | 両方 `cpu` |
