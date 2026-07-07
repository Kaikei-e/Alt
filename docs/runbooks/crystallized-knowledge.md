---
title: 結晶化知識ランブック — ADR 940 本 / ポストモーテム 46 本の横断抽出
date: 2026-07-07
tags:
  - runbook
  - knowledge
  - incident
  - alt
---

# 結晶化知識ランブック

ADR-000001〜000940 (940 本) と PM-2026-001〜046 (46 本) を全件解析し、**今後も効き続ける運用知識**を障害パターン別に結晶化したもの。

> **位置づけ**: source of truth は各 ADR / PM。本書は「症状 → パターン → 即応 → 予防」の横断層であり、[[wiki/HOME]] (今の判断の地図) と個別ランブック (手順書) の中間に立つ。個別手順は [[README|runbooks 索引]] から辿る。

## 使い方

1. **オンコール中**: 下の「診断の定石」で症状からパターン節に飛ぶ → 即応と該当ランブックへ
2. **設計・レビュー中**: 触る領域のパターン節の「予防原則」を差分に当てる
3. **ADR を掘る前**: 末尾の「ADR 時代区分マップ」で該当サーガの番号帯を特定する

---

## 診断の定石（症状 → 最初に見る場所）

| 症状 | 疑うパターン | 最初の一手 |
|---|---|---|
| UI は正常・データだけ空 | §1 silent fallback / 配線漏れ | 起動ログの `*_enabled` / `*_disabled`、DI 配線、scheduler 登録 |
| healthy なのに機能死 | §1, §3 | healthcheck が「機能そのもの」を見ているか。cert / netns / listener |
| ちょうど 30s / 100s / 5min で切断 | §9 streaming | Cloudflare (100s) / nginx buffering / `http.Client.Timeout` / JWT TTL |
| コード修正したのに直らない | §2 配布非対称 | `--build` 漏れ、`.env` override、`latest` タグ、`--force-recreate` |
| 同一エラー文言の再発 | §14 検知ギャップ | **文言一致 ≠ 同一原因** (実例 4 連発)。DB の state table を区分集計 |
| `connection refused` (TCP RST) | §3 netns 幽霊化 | `docker inspect <sidecar> --format '{{.HostConfig.NetworkMode}}'` が旧親 ID か |
| `no such host` | §11 | コンテナ不在 (restart policy / depends_on 欠落)。refused とは別カテゴリ |
| TTFT が数十秒〜数百秒 | §8 LLM | COLD_START (モデル名不一致 / options 不一致 / Recreate)、セマフォバイパス |
| 検索が常に 0 件 | §8, §4 | locale 制限、user_id フィルタ、filter フィールド未 reindex |
| Reproject して初めて壊れた | §5 | live の merge-safe preserve が初回 INSERT バグを隠していた (reproject-safety 違反の早期警報) |
| キュー滞留 / 同一 ID 連打 | §6 | 無限再エンキュー (placeholder 欠落)、リトライストーム (backoff 不在) |
| ログ調査で迷子 | §14 | **ログより DB state table** (`recap_failed_tasks` / `*_status_history`) の集計から入る |

---

# 第 I 部: 障害パターン百科

## §1. Silent fallback / DI 配線漏れ（最頻出クラス）

**定義**: 未配線・初期化失敗・認証失敗・依存欠落を warning / nil-guard / degrade で吸収し、「healthy のまま機能停止」する。全 PM 中もっとも再発したクラスで、CLAUDE.md Critical Rules 8/9 の出典。

**代表事例**: PM-2026-001/011/014/017/023/025/026/035/036/038/042/045、[[000247]] [[000266]] [[000273]] [[000414]] [[000463]] [[000566]] [[000811]] [[000825]] [[000928]] [[000933]]

**症状シグナル**: HTTP 全 200・healthcheck green・Pact green のままデータが空 / 品質が崩壊 / 副作用が起きない。E2E ですら「空でも正常応答」に見える。

**即応**:
- 起動ログで `*_enabled` / `*_disabled` の宣言を確認。無ければ配線漏れを第一仮説に
- 「入口 (event/API) と出口 (DB/UI) は実装済みで中間が繋がっていない」構図を疑う: scheduler 登録、DI 注入、handler マッピング、compose include ([[000578]])
- 401/404 を warning で握りつぶしていないか grep (PM-2026-025/026)

