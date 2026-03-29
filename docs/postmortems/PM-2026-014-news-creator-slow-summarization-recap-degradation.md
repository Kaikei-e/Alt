# PM-2026-014: news-creator 要約遅延と Recap 生成数激減 — セマフォスロットリーク・COLD_START・クラスタリング失敗の複合障害

## メタデータ

| 項目 | 値 |
|------|-----|
| 重大度 | SEV-3（主要機能の部分的劣化：要約遅延 + Recap ジャンル欠損。完全停止ではない） |
| 影響期間 | 2026-03-28 〜 2026-03-29（構造的問題のため、複数の障害が独立に発生・継続） |
| 影響サービス | news-creator, pre-processor, recap-worker, recap-subworker |
| 影響機能 | 記事要約（BE バッチ + FE ストリーミング）、Recap 3-day 生成 |
| 関連 ADR | [[000610]], [[000611]], [[000609]], [[000601]], [[000606]], [[000563]] |
| 関連 PM | [[PM-2026-012-visual-swipe-summarize-semaphore-slot-leak]], [[PM-2026-013-ask-augur-follow-up-timeout-clarification-drop]] |

## サマリー

news-creator での要約処理が著しく遅延し、同時に Recap の生成数が激減した。コンテナログの調査により、**6つの独立した問題が同時に存在**していることが判明した。最もクリティカルなのは (1) HybridPrioritySemaphore の `be_slots=0` 構成でのスロット消失（109回の INVARIANT VIOLATION）と (2) pre-processor の古いバイナリが `gemma3-4b-8k` を送信し続けることによる 610回の COLD_START であった。Recap 側では (3) recap-worker の EmbeddingService 初期化失敗と (4) recap-subworker の numpy 2.0 互換性問題により、30ジャンル中5ジャンルのクラスタリングが失敗していた。

## 影響

- **要約遅延**: BE リクエストのキュー待ちが最大 **281秒**（通常 0秒）。プリエンプション発動時に 500 Internal Server Error
- **COLD_START**: 610回のモデルリロード（各 0.1〜0.35秒の追加レイテンシ）
- **Recap ジャンル欠損**: 30ジャンル中 5ジャンル（software_dev: 602件, ai_data: 229件, cybersecurity: 156件, culture_arts, consumer_tech）が 3日連続で欠損
- **リモート GPU**: 分散 BE の 1/3 キャパシティ喪失（1192回連続ヘルスチェック失敗）
- **データ損失**: なし
- **SLO/SLA 違反**: なし（パフォーマンス劣化のみ）

## タイムライン

| 時刻 | イベント |
|------|---------|
| 2026-03-27 17:00頃 | recap-worker 再起動。EmbeddingService は起動時に正常初期化（subgenre 分割に成功） |
| 2026-03-27 17:59 | **発生（問題4）**: recap-worker の次回パイプライン実行で `embedding service unavailable` が発生。EmbeddingService が後続 run で利用不能に |
| 2026-03-28 16:11 | **発生（問題2）**: news-creator 再起動（20時間前）。pre-processor の quality checker が `gemma3-4b-8k` でリクエスト送信開始。COLD_START が頻発 |
| 2026-03-28 17:04 | **発生（問題4継続）**: recap パイプラインで 10ジャンルが `embedding service unavailable` で subgenre 分割不能 |
| 2026-03-29 10:00 | recap パイプライン実行。tag-generator の DB 認証失敗（問題6）、EmbeddingService unavailable（問題4）、HDBSCAN TypeError（問題5）が複合 |
| 2026-03-29 10:03 | 5ジャンルのクラスタリングが failed（run 9318, 9320, 9329, 9333）。`genres_stored: 24/30` |
| 2026-03-29 10:39 | classification evaluation が `np.nan` エラーで 400 Bad Request |
| 2026-03-29 11:50 | **検知**: `SLOT INVARIANT VIOLATION` が ERROR レベルでログ出力。`rt_available=0, be_available=0, acquired_count=0` |
| 2026-03-29 11:51 | プリエンプション発動 → 500 Internal Server Error。`Long queue wait detected: 15.64s` |
| 2026-03-29 11:55 | BE リクエストが **281秒** キュー待ち後に priority promotion |
| 2026-03-29 12:00頃 | **原因特定開始**: コンテナログの徹底調査を開始 |
| 2026-03-29 （同日） | **修正実装**: ADR-610（セマフォ修正）、ADR-611（recap 修正）、pre-processor 再ビルド |

