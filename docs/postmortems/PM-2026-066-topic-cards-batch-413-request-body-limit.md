---
title: "ポストモーテム: 日次 Topic Cards バッチが recap-subworker のリクエストボディ上限（64 MiB）で HTTP 413 拒否され、1 枠分更新されなかった障害"
date: 2026-11-20
tags:
  - postmortem
  - recap-worker
  - recap-subworker
  - topic-cards
  - recap
---

# ポストモーテム: 日次 Topic Cards バッチが recap-subworker のリクエストボディ上限（64 MiB）で HTTP 413 拒否され、1 枠分更新されなかった障害

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-066 |
| 発生日時 | 2026-11-19 02:30:00 (JST)（定時スロット 17:30 UTC の実行が 02:33:18 に `failed` で終了。直前の成功実行は 2026-11-18 02:34:20 JST） |
| 検知日時 | 2026-11-20 00:07 (JST)（運用者が本番で別リリースの確認中に HTTP 413 を発見。アラートによる検知ではない） |
| 復旧日時 | 2026-11-20 01:31:36 (JST)（キャッチアップバッチ完了、12 カード生成） |
| 影響時間 | 23 時間 01 分（2026-11-19 02:30〜2026-11-20 01:31:36、定時 1 枠が欠落） |
| 重大度 | SEV-3（日次 Topic Cards バッチが 1 枠欠落（計 7 試行が失敗）し、23 時間 01 分にわたり前回生成分を表示。データ損失なし・他の recap バッチへの波及なし） |
| 作成者 | Recap 担当 |
| レビュアー | — |
| ステータス | Draft |

## サマリー

2026-11-19 02:30:00 (JST) の日次 Topic Cards バッチから翌未明の復旧まで、`recap_jobs` 台帳（`trigger_source = 'cards'`）上で定時枠と 15 分間隔の再試行 2 回（02:30:00〜02:33:18、02:48:18〜02:50:58、03:05:58〜03:08:35）、さらに同日夜の無関係なリリース 2 回に伴う `recap-worker` 再起動後のキャッチアップ実行 4 回（22:39:29〜22:42:45、23:11:37〜23:14:29、23:29:29〜23:32:09、23:47:09〜23:49:56）の計 7 試行が連続で `failed` となった。このうち最後の 3 回（23:14、23:32、23:49）はコンテナログから `recap-subworker` の `POST /v1/cluster-stories` による HTTP 413（リクエストボディ上限超過）での拒否を確認しており、未明 3 回と 22:42 の失敗理由は再起動に伴うコンテナログ消失・ログストア未収集・台帳へのエラー文言非保持のため未計測である（なお 02:30 枠の 72 時間ウィンドウは 5,651 件を保持しており、HTTP 413 上限の約 5,209 件を上回る点で整合する）。原因は、`recap-subworker` の `RequestSizeLimitMiddleware` に設定されていた上限 `RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES` が 2026-09-22 以降 64 MiB（67,108,864 バイト、1,024 次元埋め込みベクトル付きJSON で約 5,209 件相当）に固定されており、72 時間ウィンドウの取得件数が約 5,531〜5,651 件へ増大したことで上限を超えたためである。また、`recap-worker` は失敗カウンタ `recap_surface_consecutive_failures{surface="topic_cards"}` を全期間出力しており、該当アラートルールも 2026-11-09 にリポジトリへ追加済みだったが、本番の Prometheus が 2026-10-31 以降一度も設定を reload していなかったためアラートが発報せず、約 21 時間半にわたり無検知となった。

修正では当初 128 MiB への引き上げを検討したが、クラスタリング時のメモリ消費がバッチサイズの二乗に比例して増大する（開発計測で約 54 バイト/アイテム対、128 MiB ≒ 約 10,400 件ではコンテナ上限 8 GiB に対して約 5.8 GiB に達する）ことが判明したため、上限を 96 MiB（100,663,296 バイト、約 7,800 件）に抑え、コンテナメモリ上限の半分以内に収まることおよび compose とコード既定値の一致を固定するテストを追加した（PR #381）。2026-11-20 01:15:11 までに本番反映を完了し、01:31:36 にキャッチアップバッチが成功（fetched 5,459 件、dedup 後 5,141 件、12 カード）して復旧した。ただし復旧時の 5,141 件（約 66.2 MB）は旧 64 MiB 上限（67.1 MB）もわずかに下回っており、新上限下で 64 MiB を超える実バッチの挙動は未計測である。

## 影響