**予防原則**:
- optional 依存の `if x == nil { return nil }` は「DI 忘れ」と「意図的無効」を同形化する — 起動時 loud ログ + 業務経路では panic ([[000928]] = PM-2026-045、詳細 `.claude/rules/di-wiring.md`)
- 必須設定・artefact・モデルは起動時 fail-closed validator で確定的に落とす。lazy init は「/health は通るが初リクエストで死ぬ」温床 ([[000811]] [[000825]] [[000838]])
- fail-closed は **backend の全選択肢に対称に** 付ける。片方だけだと default 切替の瞬間に「片肺 fail-closed」(PM-2026-036)
- `Ok(_)` / `.ok()` / `except Exception` で成功判定しない。結果オブジェクトの中身 (`genres_stored` 等) を検査 ([[000149]] PM-2026-001/014)
- 「実装完了」「ADR Accepted」と「DI 配線完了」「runtime で成立」は別物 ([[000127]] [[000458]] [[000566]])。移行 stub の `return nil, nil` は fail-silent ([[000247]])
- 空条件 early return には必ずユーザー/オペレータ向けフィードバック (PM-2026-011)
- fallback には理由の型 (`error_type`) を付け、起動頻度そのものを品質監視指標にする ([[000706]] [[000807]])

## §2. 設定・成果物・配布実態の非対称

**定義**: コード/設定/artefact/バイナリの一部だけが更新され、残りが旧状態のまま silent に潜伏する。

**代表事例**: PM-2026-005/013/014/016/035/036/037/038/043、[[000152]] [[000499]] [[000564]] [[000571]] [[000800]] [[000825]] [[000826]] [[000886]]

**症状シグナル**: 「修正したはずが直らない」「デプロイ成功したのに旧挙動」「特定サービスだけ旧モデル名を送る」。

**即応**:
- コンパイル系サービスの `--build` 漏れ (PM-2026-014/016: モデル名変更は **env で参照する全コンテナ** が再ビルド対象)
- `.env` が compose default を黙って上書きしていないか ([[000499]] [[000571]] PM-2026-013)
- `latest` タグはコンテナ作成時点で暗黙固定 (PM-2026-005 [[000564]] [[000887]])
- `docker compose up --wait` は再ビルドも同一タグ recreate もしない — pre-build + `--force-recreate` ([[000761]])

**予防原則**:
- 設定は「配管 → 実効化」の 2 段で空振りする。code に入れても compose environment に載せなければ死んでいる ([[000800]] [[000886]])
- **file-scoped bind mount は原則禁止**。host source 欠落を warning なしに空ディレクトリ化し `Path.exists()` を素通りする — `is_file()` + directory bind / named volume (PM-2026-036 [[000825]])
- init container (`service_completed_successfully`) は rolling deploy (`--no-deps`) で一度も走らない。compose パターンは deploy model との整合を ADR セルフチェック項目に (PM-2026-037 [[000826]])
- host bind artefact を導入する PR は populate 責任 (誰が/いつ/どのコマンド) を同一 PR で確立 (PM-2026-038)
- 永続化ファイル (token 等) は tmpfile + rename + fsync。disk full 中の非 atomic write は 0 byte 化する (PM-2026-043)
- デフォルト値はハードコードより危険 — 各サービスに残る古い default が env 上書き後も再発源になる ([[000152]])

## §3. mTLS / PKI / 証明書ライフサイクル

**定義**: cert の発行・更新・ロード・提示のどこかが欠け、「cert は更新された ≠ サービスが新 cert を使っている」となる。PM-2026-028→034 の 7 連鎖 (netns 幽霊化は計 4 回再発)。

**代表事例**: PM-2026-028〜034、[[000747]] [[000748]] [[000757]] [[000773]] [[000774]] [[000782]] [[000783]] [[000784]] [[000802]]

**即応**: [[pki-agent-recovery]] / [[mtls-cutover]] へ。切り分け定型:
- `docker cp` + `openssl x509 -dates` で「ディスク上の cert」と「提示されている cert」の両端比較 (PM-2026-032)
- `dial tcp: connect: connection refused` (TLS エラーでなく TCP RST) + `docker inspect` の NetworkMode が旧親 ID → netns 幽霊化。復旧は `--force-recreate` のみ (restart 不可)
- 復旧自体は大抵 1 分 (restart / force-recreate / cert 削除→再 init)。恒久対応が本体

