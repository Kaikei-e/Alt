# PM-2026-042: ステージング slice の Knowledge Projector content-type ループによる Docker host ディスク逼迫（near-miss）

## メタデータ

| 項目 | 値 |
|------|-----|
| 重大度 | SEV-3（near-miss。staging 限定でユーザー影響なしだが、shared Docker host のディスク逼迫はインフラ全停止リスクとして顕在化していた） |
| 影響期間 | 2026-05-04〜05 頃 〜 2026-05-07（推定 48〜72 時間。ephemeral slice のログが消失したため確定不能） |
| 影響サービス | ステージング slice（識別子伏字）の alt-backend、shared Docker host |
| 影響機能 | staging 環境の Knowledge Projector / Knowledge Loop。本番影響なし |
| 関連 ADR | — |
| 関連 PM | [[PM-2026-002-unsummarized-infinite-enqueue-loop]]、[[PM-2026-010-knowledge-home-reproject-checkpoint-gap]]、[[PM-2026-027-pre-processor-summarize-retry-storm-false-dead-letter]] |

## サマリー

2026-05-07、shared Docker host の container log root が容量上限直前まで埋まっていることを検知した。原因はステージング slice（識別子伏字）の alt-backend が knowledge-sovereign に対する Connect-RPC で content-type 不整合を起こし、`KnowledgeProjector` の listener fallback ループが指数バックオフ無し / circuit breaker 無しで永久リトライを継続し、推定 48〜72 時間で **約 148GB** の container JSON ログを生成していたことであった。エラーは 3 種（`GetActiveProjectionVersion` / `GetProjectionCheckpoint` / `WatchProjectorEvents`）で、`invalid content-type: "application/json"; expecting "application/proto"` という **レスポンス側 Content-Type の mismatch** を毎サイクル吐き続けていた。本番環境および本番 alt-backend は image tag / compose stack が分離されており影響なし、データ損失・ユーザー影響もなし。一方、半日遅れていれば host 全停止 / 共有 Docker daemon stall に至っていた可能性があるため、near-miss として記録する。

## 影響

- **shared Docker host の container log root**: 容量上限直前まで使用率が上昇。約 318GB を解放してバッファを確保するまで他コンテナの I/O / inode 圧迫リスクが顕在化していた
- **ステージング slice 上の Knowledge Projector / Knowledge Loop**: 連続失敗状態が継続（機能観点では service degradation）
- **本番影響**: なし（本番 alt-backend は別 image tag / 別 compose stack で分離）
- **データ損失**: なし（`knowledge_events` 等の source of truth および projection は無傷）
- **ユーザー影響**: なし（staging 限定）
- **副次影響**: 共有 Docker daemon の I/O 圧迫の可能性。検知時点では未顕在化
- **潜在的影響**: 半日遅れれば inode 枯渇 / daemon stall によりホスト上の他サービスが連鎖停止していた可能性（near-miss として最大の懸念）

## タイムライン

| 時刻 (JST) | イベント | 確度 |
|---|---|---|
| 2026-03-23 | sovereign_client 初版が codec オプション未指定で実装される（commit `bbc99d75a`） | 確定 |
| 2026-04-23 | Knowledge Loop driver が sovereign_client に切替（commit `7e9ee9f04`） | 確定 |
| 2026-04-23 | sovereign 向け Pact CDC 追加（commit `c0e0a95f3`） | 確定 |
| 2026-04-26 | Knowledge Loop projection の重い経路を sovereign に移管。sovereign 経路のトラフィック増（commit `fcc2b5a07`） | 確定 |
| 2026-04-26 | rag-orchestrator 側に `connect.WithProtoJSON()` 追加（commit `756fcebff`）。alt-backend には未伝播 | 確定 |
| 2026-04-29 | `KnowledgeHomeAdminService` の JSON codec 修正（commit `5649505f7`）。sovereign client 本体への横展開はなし | 確定 |
| 2026-05-04 〜 05 頃 | ステージング slice（識別子伏字）がデプロイされ、alt-backend が永久リトライループ突入 | 推定（slice / ログ消失のため確定不能） |
| 2026-05-07 | shared Docker host の container log root が容量上限直前であることを検知 | 確定 |
| 2026-05-07 | 原因コンテナを特定。ログサンプリングで `invalid content-type` ループを確認 | 確定 |
| 2026-05-07 | 該当コンテナ停止 / コンテナログ削除により約 318GB を解放、249GB のバッファを確保 | 確定 |
| 2026-05-07 | ポストモーテム執筆および根本修正計画の確定 | 確定 |

## 検知

