# ポストモーテム: morning_update Job が batch Recap の boot-time resume loop に閉じ込められ長時間 zombie 化

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-024 |
| 発生日時 | 2026-04-13 13:11 (JST) |
| 復旧日時 | 2026-04-13 20:28 (JST) |
| 影響時間 | 約 7 時間 17 分 |
| 重大度 | SEV-3 |
| 作成者 | recap-worker on-call |
| レビュアー | — |
| ステータス | Draft |

## サマリー

2026-04-13 朝、`POST /v1/morning/letters/regenerate` で kick された
morning_update Job (`f67ffd79-50e2-48b3-bfc1-38c0f6343221`) がプロセス
クラッシュ後に `status='running', last_stage='evidence'` のまま `recap_jobs`
テーブルに残留。以降、同日中に複数回行われたコンテナ再ビルドのたびに
daemon の `find_resumable_job()` がこの行を拾い、**batch Recap pipeline
として** 誤 resume する無限ループに陥った。

resume ごとに subworker clustering が 409 "run already in progress" を
返して多くの genre が skip、summary phase 途中で crash → 次 boot で
再び resume、を 5 回以上繰り返す間、Job は完了せず `running` 状態で
ダッシュボードに居座り続け、ユーザーから「Job が大量に走っている / スタック
している」と報告されて発覚した。user 向け Morning Letter の更新は
この間に 1 度も成功せず、余計な LLM 負荷 + daemon ブロッキングが蓄積
した。根本原因は `recap_jobs.trigger_source` という discriminator カラム
が spec 上存在しつつ実装で一切書き分けておらず、morning 起源と batch
起源の Job が区別できなかったこと。

## 影響

- **影響を受けたサービス:**
  - recap-worker (Rust) — 無駄な pipeline 実行で CPU/RAM 占有
  - news-creator (Python) — 意図外の batch summary LLM 呼び出し
  - alt-frontend-sv — Morning Letter ページに古い Letter (前日以前の
    モデル extractive-fallback) が出続けた
- **影響を受けたユーザー数/割合:**
  開発 / 単一オペレータ環境のため 1 ユーザー。本番相当トラフィックは
  受けていない。
- **機能への影響:**
  - **Morning Letter 更新**: 部分的劣化 (regenerate が手動 kick しても
    実体は morning pipeline ではなく batch Recap として走り、morning_letters
    行が更新されない)。
  - **dashboard `running_jobs` 表示**: `success_rate_24h = 0.16`,
    `total_jobs_24h = 12` という誤解を招く数値が露出し、運用者の判断を
    鈍らせた。
- **データ損失:** なし。`morning_letters` / `recap_jobs` とも破壊はなく、
  `f67ffd79` は履歴行として残存。
- **SLO/SLA違反:**
  内部指標のみ。user-facing SLO は未定義のため正式な違反はなし。
- **計算コスト:**
  resume 1 サイクル ≈ 35 min の news-creator `/v1/summary/generate`
  バッチ呼び出し (chunk_size=3 × 7 chunks)。5 サイクル以上観測された
  ため、概算 **3 時間弱の不要な Gemma4 LLM 消費** が発生。

## タイムライン