**予防原則**:
- cert hot-reload は **サーバ・クライアント両側で対称に** (handshake ごと mtime 比較 → 再読込)。片側だけだと 24h ローテで必ず発火 ([[000773]] PM-2026-032)
- nginx は起動時にしか cert をロードしない。TLS 終端は reload 機構をミドルウェア選択の一級要件に ([[000748]] PM-2026-029)。uvicorn/pyqwest も hot-swap 不可 — Python 系は pki-agent reverse-proxy sidecar に寄せる ([[000774]])
- cert-init の「ファイルが存在すれば exit 0」は期限切れを温存する致命的欠陥。期限切れは renew でなく新 OTT re-enrollment ([[000747]])
- healthcheck は cert 鮮度でなく **担う機能そのもの** (listener liveness / netns topology) をプローブ。self-probe は旧 netns 内 loopback で常に成功する罠 (PM-2026-034 [[000784]] → [[000802]])
- Docker の HEALTHCHECK unhealthy は restart を trigger しない (K8s livenessProbe と違う)。自己治癒は deploy レイヤの cascade recreate に委譲 (PM-2026-034)
- outbound `MTLS_ENFORCE=true` と callee サーバ側 mTLS 化は対称対 — 起動時に URL scheme を assert (PM-2026-033)
- cutover の残タスクは ADR の Cons に書くだけでは実行されない — 担当・期限付き Action Item に転記 (PM-2026-031)
- compose / Docker の仕様前提 (restart policy の挙動等) は **live test で invalidate してから** ADR を accepted にする (PM-2026-034)

## §4. 層またぎ暗黙契約 drift（wire schema / 認証境界）

**定義**: 型システムで表現できない層間契約 (JSON tag / NULL 意味論 / 認証要件 / pagination 前提) が片側だけ変わり silent に破れる。

**代表事例**: PM-2026-025/026/040/041/044、[[000248]] [[000276]] [[000306]] [[000735]] [[000843]] [[000865]] [[000867]] [[000868]]

**予防原則**:
- **provider が要件を強める変更 (認証必須化・mTLS 化) は consumer 全数列挙 + Pact CDC RED 先行が必須** (Provider-adds-requirement Playbook [[000735]])。Pact が存在しない consumer は CDC で守られない (PM-2026-025/026) — Critical Rule 7 の出典
- **自己 round-trip テストは contract テストではない**。consumer struct を marshal→unmarshal しても tag drift を検出できない。wire-form contract test は raw `map[string]any` で組む ([[000865]] [[000867]] PM-2026-041)
- 1 概念 2 名 (Link vs URL) は Go field / DB column / proto / wire JSON / TS の 5 層 drift を量産する。glossary で canonical 名を pin + CI lint ([[000867]] [[000868]])
- Go zero-value nil (`json.RawMessage`) は pgx で明示 NULL になり `NOT NULL DEFAULT` は発動しない (省略時のみ)。driver 層の defense-in-depth helper (`emptyJSONIfNil`) で吸収 (PM-2026-040 [[000454]])
- proto にフィールドが無い = 下流で空文字が UUID カラムへ。RPC 化ではフィールドの端-端伝播をチェーン全体で検証 ([[000248]] [[000276]] [[000306]])
- 動的 ranking の検索 backend に offset pagination の disjoint 前提を置かない — FE 側 dedupe (`appendUniqueById`) 必須 (PM-2026-044)
- 「producer を直した」と言う前に全 producer 経路を grep — 3 経路中 1 経路だけ直して supersede された実例 ([[000865]]→[[000867]])

## §5. Projection / イベントソーシング違反

**定義**: append-first / reproject-safe / merge-safe / versioned の不変条件が破れ、投影が欠落・後退・二重加算する。Knowledge Home / Loop / Sovereign の中核パターン。

**代表事例**: PM-2026-010/041、[[000418]] [[000423]] [[000452]] [[000598]] [[000599]] [[000831]] [[000846]] [[000870]] [[000880]] [[000919]] [[000939]]

**即応**: [[knowledge-home-projection-recovery]] / [[knowledge-home-reproject-operations]] / [[knowledge-loop-reproject]] へ。原則は「**手 SQL パッチ禁止、event replay で直す**」([[000418]] [[000452]])。

**予防原則**:
- projector は event payload (+ event が指す version artifact) のみを読む。latest state 参照・`time.Now()` は replay を非決定にする。mutable 参照が要るなら event 化の瞬間に payload へ snapshot を焼き込む ([[000599]] [[000831]] [[000846]])
- 業務時刻は `event.OccurredAt` 起点。window の右端も wall-clock でなく consume 中 event の occurred_at ([[000911]] [[000919]])。「異なる clock で 2 回回して byte-identical」の invariants test で CI に固定 ([[000924]] [[000933]])
- merge-safe upsert の canonical form は `COALESCE(NULLIF(EXCLUDED.x, ...), old.x)` / `GREATEST`。業務判定の CASE 式は一方向遷移を静かに破る ([[000870]] [[000886]] [[000423]])
- 到達状態 (ready) は不可逆、可視性フラグ (dismissed_at) は monotonic。解除は専用イベントのみ ([[000424]] [[000455]] [[000457]])
- checkpoint は projection version に紐づける。reproject swap と checkpoint リセットは不可分 (PM-2026-010 [[000598]])
- 遡及修復は corrective event (`*Backfilled`) を append。dedupe registry は ingest-only barrier で reproject で TRUNCATE しない。再 emit は dedupe namespace bump (v2) で ([[000846]] [[000880]] [[000882]])
- mapping / placement ロジックを変えたら version 定数 (WhyMappingVersion 等) を bump し full reproject。優先順位の並べ替えだけでも bump 対象 ([[000841]] [[000896]] [[000925]])
- projector が別 projection を読む co-projection は 5 条件 (同一 pass・Derive-before-Apply・accumulator も disposable・writer 単一・occurred_at 窓) を満たす場合のみ ([[000939]])
- 「live でしか preserve が効かない field」は reproject-safety 違反の早期警報 — Reproject で初めて爆発する (PM-2026-041)
- テスト seed は本番密度で。`LIMIT 256` の再 scan が 3,757 件 window で silent 破綻した — 2 件 seed では再現しない ([[000939]])