- **検知方法**: ホスト目視および `df` 系コマンドによる手動確認。alert 駆動ではない
- **検知までの時間 (TTD)**: 推定 48〜72 時間（ループ突入時刻が ephemeral slice 消失のため確定不能）
- **検知の評価**: 著しく遅い。ホストディスク使用率の閾値 alert および per-container log volume の metric が未整備で、log flooding を能動的に検知する仕組みが存在しなかった

## 根本原因分析

### 直接原因

ステージング slice の alt-backend が呼び出す knowledge-sovereign の baseURL が、**JSON を返す別 component**（nginx 404 / envoy default route / 別 service の error response 等のいずれか）に routing されていた。Connect-Go client はリクエストを binary proto（デフォルト codec）で送信していたが、レスポンスが `application/json` で返ってくるため、レスポンス側の Content-Type 検証で `invalid content-type: "application/json"; expecting "application/proto"` を毎回投げ続けた。

> 当初仮説（"client が JSON で送っている"）は誤り。実コードでは `NewKnowledgeSovereignServiceClient(httpClient, baseURL)` を無オプションで構築しており、Connect protocol のデフォルト送信 codec は binary proto。エラー文中の `application/json` はレスポンス側の Content-Type を指す。production では同じ image でも問題が出ていない事実とも整合する（production の sovereign baseURL は本物の sovereign に届いている）。

### Five Whys

1. **なぜ shared Docker host のディスクが埋まったか？**
   → ステージング slice の alt-backend が約 148GB の container JSON ログを 2 日で生成していた

2. **なぜそんな量のログが出たか？**
   → `KnowledgeProjector` の listener fallback ループが、3 種のエラー（`GetActiveProjectionVersion` / `GetProjectionCheckpoint` / `WatchProjectorEvents`）を毎サイクル構造化ログとして出し続けていた。本来 5 秒間隔の poll であるが、`WaitForNotification` がエラーで即座に return する fast-path に陥っていたため待機を挟まず ms 単位でループが回転していた

3. **なぜ毎サイクル失敗していたか？**
   → 失敗は systemic（永久に成功しない構成不整合）であったが、ループ側に bounded backoff も circuit breaker も無く、systemic / transient を区別せず同じ間隔でリトライし続けた

4. **なぜ systemic に失敗していたか？**
   → upstream の sovereign baseURL が、Connect-RPC compatible でない component（JSON を返す proxy / エラーページ等）を指していた。staging slice 固有の routing 構成で生じた誤配線

5. **なぜそれが起動時に検知できなかったか？**
   → `sovereign_client.NewClient` は config の baseURL を受け取って HTTP client を構築するだけで、その先が **本当に Connect-RPC compatible な sovereign か** を確認する health probe を行わない。誤配線は runtime まで持ち越され、最初の RPC コールで初めて顕在化し、その後は永久ループに移行する設計であった

### 根本原因（2 つ）

1. **失敗ループに bounded backoff / circuit breaker が無く、systemic 失敗が log flooding 装置になった**
   `KnowledgeProjectorRunner` は固定 5 秒間隔の retry のみで、`WaitForNotification` の error path では待機を挟まず即座に listener factory を再呼び出ししていた。systemic 失敗（永久に成功しない）と transient 失敗（一時的）を区別する仕組みが無く、log volume が無制限に成長する構造であった。これがログ量を作った最大要因

2. **upstream baseURL の正当性を起動時に検証する仕組みが無かった**
   `sovereign_client.NewClient` は config の baseURL を信用するだけで、Connect-RPC compatible なエンドポイントか否かを起動時に確認しない。誤配線が runtime まで持ち越され、最初の RPC 失敗で永久ループに突入する設計

### 寄与要因

- **staging slice の sovereign baseURL 構成不整合**: production と異なる routing で JSON を返す component に到達していた。ephemeral slice の構成生成過程で混入した誤配線と推定
- **ephemeral staging slice の構成永続化なし**: slice 生成スクリプトが一時ディレクトリに compose を生成し、削除後に痕跡を残さない設計。事後の正確な再現が困難
- **daemon-wide log-opts 設定なし**: 個別 compose には `max-size: 10m, max-file: 3` が設定済みだが、ephemeral slice や設定漏れサービスに対する保険として daemon 全体のデフォルト log-opts が無い
- **ディスク使用率 / per-container log volume の alert 不在**: 観測 runbook にディスク逼迫 alert / log volume alert の定義が無く、log flooding が能動的に検知されない
- **過去 PM の retry storm 教訓が新規 retry loop の設計に反映されていなかった**: PM-2026-002 / PM-2026-027 で retry storm の教訓があったにも関わらず、新規 retry loop の実装で bounded backoff / circuit breaker が組み込まれなかった。横展開メカニズム自体の課題

## 対応の評価

