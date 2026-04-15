# ポストモーテム: pre-processor の要約リトライストームと false-negative な dead_letter

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-027 |
| 発生日時 | 2026-04-15 13:23 (JST)（観測開始時刻） |
| 復旧日時 | 2026-04-15 14:27 (JST)（修正デプロイ完了） |
| 影響時間 | 約1時間（観測時点。実際の潜在発生期間は長期。下記参照） |
| 重大度 | SEV-3（ヒヤリハット寄り。ユーザー可視の全面停止ではないが、記事要約の silent データ不整合あり） |
| 作成者 | pre-processor チーム |
| レビュアー | — |
| ステータス | Draft |

## サマリー

news-creator のログ上で、同一 `article_id` に対する12チャンクの Map-Reduce 要約が並行実行され、2本目以降が全チャンク `Queue full, rejecting request` で落ちる現象をユーザが発見した。原因は pre-processor 共有 HTTP クライアントの `ResponseHeaderTimeout = 20s` が news-creator の Map-Reduce 応答時間より短く、クライアント側で切断→即リトライ→news-creator 側で重複処理という連鎖である。元のリクエスト自体は最終的に成功していたため、結果として「summary は DB に存在するのに `summarize_job_queue` が dead_letter」という silent な不整合が発生していた。pre-processor 側に三層防御（タイムアウト分離／in-flight dedup／dead_letter 直前 recheck）を導入して復旧した。

## 影響

- **影響を受けたサービス:** pre-processor, news-creator
- **影響を受けたユーザー数/割合:** ユーザー可視の直接影響は確認されず。ただし記事要約の品質指標では、`summarize_job_queue.status = 'dead_letter'` かつ `article_summaries.summary IS NOT NULL` の不整合レコードが観測時点で数件〜（正確件数は未集計）存在
- **機能への影響:** 部分的劣化
  - news-creator の GPU/モデル資源を重複リクエストで無駄使い（同一記事が最大2本並走）
  - `summarize_job_queue` のジョブ状態が実態と乖離（dead_letter 扱いだが実際は成功済み）
  - pre-processor から news-creator へのリトライストームによりログが大量発生
- **データ損失:** なし（summary データは最終的に永続化されていた）
- **SLO/SLA違反:** 本サービスに明示的 SLO はなし。ただし news-creator GPU 使用率の面では劣化

## タイムライン

| 時刻 (JST) | イベント |
|-------------|---------|
| （長期） | 共有 HTTP クライアントの `ResponseHeaderTimeout = 20s` は以前から存在。news-creator の Map-Reduce 実行時間がこの値を超える条件（長記事 × 12チャンク）で症状が顕在化 |
| 13:23 | ユーザーが news-creator ログで同一 `article_id=b0037108…` に対する重複 summarize request を目視 |
| 13:25 | **検知** — ユーザーが pre-processor / news-creator 両方のログを突合し、重複リクエストのパターンを指摘 |
| 13:30 | **対応開始** — pre-processor と news-creator の両方向からコードパス探索（HTTP クライアント構成・queue_guard・semaphore） |
| 13:45 | **原因特定** — `http_client_manager.go:75` の `ResponseHeaderTimeout: 20 * time.Second` と、`queue_guard.go` に in-flight チェックが無いこと、`summarize_queue_worker.go` の dead_letter 遷移が upstream 成功を考慮していないことを確認 |
| 13:55 | 修正方針確定（三層フル修正 + pre-processor 側 dedup） |
| 14:00〜14:20 | TDD で RED→GREEN を反復実装、全テスト green |
| 14:20 | `/security-auditor` で diff audit 実施。OWASP Top 10:2025 / ASVS 5.0 に照らし、Medium 1件（受容可能）のみ |
| 14:27 | **緩和策適用** — `docker compose up --build -d pre-processor` で新バイナリを起動。healthy 確認 |
| 14:30 | **復旧確認** — pre-processor が `ArticleUpdated` イベント 892件を backfill 正常処理、再発なし |

## 検知

- **検知方法:** ユーザーの目視（news-creator ログの繰り返しパターンに気付いた）
- **検知までの時間 (TTD):** 正確な発生時刻が不明。`ResponseHeaderTimeout=20s` の設定は少なくとも数週間前から存在し、「長記事 × 並走」条件で随時発火していた可能性が高い
- **検知の評価:** 不十分。重複 summarize 呼び出しや dead_letter / summary の整合性を監視するメトリクス・アラートが無く、「気付いた時に見た人が気付く」状態だった。類似の silent データ不整合を検知する仕組みが必要

## 根本原因分析

### 直接原因

