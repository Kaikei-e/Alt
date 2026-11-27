# Summarize SSE Stream Termination (`news-creator`)

## Sub-features

- **Canonical SSE framing & headers**: `create_sse_response` が `media_type="text/event-stream; charset=utf-8"` と `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no` を返し、データチャンクを `data: "<chunk>"\n\n` として逐次送出する。
- **Guaranteed `aclosing` cleanup & terminal sentinel (C-07)**: 上流の `AsyncGenerator` を `async with aclosing(original_stream)` で包むことで、`aclose()` が `RuntimeError` を送出しても `except Exception` で `event: error\ndata: {"error": "summary generation failed"}\n\n` を通知し、`finally` で終端センチネル `None` を確実に投入して HTTP ストリームをハングさせずに EOF で閉じる。
- **C-07 mutation detection**: `async with aclosing(...)` を `finally:` 内の手動 `await original_stream.aclose()` へ書き換えた場合（`scripts/harness/corpus/C-07/after.py`）、`aclose()` 例外で終端センチネル投入がスキップされ、実 HTTP SSE 読み取りがタイムアウト失敗することをライブで検知できる。

## How to get to it

- **Canonical SSE helper**: `news-creator/app/news_creator/handler/sse_stream.py`
  - `_forward_stream_chunks`: `async with aclosing(original_stream) as stream:` による安全な転送・エラー通知・`None` センチネル投入
  - `stream_sse_with_heartbeat`: `data_queue` 消費・アイドルハートビート・タスクキャンセル後始末
  - `create_sse_response`: `StreamingResponse` と `SSE_HEADERS` 構築
- **Production endpoint**: `news-creator/app/news_creator/handler/summarize_handler.py`
  - `POST /api/v1/summarize`（`stream: true` かつ `len(content.strip()) >= 100`）が `_create_streaming_response` → `create_sse_response(..., error_message="summary generation failed")` を呼ぶ。
- **Corpus fixture (C-07)**: `scripts/harness/corpus/C-07/{before.py,after.py,meta.toml}`
- **Runtime**: `news-creator/app/.venv/bin/python`（オフライン利用可; 未作成時は `fastapi`+`uvicorn` 利用可能な `python3` または `uv run --offline`）。

## Driving it with the harness

```bash
# news-creator 実 SSE ハンドラを 127.0.0.1:0 の FastAPI/Uvicorn で起動しライブ検証
./scripts/verify/verify-live.sh run --task check-sse --probe sse --check-c07-mutation

# C-07 コーパス（before=PASS / after=FAIL）のみを単独ライブ検証
./scripts/verify/verify-live.sh check-c07-corpus \
  --before scripts/harness/corpus/C-07/before.py \
  --after scripts/harness/corpus/C-07/after.py
```

期待される観測結果（`.harness/evidence/<task>/sse.json`・`sse.log`）:
1. `GET /health` → `200 OK`
2. 正常ストリーム（`POST /probe/sse-normal`）→ `200 OK`、`Content-Type` に `text/event-stream`、`Cache-Control: no-cache`、`X-Accel-Buffering: no`、チャンク受信後にタイムアウト内でクリーン EOF。
3. クローズ時例外ストリーム（`POST /probe/sse-aclose-error`）→ `200 OK`、送出済みチャンク + `event: error`（`summary generation failed`）を受信し、ハングせずタイムアウト内でクリーン EOF。
4. `check-c07-corpus` → `before.py` がクリーン終了（`before_passed: true`）、`after.py` が SSE 終端ハングでタイムアウト失敗（`after_passed: false`, signal `VERIFY_04_SSE_TERMINATION_HANG`）し、コマンド全体が終了コード 0 で検知能力を証明する。

## Gotchas

- **静的検査だけでは安心しない**: C-07 の実障害では `news-creator` の 1,259 件の単体テストが全件通ったまま本番 SSE がハングした。モックキューではなく実 TCP/HTTP ソケット経由のストリーム読了＋タイムアウトで検証することが必須。
- **`ast-grep` ルール**: `scripts/lint/ast-grep/rules/python-no-manual-aclose-in-finally.yml` が `finally:` 内の `await <obj>.aclose()` を禁止している。テスト用フィクスチャを `news-creator/app/` 配下に直接書くと引っかかるため、検証ハーネスの一時スクリプトは必ず `os.MkdirTemp` に生成し実行後削除する。
- **本番 Ollama / GPU へ接続しない**: ライブプローブは `create_sse_response` へ決定論的なインプロセス `AsyncGenerator` を注入するため、稼働中の Ollama や `-p alt` コンテナには一切接続しない。