## §6. キュー・リトライ暴走・無限ループ

**定義**: リトライ・再エンキュー・ポーリングが「失敗しても間隔・結果が変わらない」構造で暴走し、リソースを焼き尽くす。

**代表事例**: PM-2026-002/017/027/042、[[000137]] [[000139]] [[000185]] [[000203]] [[000251]] [[000387]] [[000509]] [[000551]]

**予防原則**:
- **恒久的に処理不能なアイテムには「処理済みの証拠」(placeholder) を書く**。書かないと無限再エンキュー (63 記事で 1,349 job、PM-2026-002 [[000551]])。サイズ超過等のリトライ無意味な失敗は非再試行で即 completed
- 2 ストア間ガード (summaries × job_queue) は補償トランザクション必須。PM-2026-002 と 017 は対になる逆パターン
- bounded exponential backoff + circuit breaker は構造的要件。ms 単位永久リトライは 148GB のログ洪水を起こした (PM-2026-042)。失敗時にポーリング間隔が変わらないワーカーは CPU を燃やす ([[000275]] [[000276]])
- タイムアウトはキュー待ち時間より長く。逆だと「タイムアウト→リトライ→キュー増悪」の正帰還 ([[000137]] [[000121]])
- キュー飽和対策 3 層: 下流 429 + Retry-After / リトライ中は slot 保持 (`hold_slot`) / 上流 指数バックオフ ([[000185]])
- dequeue は単一ステートメントで原子的に (`UPDATE ... WHERE id IN (SELECT ... FOR UPDATE SKIP LOCKED) RETURNING`) ([[000509]])
- `running` には stuck リカバリ (時間閾値で pending に戻す)、リトライ上限超過は dead_letter へ隔離。dead_letter は「真の障害」専用で想定内スキップを混ぜない ([[000139]] [[000387]] [[000388]])
- dead_letter 確定前に source of truth を recheck (summary 実在なら偽 dead_letter、PM-2026-027)
- LLM の退化出力 (空白のみ) は非再試行エラーに分類 — 502 のままだと GPU 時間を無限浪費 ([[000203]] [[000214]])

## §7. 並行性・ライフサイクル race

**定義**: スロット所有権・キャンセル・stale 応答・並行 helper の暗黙契約が破れる。HybridPrioritySemaphore は 5〜6 回連続でインシデント化した。

**代表事例**: PM-2026-003/004/012/013/014/015/046、[[000243]] [[000552]] [[000601]] [[000606]] [[000610]] [[000612]] [[000718]]

**予防原則**:
- セマフォはスロット所有権 (home_pool / slot_id) を明示追跡。caller priority からの推論は破綻 ([[000601]] [[000606]])
- release パスに invariant チェック (`available + acquired == total_slots`)。ただし「転送中」の transient window を除外しないと false positive が真のリークを隠す (PM-2026-015)
- CancelledError ハンドラは「取得済み・未返却のリソース」を棚卸しして回収 ([[000612]])。同一スレッド asyncio で `call_soon_threadsafe` は不要かつ race 源 (PM-2026-014 [[000610]])
- PEP 525: async generator の finally は実行保証がない — `contextlib.aclosing` 等の多層防御 ([[000243]])
- ブロッキング await はプリエンプトできない — `asyncio.wait(FIRST_COMPLETED)` で cancel と競争させる ([[000556]])
- 非同期コールバックには stale-response guard (呼び出し時点 ID キャプチャ + 現在値比較)。AbortController だけでは不十分 (PM-2026-003 [[000552]])
- RWMutex の RLock→Lock 昇格は TOCTOU アンチパターンで race detector にも掛からない ([[000718]])
- 並行 CI helper は「自プロジェクト以外に touch しない」が第一不変条件。`docker network rm` の安全保証は attach 後のみ (PM-2026-046)
- 同一コンポーネントの連続インシデントはテスト設計の盲点 — property-based testing を導入せよ (PM-2026-015)