| 時刻 (JST) | イベント |
|-------------|---------|
| 2026-04-13 13:11 | 手動 `POST /v1/morning/letters/regenerate` で morning_update Job `f67ffd79` kick。`JobContext::new_with_window(job_id, [], 1)` で recap_jobs に insert。**ここで morning 由来にも関わらず `trigger_source='system'` がデフォルト書き込みされる**。|
| 2026-04-13 13:11〜(以降) | プロセスクラッシュ (コンテナ再ビルドの強制終了 or OOM 等。正確な発生時刻は trace なし)。Job 行は `status='running', last_stage='evidence'` のまま残留。|
| 2026-04-13 13:11〜19:25 | 複数回のコンテナ再ビルド (ADR-000706, 000707 の FE/BE 改修作業)。**各 boot で daemon が find_resumable_job() で `f67ffd79` を再選択** → run_job() で batch Recap pipeline 実行 → subworker clustering 409 → mid-pipeline crash → 次 boot で同じループ。|
| 2026-04-13 20:03 | **検知 (ユーザー報告)** — 「なぜ Recap Job が大量に走っているの？」。 alert なし、ダッシュボード観察から。|
| 2026-04-13 20:05 | 対応開始。初回調査で「12 jobs in 24h は読み取りが混じった履歴で、実 Job run は 1 件」と誤結論。|
| 2026-04-13 20:25 | ユーザー再指摘「3-day Recap は 02:00 JST のみのはず」。再調査で `recap_job_status_history` + コンテナ起動時刻の相関から、morning_update が batch Recap pipeline として resume されている事実に到達。|
| 2026-04-13 20:30 | **寄与要因発見**: ADR-000708 実装中に `cleanup_old_jobs` / `mark_abandoned_jobs` が `make_interval(days/hours => $1)` に `f64` を bind していて常時 SQL error で黙って失敗していたのも判明。|
| 2026-04-13 20:47 | **緩和 (部分)** — ADR-000708 適用 (boot-time hygiene + bind 型 fix)。以降、zombie 行は boot 時に seal されるようになるが、**resume 候補に選ばれた 1 件は keep されるため `f67ffd79` は依然として拾われる**。|
| 2026-04-13 21:05 | **原因特定 (真)** — `recap_jobs.trigger_source` カラムが schema にはあるが全行 default `'system'` で書き分けなし。morning と batch が区別不能。|
| 2026-04-13 21:10 | ADR-000709 実装 (trigger_source 書き分け + `find_resumable_job` の WHERE filter)。新規 morning regenerate 行は `'morning'` で tag されるが、**既存 `f67ffd79` は retroactive には直らず** resume loop が続く。|
| 2026-04-13 21:25 | ユーザー「いい加減にして」+ 詳細追跡依頼。subworker clustering の 409 "run already in progress" を新規発見。|
| 2026-04-13 21:28 | **復旧 (緩和)** — `UPDATE recap_jobs SET status='failed', last_stage='stuck_resume_loop' WHERE job_id='f67ffd79-...'` を手動実行。recap-worker restart で `find_resumable_job` が None を返し、daemon は 02:00 JST まで sleep へ遷移。`sealed orphaned recap jobs as failed marked_failed=1 keep_job_id=None` のログで確認。|
| 2026-04-13 21:29 | **予防** — `RECAP_RESUMABLE_MAX_AGE_HOURS` のデフォルトを 12h → 4h に短縮。同種の pre-existing zombie が再出現しても 4h を超えた時点で auto-resume 対象から外れる。|

- **Time to Detect (TTD):** ≈ 6h 50m (13:11 発生 → 20:03 ユーザー報告)
- **Time to Mitigate (TTM):** ≈ 1h 25m (20:03 検知 → 21:28 手動 unstick)
- **Time to Repair (TTR):** ≈ 1h 26m (20:03 → 21:29 再発防止の tightening 完了)

## 根本原因分析

### Five Whys

1. **Q:** なぜ morning_update Job が 7 時間以上 `running` のままゾンビ化
   したのか？
   **A:** コンテナ再起動のたびに daemon が同じ行を resume candidate として
   拾い、batch Recap pipeline として実行 → 失敗 → 行は `running` のまま、
   を 5 回以上繰り返したため。

2. **Q:** なぜ daemon は morning_update 行を resume 候補にしたのか？
   **A:** `find_resumable_job` の SQL が `WHERE status IN ('pending',
   'running', 'failed')` だけで kind / trigger_source を見ておらず、
   age window (`kicked_at > NOW() - INTERVAL '12 hours'`) に入っていれば
   どんな kind の行でも選んでいた。

3. **Q:** なぜ kind を見ていなかったのか？
   **A:** `recap_jobs.trigger_source TEXT NOT NULL DEFAULT 'system'`
   というスキーマは存在したが、**どの insert path も値を上書きしていなかった**
   ため、batch 起源も morning 起源も user 起源も全て `'system'` で保存
   されていた。discriminator は **spec 上だけ存在して実装で死んでいた**。