pre-processor の summary 向け HTTP クライアントの `ResponseHeaderTimeout` が、news-creator の Map-Reduce 要約が応答ヘッダを返すまでに要する時間（長記事で60〜120秒以上）より短い20秒に設定されていた。

### Five Whys

1. **なぜ重複リクエストが発生したのか？** → pre-processor がリクエスト送信後20秒で client 側タイムアウトし、同じ `article_id` でリトライした。news-creator 側は元のリクエストを継続処理していたため、2本目が並走した。
2. **なぜ20秒のタイムアウトで切られたのか？** → 全 HTTP クライアント共通の `createOptimizedClient` が `ResponseHeaderTimeout = 20 * time.Second` をハードコードしていた。summary クライアントにだけ長い想定時間を設定する分離が無かった。
3. **なぜ summary クライアントだけ別扱いにしていなかったのか？** → `createOptimizedClient(timeout)` のシグネチャが `timeout` 1引数のみで、ResponseHeaderTimeout は内部でハードコードされていた。OWASP ASVS V14.1 の「layered timeouts」ハードニング要件を守るための一律設定で、長時間ストリーミングのユースケースが考慮されていなかった。
4. **なぜ重複リクエストを enqueue 段階で防げなかったのか？** → `queue_guard.go` は `summary_exists`（既に要約済み）と `recent_success`（24時間以内に成功）しかチェックしておらず、**「現在 in-flight なジョブがある」状態を検知する経路がなかった**。`summarize_job_queue.status` には `running` が存在するが、guard 側が見ていなかった。
5. **なぜ dead_letter 遷移が upstream の成功を考慮していなかったのか？** → `UpdateJobStatus(Failed)` が `retry_count+1 >= max_retries` で無条件に dead_letter に昇格する設計で、「エラー原因が client タイムアウトだが upstream は成功しているケース」を想定していなかった。従来は upstream が 502 を返したら upstream 自体が失敗している前提だった。

### 根本原因

**「長時間ストリーミングする upstream に対する client タイムアウトと重複防止が設計レイヤで分離されていなかった」こと**。具体的には:

1. HTTP トランスポート層の slowloris 対策（short header timeout）と、Map-Reduce 型の長時間推論 RPC の応答待機が同じ `createOptimizedClient` 設定に相乗りしていた。
2. `summarize_job_queue` テーブルには `running` 状態が存在するにもかかわらず、enqueue ガード側が `running` を見ない設計になっており、重複リクエストを事前に弾けなかった。
3. dead_letter 遷移が source-of-truth（`article_summaries.summary` の存在）を再確認しない設計で、upstream の非同期成功を取りこぼしていた。

### 寄与要因

- news-creator 側の Map phase が best-effort で全チャンク失敗まで RuntimeError を出さないため、12個全部が queue full になるまで 502 が返らない。結果、dead_letter に至るまでのログが大量に出てノイズ化し、検知が遅れた。
- pre-processor のリトライに明示的なバックオフが無く（`ticker.Reset()` による指数バックオフは `ErrServiceOverloaded` (429) のみ対象）、client タイムアウトと 502 の両方でバックオフが効かず retry storm を助長していた。
- `summarize_job_queue` と `article_summaries` の整合性を監視するメトリクスが無かった。

## 対応の評価

### うまくいったこと

- ユーザーの目視検知後、news-creator と pre-processor の両方のログを突合することで30分以内に原因を特定できた。
- TDD で RED→GREEN を守ったため、各層の変更が個別にテストで担保されている。
- `/security-auditor` による diff audit で `ResponseHeaderTimeout=0` の slowloris リスクを明示的に評価し、context timeout 必須の設計前提をコメントとテストで固定化できた（将来のリグレッション検知）。
- pre-processor 側のみの変更で閉じ、news-creator のインターフェース契約を変えずに済んだ。

### うまくいかなかったこと

- **検知が人間の目視に依存していた**。`summarize_job_queue.status = 'dead_letter'` かつ `article_summaries.summary IS NOT NULL` のような整合性違反を検知する監視クエリ・アラートが無かった。
- **根本原因の問題（timeout 分離の不在）は以前からあった**可能性が高い。長記事が少ないときは症状が出なかっただけで、潜在的にはリリース以降ずっと存在していた。
- リトライバックオフ対象が `ErrServiceOverloaded` のみだったため、client timeout / 502 で retry storm を止める仕組みが無かった。今回の修正で改善。

### 運が良かったこと