- **影響を受けたサービス:** recap-worker（Topic Cards バッチ生成）、recap-subworker（`POST /v1/cluster-stories` で 413 を返却）、alt-frontend-sv（Topic Cards 画面の表示更新停止）
- **影響を受けたユーザー数/割合:** 全利用者（単一利用者の環境）
- **機能への影響:** Topic Cards の日次更新が 1 枠欠落（2026-11-19 02:30〜2026-11-20 01:31:36 の 23 時間 01 分）
  - 影響期間中、表示される Topic Cards は 2026-11-18 02:34 時点の生成結果のまま据え置かれた（なお `recap_jobs` には 11-19 02:33:18 から `trigger_source = 'cards'` の最新実行として `failed` が記録されていた。コード読解上、`recap-worker` の `GET /v1/topic-cards` が `get_latest_cards_run` で最新実行状態を取得して `surface_status.latest_run` に設定し、`alt-backend` 経由で受け取った `alt-frontend-sv` の `surface-status.ts` が `isStale` より先に `latestRun?.status === "failed"` を判定するため、02:33:18 以降（再試行実行中の `running` を除く）は失敗バナーが表示されるはずの実装だった。ただし障害発生中に画面を実際に閲覧した者はおらず、実表示は未確認である）
  - 他の recap バッチは影響を受けず、themes バッチは同夜に完了した（適格クラスタ 41 件、テーマ版 5 件）
- **データ損失:** なし（元記事・フィードデータは保持されており、復旧バッチで 12 カードが正常生成された）
- **SLO/SLA違反:** 判定不能（Topic Cards の日次スロット欠落に対する明示的な SLO は未定義。既存の `observability/prometheus/rules/recap-alerts.yml` には 36 時間鮮度超過や連続失敗のルールが定義されていたが、本番 Prometheus が未 reload で読み込んでいなかったため本件でアラートは発報しなかった）

## タイムライン

すべて JST。

| 時刻 (JST) | イベント |
|-------------|---------|
| 2026-09-22 | `recap-subworker` のリクエストボディ上限（`RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES`、`RequestSizeLimitMiddleware` で強制）が 64 MiB（67,108,864 バイト）に設定され、以後変更されないまま運用される（コミット `22a040c28`） |
| 2026-10-31 08:15 | 本番の Prometheus プロセスが起動し、同時刻に設定を読み込む（以後、本件調査まで一度も reload されず、`recap-alerts.yml` からは `RecapSubworkerDown` 1 本のみで稼働） |
| 2026-11-09 | `observability/prometheus/rules/recap-alerts.yml` に `RecapSurfaceStale`、`RecapSurfaceConsecutiveFailures`（`>= 2`、`for: 5m`）、`RecapSurfaceMetricsAbsent` が追加される（コミット `42d52f13c`、`28526e944`）。コンテナへは読み取り専用バインドマウントで反映されたが、Prometheus の reload は行われない |
| 2026-11-18 02:34:20 | 障害前最後の成功バッチが完了（fetched 5,135 件、dedup 後 4,816 件、クラスタ 2,174 件、12 カード、クラスタリング 3,960 ms、合計 260 s）。なお `recap_card_job_stats`（成功時のみ記録）上の障害前最大成功実績は dedup 後 5,184 件、最大取得ウィンドウは 5,514 件だった |
| 2026-11-19 02:30:00〜02:33:18 | **最初の異常（定時枠の失敗 — 1 回目）** — 日次定時実行（17:30 UTC）が起動し、`recap_jobs`（`trigger = cards`）に `failed`（stage `cards_pipeline`）で記録される。`recap_card_job_stats` は成功時のみ挿入されるため失敗行は残らない。事後に元データ行から数えた当該 72 時間ウィンドウの取得対象は 5,651 件（旧上限約 5,209 件超過）であり HTTP 413 と整合するが、失敗理由は未計測（後の再起動で当時のコンテナログが失われ、ClickHouse にも `recap-worker` のログ行が存在せず、台帳もエラー文言を保持しない）。02:35 時点の `recap_surface_consecutive_failures{surface="topic_cards"}` は `1` |
| 2026-11-19 02:48:18〜02:50:58 | 定時枠の再試行 1 回目（2 回目の失敗）が `failed`（理由未計測）。02:55 時点の `recap_surface_consecutive_failures` は `2` |
| 2026-11-19 03:00 頃 | **通知されなかった理論上の発火時刻** — `RecapSurfaceConsecutiveFailures`（`>= 2` で `for: 5m`）が Prometheus に読み込まれていれば firing に転じていた時刻（人手検知より約 21 時間前） |
| 2026-11-19 03:05:58〜03:08:35 | 定時枠の再試行 2 回目（3 回目の失敗）が `failed`（理由未計測）。03:10 時点の `recap_surface_consecutive_failures` は `3` |
| 2026-11-19 22:24 頃 | 本件と無関係なリリース 1 回目により `recap-worker` が再起動（起動 15 分後にキャッチアップが開始することからの逆算。正確な再起動時刻は未計測） |
| 2026-11-19 22:39:29〜22:42:45 | 1 回目の再起動後キャッチアップ実行（4 回目の失敗）が `failed`（理由未計測）。22:45 時点の `recap_surface_consecutive_failures` は `4` |
| 2026-11-19 22:56 頃 | 本件と無関係なリリース 2 回目により `recap-worker` が再起動（次回キャッチアップ初回試行 23:11:37 からの逆算。正確な再起動時刻は未計測） |
| 2026-11-19 23:11:37〜23:14:29 / 23:29:29〜23:32:09 / 23:47:09〜23:49:56 | 2 回目の再起動後キャッチアップ実行 3 回（5〜7 回目の失敗、`RECAP_CARDS_MAX_ATTEMPTS=3`、15 分間隔）がすべて `failed`。コンテナログ上で 3 回とも `POST /v1/cluster-stories` からの HTTP 413 を確認（72 時間ウィンドウは fetched 5,531 件）。`recap_surface_consecutive_failures` は 23:15 に `5`、23:35 に `6`、23:55 に `7` を記録 |
| 2026-11-20 00:07 | **検知・対応開始** — 運用者が本番環境で別リリースの動作確認をしていた際に HTTP 413 を発見（アラートによる検知ではない）。調査を開始 |
| 2026-11-20 00:09 | **原因特定** — リクエスト形状（1 記事あたり 1,024 次元埋め込みで JSON 12,770 バイト + アイテム固定部 113 バイト + エンベロープ 107 バイト ⇒ 64 MiB の許容上限は約 5,209 件）と実ウィンドウ件数を照合し、リクエストボディ上限超過を特定 |
| 2026-11-20 00:09〜00:54 | 修正設計を実施。初版では上限を 128 MiB へ引き上げたが、クラスタリングのメモリ消費がバッチサイズの二乗に比例する（開発計測で約 54 バイト/アイテム対）ことから、128 MiB（約 10,400 件）では約 5.8 GiB となりコンテナの 8 GiB 制限（`mem_limit: 8192m`、シングルワーカー、待機時約 0.56〜0.66 GiB）に対して過大と判明。96 MiB（約 7,800 件）へ再設計し、最大許容バッチがコンテナ上限の半分以内に収まることおよび compose とコード既定値の一致を固定するテストを追加（PR #381: 失敗テスト `1fc2af7e3`、修正 `971ea4c5c`、全 83 チェック通過） |
| 2026-11-20 00:54:22 | PR #381 がマージされる（`5720a4623`） |
| 2026-11-20 01:00:50〜01:15:11 | **緩和・修正反映** — リリースパイプラインが実行され成功。`recap-subworker` と `recap-worker` が新版・healthy で起動し、環境変数の上限値 `100,663,296` を確認 |
| 2026-11-20 01:13:34 | 新版 `recap-worker` が起動時の未完了スロット検知で `batch_catchup_scheduled`（15 分の猶予遅延）を出力 |
| 2026-11-20 01:28:34 | キャッチアップバッチのジョブが開始（`recap_jobs` 上の開始時刻。01:28:51 に最初のパイプラインログを出力） |
| 2026-11-20 01:31:11 | `POST /v1/cluster-stories` が HTTP 200 を返却（クラスタリング 3,274 ms） |
| 2026-11-20 01:31:36 | **復旧** — バッチ完了（`completed`、fetched 5,459 件、ノイズフィルタ後 5,175 件、dedup 後 5,141 件、クラスタ 2,373 件、12 カード、合計 181 s）。次回は通常どおり 02:30 定時枠に設定され、01:35 時点で `recap_surface_consecutive_failures{surface="topic_cards"}` は `0` へ復帰した |