### うまくいったこと

- 検知後、原因コンテナの特定とログサンプリングを短時間で完了し、約 318GB を即時解放できた
- 本番への影響波及がゼロ（image tag / compose stack による slice 分離が想定通り機能した）
- `knowledge_events` をはじめとする source of truth および projection に一切影響がなく、データ修復は不要だった
- 解放後 249GB のバッファを確保でき、当面のディスク逼迫リスクが解消した

### うまくいかなかったこと

- 検知が推定 48〜72 時間遅れた。能動的なディスク使用率 / log volume alert が無く、目視に依存していた
- ephemeral slice の構成が削除後に追跡不能で、誤配線の原因 component を特定できなかった
- 既存の codec / Connect-RPC 修正（`5649505f7`、`756fcebff`）が他経路に横展開されておらず、retry storm 系の過去 PM 教訓も新規 runner の設計に反映されていなかった

### 運が良かったこと

- shared Docker host の全停止前に気付けた。半日遅れていれば inode 枯渇 / daemon stall によりホスト上の他サービスが連鎖停止する可能性があった
- 影響が staging 限定で完結し、本番ユーザーへの波及が無かった
- イミュータブルデータモデル（append-first event log）により、source of truth は何の修復作業も必要としなかった

## アクションアイテム

### 予防（Prevent）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| P-1 | `KnowledgeProjectorRunner` に **bounded exponential backoff**（初期 1s、係数 2、上限 30s、成功で reset）と **circuit breaker**（連続失敗 5 回で open、open 中は per-cooldown の単発ログのみ、cooldown 60s）を導入 | 開発担当 | 2026-05-31 | **完了 (2026-05-07)** |
| P-2 | `sovereign_client.NewClient` に **起動時 health probe** を追加。cheap な unary RPC（`GetActiveProjectionVersion`）を 1 度叩き、`invalid content-type` 等の非 Connect 応答を検出した場合は **operator 向けに loud warning ログを出す**（client は `enabled=true` のまま維持）。当初は misroute 検出時に degrade させる設計としたが、Connect-RPC の "endpoint 未実装" 応答 (test stub の catch-all 等) と "misroute" 応答 (本番 nginx 404 等) が wire 上区別不能であり、CI 環境の deps-stub catch-all で false-positive degrade を起こしてカスケード障害を生んだため (PM-2026-042 と同セッション内に発覚、`fix(alt-backend): make sovereign client health probe observation-only`)、observation-only に改訂した。runtime 防御は P-1 (bounded backoff + circuit breaker) が担う | 開発担当 | 2026-05-31 | **完了 (2026-05-07、改訂を含む)** |
| P-3 | staging slice 生成フローで sovereign baseURL の到達性を事前検証。production と routing が乖離している場合は CI で失敗させる | 開発担当 | 2026-05-31 | 未着手 |

### 検知（Detect）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| D-1 | shared Docker host のディスク使用率 alert（>80% / >90% / >95% の三段）を Prometheus + alertmanager に追加 | 運用担当 | 2026-05-31 | 未着手 |
| D-2 | per-container log volume metric を取得し、5 分で 100MB を超えた場合に warn する rule を追加 | 運用担当 | 2026-05-31 | 未着手 |
| D-3 | `KnowledgeProjectorRunner` の連続失敗カウンタを `slog` に出し、N 回連続失敗で error level に escalation する仕組みを追加（P-1 の circuit breaker と相補） | 開発担当 | 2026-05-31 | 未着手 |

### 緩和（Mitigate）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| M-1 | 同種エラーログの dedupe / sampling: 連続同一 error message は最初の N 回後 per-minute 集約に切替（P-1 の circuit breaker が入れば不要になる可能性あり、実装後に再判断） | 開発担当 | 2026-05-31 | 未着手 |
| M-2 | Docker daemon の `/etc/docker/daemon.json` に daemon-wide log-opts（`max-size: 100m`、`max-file: 3`）を設定し、設定漏れ slice の保険を有効化 | 運用担当 | 2026-05-31 | 未着手 |
| M-3 | ephemeral staging slice 生成スクリプトが生成する compose を専用ディレクトリにコピー保存し、削除前に追跡可能化 | 運用担当 | 2026-05-31 | 未着手 |

### プロセス（Process）