## §8. LLM / GPU / Ollama 運用

**定義**: VRAM 制約下の共有 GPU で、モデル切替・options 不一致・structured output・言語特性が絡む障害クラス。

**代表事例**: PM-2026-005/006/007/008/016/020、[[000151]] [[000152]] [[000579]] [[000632]] [[000640]] [[000665]] [[000801]] [[000887]]

**予防原則**:
- **全サービスは同一 Ollama モデルを共有** — モデル切替 = COLD_START (TTFT 100s 超)。モデル名は default / env / ハードコードの全てを統一 ([[000151]] [[000152]])
- **options (num_batch/num_keep/stop) は全リクエストパスで一元管理** — 不一致は runner 再構成ピンポン (PM-2026-008 [[000579]])
- 共有 GPU への全クライアントは同一スケジューリング機構 (セマフォ) を通す。透過プロキシは GPU 共有環境では危険 (PM-2026-006)
- Gemma 4 の thinking は CJK 入力で暗黙発動し num_predict を食い潰す — free-text 生成の全箇所で `think=false` を明示 pin ([[000801]])。thinking tokens は num_predict に含まれる / `format` は GBNF 構文保証のみで意味は見ていない ([[000632]] [[000665]])
- 「LLM を直そうとしすぎない」— deterministic 主経路 + LLM 副経路 + fallback の 3 層。「LLM が成功しないと困る層」を作らない ([[000675]] [[000677]] [[000698]])
- few-shot 例は内容をコピーされる — 抽象プレースホルダ化 ([[000648]] [[000650]])。reasoning フィールドを decision より前に置く reasoning-first 順序 ([[000632]] [[000665]])
- モデル移行チェックリスト: チャットテンプレートトークン差異を一次ソースで確認 ([[000640]])、モデル名を env 参照する全コンテナを `--build` (PM-2026-016)、embedding モデル切替は timeout/retry も見直す ([[000899]])
- Ollama の JSON Schema→GBNF 翻訳は `\d` `\w` `\s` を拒否 — 渡す全 schema の pattern を CI で sweep ([[000887]])
- embedder は SPOF にしない — BM25-only degraded mode を設計 (PM-2026-020 [[000693]])。retrieval 系の変更は単独で eval を回す ([[000696]])

## §9. ストリーミング多層タイムアウト・バッファリング

**定義**: アプリ → BFF → nginx → CDN の全層に切断・バッファリング要因が分散し、1 層直すと次が露出するモグラ叩きになる。

**代表事例**: PM-2026-004/045、[[000284]] [[000289]] [[000292]] [[000295]] [[000553]] [[000554]] [[000555]] [[000929]]

**即応**: 予防チェックリストは [[connect-rpc-streaming-checklist]] (5 軸: auth TTL / nginx location / cursor persist / emit ownership / dedupe key)。

**予防原則**:
- 「ちょうど N 秒で切断」は層で特定: 30s = 各種 client Timeout、100s = Cloudflare 524、5min ごと再接続 = JWT TTL ([[000929]] PM-2026-045)
- Go `http.Server.WriteTimeout` / unary 用 `http.Client` (Timeout 付き) はストリーム全体を殺す — streaming は Timeout 0 + context deadline 一元管理 (PM-2026-004 [[000478]] [[000553]])
- BFF/プロキシの `io.ReadAll` は最終ボス — streaming RPC はパス判定でキャッシュ・CB をバイパス ([[000295]])
- heartbeat は最初のバイトを即時送信 + **全フェーズ** (プロンプト構築中・LLM 接続待ち含む) をカバー ([[000292]] [[000553]] [[000623]])
- nginx は streaming 専用 location (`proxy_buffering off`) 必須。SSE はさらに `proxy_request_buffering off` + `X-Accel-Buffering: no`。streaming 判定は `application/connect+` プレフィックス ([[000554]] [[000555]] [[000929]])
- `Connection "upgrade"` リテラルはアンチパターン — `map $http_upgrade $connection_upgrade` が canonical ([[000898]])
- 新 streaming サービス追加時は nginx location 更新が必須 (サービス名がハードコード) — 設定検証テストで回帰防止 ([[000555]])
- silent failure は HTTP status で検知不能 — body size / stream 数 / 再接続間隔を SLI 化 (PM-2026-045)

## §10. PgBouncer / pgx / PostgreSQL の型・プロトコル罠