## 検知

- **検知方法**: ユーザーによる手動確認（要約遅延の体感 + Recap 生成数の確認）
- **検知までの時間 (TTD)**: 不明（構造的問題のため発生時刻が特定不能。セマフォスロットリークは 2026-03-28 の news-creator 再起動直後から発生していた可能性が高い）
- **検知の評価**: `SLOT INVARIANT VIOLATION` は ERROR レベルでログ出力されていたが、外部監視（Grafana 等）への連携がなく、手動ログ分析でのみ検知可能だった。COLD_START は WARNING レベルで 610回発生していたが、閾値ベースのアラートが未設定

## 根本原因分析

本インシデントは6つの独立した問題の複合であり、それぞれに異なる根本原因がある。

### 問題 1: HybridPrioritySemaphore スロット消失 (Critical)

#### 直接原因

`total_slots=1, rt_reserved=1`（`be_slots=0`）構成で、`release()` 内の `call_soon_threadsafe` による遅延 `set_result()` が、waiter の future キャンセルとの race condition を引き起こし、スロットが永久消失。

#### Five Whys

1. **なぜスロットが消失したか？** → `release()` が `call_soon_threadsafe(future.set_result, home_pool)` でスロット転送をスケジュールした後、waiter の future がキャンセルされ、`set_result()` が `InvalidStateError` で失敗したため
2. **なぜ `call_soon_threadsafe` を使っていたか？** → 元の実装が thread-safe 性を考慮して `call_soon_threadsafe` を使用していたが、`release()` は常にイベントループスレッドから呼ばれるため不要だった
3. **なぜ `InvalidStateError` でスロットが失われたか？** → `woke_up=True` に設定済みのためプールカウンタが加算されず、`set_result()` の失敗時にスロットを回収するリカバリロジックが存在しなかった
4. **なぜ `be_slots=0` で顕在化したか？** → [[000609]] で `OLLAMA_NUM_PARALLEL=1` に統一した結果、`total_slots=1, rt_reserved=1` となり、BE リクエストが RT プールにフォールバック → プリエンプション → スロット転送のパスが高頻度で実行されるようになった
5. **なぜテストで発見されなかったか？** → 既存テストは `be_slots >= 1` 構成のみをカバーしており、`be_slots=0` の BE→RT フォールバックパスのテストが不足していた

### 問題 2: COLD_START 連鎖 (Critical)

#### 直接原因

pre-processor の quality checker が `gemma3-4b-8k` でリクエストを送信し、FE ストリーミングは `gemma3-4b-12k` で送信。交互に来るリクエストでモデルリロードが発生。

#### Five Whys

1. **なぜ COLD_START が頻発したか？** → `gemma3-4b-8k` と `gemma3-4b-12k` の異なるモデルが交互にリクエストされ、`OLLAMA_NUM_PARALLEL=1` で同時に 1 モデルしか VRAM に載らないため
2. **なぜ異なるモデルが使われたか？** → pre-processor コンテナが 4日前のビルドのまま稼働しており、`quality_judger.go` の `modelName` が古い `gemma3-4b-8k` を参照していた
3. **なぜ pre-processor が再ビルドされていなかったか？** → コミット `4e99146d`（2026-03-29 00:48、`gemma3-4b-12k` 統一）の後に pre-processor の `--build` 再ビルドが実施されなかった
4. **なぜモデル名変更時に全サービスの再ビルドが漏れたか？** → モデル名はコンパイル済みバイナリに埋め込まれる静的変数であり、Go バイナリの `--build` 再ビルドが必須だが、変更影響範囲の確認チェックリストが存在しなかった

