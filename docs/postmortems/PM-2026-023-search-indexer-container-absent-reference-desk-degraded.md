# ポストモーテム: search-indexer コンテナが消失し Reference Desk の articles / recaps が約 36 時間 silent degradation していた問題

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-023 |
| 発生日時 | 2026-04-12 12:40 (JST) ※ `search-indexer-logs` サイドカー再起動ログ `Discovery error: Container not found` を起点 |
| 検知日時 | 2026-04-13 09:20 (JST) 頃 ※ ユーザーが Reference Desk で degradation バナーを報告 |
| 復旧日時 | 2026-04-13 09:45 (JST) ※ `search-indexer` 再ビルド完了・`healthy` 到達 |
| 影響時間 | 約 33 時間（silent degradation。ユーザー側の致命停止は無し） |
| 重大度 | SEV-3（Reference Desk の articles / recaps セクションのみ劣化、tags セクションと他エンドポイントは正常） |
| 作成者 | オンコール担当者 |
| レビュアー | — |
| ステータス | Resolved |

## サマリー

Alt プロジェクトの `search-indexer` サービスコンテナが 2026-04-12 12:40 (JST) 頃から `alt-network` から消失し、alt-backend の Reference Desk (`/search`) で articles / recaps セクションが `unavailable: dial tcp: lookup search-indexer on 127.0.0.11:53: no such host` で連続失敗する状態が約 33 時間継続した。`global_search_usecase` の graceful degradation ロジックが想定通り動作したため UI は「Some sections are unavailable」バナーで劣化を明示していたが、アラートが設定されておらず、ユーザー報告で初めて異常が検知された。原因は (1) search-indexer のサービスコンテナが存在しないこと、(2) `search-indexer` に `restart: always` が設定されていなかったこと、(3) `alt-backend` の `depends_on` に search-indexer が含まれていなかったことの 3 点が重なった結果。即時対応として search-indexer を再ビルド・起動し、恒久対策として compose 設定に `restart: always` と `depends_on` を追加した。

## 影響

- **影響を受けたサービス:** alt-backend（Reference Desk ハンドラのみ劣化）、alt-frontend-sv（UI 上で degradation バナー表示）
- **影響を受けた機能:** Reference Desk の articles セクションと recaps セクションの検索結果が常に 0 件・degraded バナー付きで返る状態。tags セクションは正常に動作
- **影響を受けたユーザー数:** 不明。Reference Desk はユーザー操作起点のため、期間中に検索を試みたユーザー全員が degradation を観測したはず
- **代替手段:** `/feeds/search`（記事検索専用 UI）と `/articles/by-tag`（タグ経由記事取得）は search-indexer を別の内部クライアント経由で使用しており、こちらは同じ問題の影響を受ける可能性が高いが、今回のユーザー報告は Reference Desk のみ。tag-based 導線（PostgreSQL 経由）は正常
- **データ損失:** なし（記事・recap の永続データは別 DB / Meilisearch に残存）
- **SLO/SLA違反:** Reference Desk 個別の SLO 未定義。可観測性指標としては degraded 率が明示されておらず、継続劣化が監視で検知されなかった

### 定量的影響

| メトリクス | 期待値 | 実際（修正前） | 修正後 |
|---|---|---|---|
| Reference Desk クエリあたりの degraded セクション数 | 0 | 2（articles + recaps） | 0（復旧確認済） |
| alt-backend ログ上の `no such host` 件数 / 2 分 | 0 | ユーザー検索ごとに 2 行（articles + recaps） | 0 件（修正後 2 分間） |
| search-indexer コンテナ稼働率 | 100% | 0%（約 33 時間不在） | 100%（復旧後） |
| tags セクションの動作 | 正常 | 正常（Postgres 直読のため不影響） | 正常 |

## タイムライン