**代表事例**: PM-2026-019/040、[[000327]] [[000328]] [[000417]] [[000454]] [[000456]] [[000470]] [[000521]] [[000577]]

**予防原則**:
- **JSONB への Go 値の渡し方は接続経路 (simple protocol / extended) で逆になる**: PgBouncer + simple protocol は `string()` 必須 ([[000417]] [[000470]])、直結 pgx は `[]byte` ([[000577]])。新規経路は自作せず既存 driver helper を踏襲し、統合テストで検証する
- 空 `json.RawMessage` / nil は 22P02 か明示 NULL — driver 層共通 helper (`len==0 → "{}"`) ([[000454]] PM-2026-040)
- `jsonb_array_elements()` は scalar shape で projector 全体を止める — `jsonb_typeof = 'array'` ガード ([[000456]])
- `[]uuid.UUID` 配列は simple protocol で失敗 — `[]string` + `ANY($1::uuid[])` ([[000379]])
- トランザクションは無条件 `defer tx.Rollback(ctx)` が唯一安全 ([[000328]])。DDL / session lock を使うマイグレーションは PgBouncer をバイパスして直結 ([[000327]])
- UPDATE の `rows_affected == 0` を必ず検証 — silent success が状態不整合の温床 ([[000113]] [[000538]])
- 長さ比較は単位を揃える (`LENGTH()` は文字数 / Go `len()` はバイト数 — 日本語で 3 倍差) — `OCTET_LENGTH` ([[000548]])
- 削除で縮む集合に offset pagination 禁止 — keyset へ ([[000481]] [[000529]])。`FOR UPDATE SKIP LOCKED` は autocommit でロック即解放 ([[000282]])
- PostgreSQL コンテナに `shm_size` 明示 (default 64MB は 53100 で死ぬ) ([[000521]])。キャッシュ導入は「ミス時 fan-out コスト × 共有プール副作用」込みで評価 (PM-2026-019)

## §11. Docker / Compose 構造的 footgun

**代表事例**: PM-2026-023/030/034/036/037、[[000578]] [[000761]] [[000782]] [[000802]] [[000809]] [[000825]] [[000826]] [[000895]]

**予防原則**:
- `network_mode: service:X` は作成時に container id を一度だけ解決 — 親 force-recreate で sidecar は旧 netns に孤立。`depends_on.<parent>.restart: true` は force-recreate では発火しない ([[000782]] [[000802]])
- `docker compose restart` は host port を再 bind しない — bind 失敗コンテナは `--force-recreate` (運用 memory 由来)
- `depends_on.condition: service_healthy` の参照先に healthcheck が無いと `up --wait` が abort — compose config render の CI ゲートで検知 ([[000809]])
- 長時間稼働サービスに `restart: always` + 依存元に `depends_on` を明示 — 欠けると 33h silent degradation (PM-2026-023 [[000703]])
- compose ファイルの include 漏れは silent — altctl の stack registry テストが検知網 ([[000578]])
- `logging` 未設定のコンテナは OOM-restart ループで disk を食い潰す — json-file max-size/max-file を pin ([[000895]])
- named volume は root:root で作られ distroless nonroot は書けない ([[000063]])。`.dockerignore` の `.venv/` は top-level のみ — `**/.venv/` ([[000819]])
- Compose V2 は最初の `-f` の親ディレクトリの `.env` を読む — ツールからは `--env-file` 明示 ([[000189]])

## §12. Svelte 5 リアクティビティ / FE fetch storm

**代表事例**: PM-2026-039/044、[[000226]] [[000228]] [[000320]] [[000441]] [[000847]] [[000898]] [[000902]]

**予防原則**:
- `$effect` はコールスタックを越えて読んだ全 reactive source を track — 自己再発火ループ (毎秒 30 fetch) の源。ガードは `untrack()` / `$derived` の値等価ゲート ([[000320]] [[000441]] PM-2026-039)
- stream-driven refresh は debounce + single-flight (coalescer) + スコープ付き `invalidate(name)` が標準処方。無条件 `invalidateAll()` は正帰還ループ ([[000847]] PM-2026-039)
- keyed `{#each}` の重複キーは警告なしで reconcile をクラッシュさせる — backend の pagination disjoint 契約に依存している自覚を持つ ([[000228]] PM-2026-044)
- SPA deploy 後の chunk 404 は多層自己治癒: hooks.client 検知 + `version.pollInterval` + `updated.current` + nginx 404→200 stub + HTML `Cache-Control: no-cache` ([[000898]] [[000902]] [[000412]])
- バックエンドにも雪崩防御 (`singleflight.Group`) を置く ([[000320]])。client-side エラーは server metrics に出ない — FE エラートラッキング / fetch rate SLI (PM-2026-039/044)