## 検知

- **検知方法:** 運用者による目視確認（本番で別リリースを確認中に `recap-subworker` への HTTP 413 エラーを発見）。自動アラートによる検知ではない
- **検知までの時間 (TTD):** 21 時間 34 分（2026-11-19 02:33:18 の初回失敗完了 → 2026-11-20 00:07 の発見。23:14 の失敗からは約 53 分）
- **検知の評価:** 不十分。失敗メトリクスの出力とアラートルールの定義は揃っていたが、稼働中の Prometheus が設定を一度も reload していなかったこととログ収集の欠損が重なり、人手での確認まで気付けなかった
  - **連続失敗メトリクスは全期間出力されていた:** `recap-worker` は `recap_surface_consecutive_failures{surface="topic_cards"}` を継続出力しており、5 分解像度で 11-19 02:35 に `1`、02:55 に `2`、03:10 に `3`、22:45 に `4`、23:15 に `5`、23:35 に `6`、23:55 に `7` を記録し、11-20 01:35 に `0` へ戻った
  - **Prometheus が未 reload でルールを読んでいなかった:** `observability/prometheus/rules/recap-alerts.yml` には 2026-11-09 に `RecapSurfaceStale`、`RecapSurfaceConsecutiveFailures`（`>= 2`、`for: 5m`）、`RecapSurfaceMetricsAbsent` が追加されていた（コミット `42d52f13c`、`28526e944`）。しかし本番の Prometheus プロセスは 2026-10-31 08:15 JST に起動し同時刻に設定を読み込んだまま一度も reload されておらず、マウント先の `recap-alerts.yml` に 4 ルールが存在するにもかかわらずメモリ上では 1 ルール（`RecapSubworkerDown`）だけで稼働していた。直近 48 時間のアラート履歴にも `RecapSurface*` はなく、発火していたのは常時稼働の watchdog のみだった。ルールが読み込まれていれば 11-19 03:00 頃（人手検知より約 21 時間前）に `RecapSurfaceConsecutiveFailures` が発火していた
  - **既知の reload ギャップとタイマー未導入:** ルールディレクトリはリポジトリのチェックアウトから読み取り専用でマウントされているためファイル更新自体はコンテナ内へ届くが、Prometheus は起動時か明示的な reload 時にしかルールを読み直さず、リリースパイプラインのどの段階も reload を実行しない。この構造は `docs/runbooks/observability-config-reload.md`（2026-08-10 作成）で既に記録されており（2026-08-02 起動の Prometheus が 08-09 追加の 9 ルールを読まないまま 8 日間稼働した事例）、差分検知と反映を行う `scripts/observability-reload.sh`（`--check` でドリフト報告）、systemd timer（`make observability-reload-install`）、および `PrometheusConfigReloadStale` アラート案が記載されていた。しかし本番ホストに当該 timer は導入されておらず、`PrometheusConfigReloadStale` も `observability/` 配下に未定義のままだった。なお、本件調査中に実施した読み取り専用のドリフト検査では、ディスク上には存在するが未読込のルールとして `RecapSurfaceConsecutiveFailures`、`RecapSurfaceMetricsAbsent`、`RecapSurfaceStale`、`HarvesterOutboxEventsFailing`、`AugurDocumentOwnerCoverageLow`、および記録ルール群 `admin-monitor-recordings.yml` 全体が報告された（ディスク上のルールファイル自体はすべて検証を通過している）
  - **ルール側の別の罠（廃止サーフェス残存）:** 未読込の `RecapSurfaceStale`、`RecapSurfaceConsecutiveFailures`、`RecapSurfaceMetricsAbsent` はいずれも 2026-11-19〜20 に廃止された `evening_pulse` を対象に含めたままになっている。このまま Prometheus を reload すると 30 分後に存在しないサーフェスに対して `RecapSurfaceMetricsAbsent` が発火するため、ルール修正（別途進行中）の完了後に reload を行う必要がある
  - **`recap_card_job_stats` の仕様とログ収集欠損:** `recap_jobs` には失敗試行も `failed`（stage `cards_pipeline`）で記録されるがエラー文言は保持せず、`recap_card_job_stats` は `recap-worker/recap-worker/src/pipeline/cards/pipeline.rs` の最終段 `stage_persist`（`persist_pipeline_output`）で成功時のみ挿入されるため失敗行が存在しない。加えて `recap-worker` のログがログストア（ClickHouse）に入っていないことが本件調査で判明し、再起動前の 02:30 枠 3 回と 22:42 の失敗理由がコンテナログ消失とともに追えなくなった
  - **サイズ上限に対する事前監視・テストの不在:** リクエストボディ上限は固定バイト数（64 MiB）のまま放置されており、72 時間ウィンドウのアイテム数増加や日次変動（fetched 約 4,500〜5,650 件）に対して事前に警告・拒否するテストやアラートが無かった