| 時刻 (JST) | イベント |
|---|---|
| 2026-04-12 12:40 | **起源（推定）**: `alt-search-indexer-logs-1` サイドカーが再起動直後に `Discovery error: Container not found for service: search-indexer` を記録。サービスコンテナ本体が `alt-network` から消失済みだったことを示唆 |
| 2026-04-12 12:40 〜 2026-04-13 09:20 | **潜在**: alt-backend が Reference Desk リクエストごとに `lookup search-indexer on 127.0.0.11:53: no such host` を ERROR ログとして記録。UI には「Some sections are unavailable: articles, recaps」バナー表示。アラート未設定のため気づかれず |
| 2026-04-13 00:20 頃 | **最初期の失敗ログ（解析時確認）**: クエリ "LLM" で articles / recaps の `degradedSections` が発生。trace_id 単位で継続的に再発 |
| 2026-04-13 09:20 | **検知**: ユーザーが Reference Desk の挙動を共有し、degradation バナーが恒常的に出ていることを報告 |
| 2026-04-13 09:25 | **初動切り分け**: tags セクションは動作、articles / recaps のみ劣化 → Meilisearch / search-indexer 経路を疑う |
| 2026-04-13 09:30 | **alt-backend ログ確認**: `lookup search-indexer on 127.0.0.11:53: no such host` を特定。DNS レベルでコンテナ不在 |
| 2026-04-13 09:32 | **docker compose ps 確認**: `search-indexer-logs` は稼働、`search-indexer` 本体は不在。meilisearch / redis-streams は healthy |
| 2026-04-13 09:35 | **サイドカーログ確認**: `Discovery error: Container not found for service: search-indexer` を確認 |
| 2026-04-13 09:38 | **原因特定**: 「コンテナが存在しない状態」であり DNS 解決失敗自体が即エラーとなり 3 秒タイムアウトを待たずに `unavailable` 応答 |
| 2026-04-13 09:40 | **恒久対策の設計**: `restart: always` の欠落と `depends_on` の未宣言という構造的要因を特定 |
| 2026-04-13 09:42 | **即時対応**: `docker compose -f compose/compose.yaml -p alt up --build -d search-indexer` 実行 |
| 2026-04-13 09:43 | **compose 設定修正**: `compose/workers.yaml` に `restart: always`、`compose/core.yaml` の alt-backend `depends_on` に `search-indexer: service_healthy` を追加 |
| 2026-04-13 09:45 | **復旧確認**: `docker compose ps search-indexer` が `running / healthy` を報告 |
| 2026-04-13 09:47 | **検証**: 修正後 2 分間の alt-backend ログで `no such host` / `article search failed` / `recap search failed` の出現が 0 件であることを確認 |

## 検知

- **検知方法:** ユーザーからの報告（Reference Desk 画面のスクリーンショット相当の情報共有）
- **検知までの時間 (TTD):** 約 33 時間
- **検知の評価:** 大幅に遅れた。原因は以下の通り:
  1. `search-indexer` 消失に対するアラートが無い
  2. Reference Desk の `degradedSections` に対する SLO / メトリクスが未定義
  3. alt-backend の `level:ERROR` ログは大量の feed 取得エラーに埋もれており、`lookup search-indexer` 系の頻出エラーがシグナルとして浮き上がらなかった
  4. `search-indexer-logs` サイドカーが `Container not found` を ERROR レベルで出していたが、rask-log-aggregator 側で該当シグナルを拾う alert rule が設定されていなかった

## 根本原因分析

### 直接原因

`search-indexer` サービスコンテナが `alt-network` から消失していた。DNS 解決自体が失敗するため、alt-backend の Connect-RPC クライアントは 3 秒タイムアウトを待たずに即座に `unavailable` を返し、`global_search_usecase` が articles / recaps を `degradedSections` に追加していた。

### Five Whys

1. **なぜ articles / recaps セクションが unavailable になっていたのか？**  
   → alt-backend から `http://search-indexer:9301` へのリクエストが DNS lookup 段階で失敗していたため。