- 元のリクエストが news-creator 側で最終的に成功していたため、**ユーザー可視のデータ損失には至らなかった**。もし news-creator が途中で OOM や GPU 枯渇でクラッシュしていたら、summary 未生成のまま dead_letter が確定し、ユーザーに要約のない記事を出していた可能性がある。
- `summarize_job_queue.status` が dead_letter でも `article_summaries` が source of truth として正しかったため、フロントエンド表示は破綻しなかった。

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | pre-processor の `createOptimizedClient` をクライアント別 timeout に分離、summary client のみ `ResponseHeaderTimeout=0` | pre-processor チーム | 2026-04-15 | Done（本修正で完了） |
| 2 | 予防 | `queue_guard.ShouldQueueSummarizeJob` に in-flight チェックを追加、`summarize_job_repository.HasInFlightJob` を新設 | pre-processor チーム | 2026-04-15 | Done |
| 3 | 予防 | dead_letter 遷移直前に `summaryRepo.Exists` で upstream 成功を recheck | pre-processor チーム | 2026-04-15 | Done |
| 4 | 予防 | `ErrUpstreamBusy` sentinel を追加し 502/503/504/timeout を分類、`BackoffOnErrors` に追加して指数バックオフを効かせる | pre-processor チーム | 2026-04-15 | Done |
| 5 | 検知 | `summarize_job_queue.status = 'dead_letter'` かつ `article_summaries.summary IS NOT NULL` の件数を定期集計するメトリクス／アラートを追加 | 観測チーム | 2026-04-30 | TODO |
| 6 | 検知 | news-creator の `Queue full, rejecting request` 発火頻度をメトリクス化し、閾値アラートを設定 | 観測チーム | 2026-04-30 | TODO |
| 7 | 緩和 | `inFlightJobWindow` (10min) と `RecoverStuckJobs` の stale 判定 (10min) を同一定数に集約するヘルパを追加 | pre-processor チーム | 2026-05-15 | TODO |
| 8 | プロセス | 長時間ストリーミング RPC（summarize, stream 等）を追加する際は、`createOptimizedClient` の timeout 設計を必ずレビューするチェック項目を追加 | Alt全体 | 2026-04-30 | TODO |
| 9 | プロセス | dead_letter に落ちたジョブの source-of-truth 整合性チェックを運用手順（runbook）に追加 | 運用チーム | 2026-04-30 | TODO |

### カテゴリの説明

- **予防:** 同種のインシデントが再発しないようにするための対策
- **検知:** より早く検知するための監視・アラートの改善
- **緩和:** 発生時の影響を最小化するための対策
- **プロセス:** インシデント対応プロセス自体の改善

## 教訓

### 技術的な学び

1. **Slowloris 対策のハードニングと長時間 RPC は両立させる必要がある**。`ResponseHeaderTimeout` を全クライアント一律に短く設定するのは攻撃面では正しいが、長時間ストリーミングを前提とする upstream に対しては context timeout に責任を委譲する明示的な例外が必要。今回はクライアント別のシグネチャに分離し、テストで例外を明示した。
2. **ジョブキューの guard は `running` を無視してはいけない**。`pending` / `running` / `completed` / `failed` / `dead_letter` の5状態があるテーブルで、enqueue guard が `completed` だけ見ていると「in-flight なのに再 enqueue される」レースが発生する。in-flight を含めた状態遷移全体を意識すべき。
3. **dead_letter 遷移は source of truth の再確認が必要**。「リトライ回数を使い切った = 失敗確定」は、非同期で upstream が成功し得る設計では成立しない。最終化の直前で source of truth を再参照する recheck は、分散システムでは一般的に必要。
4. **エラー分類は retry/backoff 戦略の前提**。単一の「失敗」として扱うと、「ユーザーに再送して欲しい」「バックオフして待つ」「即 dead_letter」といった異なる挙動が混在して扱えない。sentinel error と errors.Is ベースの分類は、戦略分岐の最も軽量な基盤となる。

### 組織的な学び

1. **silent なデータ不整合は、目視に依存する限り気付けない**。`dead_letter` と `summary_exists` の突合のような、source of truth との整合性チェックは定期実行するべきで、一度きりの調査で終わらせてはいけない。
2. **共有ユーティリティの一律設定は「意識しない」状態を作りやすい**。`createOptimizedClient(timeout)` の1引数 API は「既存の設定を信じる」が基本動作になり、特殊なユースケースでも設定を見直すきっかけが得にくい。シグネチャ側で明示的に分岐させると、呼び出し側が timeout 設計を意識せざるを得なくなる。

## 参考資料

- 関連 ADR: [[000738]] pre-processorからnews-creatorへの要約リクエスト重複を抑止する三層防御を導入
- 関連 ADR: [[000139]] Dead Letter Queue パターン導入
- 関連 ADR: [[000550]] HybridPrioritySemaphore による RT/BE スケジューリング
- OWASP ASVS 5.0 V14.1 Resource consumption limits
- Go net/http `Transport.ResponseHeaderTimeout` のドキュメント: https://pkg.go.dev/net/http#Transport

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