## 根本原因分析

### 直接原因

`recap-worker` の Topic Cards バッチが 72 時間ウィンドウ内の記事（埋め込みベクトル付き JSON）を `recap-subworker` の `POST /v1/cluster-stories` へ送信した際、取得件数が約 5,531〜5,651 件規模へ増加した結果、リクエストボディが `RequestSizeLimitMiddleware` の 64 MiB（67,108,864 バイト、約 5,209 件上限）を超過し、HTTP 413 で拒否された（最後の 3 試行はコンテナログで HTTP 413 を確認、未明の 3 試行と 22:42 の試行はログ消失のため理由は未計測）。

### Five Whys

1. **なぜ Topic Cards の日次バッチが失敗し更新が止まったか？** → `recap-worker` から `recap-subworker` の `POST /v1/cluster-stories` へのリクエストが HTTP 413（`Request body too large`）で拒否されたから
2. **なぜリクエストが 64 MiB の上限を超えたか？** → 2026-09-22 に設定された 64 MiB（1,024 次元埋め込みの JSON 表記で約 5,209 件相当）の上限に対し、直近の 72 時間ウィンドウが fetched 5,531〜5,651 件に成長していたから（過去の最大成功は dedup 後 5,184 件・fetched 5,514 件であり、ちょうど境界を跨いでいた）
3. **なぜリクエストサイズが件数に比例して無制限に膨らむ構造になっていたか？** → `recap-worker` 側のクラスタリング前に件数上限（またはコンパクトなベクトル符号化）を設けておらず、72 時間分の全アイテムの 1,024 次元浮動小数点配列をそのまま JSON テキスト（1 件あたり約 12,883 バイト）で単一リクエストに載せていたから
4. **なぜデータ増加による上限到達を事前に検知・防止できなかったか？** → リクエストボディ上限を固定バイト値として設定しただけで、ウィンドウ件数の成長傾向やメモリ予算と結び付けたテスト・しきい値監視が存在しなかったから。さらにクラスタリング側のメモリ消費もアイテム数の二乗（$O(N^2)$）で増えるため、単純にバイト上限だけを大きくするとコンテナ OOM を招く関係にあった
5. **なぜシステム全体として運用者が気付くまで 21 時間 34 分放置されたか？** → `recap-worker` は連続失敗メトリクスを出力し、該当アラートルールも 2026-11-09 にはリポジトリへ追加されていたが、本番の Prometheus が 2026-10-31 以降一度も reload されておらず（リリースパイプラインに reload が無く、ランブック記載の自動 reload timer も本番ホストに未導入、`PrometheusConfigReloadStale` も未定義だった）、新ルールが読み込まれていなかったうえ、`recap-worker` のログもログストアに届いていなかったから