| # | アクション | 担当 | 期限 | 状態 |
|---|---|---|---|---|
| R-1 | 過去 PM のアクションアイテム未実装が再発を招くパターンに対する横展開チェックリストを整備。新規 retry loop / 新規 RPC client の追加時に PM-2026-002 / PM-2026-027 / 本 PM の教訓が必須レビュー項目になる仕組みを導入 | 開発担当 | 2026-05-31 | 未着手 |
| R-2 | `utils/logger` の global Logger を package init() で pre-initialize し、`InitLoggerWithOTel` は内部 `dynamicHandler` (`atomic.Pointer[slog.Handler]`) を atomic swap する設計に変更。Logger ポインタ自体を不変にしたことで、test 間で漏れた fire-and-forget goroutine と test setup の `InitLogger()` 呼び出しが衝突する data race を構造的に排除。Go stdlib `log/slog` の `defaultLogger atomic.Pointer[Logger]` パターンと uber-go/zap の `ReplaceGlobals` 設計をベースに採用 | 開発担当 | 2026-05-07 | **完了 (2026-05-07)** |

## 教訓

### 技術的教訓

1. **bounded backoff + circuit breaker は optional ではなく構造的要件**: 5 秒固定リトライ（fast-path では待機なし）は systemic 失敗に対しては log flooding 装置でしかない。外部 RPC 依存を持つ retry loop の実装には bounded backoff と circuit breaker を最初から組み込むべき
2. **外部 RPC client は起動時に upstream の正当性を観測すべき (ただし degrade はしない)**: baseURL の構成不整合は ephemeral 環境で容易に混入する。startup health probe を入れることで operator-facing logs に loud な警告を出せる。一方、content-type シグナル単独で client を degrade してしまうと、Connect-compatible だが該当 RPC 未実装の test stub / catch-all 環境で false-positive のカスケード障害を起こす (本 PM の P-2 改訂で実証)。runtime 防御は bounded backoff + circuit breaker (P-1) に任せる
3. **container log volume は health の first-class metric**: ディスク逼迫 alert がないインフラでは、log flooding が起きると必ず全停止リスクに直結する。host 共有 Docker daemon を使う構成では特に重要
4. **Connect-RPC の content-type 不整合エラーは upstream routing をまず疑う**: client codec のデフォルトは binary proto なので、`expecting "application/proto"` というエラーが出たらレスポンス側 Content-Type を見る。client codec を変える対処（`WithProtoJSON()` 追加等）は最後の選択肢

### 組織的教訓

1. **過去 PM の教訓が横展開されないと再発する**: PM-2026-002 / PM-2026-027 で同種の retry storm 教訓があったにも関わらず、新規 retry loop の設計に反映されなかった。アクションアイテム単位の追跡だけでなく、教訓そのものを設計レビューに組み込むメカニズムが必要
2. **ephemeral 環境の構成は永続化しないと事後分析が困難**: 一時ディレクトリに生成して削除する設計は、再現が必要な障害が起きた時に致命的に情報が欠ける。最低限、生成物のスナップショットを保存する設計にすべき
3. **near-miss も記録する価値がある**: 本件は本番影響ゼロだが、半日のタイミング差で host 全停止に至る寸前であった。near-miss を記録することで、まだ表面化していない構造的脆弱性に対する体質改善対策を確定できる

## 参考資料

- `alt-backend/app/job/knowledge_projector_runner.go` — retry loop（P-1 適用済）
- `alt-backend/app/job/knowledge_projector_runner_test.go` — backoff / breaker テスト（P-1 適用済）
- `alt-backend/app/driver/sovereign_client/client.go` — sovereign client（P-2 適用済）
- `alt-backend/app/driver/sovereign_client/client_test.go` — health probe テスト（P-2 適用済）
- `alt-backend/app/utils/logger/init.go` — dynamicHandler + atomic.Pointer 化（R-2 適用済）
- 参考: [Go log/slog 公式 — defaultLogger atomic.Pointer 実装](https://go.dev/src/log/slog/logger.go)
- 参考: [uber-go/zap global.go — ReplaceGlobals](https://github.com/uber-go/zap/blob/master/global.go)
- `alt-backend/app/driver/sovereign_client/watch_client.go` — streaming client
- `knowledge-sovereign/app/main.go` — server handler 登録
- `proto/services/sovereign/v1/sovereign.proto` — proto 定義
- `compose/acolyte.yaml` — 個別 compose の log-opts 例
- `docs/runbooks/admin-observability.md` — 観測 runbook（D-1 / D-2 の追加対象）
- [[PM-2026-002-unsummarized-infinite-enqueue-loop]] — 関連 PM（要約パイプラインの retry storm）
- [[PM-2026-010-knowledge-home-reproject-checkpoint-gap]] — 関連 PM（同 projector 周辺）
- [[PM-2026-027-pre-processor-summarize-retry-storm-false-dead-letter]] — 関連 PM（pre-processor の retry storm）
- 関連 commit: `bbc99d75a`、`7e9ee9f04`、`c0e0a95f3`、`fcc2b5a07`、`756fcebff`、`5649505f7`

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
