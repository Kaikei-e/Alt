# Recap Subworker Implementation Guide

この手順書は `PLAN2.md` で整理した設計指針をもとに、Alt モノレポ内で `recap-subworker` サービスを実装・検証・展開するための詳細な TODO を段階別にまとめたものです。Rust 製 `recap-worker`（docs/recap-implementation-plan.md）と同一 Compose プロファイルで動作させることを前提とします。

---

## 1. コンテキストと前提

- 目的：ジャンル単位のコーパスを CPU ベースで前処理し、Gemma 3 4B（Compose `ollama` プロファイル経由）に渡す JSON 形式のエビデンス束を生成。citeturn5search2turn5search6turn5search0
- 技術選定：多言語対応埋め込みに BGE-M3（8192 tokens / 1024 dim）を採用し、必要に応じて distill 版へフェイルオーバー。citeturn1search1turn1search8turn13search1
- フレームワーク：FastAPI 0.115.12 + Uvicorn、Pydantic v2、orjson 3.11.4、uv 0.9.7、prometheus-fastapi-instrumentator 7.1.0 を統一採用。citeturn4search4turn12search4turn6view0turn2search0
- 依存：`docs/recap-implementation-plan.md` で定義された Rust パイプライン、`news-creator` の Gemma エンドポイント、共有メトリクススキーマ。

---

## 2. 環境セットアップ

1. **uv プロジェクトの初期化**
   ```bash
   uv init recap-subworker
   uv tool install --upgrade uv==0.9.7
   ```
   - `pyproject.toml` に `fastapi>=0.115.12`, `pydantic>=2.9`, `orjson==3.11.4`, `prometheus-fastapi-instrumentator==7.1.0`, `fastjsonschema==2.21.2` を明記。citeturn4search4turn12search4turn2search0turn9search5
2. **モデル資産の取得**
   - `SentenceTransformer("BAAI/bge-m3")` を優先。CPU 圧迫時は `bge-m3-distill-8l` を `optional-dependencies = ["distill"]` として切り替えられるよう DI 設計。citeturn7search3turn13search1
3. **ローカル Compose**
   - `make up -- ollama profile` を実行し、`recap-worker`・`news-creator` と同じネットワークで Uvicorn をポート公開。
   - `.env` に `UVICORN_WORKERS`, `EMBED_BATCH`, `HDBSCAN_MIN_CLUSTER_SIZE` などサービス用環境変数を追加し、Pydantic `Settings` で読み込む。

---

## 3. 実装ステップ（Phase A → D）

### Phase A – 基盤整備

1. **モジュールスケルトン生成**
   - `app/main.py`, `app/deps.py`, `domain/models.py`, `services/embedder.py`, `services/pipeline.py`, `infra/telemetry.py` を空ファイルで配置。
   - `main.py` で FastAPI アプリと `Instrumentator().instrument(app).expose(app)` を定義し `/metrics` を有効化。citeturn2search0
2. **Pydantic モデル**
   - Request/Response/Config を v2 API (`BaseModel`, `field_validator`, `model_dump`) で定義し、JSON Schema を `domain/schema.py` に出力。
   - Schema CI 用に `fastjsonschema.compile(schema)` をプリロード。citeturn15search0turn9search5
3. **Embedder 実装**
   - Sentence-Transformers 5.1.2 を初期化し、`encode(sentences, batch_size, normalize_embeddings=True)` のラッパを提供。citeturn7search3
   - 8192-token 入力を安全に扱うよう、バッチサイズは推定トークン数で動的制御。citeturn1search8

### Phase B – 精度向上

1. **UMAP + HDBSCAN パイプライン**
   - `services/clusterer.py` で UMAP(0.5.9.post2)→HDBSCAN(0.8.40) を組み合わせる。ハイパーパラメータはジャンル別 Config に外だし。citeturn10search0turn11search0
   - 高密度データ用に `fast-hdbscan` fallback を `extras` で用意し、CPU 低遅延が必要なケースをカバー。citeturn17search0
2. **c-TF-IDF トピック抽出**
   - `domain/topics.py` で n-gram ベースの c-TF-IDF を実装し、`bm25_weighting` を選択可能に。citeturn1search6