## §13. マイグレーション規律

**代表事例**: [[000059]] [[000162]] [[000239]] [[000387]] [[000746]] [[000853]] [[000871]] [[000877]]

**予防原則**:
- 適用済みマイグレーションは不変。修正は新規増分 + 冪等 (`IF NOT EXISTS`)。ハッシュ不整合は `atlas migrate set` ([[000162]])
- 破壊的変更は Expand-Contract 3 段階 + 影響レコード数の事前検証 (単一 Tx の DDL+DML 混合で全データ消失事故) ([[000059]] [[000040]])
- DB 分離したらマイグレーションの適用先も分岐 — 対応漏れで 4.1M 件バックログ ([[000387]])
- `CREATE OR REPLACE VIEW` は列を位置照合 — 新列は必ず SELECT 末尾 ([[000877]] [[000918]])。atlas.sum は git 追跡必須 ([[000877]])
- migrate → deploy の順厳守 (逆は新アプリが旧スキーマで healthcheck 落ち→自動 rollback) ([[000746]] [[000940]])
- migrator image は build-time network 依存ゼロ + DSN に `lock_timeout` ([[000871]])。Atlas は `CREATE INDEX CONCURRENTLY` 不可 ([[000282]] [[000422]])
- RLS 有効化は caller-first (SET LOCAL 配線 → 本番観測 → ENABLE) の順序が不可分 ([[000853]])
- 「適用済み」記録とスキーマ実態は乖離しうる (ファントムマイグレーション) — 存在チェック付き冪等修正 ([[000239]])

## §14. 検知ギャップの構造（メタパターン）

**定義**: 障害そのものではなく「なぜ発見が遅れたか」の共通構造。46 PM のほぼ全件がユーザー報告で検知 (TTD 最長 8 日〜4 週間)。

**結晶化した規律**:
- **調査はログより DB state table から**。`recap_job_status_history.reason` に真因が 4 日分明記されていた実例 (PM-2026-031)。ログはノイズに埋没する
- **エラー文言完全一致 ≠ 同一根本原因**。`classification returned 0 results` は 4 つの PM で 4 つの別原因 (PM-2026-033/035/036/037) — 「前と同じやつ」と誤認して 3 回誤誘導された
- **PM の Action Item 未着手が次の障害を生む**。同型指摘 3 連続 (PM-008→016→020) の実績から「同じ AI が 3 PM 連続未着手なら全開発停止して着手」がルール化
- **類似 incident の水平展開監査は即日**。PM-025 の翌日に同型が別 consumer で発火 (PM-026) — 1 日で回避可能だった。「発火 1 件で満足せず同型を同日全根治」(PM-2026-032 は 6 サービスに同型 silent 分散)
- **計器の嘘を疑う**: rules ディレクトリ mount 忘れで全 alert silent load 失敗 ([[000892]])、低流量で構造的に発火不能な rate alert、実 evidence を見ない ratio metric ([[000937]] [[000939]])。アラートは実データでの発火テストまでやる ([[000286]])
- graceful degradation はユーザー向けフェールセーフでありオペレータへのシグナルではない — **degradation 発生自体を観測指標に** (PM-2026-023)
- log volume / disk 使用率 / CI flaky rate は first-class health metric (PM-2026-042/046)
- 補完経路・低頻度 admin 機能ほど観測投資が必要 — 「使われない機能は腐る」(PM-2026-040/043)

---

# 第 II 部: 不変条件と Critical Rules の出典

CLAUDE.md Critical Rules / wiki 不変条件がどの障害から結晶化したかの対応表。ルールの「なぜ」を失わないための台帳。

| ルール | 出典 |
|---|---|
| Append-first / reproject-safe / versioned / disposable projections | [[000398]]〜[[000400]] で始動、[[000114]] (Recap 履歴テーブル) が原型。修復容易性は PM-2026-010/041 で実証 |
| No silent fallback (Rule 8) | [[000928]] = PM-2026-045 が直接出典。原型は [[000247]] [[000463]] [[000566]]、PM-2026-001/011/023 |
| Fail-fast startup config (Rule 9) | [[000273]] [[000811]] [[000825]] [[000838]]、PM-2026-035/036/038/043 |
| Producer wiring は CDC RED first (Rule 7) | [[000928]]、PM-2026-025/026 (Pact 不在 consumer)、[[000735]] Playbook |
| Stream consumer: ACK after durable + reclaim (Rule 10) | [[000083]] [[000089]] (基盤)、[[000509]] (原子的 dequeue)、PM-2026-027 (偽 dead_letter) |
| 5 秒レート制限 | クローリング用。ユーザー操作起因の別性質アクセスは別値を正当化して良い ([[000342]]) |
| Rebuild on compiled changes | PM-2026-005/014/016 (COLD_START 610 回 / EOF 完全停止) |
| SQL にビジネスロジック不可 | [[000471]] [[000548]] [[000558]] |
| time.Now() を business fact に使わない | [[000468]] [[000919]] [[000924]]。hook による機械的ブロックまで昇格 |
| TDD outside-in (E2E→CDC→Unit) | [[000588]]〜[[000591]] [[000616]] (Pact 導入)、[[000763]]〜[[000791]] (Hurl E2E 展開) |