### 根本原因

**データ量の成長に依存する単一 JSON リクエスト上限（64 MiB、約 5,209 件）が、入力件数の上界制御もメモリ予算（$O(N^2)$ のクラスタリング空間計算量とコンテナ `mem_limit: 8192m`）との連動テストも無いまま固定値で運用されており、さらにこの種の失敗を捕捉するアラートルールが 10 日前にリポジトリへ追加済みだったにもかかわらず、Prometheus の設定 reload 機構（パイプライン連携・自動 reconcile timer・reload 停滞アラート）が稼働環境に導入されておらず未読込のまま放置されていたこと。**

日次ウィンドウ件数は約 4,500〜5,650 件の間で日々変動するため、根本修正が無ければ境界付近で成功と HTTP 413 失敗を交互に繰り返す状態だった。

### 寄与要因

- **JSON 埋め込みペイロードの肥大化と上限の固定値化:** 1 アイテムあたり 1,024 次元埋め込みの JSON が 12,770 バイト（アイテム合計 12,883 バイト）を占め、2026-09-22（当時約 2,000 件・約 25 MB 前提で 64 MiB を設定）から 2 か月弱のフィード増加で許容上限（約 5,209 件）に到達した
- **クラスタリングの $O(N^2)$ メモリ消費:** `StoryClustererService.cluster_stories` は全ペアの類似度・時間減衰・距離行列（$N \times N$）を構築するためメモリが件数の二乗で増大し、単純なボディ上限の倍増（128 MiB ≒ 10,400 件 ≒ 約 5.8 GiB）では 8 GiB コンテナの安全余裕を脅かす制約があった
- **HTTP 413 に対するバッチ単位の再試行:** `SubworkerCardsClient::cluster_stories` は HTTP 413 を非リトライとしている一方、`CardsBatchTarget::is_fatal_error` は認証エラー（401/403）のみを終端扱いとするため、`CardsBatchDaemon` は同じ過大ペイロードのまま 15 分間隔で 3 回ずつ（02:30 枠で 3 回、夜間の再起動後 23:11〜23:49 で 3 回など計 7 試行）バッチ全体を再試行した
- **`recap-worker` ログのログストア欠損と再起動によるログ揮発:** `recap-worker` のログが ClickHouse に収集されていなかったため、22:24 頃および 22:56 頃の 2 回のコンテナ再起動によって 02:30 定時枠 3 回と 22:42 実行分のコンテナログが完全に失われた
- **Prometheus の未 reload とランブック対策の未導入:** ルールディレクトリはチェックアウトから読み取り専用でマウントされているが、Prometheus の設定反映（reload）がリリースパイプラインに組み込まれておらず、`docs/runbooks/observability-config-reload.md`（2026-08-10）に記載された systemd timer も本番ホストに未導入、`PrometheusConfigReloadStale` も未定義のままだった。結果として 11-09 追加の `RecapSurface*` ルールを含む複数ルールが 10 日間以上メモリへ読み込まれなかった

### 復旧における前提条件・不確実性（明示的留保事項）

本件は 2026-11-20 01:31:36 のキャッチアップ成功をもって稼働面は復旧したが、次の 2 点が**未検証・未確定の留保事項**として残っている。

1. **新上限（96 MiB）下で旧上限（64 MiB）を超えるバッチは未観測（未計測）:** 復旧に成功した 01:31:36 のバッチは dedup 後 5,141 件であり、実測の 1 件あたりサイズで換算すると約 66.2 MB と旧 64 MiB 上限（67,108,864 バイト ≒ 67.1 MB）をわずかに下回る規模だった。そのため、96 MiB への引き上げ自体が 64 MiB 超のリクエストを本番で通す実証はまだ完了していない。
2. **本番コンテナ内のクラスタリング実メモリ消費がテスト前提定数を上回る可能性:** 復旧バッチ後の本番コンテナ実測では、コンテナ統計上のメモリ使用量が待機時 556 MiB、バッチ後が 857 MiB、コンテナ起動以降にカーネルが記録したメモリピークは 2,480,824,320 バイト（約 2.31 GiB）、OOM イベント 0 件だった。このピークには起動時のメモリ確保も含まれておりクラスタリング分だけを切り分けることはできないが、仮に全量を 5,141 件のクラスタリング消費とみなすと約 72 バイト/アイテム対に相当する。PR #381 のメモリ予算テストは開発計測の 54 バイト/アイテム対を用いて「96 MiB 上限でもコンテナ 8 GiB の半分以内に収まる」と固定しているが、72 バイト/アイテム対で外挿すると 96 MiB 上限（約 7,800 件）では約 4.9 GiB となり、8 GiB 上限には収まるものの「コンテナ上限の半分（4 GiB）」というテスト上の安全余裕を上回る計算になる。