4. **Q:** なぜ discriminator が有効活用されていなかったのか？
   **A:** `recap_jobs` テーブルが「batch Recap 用の Job 管理」として
   設計された後、morning_update と user-triggered という別 kind の Job が
   同じテーブルに相乗りしたが、その時点で **kind 別ルーティングの責務を
   誰も持っていなかった**。`create_job_with_lock_and_window()` の API が
   `trigger_source` 引数を受けない形で固まっており、呼び出し側は渡す手段が
   なかった。

5. **Q:** なぜそもそも morning_update が batch Recap pipeline (`run_job`)
   として resume されてしまうのか？
   **A:** daemon は resume 時に常に `scheduler.run_job(JobContext)` を
   呼ぶ。`run_job` は classification → clustering → evidence → summary
   → reduce という batch Recap の処理のみ。morning 用の
   `MorningPipeline::execute_update` に振り分ける分岐は存在しない。
   **「どんな kind の Job でも boot 時は一律 batch Recap として再開する」**
   という暗黙の前提が daemon 側に埋め込まれていた。

### 寄与要因 (contributing factors)

- **A. `make_interval` SQL bind 型バグ**:
  `cleanup_old_jobs` / `find_resumable_job` / `mark_abandoned_jobs` の
  `make_interval(days => $1)` / `(hours => $1)` に `f64` を bind していた。
  Postgres の `make_interval` は `INT` を要求するため、これらの SQL は
  **常に "function does not exist" で失敗し黙って 0 行扱い**になっていた。
  気付かれなかった理由: 当該パスがそもそも batch 完走後にしか呼ばれず、
  daemon が走らない日は誰も呼ばなかった。
- **B. subworker clustering の orphan run**:
  recap-worker プロセス死亡時、subworker 側に in-progress clustering run
  が残る。次回 resume 時、同じ `(job_id, genre)` での clustering 要求に
  対し 409 Conflict を返す実装。clustering orphan を reap する経路は
  subworker にも recap-worker にも存在せず、**一度 stuck した Job は
  永久に clustering phase を抜けられない**。
- **C. resume 試行のメータリング不在**:
  resume を何回試みたかのカウンタが DB に無く、N 回失敗した Job を強制
  abandon する仕組みがなかった。
- **D. 検知手段の不在**:
  `running` 状態が N 時間以上続いている Job を alert する仕組みがない。
  ダッシュボードは stats を表示するだけで、運用者が能動的に見るまで
  検知されない。
- **E. 運用コンテキストの錯綜**:
  本日の作業は ADR-000705/706/707/708 を並行実装する長丁場で、コンテナ
  再ビルドが頻発した。通常運用では再起動頻度は低く、resume loop が 1 日で
  5 回以上発火する条件は異常。通常運用負荷下なら stuck しても 1 日 1 ループ
  程度で済んでいた可能性がある。

## 対応の評価

### うまくいったこと

- `recap_job_status_history` に全 status transition が immutable に
  記録されていたため、post-hoc 解析で各 resume の履歴が完全に辿れた。
- boot-time hygiene (ADR-000708) の **大枠は正しい方向** だった。実装
  バグ (bind 型) を差し引けば、`mark_abandoned_jobs` / `cleanup_old_jobs`
  で同種のゾンビは抑止できる設計。
- ADR-000709 で discriminator を活かす設計に移行できたため、**新規の
  morning_update 行は同じ罠に落ちない**。

### 運が良かったこと

- 本件は単一オペレータ環境で発見された。マルチユーザー本番環境では、
  同じ resume loop が他ユーザーの recap pipeline 予約実行を
  遅延・干渉させる恐れがあった。
- subworker が 409 を返す実装だったおかげで、**同時実行による data
  corruption は起きなかった**。もし lock なしで重複 run を許容する実装
  なら DB 上の clustering 結果が混ざる危険があった。
