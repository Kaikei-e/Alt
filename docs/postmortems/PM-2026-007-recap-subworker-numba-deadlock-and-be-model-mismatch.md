# PM-2026-007: recap-subworker の Numba threading デッドロックおよび BE 要約モデルミスマッチ

## メタデータ

| 項目 | 値 |
|------|-----|
| 重大度 | SEV-3（recap パイプラインのヘルスチェック応答不能 + BE 要約の一部ジョブ遅延） |
| 影響期間 | 不明 〜 2026-03-25（構造的問題のため、UMAP/HDBSCAN の並行実行時に常に再現） |
| 影響サービス | recap-subworker、recap-worker、news-creator |
| 影響機能 | recap クラスタリングパイプラインのヘルスチェック、BE article summarization の一部 |
| 関連 ADR | [[000575]], [[000566]], [[000497]] |

## サマリー

recap-subworker の `/health` エンドポイントがタイムアウトし、連鎖的に recap-worker の `/health/ready` も応答不能になった。原因は Numba の threading layer が非スレッドセーフな `workqueue` にフォールバックしていたこと。`tbb` パッケージは依存に含まれておらず、UMAP/HDBSCAN の並行実行時に concurrent access エラーが発生しスレッドがブロックされていた。同時に、`DISTRIBUTED_BE_MODEL_OVERRIDES` の設定ミスにより一部リモートが RAG 用モデルで BE 要約を処理し、ジョブ所要時間が 12〜30 秒から 163 秒に劣化していた。`tbb` の追加と `NUMBA_THREADING_LAYER=tbb` の明示設定、およびモデルオーバーライドの削除で両方を修正した。

## 影響

- **recap-subworker `/health`**: タイムアウト（応答不能）。ただしクラスタリング処理自体は `/v1/runs/` で 200 OK を返しており、機能的にはまだ動作していた
- **recap-worker `/health/ready`**: recap-subworker への ping が失敗するため連鎖タイムアウト（30 秒超）
- **BE 要約遅延**: 特定リモート経由のジョブが 163 秒（通常の 5〜13 倍）。全体の約 1/3 のジョブが影響（3 台中 1 台のリモートが対象）
- **機能への影響**: 部分的劣化（ヘルスチェック不能だがサービス自体は稼働）
- **データ損失**: なし
- **SLO/SLA 違反**: なし

## タイムライン

| 時刻 (JST) | イベント |
|---|---|
| 不明 | **発生**: recap-subworker の初期構築時から `tbb` が依存に含まれておらず、Numba が `workqueue` を使用。UMAP/HDBSCAN の並行実行回数が増えるにつれ concurrent access エラーが顕在化 |
| 2026-03-25 05:35 | **発生**: [[000566]] の修正（`SummarizeUsecase` の DistributingGateway バイパス修正）により、article summarization が全リモートにディスパッチされるようになった。`DISTRIBUTED_BE_MODEL_OVERRIDES` の RAG モデル設定が BE 要約に影響を与え始めた |
| 2026-03-25 14:30 | **検知**: BE 要約の全体ヘルス確認中に recap-subworker の `/health` タイムアウトを発見。`docker compose logs` で Numba concurrent access エラーを確認 |
| 2026-03-25 14:35 | `/queue/status` で 163 秒の異常遅延ジョブを発見。モデル名 `gemma3-4b-rag:latest` が BE 要約に使用されていることを確認 |
| 2026-03-25 14:40 | **原因特定（問題 1）**: `numba.config.THREADING_LAYER` が `default`、`tbb` パッケージ未インストール → `workqueue` フォールバック |
| 2026-03-25 14:42 | **原因特定（問題 2）**: `.env` の `DISTRIBUTED_BE_MODEL_OVERRIDES` に RAG モデルオーバーライドが設定されていることを確認 |
| 2026-03-25 14:50 | **対応開始**: `pyproject.toml` に `tbb>=2022.0` 追加、環境変数設定 |
| 2026-03-25 14:55 | `LD_LIBRARY_PATH` の設定漏れにより Numba が TBB を検出できない問題を発見。Web 検索で pip install 時の既知問題と確認 |
| 2026-03-25 15:00 | `LD_LIBRARY_PATH=/app/.venv/lib` を Dockerfile・compose に追加。テスト 3/3 GREEN |
| 2026-03-25 15:05 | `.env` の `DISTRIBUTED_BE_MODEL_OVERRIDES` を空に修正 |
| 2026-03-25 15:10 | recap-subworker 再ビルド・デプロイ、news-creator 再起動 |
| 2026-03-25 15:15 | **復旧確認**: `/health` 即時応答、`/health/ready` 即時応答、Numba threading layer `tbb` 確認、3 リモート全て healthy |