## 失敗の型と再発

- 失敗の型: FP-16, FP-13, FP-05, FP-07, FP-19
  - `FP-16`（検知が届かない）: 「アラートルールやプローブは書いたが、配送経路に繋がっていない」「新しいアラートルールは、プロビジョニングパスと scrape 対象に含まれ、Alertmanager 等の通知先まで届く E2E 経路が検証されているか」（先例 `ADR-000286`, `PM-2026-054`, `PM-2026-062`）に該当。`recap-alerts.yml` にルールを書き `recap_surface_consecutive_failures` も出力されていたが、Prometheus 未 reload により評価・通知へ到達せず、`recap-worker` のログもログストアへ届いていなかった
  - `FP-13`（存在と鮮度の取り違え）: 「ディスク上の成果物が差し替わっても、常駐プロセスが起動時にメモリへ読み込んだコピーを握り続ける」「証明書・トークン・設定・モデルをロードする長期稼働プロセスは、ディスク上の更新やローテに追随して再読み込みするか」（先例 `PM-2026-029`）に該当。ルールファイルは読み取り専用マウントでディスク上に更新されていたが、2026-10-31 起動の Prometheus が起動時設定をメモリに保持し続け、11-09 追加の 3 ルールを読み込まなかった
  - `FP-05`（終端しない失敗と、外側が先に切れる待ち）: 「再試行しても成功しない失敗（参照先なし、継続する 403、入力が規約外）は、候補集合から外れる終端状態を持つか」（先例 `ADR-000296`）に該当。`CardsBatchTarget::is_fatal_error` が 401/403 のみを終端扱いとしたため、入力超過の HTTP 413 でも同一ペイロードで 15 分間隔の再試行（各回最大 3 回）を繰り返した
  - `FP-07`（検証が本番の失敗形を見ない）: `observability-validate` がディスク上の構文・ルール検証を通しても、`scripts/observability-drift-check.py` は手動実行前提で稼働中プロセスとの乖離を CI で見られず（`FP-16` 機械的検査欄の指摘と一致）、また 64 MiB 上限にもウィンドウ成長・$O(N^2)$ メモリ予算と連動するテストが無かった
  - `FP-19`（持ち越された宿題が次の障害になる）: 「ポストモーテムのアクションアイテム…が…持ち越される。あるいは設定ファイルを作っただけで配送や実経路の E2E 確認なしに『完了』にされる」「実経路で発火・配送・拒否されることまで確認したか」（先例 `PM-2026-054`, `PM-2026-062`）に該当。`PM-2026-052` #8 → `PM-2026-062` #5 の日次ジョブ失敗アラートはルール追加後も未 reload のまま持ち越され、さらに 2026-08-10 の `docs/runbooks/observability-config-reload.md` が記録した 8 月の未読込事例に対する自動 timer と `PrometheusConfigReloadStale` も本番へ未導入のまま同じ穴を踏んだ
- 再発: PM-2026-062（日次 Recap バッチ失敗が無検知のまま長期化した構造の再演。なお 2026-08-02 起動の Prometheus が 08-09 追加の 9 ルールを 8 日間読み込まなかった 8 月の事例は `docs/runbooks/observability-config-reload.md` に記録があるが、対応するポストモーテムは存在しない）
- 持ち越し: PM-2026-062 #5（および前身の PM-2026-052 #8：Topic Cards を含む日次 Recap ジョブの連続失敗・経過時間アラート。`observability/prometheus/rules/recap-alerts.yml` にルール自体は 2026-11-09 に追加されていたが稼働中 Prometheus へ未読込であり、台帳上も TODO のまま残っていたため本件でも発報に至っていない）
- 止められたはずの検査: (1) 72 時間ウィンドウの実件数成長と `O(N^2)` クラスタリングメモリ予算（`mem_limit: 8192m` の半分以内）および compose 設定値・コード既定値の一致を検証するユニットテスト（PR #381 で追加）。(2) 本番ホストへの `make observability-reload-install`（5 分間隔の差分 reload と 26 時間心拍）および `PrometheusConfigReloadStale` アラートとリリースパイプライン末尾での自動 reload。(3) ルール内の廃止サーフェス参照の検出、および `recap-worker` のログ収集経路（ClickHouse への到達）を継続検証する監視

## 対応の評価

### うまくいったこと