2. **なぜ DNS lookup が失敗したのか？**  
   → `alt-network` に `search-indexer` という名前のサービスコンテナが存在しなかったため。コンテナが crash したのではなく、除去された状態のまま復帰していなかった。

3. **なぜ除去されたコンテナが復帰しなかったのか？**  
   → `compose/workers.yaml` の `search-indexer` 定義に `restart: always` が設定されていなかった。restart policy が無いコンテナは一度停止・除去されると Docker Engine が自動で再作成しない。

4. **なぜ `restart: always` が設定されていなかったのか？**  
   → compose 定義を追加した時点で `restart` フィールドが見落とされ、その後も対象的なレビューが行われなかった。他の長時間稼働サービス（alt-backend, mq-hub, auth-hub 等）は `restart: always` を持つが、search-indexer だけ欠落している状態が長期間見逃されていた。

5. **なぜ欠落が見逃されていたのか？**  
   → compose 構成の差分レビューに「全長時間稼働サービスが restart policy を持つこと」を強制する lint / チェックが存在しないため。かつ alt-backend の `depends_on` に search-indexer が含まれていなかったため、search-indexer 不在でも alt-backend が起動し UI が silently 劣化する構成となっていた。異常が「UI バナー」としてしか可視化されず、オペレータに届くシグナルが無かった。

### 根本原因

**compose 構成上、`search-indexer` は `restart: always` も `alt-backend` の `depends_on` も持たず、かつ degradation を検知するアラートも存在しなかったため、コンテナが除去された状態が自動復旧せず、長時間 silent degradation を続けた。** 単一のバグではなく、以下 3 層の防壁がいずれも機能しない構成であった:

- Engine 層の自動復旧（restart policy）が無効
- Orchestration 層の依存宣言（depends_on）が不足
- 観測層のアラート（コンテナ不在 / degradation 率）が未整備

### 寄与要因

- `search-indexer` が `alt-network` から消失した具体的なトリガー（手動 `docker rm` / OOMKiller / 別作業中の誤操作 等）は本調査では特定できず。ログ保持期間を超えている可能性が高い。仮にこれが再発しても再発防止策（restart policy）で復帰するため、本件では深追いしない。
- alt-backend のログに feed 取得関連の ERROR が継続的に出ており、新規 ERROR パターンがノイズに埋もれやすい状態だった。
- `global_search_usecase` の graceful degradation パターンが「ユーザー体験を守る」ことには成功していたが、「オペレータに異常を届ける」機構と連携していなかった。

## 対応の評価

### うまくいったこと

- `global_search_usecase` の graceful degradation が設計通り動作し、articles / recaps 不在でも tags セクションだけで Reference Desk が完全停止することを防いだ。
- 根本原因の特定が高速だった（検知から約 20 分）。alt-backend の構造化ログに明示的なエラーメッセージが含まれており、`docker compose ps` の出力と突き合わせるだけで原因が確定できた。
- [No compose down] 方針に従い、該当サービス 1 つだけを再ビルド・起動したため、他のサービスに影響を与えずに復旧できた。

### うまくいかなかったこと

- 検知がユーザー報告依存で 33 時間遅延した。`search-indexer-logs` サイドカーが `Container not found` を出し続けていたにも関わらず、それを拾うアラートが無かった。
- Reference Desk の `degradedSections` に対する観測指標が無く、劣化が恒常化していたことが一切通知されなかった。
- compose 設定の一貫性（`restart` / `depends_on` の網羅性）を担保する仕組みが無い。

### 運が良かったこと