## 検知

- **検知方法**: 手動確認（BE 要約の全体ヘルスチェック中に発見）
- **検知までの時間 (TTD)**: 不明（構造的問題のため発生時刻が特定不能）
- **検知の評価**: recap-subworker のヘルスチェック応答不能は Docker の healthcheck でも検出可能だったが、`docker compose ps` の healthcheck が recap-subworker に設定されていなかった（recap-worker のみ `healthcheck` あり）。recap-subworker に healthcheck を追加すべき

## 根本原因分析

### 直接原因

**問題 1**: Numba が非スレッドセーフな `workqueue` threading layer で動作中に、UMAP/HDBSCAN の並行実行が発生しスレッドがブロック。

**問題 2**: `DISTRIBUTED_BE_MODEL_OVERRIDES` に RAG 用モデルが設定されており、[[000566]] の修正後に BE article summarization が当該リモートに送られた際に低速モデルが使用された。

### Five Whys（問題 1）

1. **なぜ `/health` がタイムアウトしたか？**
   → Numba の concurrent access エラーでスレッドがブロックし、FastAPI のイベントループが応答不能になったため

2. **なぜ concurrent access エラーが発生したか？**
   → `workqueue` threading layer は非スレッドセーフだが、recap-subworker が `multiprocessing.Pool(2)` + `ThreadPoolExecutor` で並行実行しているため

3. **なぜ `workqueue` が使われていたか？**
   → Numba のデフォルト選択順は `tbb > omp > workqueue` だが、`tbb` パッケージが未インストールのためフォールバック

4. **なぜ `tbb` が依存に含まれていなかったか？**
   → `umap-learn` や `hdbscan` は Numba に依存するが、`tbb` は optional dependency であり、明示的に追加する必要がある。初期構築時にこの要件が認識されていなかった

5. **なぜ初期構築時に気づかなかったか？**
   → `workqueue` は単一スレッド環境では正常動作するため、`RECAP_SUBWORKER_PIPELINE_WORKER_PROCESSES=2` で並行度を上げるまで問題が顕在化しなかった

### Five Whys（問題 2）

1. **なぜ BE 要約ジョブが 163 秒かかったか？**
   → 特定リモートで `gemma3-4b-rag:latest`（RAG 用モデル）が使用されたため

2. **なぜ RAG モデルが BE 要約に使われたか？**
   → `DISTRIBUTED_BE_MODEL_OVERRIDES` に当該リモートの override が設定されていたため

3. **なぜ override が設定されていたか？**
   → 当該リモートが RAG 専用機として運用されており、RAG モデルの VRAM evict を防ぐ目的でモデル名を固定していた

4. **なぜ BE 要約に影響したか？**
   → [[000566]] の修正で `SummarizeUsecase` が DistributingGateway 経由になり、当該リモートにも BE article summarization がディスパッチされるようになった。override はリクエスト種別を区別せず全てのリクエストに適用される

5. **なぜ ADR-566 の修正時に気づかなかったか？**
   → ADR-566 は wiring バグの修正に焦点を当てており、既存の `DISTRIBUTED_BE_MODEL_OVERRIDES` の副作用が確認されなかった

### 寄与要因

- **Docker healthcheck の不在**: recap-subworker に Docker の `healthcheck` が設定されておらず、ヘルスチェック応答不能がコンテナオーケストレーションレベルで検知されなかった
- **DistributingGateway の fallback ログレベル**: リモートディスパッチの詳細ログが `DEBUG` レベルで出力されており、`INFO` レベルでは確認できなかった（[[000566]] でも指摘済み）
- **`pip install tbb` の既知の制限**: pip で TBB をインストールしても `LD_LIBRARY_PATH` が自動設定されず、Numba が `libtbb.so` を検出できない（numba/numba#9740, numba/numba#7148）

## 対応の評価

### うまくいったこと

- `/queue/status` エンドポイントがリモートごとの `healthy`, `busy`, `in_flight_count`, `consecutive_failures` を返すため、リモートの状態把握が迅速だった
- pre-processor の構造化ログにモデル名・所要時間が記録されており、異常遅延ジョブの特定が容易だった
- TBB が Numba の既知の推奨 threading layer であり、修正方針の判断が明確だった
- リモートに `gemma3:4b-it-qat` が既にロード済みだったため、`ollama pull` が不要で即時修正可能だった