---

# 第 III 部: ADR 時代区分マップ（サーガ索引）

過去の判断を掘るときの番号帯ガイド。詳細は各バッチの ADR を、テーマ別の現在の判断は `wiki/decisions/` を見る。

| 番号帯 | 期間 | 主サーガ |
|---|---|---|
| 000001–080 | 2025-12〜2026-01 | Connect-RPC 全面移行の定石確立、URL 正規化ゼロトラスト、K8s 路線放棄→Compose-first、Expand-Contract 確立 |
| 000081–160 | 2026-01 | Trace context 8 連鎖 (「Accepted ≠ 実装済み」)、単一モデル共有の確立、Recap イベントソーシング原型 [[000114]] |
| 000161–240 | 2026-01〜02 | LLM キュー飽和の多層防御、全サービス Clean Architecture 波、Svelte 5 infinite scroll サーガ、日本語 NLP 固有バグ群 |
| 000241–320 | 2026-02〜03 | Shared DB 解消とスタブの罠、OGP 4 世代進化、streaming 30 秒戦記、SQL 総点検 (LATERAL 960 倍) |
| 000321–400 | 2026-03 | PgBouncer 導入と余波、計測ファースト負荷試験、Legacy 大掃除、**Knowledge Home append-first 始動 [[000398]]** |
| 000401–480 | 2026-03 (3 日間!) | Knowledge Home Phase 2–6。projection 教訓の宝庫 (merge-safe / 配線欠落 / backend truth) |
| 000481–560 | 2026-03 | 要約キュー整合性再建、**Knowledge Sovereign 分離サーガ [[000532]]〜[[000543]]**、BE/RT 分散、streaming 多層落とし穴 |
| 000561–640 | 2026-03〜04 | セマフォリーク根絶 4 連鎖、**Pact CDC 導入 [[000588]]**、Agentic RAG 品質戦、event store lifecycle |
| 000641–720 | 2026-04 | **Acolyte 誕生**、LLM 責務最小化への収束、Alt-Paper 全面刷新、X-Service-Token (→mTLS で退役) |
| 000721–800 | 2026-04 | **mTLS cutover と pki-agent 障害史**、deploy pipeline 3 世代 (手動→c2quay→alt-deploy)、Hurl E2E 全面展開 |
| 000801–870 | 2026-04 | silent failure 根絶 fail-closed 化、image 圧縮キャンペーン、**Knowledge Loop 構築アーク**、Link/URL 5 層 drift |
| 000871–940 | 2026-04〜06 | Knowledge Loop 反復失敗の構造診断 → **[[000940]] で Knowledge Trail へ retire**、DI wiring ルール出典 [[000928]]、event-time purity の CI 固定 |

**大きな supersede の変遷** (同じ轍を踏まないための系譜):
- 自動全文取得: 導入 [[000486]]〜[[000488]] → 3 日で全廃 [[000533]] (ユーザー起点が正)
- 7-day Recap 自動バッチ → 3-day 化 [[000184]] (キュー飽和が契機、新規に 7 を入れない)
- X-Service-Token [[000717]]〜[[000720]] → mTLS 一本化 [[000743]] (shared secret の silent 劣化が構造要因)
- deploy 基盤: CI gate [[000740]] → 手動 script [[000746]] → c2quay [[000758]] → alt-deploy pull 型 [[000763]]。「設計したパイプラインは初回実走で必ず壊れる」「移行時は旧実装の副次機能 (sidecar cascade 等) の棚卸し必須」
- Knowledge Loop read 表層 → Knowledge Trail [[000940]]。「3 軸直交はユーザも実装者も LLM も保持できず必ず一軸に退化する」「event log 永久保存のおかげで retire しても Trail に再射影できた」

## Sources

- `docs/ADR/000001.md`〜`000940.md` (940 本、2026-07-07 時点全件)
- `docs/postmortems/PM-2026-001`〜`046` (46 本)
- [[wiki/HOME]] — 結晶化ナビゲーション層 (今の判断)
- [[README|runbooks 索引]] — 個別手順書への入口