### 問題 3: リモート GPU ダウン (High)

#### 直接原因

リモート GPU マシンが完全に到達不能（`ServerDisconnectedError` が 1192回連続）。Tailscale VPN 接続の問題と推定。

### 問題 4: EmbeddingService 初期化失敗 (High)

#### 直接原因

recap-worker の `EmbeddingService::new()` が rust-bert の `AllMiniLmL12V2` モデル初期化に失敗。`.ok()` でエラーが握り潰され、原因不明のまま `None` に変換。

### 問題 5: HDBSCAN TypeError (Medium)

#### 直接原因

[[000563]] の Python 3.14 アップグレードで numpy 1.x → 2.0 に更新。numpy 2.0 の配列→スカラー変換厳密化により `TypeError` が発生し、optuna objective の `except (IndexError, ValueError, RuntimeError)` で捕捉されなかった。

### 問題 6: tag-generator DB 認証失敗 (Medium)

#### 直接原因

recap-worker から tag-generator への呼び出し時に `password authentication failed for user "tag_generator"` が発生。間欠的な接続問題。tag-generator 自身のバッチ処理は正常動作。

### 寄与要因

- **設定ドリフト**: `.env` の `LLM_MODEL=gemma3:4b` と `compose/ai.yaml` の `LLM_MODEL=gemma3-4b-12k` が混在。compose がオーバーライドするため news-creator 自体は正しいが、紛らわしい
- **コンパイル済みバイナリの陳腐化**: Go/Rust サービスはソースコード変更だけでは反映されず、`--build` 再ビルドが必須。これが見落とされやすい
- **可観測性の不足**: `SLOT INVARIANT VIOLATION` と `COLD_START` の外部アラートが未設定。手動ログ分析でしか検知できない

## 対応の評価

### うまくいったこと

- news-creator の構造化ログ（`SLOT INVARIANT VIOLATION`、`COLD_START detected`、`Long queue wait detected`）が詳細な定量データを含んでおり、問題の特定と重大度の評価が迅速に行えた
- recap-worker の `genres_stored: 24/30, genres_failed: 5` のようなサマリーログにより、ジャンル単位の影響範囲が即座に判明
- `/queue/status` エンドポイントのリモートヘルスチェック情報（`consecutive_failures: 1192`）で、リモート GPU の障害が定量的に確認できた
- PM-2026-012/013 で導入された `home_pool` 追跡と invariant チェックが、今回のスロットリークの検知に直接役立った

### うまくいかなかったこと

- `SLOT INVARIANT VIOLATION` が ERROR レベルでログに出力されていたにもかかわらず、外部アラートが設定されておらず、ユーザーの体感劣化での検知となった
- pre-processor のモデル名変更（`4e99146d`）後の `--build` 再ビルドが漏れた。影響サービスの再ビルドチェックリストが存在しない
- recap-worker の `EmbeddingService::new().ok()` がエラーを完全に握り潰しており、3日間原因不明のまま放置された
- 6つの独立した問題が同時に存在し、相互に影響を増幅していた（例: COLD_START + スロットリーク → 要約遅延が指数的に悪化）

### 運が良かったこと

- 要約処理自体は（遅延しながらも）完走しており、未要約記事の蓄積は最小限だった
- Recap の 24/30 ジャンル（80%）は正常に生成されており、完全停止には至らなかった
- リモート GPU ダウンは 3台中 1台であり、残りの 2台で分散 BE が継続していた

## アクションアイテム