- 00:07 の発見から約 2 分（00:09）でアイテム数・1 件あたり JSON サイズ・64 MiB 上限の実測突合により原因を特定できた
- 修正時に 128 MiB 案をそのまま出さず、クラスタリングの二乗メモリ増加を開発計測で確認して 96 MiB へ引き下げ、メモリ予算と compose/コード整合を固定するテスト（`1fc2af7e3` → `971ea4c5c`）まで PR #381 として 1 時間未満でマージ・デプロイできた
- 再起動後の `recap-worker` のキャッチアップ機構（`batch_catchup_scheduled` による 15 分猶予後の自動再実行）が期待どおり動作し、手動バッチ起動なしで 01:31:36 に自動復旧した
- なお、フロントエンド側の鮮度・古い表示については、コード確認の結果 `recap-worker` の `GET /v1/topic-cards` が `get_latest_cards_run` から `latest_run` を返し、`alt-frontend-sv/src/routes/(app)/recap/cards/+page.svelte`（`Updated ...`）および `SurfaceStatusBanner.svelte` / `surface-status.ts`（鮮度超過より先に `latestRun.status === "failed"` をエラーバナーとして表示する実装）が既に組み込まれていることを確かめられた（実画面での表示自体は未確認）

### うまくいかなかったこと

- `recap_surface_consecutive_failures` とアラートルール（11-09 追加）は存在していたにもかかわらず、本番 Prometheus が 10-31 以降一度も reload されておらず（8 月のランブック記載の timer も未導入）、03:00 頃に鳴るはずだったアラートが鳴らず約 21 時間半にわたり目視発見待ちになった
- `recap-worker` のログが ClickHouse に存在せず、再起動前の 02:30 枠 3 回と 22:42 の失敗理由がコンテナログ消失とともに未計測となった
- 復旧したキャッチアップ実行が 5,141 件（約 66.2 MB）だったため、新上限（96 MiB）が 64 MiB 超の実バッチを捌けるかはその場で確認しきれなかった
- 復旧後のコンテナメモリピーク（約 2.31 GiB）の切り分けが不可能なため、テスト上の定数（54 バイト/対）と最悪値（約 72 バイト/対）の乖離が残課題として残った

### 運が良かったこと

- 他の日次 Recap バッチ（themes バッチ等）へ波及せず、元データの損失も発生しなかった
- 単一利用者の環境であり、表示上の影響範囲が限定されていた

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|----------|-----------|------|------|-----------|
| 1 | 検知 | ルールファイルから廃止済みサーフェス（`evening_pulse`）を除外する修正を反映した後、既存スクリプト（`scripts/observability-reload.sh` / `make observability-reload`）で本番 Prometheus を reload し、ドリフト検査（`make observability-drift-check`）で未読込ルールが無いこととテスト通知が通知先へ到達することを確認する（[[PM-2026-062]] #5 / [[PM-2026-052]] #8 の完了確認を含む） | 運用者 | 2026-11-24 | TODO |
| 2 | 検知 | 本番ホストにランブック記載の自動 reload timer（`make observability-reload-install`）を導入し、26 時間心拍の途絶を検知する `PrometheusConfigReloadStale` アラートを `observability/prometheus/rules/` へ同時に追加して、ルール変更が未読込のまま放置されないようにする | プラットフォーム担当 | 2026-12-01 | TODO |
| 3 | 予防 | リリースパイプラインの最終段へ `scripts/observability-reload.sh` の実行を組み込む等、マージされたルール変更が人手なしで稼働中 Prometheus に反映される仕組みを整備する（`docs/runbooks/observability-config-reload.md` §7 で未解決の課題として挙げられている項目） | プラットフォーム担当 | 2026-12-10 | TODO |
| 4 | 検知 | `recap-worker` のログがログストア（ClickHouse）へ収集・格納されていない原因を修復し、再起動を跨いでもバッチ失敗ログを照会できることを確認する | プラットフォーム担当 | 2026-12-01 | TODO |
| 5 | 予防 | 本番コンテナ内でバッチ実行前後のカーネル記録メモリピークを計測してアイテム対あたりの実クラスタリング消費量（54 bytes/pair vs 逆算上限 72 bytes/pair）を確定し、メモリ予算テストの定数を実測値へ更新した上で 96 MiB 上限が `mem_limit: 8192m` の安全余裕内に収まるかを判定する | Recap 担当 | 2026-12-08 | TODO |
| 6 | 予防 | ウィンドウ件数の増加によってボディ上限や $O(N^2)$ のクラスタリングメモリ上限に到達しないよう、クラスタリング前のアイテム件数上限（または埋め込みベクトルのコンパクト符号化）を導入する | Recap 担当 | 2026-12-17 | TODO |
| 7 | 緩和 | `recap-worker` の `CardsBatchTarget::is_fatal_error` が認証エラー（401/403）のみを即時停止対象としており HTTP 413 でも 15 分間隔で 3 回全量再試行してしまうため、HTTP 413 等の入力サイズ超過を非再試行エラーとして早期に失敗確定させる | Recap 担当 | 2026-12-10 | TODO |
| 8 | プロセス | 新しい 96 MiB 上限の下で旧 64 MiB 上限（約 5,209 件）を超える初回バッチ実行を確認し、アイテム数・クラスタリング所要時間・コンテナメモリ使用量を記録する | 運用担当 | 2026-11-27 | TODO |

### カテゴリの説明

