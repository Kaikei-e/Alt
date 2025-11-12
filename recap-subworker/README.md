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