- 02:00 JST の scheduled batch はこの日未発火だったため、resume loop と
  competitng しなかった。同 batch 実行中に resume loop が走ると pipeline
  競合が起きていた可能性がある。

### うまくいかなかったこと

- **初動診断が 1 階層浅かった**: 「12 jobs in 24h は履歴で実 run は 1 件」
  と誤って結論し、ユーザーの「3-day Recap は 02:00 のみ」指摘で軌道修正
  が必要になった。根拠として `dashboard/job-stats` のカウントのみを見て、
  `recap_job_status_history` の transition 頻度を先に見ていれば resume
  loop に直接到達できた。
- **bind 型バグを boot-time hygiene 実装時に即検知できなかった**。
  エラーログを anyhow::Context で wrap していたため、SQL error の詳細が
  握り潰されて "failed to delete old jobs" しか見えなかった。
- **pre-existing zombie の retroactive cleanup が未設計**。ADR-000709
  で新規行は保護されたが、既に `'system'` で保存された同種の行 (`f67ffd79`)
  を自動で再タグ付けする migration は作らなかったため、手動 SQL が必要
  になった。

## アクションアイテム

### 予防 (Prevent)

- [x] **AI-P1** `recap_jobs.trigger_source` を実際に書き分ける
  (`'system'` / `'morning'` / `'user'`)。 `find_resumable_job` が
  `'system'` のみ resume 候補とする。 — **完了**: ADR-000709 で実装、
  `recap-worker/src/store/dao/job.rs`、`scheduler/jobs.rs:JobContext`。
- [x] **AI-P2** `make_interval(days/hours => $1)` の bind を `i32` に
  修正。`delete_old_jobs` / `find_resumable_job` / `mark_abandoned_jobs`
  が実際に動くようにする。 — **完了**: ADR-000708。
- [x] **AI-P3** `RECAP_RESUMABLE_MAX_AGE_HOURS` のデフォルトを 12h → 4h
  に短縮。pre-existing の古いゾンビ行は age で自動除外。 — **完了**:
  ADR-000709 追記。
- [ ] **AI-P4** `recap_jobs` に `resume_attempt_count INT NOT NULL
  DEFAULT 0` カラム追加 + 3 回超で強制 seal。 — **担当:** recap-worker
  owner、**期限:** 2026-04-30。Phase 5 として別 ADR で追跡。
- [ ] **AI-P5** `recap_jobs` と `morning_jobs` の table 物理分離も検討
  (根本分離)。migration + 大きなリファクタを伴うため design doc を先に。
  — **担当:** recap-worker owner、**期限:** 2026-05-31 (design only、
  実装は 2026-Q3)。

### 検知 (Detect)

- [ ] **AI-D1** ダッシュボード `/v1/dashboard/job-stats` に「`running` が
  N 時間以上継続する Job の件数」メトリクスを追加。 — **担当:**
  recap-worker owner、**期限:** 2026-05-15。
- [ ] **AI-D2** Prometheus / alert 経路を整備し、上記メトリクスが閾値
  (既定: 2h 以上 running が 1 件以上) を超えたら通知。 — **担当:**
  monitoring owner、**期限:** 2026-05-30。
- [ ] **AI-D3** `anyhow::Context` で wrap した SQL error の下層 (`sqlx`
  ErrorKind) を `error!` ログに展開する helper を導入。bind 型バグの
  ような silent failure を再発防止。 — **担当:** recap-worker owner、
  **期限:** 2026-04-25。

### 緩和 (Mitigate)

- [x] **AI-M1** boot-time `mark_abandoned_jobs` + `cleanup_old_jobs` を
  daemon 起動時に必ず呼ぶ。 — **完了**: ADR-000708。
- [ ] **AI-M2** subworker clustering 409 "run already in progress" の
  orphan cleanup 経路を subworker 側に追加。同じ `(job_id, genre)` で
  recap-worker 側が絶対に作り直さない不変は別途担保。 — **担当:**
  recap-subworker owner、**期限:** 2026-05-10。