### うまくいかなかったこと

- Numba の threading layer 選択は暗黙的（ログ出力なし）で、`workqueue` にフォールバックしたことが起動時に通知されなかった
- `DISTRIBUTED_BE_MODEL_OVERRIDES` の副作用が [[000566]] の修正時にレビューされなかった
- `pip install tbb` が `LD_LIBRARY_PATH` を要求することは広く知られた問題だが、Dockerfile のベストプラクティスとして文書化されていなかった

### 運が良かったこと

- recap-subworker のクラスタリング処理自体は（ヘルスチェックは応答不能だが）`/v1/runs/` で 200 OK を返し続けており、recap パイプラインの完全停止には至らなかった
- OOM は発生しておらず（1.1 GiB / 8 GiB = 14%）、メモリ起因の追加障害はなかった

## 教訓

### 技術的教訓

1. **暗黙の依存はインフラを壊す**: Numba → TBB は optional dependency であり、`umap-learn` や `hdbscan` の `pip install` では自動インストールされない。ML ライブラリの threading backend は明示的に指定・テストすべき
2. **`pip install` した C ライブラリは `LD_LIBRARY_PATH` が必要な場合がある**: Python パッケージマネージャは shared library のパスを自動設定しない。Dockerfile では `LD_LIBRARY_PATH` の明示設定が必要
3. **設定変更の副作用はグラフで考える**: `DISTRIBUTED_BE_MODEL_OVERRIDES` はリモート単位の設定だが、ディスパッチ対象が変わる（[[000566]]）と影響範囲が変わる。設定とルーティングの組み合わせを検証すべき

### 組織的教訓

1. **wiring 修正時に設定の横影響を確認すべき**: [[000566]] のレビュー時に `DISTRIBUTED_BE_MODEL_OVERRIDES` の既存設定がどのリクエストに影響するかの確認が必要だった
2. **healthcheck の網羅性**: Docker Compose の `healthcheck` は全サービスに設定し、応答不能を自動検知すべき

## アクションアイテム

### 予防（Prevent）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| P-1 | `tbb>=2022.0` を依存に追加し、`NUMBA_THREADING_LAYER=tbb` + `LD_LIBRARY_PATH` を Dockerfile・compose に設定 ([[000575]]) | 開発担当者 | 2026-03-25 | **完了** |
| P-2 | `DISTRIBUTED_BE_MODEL_OVERRIDES` を空に修正し、全リモートで統一モデルを使用 ([[000575]]) | 開発担当者 | 2026-03-25 | **完了** |
| P-3 | Numba threading layer が TBB で動作することを検証するユニットテストを追加 | 開発担当者 | 2026-03-25 | **完了** |

### 検知（Detect）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| D-1 | recap-subworker の Docker `healthcheck` を compose に追加（`/health` エンドポイント） | 開発担当者 | 2026-04-07 | 未着手 |
| D-2 | BE 要約ジョブの所要時間メトリクスにアラート閾値を設定（60 秒超でアラート） | 開発担当者 | 2026-04-14 | 未着手 |

### 緩和（Mitigate）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| M-1 | DistributingGateway でモデルオーバーライド適用時にログレベルを INFO に引き上げ、意図しないモデル使用を可視化 | 開発担当者 | 2026-04-07 | 未着手 |

### プロセス（Process）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| O-1 | DistributingGateway の wiring 変更時に `DISTRIBUTED_BE_MODEL_OVERRIDES` の影響を確認するチェック項目を追加 | 開発担当者 | 2026-04-07 | 未着手 |

## 参考資料

- [[000575]] recap-subworker の Numba threading layer を TBB に切り替え、BE 要約モデルオーバーライドを修正する
- [[000566]] SummarizeUsecase の DistributingGateway バイパスを修正する
- [[000497]] Decorator Gateway パターンによる BE 要約の分散ディスパッチ
- [numba/numba#9740](https://github.com/numba/numba/issues/9740) — TBB not found when installed with pip
- [numba/numba#7148](https://github.com/numba/numba/issues/7148) — Using TBB with numba fails with pip install
- [Numba Threading Layers documentation](https://numba.readthedocs.io/en/stable/user/threading-layer.html)

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