- tags セクションが PostgreSQL 直読であり、search-indexer 経路から独立していたため、Reference Desk が完全停止にならなかった。
- ユーザーが異常を明示的に報告してくれたため、さらなる長期化を免れた。報告が無ければ週単位で継続していた可能性がある。
- `search-indexer` のビルドが問題なく通り、即時再ビルドで復旧できた。もし同期間に search-indexer のビルドが壊れていた場合、復旧時間はさらに延びていた。

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 予防 | `compose/workers.yaml` の `search-indexer` に `restart: always` を追加 | オンコール担当者 | 2026-04-13 | Done |
| 2 | 予防 | `compose/core.yaml` の `alt-backend` `depends_on` に `search-indexer: service_healthy` を追加 | オンコール担当者 | 2026-04-13 | Done |
| 3 | 予防 | compose 設定 lint（長時間稼働サービスに `restart` が設定されているか、主要サービスの `depends_on` に抜けが無いか）を CI に追加検討 | インフラ担当 | 2026-04-30 | TODO |
| 4 | 検知 | `search_indexer_up == 0` または search-indexer への DNS 解決失敗を検知する Prometheus アラートを rask-log-aggregator / Grafana に追加 | オブザーバビリティ担当 | 2026-04-27 | TODO |
| 5 | 検知 | Reference Desk の `degradedSections` 発生率をメトリクスとしてエクスポートし、1h 平均が閾値を超えた際に通知 | alt-backend 担当 | 2026-04-27 | TODO |
| 6 | 検知 | `rask-log-forwarder` の `Container not found for service:` ERROR を拾う集約層アラートを追加 | オブザーバビリティ担当 | 2026-04-20 | TODO |
| 7 | 緩和 | Reference Desk UI の degradation バナーに「時間経過後も継続する場合は runbook を参照」等の一文を添え、長期継続時にユーザー/開発者が自己診断しやすくする | alt-frontend-sv 担当 | 2026-05-10 | TODO |
| 8 | プロセス | 新規サービスを `compose/` に追加する際の PR チェックリスト（restart policy / depends_on / ヘルスチェック / ログフォワーダ）を runbook に追加 | インフラ担当 | 2026-04-30 | TODO |

### カテゴリの説明

- **予防:** 同種インシデントの再発を防ぐ構造的変更
- **検知:** より早く異常を発見するためのアラート・モニタリング整備
- **緩和:** 発生時のユーザー影響を小さくする仕組み
- **プロセス:** 今後の変更で同じ穴を作らないためのレビュー・手順整備

## 教訓

- **graceful degradation は「ユーザー体験のためのフェールセーフ」であって「オペレータへのシグナル」ではない。** `degradedSections` パターンはユーザー影響を最小化する点で正しく機能したが、これ単体では長期 silent degradation を招く。degradation の「発生そのもの」を観測指標として扱う必要がある。
- **restart policy と depends_on は「書いてあるから安全」ではなく「書いていないと危険」の対象。** 長時間稼働前提のサービスでは、どちらの欠落もコンテナ消失時の自動復旧を無効化する。compose の差分レビュー時に常に確認する項目とすべき。
- **DNS レベルのエラー（`no such host`）は connection refused / timeout とは別カテゴリで扱う価値がある。** 前者は「コンテナが存在しない」という構造的問題、後者は「起動しているが応答に問題がある」という動作的問題であり、切り分けが迅速化すると原因特定が大幅に早まる。
- **サイドカーのログは主サービスの異常を間接的に示すことがある。** `rask-log-forwarder` の `Container not found` は本件の最も早い客観的シグナルだった。サイドカー ERROR のアラート化は今後のインシデント早期発見に寄与する。

## 参考資料

- [[000703]] search-indexer の restart policy と alt-backend の depends_on を明示化し Reference Desk の silent degradation を防ぐ（本件の恒久対策 ADR）
- [[000625]] Global Federated Search 実装（Reference Desk バックエンド設計）
- [[000691]] Search UI Alt-Paper redesign（Reference Desk UI 設計）
- [[000626]] Global Search feature flag 撤去（機能の恒久有効化）
- 関連コード:
  - `alt-backend/app/usecase/global_search_usecase/usecase.go`（`degradedSections` ロジック）
  - `alt-backend/app/driver/search_indexer_connect/client.go`（Connect-RPC クライアント）
  - `compose/workers.yaml`（search-indexer 定義）
  - `compose/core.yaml`（alt-backend 定義）

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