3. **MMR + Dedup**
   - `domain/selectors.py` で MMR（λ=0.3）と類似度フィルタ（cos>0.92）を統合。類似文の理由を `diagnostics` に記録。

### Phase C – 運用拡張

1. **ONNX / 量子化**
   - Sentence-Transformers の ONNX ガイドを参照し、`Embedder.encode()` を ONNX Runtime backend に差し替え可能な設計へ。citeturn16search0
2. **ウォームアップ & キャッシュ**
   - `POST /admin/warmup` にモデルロード、およびベクトルキャッシュ Priming を実装。
   - `infra/cache.py` に LRU（size, ttl）実装を追加し、再計算を抑止。
3. **プロメトリクス拡張**
   - `telemetry.py` で `Counter`, `Histogram` を宣言し、`http_request_duration_seconds` など Instrumentator 既定メトリクスと整合。citeturn2search0turn18search0

### Phase D – 品質とロールアウト

1. **ゴールデンデータ**
   - `tests/golden/recap-subworker/*.json` を作成し、ジャンル別代表文・トピック語をスナップショット。
2. **性能検証**
   - 10k 文 / 5 ジャンルのシナリオで `pytest -m perf` を用意し、CPU 利用率とレイテンシを記録。
3. **ログ & トレース**
   - `structlog` または `logging.getLogger` + JSONFormatter で `job_id`, `genre`, `request_id` を必須フィールド化。

---

## 4. テスト & CI チェックリスト

| カテゴリ | コマンド | 備考 |
| --- | --- | --- |
| ユニット | `uv run pytest tests/unit` | selectors, topics, embedder を重点 |
| 結合 | `uv run pytest tests/integration` | ST 実ベクトルとモックを分離 |
| スキーマ | `uv run python scripts/validate_schema.py` | fastjsonschema 2.21.2 を使用。citeturn9search5 |
| パフォーマンス | `uv run pytest -m perf` | ONNX backend / distill モードを比較 |
| 静的解析 | `uv run ruff check` + `uv run mypy` | Alt 共通 QA |

CI では `plan2` ブランチを作成し、`recap-worker` の契約テスト（Rust 側の HTTP クライアント）と合同で走らせること。

---

## 5. デプロイとロールアウト

1. **イメージビルド**
   - `docker build -f recap-subworker/Dockerfile -t alt/recap-subworker:dev .`
   - Dockerfile では `python:3.11-slim` ベースに `uv sync --frozen --no-dev`、`RUN uv cache prune` でサイズ削減。citeturn6view0
2. **Compose 統合**
   - `compose.yaml` の `ollama` プロファイル内にサービスを追加し、`depends_on` で `news-creator`, `recap-worker`, `recap_db` を指定。
   - `logging` プロファイルと併用し、Rask の ClickHouse へメトリクス/ログを転送。
3. **ステージング → 本番**
   - Shadow モード：`recap-worker` からのレスポンスを DB に保存するが UI へは出さない。
   - Canary：`genre in ('ai','security')` など部分的に UI 反映。遅延・再試行率・MMR coverage を監視。
   - Full Rollout：Gemma 3 本番トラフィックへ切替、PEFT/Vertex AI 連携が必要な場合は `news-creator` の設定を同期更新。citeturn5search6

---

## 6. 付録

- 主要設定ファイル
  - `pyproject.toml`：依存バージョン、optional extras（distill, onnx）
  - `app/deps.py`：DI コンテナ（SentenceTransformer, ProcessPoolExecutor, Settings）
  - `infra/telemetry.py`：メトリクス・トレーシング定義
  - `scripts/validate_schema.py`：fastjsonschema 検証スクリプト
- 参考資料
  - Gemini/Gemma リリースノートと Vertex AI 統合情報citeturn5search2turn5search6
  - BGE-M3 モデル仕様と distill 版の比較ガイドciteturn1search1turn1search8turn13search1
  - Sentence-Transformers 5.1.2 リリースノート（OpenVINO / DirectML サポート）citeturn7search3
  - ONNX Runtime ガイドライン（Sentence-Transformers）citeturn16search0