- **予防:** 同種のインシデントが再発しないようにするための対策
- **検知:** より早く検知するための監視・アラートの改善
- **緩和:** 発生時の影響を最小化するための対策
- **プロセス:** インシデント対応プロセス自体の改善

## 教訓

- **データ成長を伴うペイロードの上限変更は、ネットワーク層だけでなく計算・メモリ層の複雑度とセットで設計する。** 埋め込みベクトル配列の JSON 送信は件数に線形（約 12.9 KB/件）で増える一方、全ペア距離行列を使うクラスタリングは二乗（$O(N^2)$）でメモリを消費する。ボディ上限だけを引き上げると、次の障害形が 413 からコンテナ OOM へ移る。
- **固定値の閾値には、実ワークロード成長やリソース上限と連動するテストを付ける。** 2 か月前に 2,000 件前提で置いた 64 MiB は、件数成長に対して無言で破綻した。設定値とメモリ上限の関係をテストで固定することで、危険な引き上げ（128 MiB 案）も事前に止められる。
- **アラートはルールファイルをリポジトリに置いた時点ではなく、稼働プロセスへの読み込みと通知到達を確認した時点で完了とする。** 本件では連続失敗メトリクスもルールファイルも 10 日前から揃っていたが、Prometheus の未 reload によって約 21 時間半にわたり無検知となった。あわせて、統計テーブル（成功時のみ挿入）とは別に失敗理由を追跡できるよう中央ログストアへの収集も維持する。
- **対策完了の判定は「設定を変えた」ではなく「境界を超える本番実行を観測した」で行う。** 復旧バッチ（5,141 件）は旧 64 MiB 上限内だったため、96 MiB 設定での境界超過確認と本番コンテナ実メモリ再計測を明示的に残課題として追跡する。

## 参考資料

- 判断記録: [[000199]]（`recap-subworker` の `RequestSizeLimitMiddleware` 導入）、[[000728]]（`RequestSizeLimitMiddleware` の pure ASGI 化）、[[001003]]（Recap 系サーフェスの鮮度・劣化データ契約と日次ジョブ監視）
- 関連ポストモーテム: [[PM-2026-052]]（Recap 日次バッチ失敗アラートの不在）、[[PM-2026-062]]（Recap 系 3 画面の長期陳腐化と [[PM-2026-052]] #8 アクションの引き継ぎ）
- 運用ランブック: `docs/runbooks/observability-config-reload.md`（2026-08-10 作成、Prometheus 未 reload 事例と `scripts/observability-reload.sh`・`make observability-reload-install`・`PrometheusConfigReloadStale` 案・デプロイ自動化未解決課題を記録）
- 関連 PR / コミット: PR #381（失敗テスト `1fc2af7e3`、修正 `971ea4c5c`、マージ `5720a4623`、main 側で反映済み。本ブランチ時点のコードは 64 MiB のまま）、`22a040c28`（2026-09-22 の 64 MiB 上限設定コミット）、`42d52f13c`・`28526e944`（2026-11-09 の `recap-alerts.yml` ルール追加コミット）
- 関連ファイル:
  - `recap-subworker/recap_subworker/app/main.py`（`RequestSizeLimitMiddleware`、64 MiB 既定値）
  - `recap-subworker/recap_subworker/infra/config.py`（`max_request_body_bytes`）
  - `recap-subworker/recap_subworker/services/story_clusterer.py`（`StoryClustererService.cluster_stories` の全ペア距離行列計算）
  - `compose/recap.yaml`（`recap-subworker` の `mem_limit: 8192m`、`RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES=67108864`、`RECAP_CARDS_*` 設定）
  - `recap-worker/recap-worker/src/scheduler/daemon.rs`（`run_daily_batch_daemon`、`run_target_with_retry`、`CardsBatchTarget::is_fatal_error`）
  - `recap-worker/recap-worker/src/pipeline/cards/pipeline.rs`（`execute_pipeline`・`stage_persist`・`persist_pipeline_output` の成功時のみ `recap_card_job_stats` 挿入）
  - `recap-worker/recap-worker/src/api/cards.rs`・`recap-worker/recap-worker/src/store/dao/cards.rs`（`get_topic_cards_with_clock`・`get_latest_cards_run` による `surface_status.latest_run` 導出）
  - `recap-worker/recap-worker/src/clients/subworker/cards.rs`（`SubworkerCardsClient::cluster_stories`）
  - `alt-frontend-sv/src/routes/(app)/recap/cards/+page.svelte`、`alt-frontend-sv/src/lib/components/recap/SurfaceStatusBanner.svelte`、`alt-frontend-sv/src/lib/utils/surface-status.ts`（生成時刻・鮮度・失敗バナー表示）
  - `observability/prometheus/rules/recap-alerts.yml`（`RecapSubworkerDown`、`RecapSurfaceStale`、`RecapSurfaceConsecutiveFailures`、`RecapSurfaceMetricsAbsent`）
  - `scripts/observability-reload.sh`（Prometheus / Alertmanager 設定反映およびドリフト検査）

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
