# recap-subworker

Alt の Recap Worker パイプラインから委譲される記事コーパスをクラスタリングし、LLM が消費するエビデンス JSON を生成する FastAPI サービスです。

詳細なアーキテクチャ、エンドポイント、パイプライン仕様は [docs/services/recap-subworker.md](../docs/services/recap-subworker.md) を参照してください。

## Runtime

- **Entry point**: `python -m recap_subworker`（Uvicorn 単一プロセス起動。PyTorch CUDA 環境における `fork()` 競合を避けるため、コンテナでも直接 Uvicorn を実行）。
- **Pipeline execution**: `PipelineTaskRunner` fans out evidence runs to a dedicated `ProcessPoolExecutor` so a wedged clustering task cannot block the HTTP loop.
- **Backpressure**: `RunManager` enforces `RECAP_SUBWORKER_MAX_BACKGROUND_RUNS` (default 2) and a hard timeout (`RECAP_SUBWORKER_RUN_EXECUTION_TIMEOUT_SECONDS`) per genre. When the backlog exceeds `RECAP_SUBWORKER_QUEUE_WARNING_THRESHOLD` a warning is logged.

## Local Development

```bash
# Build + run via Docker Compose (include-based orchestration)
docker compose -f compose/compose.yaml -p alt up -d recap-subworker

# Local execution (virtualenv synced via uv, matches container CMD)
uv run python -m recap_subworker
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

| GPU環境 | Ollama併用 | 推奨設定 |
|---------|-----------|----------|
| 専有GPU | なし | 両方 `cuda` |
| 共有GPU | あり | embedding: `cuda`, classification: `cpu` |
| CPU環境 | - | 両方 `cpu` |

## Related Documentation

- [Project CLAUDE.md](../CLAUDE.md)
- [Architecture Details](../docs/services/recap-subworker.md)