### 予防（Prevent）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| P-1 | `call_soon_threadsafe` を直接 `set_result()` に置換し `_try_wake_waiter()` を導入 ([[000610]]) | 開発担当者 | 2026-03-29 | **完了** |
| P-2 | recap-worker の `EmbeddingService::new().ok()` を `match` 式に置換しエラーを `error` レベルで出力 ([[000611]]) | 開発担当者 | 2026-03-29 | **完了** |
| P-3 | recap-subworker の optuna objective に `TypeError` を except 句に追加 ([[000611]]) | 開発担当者 | 2026-03-29 | **完了** |
| P-4 | pre-processor を `--build` で再ビルドし `gemma3-4b-12k` 統一を反映 | 開発担当者 | 2026-03-29 | **完了** |
| P-5 | `release(slot_id=...)` を全 caller で必須化し、legacy fallback 推定を段階的に廃止 | 開発担当者 | 2026-04-11 | 未着手 |
| P-6 | HybridPrioritySemaphore の property-based testing（Hypothesis）導入 | 開発担当者 | 2026-04-25 | 未着手 |

### 検知（Detect）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| D-1 | `SLOT INVARIANT VIOLATION` ログに対するアラートルール追加 | 開発担当者 | 2026-04-14 | 未着手 |
| D-2 | `COLD_START` 頻度の閾値アラート追加（10回/時間で警告） | 開発担当者 | 2026-04-14 | 未着手 |
| D-3 | Recap `genres_failed > 0` の検知アラート追加 | 開発担当者 | 2026-04-14 | 未着手 |
| D-4 | リモート GPU の `consecutive_failures > 100` アラート追加 | 開発担当者 | 2026-04-14 | 未着手 |

### 緩和（Mitigate）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| M-1 | `total_slots==1` 時にプリエンプションを自動無効化する安全弁追加 | 開発担当者 | 2026-04-07 | 未着手 |

### プロセス（Process）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| O-1 | モデル名・LLM パラメータ変更時の全サービス `--build` 再ビルドチェックリスト作成 | 開発担当者 | 2026-04-07 | 未着手 |
| O-2 | `.env` と compose defaults の乖離を検出する startup assertion の設計・実装 | 開発担当者 | 2026-04-14 | 未着手 |

## 教訓

### 技術的教訓

1. **`call_soon_threadsafe` は同一スレッドの asyncio コードでは不要かつ危険**: 遅延実行される callback と future キャンセルの race condition は、直接呼び出しで完全に排除できる。同一スレッドのイベントループでは `set_result()` を直接呼ぶべき
2. **`.ok()` によるエラー握り潰しは可観測性の敵**: Rust の `Result::ok()` は便利だが、インフラ初期化のような重要なパスでは `match` で明示的にエラーを処理・ログすべき
3. **numpy メジャーバージョンアップの影響は広範**: numpy 2.0 の型変換厳密化は、直接依存していないライブラリ（HDBSCAN、optuna 経由）にも波及する。メジャーアップグレード後は ML パイプラインの全パスを結合テストで検証すべき
4. **コンパイル済みバイナリの設定埋め込みは暗黙の依存**: Go/Rust の静的変数はソースコード変更だけでは本番に反映されない。CI/CD で影響サービスの自動再ビルドを保証する仕組みが必要

### 組織的教訓

1. **複合障害の検知は単一アラートでは不十分**: 6つの独立した問題が同時に存在する場合、個々のアラートだけでなく「要約スループット」「Recap カバレッジ率」のようなビジネスレベルのメトリクスが必要
2. **PM-2026-012/013 の未着手アクションアイテムが再発の温床に**: `be_slots=0` パス監査（P-6）が未着手のまま放置されており、今回のスロットリークに直結した。アクションアイテムの追跡と期限管理を強化すべき

## 参考資料

- [[000610]] HybridPrioritySemaphore の call_soon_threadsafe 排除によるスロットリーク修正
- [[000611]] recap パイプライン可観測性向上と numpy 2.0 互換性修正
- [[000609]] Ask Augur の follow-up タイムアウト修正（`OLLAMA_NUM_PARALLEL=1` 統一の経緯）
- [[PM-2026-012-visual-swipe-summarize-semaphore-slot-leak]] 先行するスロットリーク PM
- [[PM-2026-013-ask-augur-follow-up-timeout-clarification-drop]] 先行する設定ドリフト PM

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