- [ ] **AI-M3** daemon の resume ロジックに「pipeline 失敗時、Job 行を
  即 `failed` に確定させる」仕上げを追加。現状は crash 時 `running` のまま
  残る経路が残っている。 — **担当:** recap-worker owner、**期限:**
  2026-04-25。

### プロセス (Process)

- [ ] **AI-X1** runbook `docs/runbooks/recap-job-stuck-in-resume-loop.md`
  を新設。症状 (`running` が N 時間継続)、診断コマンド (`SELECT status,
  last_stage, kicked_at, trigger_source FROM recap_jobs WHERE
  status='running'`)、手動 unstick SQL をテンプレ化。 — **担当:**
  SRE、**期限:** 2026-04-20。
- [ ] **AI-X2** ポストモーテムレビュー定例に本件を掲載。ADR-000708/709
  の前提となる "discriminator 実装漏れ" パターンをチーム全体で認識
  共有する。 — **担当:** team lead、**期限:** 2026-04-25。

## 教訓

### 技術的教訓

1. **spec 上の discriminator は実装で使わないと意味がない**。`trigger_source`
   カラムは 3 月時点で追加されていたが、1 箇所も実際に書き分ける call
   site がなく、実質 dead schema になっていた。column を足すだけでなく、
   最低 1 つの呼び出し側で "default 以外の値を書く経路" を同時に提供
   しないと仕組みは死ぬ。

2. **`anyhow::Context` は単層 wrap では不十分**。`.context("failed to
   delete old jobs")` のように単純にラップすると、原因の SQLx error が
   隠れる。`tracing::error!` で下層エラーを `err = ?err` で必ず出す、
   あるいは `eyre::Report` / `miette` のように深い backtrace を標準で
   吐く error reporter を検討する。

3. **boot-time resume と長尺 Job の相性は悪い**。`kicked_at` だけを
   age 判定に使うと、クラッシュしても `updated_at` 側は新しい (= 活動的
   に見える) 偽陽性 Job が生まれる。**resume attempt 回数で cap する**
   のが最も信頼できる abandon 判定になる。

4. **共有 state テーブルに別 kind を相乗りさせる設計は debt**。
   `recap_jobs` に morning_update / user-triggered が相乗りしたことで、
   boot 時に "どの pipeline で re-run するか" の判断が daemon にしか
   集まらず、その daemon が kind を見ない = 全て batch として扱う、の
   片面実装になった。今回は discriminator 追加で対応したが、長期的には
   物理分離が素直。

### 組織的教訓

1. **ユーザー指摘の質を最初から信用する**。「3-day Recap は 02:00 JST
   のみ」は製品仕様を誰より知る当事者からの情報であり、一次調査で
   「大量に走っていない」と打ち消すのは誤り。一次情報に反する説明を
   出す前に、指摘側の仕様モデルを再現できるクエリ (status_history
   からの直接観測) で裏付けを取るべき。

2. **並行実装中の短時間再ビルド連打は、低頻度では見えない bug を曝す**。
   本日の ADR-000705/706/707 を並行実装したことで、通常 1 日 0 回の
   コンテナ再起動が 5 回以上発生。結果、resume loop が観測可能な密度で
   発火した。pre-production で短時間に強制 restart を走らせる
   "chaos lite" を運用準備の一環として用意する価値がある。

## 関連情報

- [[ADR-000705]] Morning Letter を event-sourced editorial に再定義
- [[ADR-000706]] Morning Letter の出力を編集的に意味のある形へ補正
- [[ADR-000707]] Morning Letter を Alt の知識グラフへの launch pad に
- [[ADR-000708]] recap-worker boot 時に orphan Recap Job を確実に sweep
- [[ADR-000709]] trigger_source で morning_update を batch Recap から分離
- Schema: `recap-migration-atlas/migrations/20260116000000_add_user_id_to_recap_jobs.sql` (trigger_source カラム導入点)
- Code: `recap-worker/recap-worker/src/store/dao/job.rs`, `scheduler/daemon.rs`, `scheduler/jobs.rs`
