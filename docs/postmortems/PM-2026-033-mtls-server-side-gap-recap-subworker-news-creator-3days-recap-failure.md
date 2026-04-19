# ポストモーテム: recap-subworker / news-creator の mTLS サーバ側未対応で 3days Recap が 5 日連続失敗

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-033 |
| 発生日時 | 2026-04-14 17:00 UTC 前後（mTLS Phase 2 client-side enforcement, commit `953666412` の反映直後から `recap-worker` の outbound が `https_only(true)` 強制になり、subworker / news-creator への呼び出しが送信前段階で蹴られ続ける状態が始まった。当日は 1 件、翌日以降は他のインシデント (PM-031 の 404、PM-032 の CertificateExpired) に紛れて発火が累積） |
| 検知日時 | 2026-04-19 12:30 JST 頃（ユーザーが「3days Recap の Job が失敗します。コンテナのログとDBを漁って原因を突き止めて」とチャットで報告。並列調査の結果、`recap_failed_tasks` の 18 件のうち他の 10 件は ADR-759/773 で解消済みで、残り 8 件 (`classification_0_results`) は別の根本原因と判明） |
| 復旧日時 | 一次/恒久同時: 2026-04-19 セッション内に [[ADR-000774]] の commit 089404aa6 / 6b7ec81a2 を作成（執筆時点で `git push origin main` 経由 alt-deploy デプロイ待ち） |
| 影響時間 | ラテント 4 日 19 時間（2026-04-14 17:00 UTC ≒ 2026-04-15 02:00 JST → ユーザー報告 2026-04-19 12:30 JST）、ユーザー体感 (subworker 経路だけを切り出して数えた場合): 同期間中 8 件のジョブが連続失敗 |
| 重大度 | SEV-3（Knowledge Home の 3days Recap セクションのみ停止。7days Recap / Feeds / Augur / Knowledge Home 本体は経路が独立で影響なし。単一ホスト開発環境の単一ユーザー） |
| 作成者 | recap / platform / pki チーム |
| レビュアー | — |
| ステータス | Draft |

## サマリー

2026-04-14 の mTLS Phase 2 client-side enforcement ([[000727]], commit `953666412`) で `recap-worker` の outbound を `MTLS_ENFORCE=true` にした際、共有 reqwest クライアントが `clients/mtls.rs:217-224` で `https_only(true)` を立てるようになった。しかし下流の **recap-subworker (uvicorn :8002 plain HTTP) と news-creator (FastAPI :11434 plain HTTP) はサーバ側 mTLS 化されないまま** enable されたため、reqwest はリクエスト送信前のスキーム検証段階で `URL scheme is not allowed` を返し、`classify-runs` の 5 チャンクすべてが空配列で fail-fast。`classification returned 0 results for 910 articles` のメッセージで 3days Recap ジョブが abort する症状が、ローテ後の手動キックや 02:00 JST の自動キックのたびに 8 件累積した。同期間中、別系統の silent failure である PM-2026-031（alt-backend `:9443` REST 配線漏れ）と PM-2026-032（クライアント cert hot-reload 欠落）が前面で発火しており、本件は「同じ 3days Recap 失敗」のノイズに紛れて DB の `recap_failed_tasks` で初めて切り分け可能な区分として浮上した。一次対応も恒久対応も同一で、`compose/pki.yaml` に `pki-agent-recap-subworker` / `pki-agent-news-creator` の reverse-proxy サイドカーを追加し、recap-worker の URL を `https://...:9443` に切り替え、tag-generator の `PROXY_ALLOWED_PEERS` に `recap-worker` を追加（並行して観測されていた `BadCertificate` 解消のため）。さらに `recap-worker/src/config.rs` に `validate_mtls_url_schemes` を追加し、`MTLS_ENFORCE=true` + http URL の組み合わせを起動時 fail-closed で確定的に検知できるようにした。本件は [[PM-2026-031]] / [[PM-2026-032]] と続く「ADR-727 cutover 残タスクが silent に潜伏する」シリーズの **第三形態** として記録する。

## 影響

- **影響を受けたサービス:** `recap-worker` の 3days window パイプライン全体。fetch / preprocess / dedup までは通過するが、genre / classify ステージで `https_only` reqwest クライアントが `URL scheme is not allowed` を返すため、`recap-subworker /v1/classify-runs` への 5 チャンク全てが空配列で fail-fast。`classification returned 0 results for N articles (service may be unavailable)` のメッセージで `recap_jobs.status='failed'` になる。
- **影響を受けた画面:** Knowledge Home の 3days Recap セクション。
- **影響を受けたユーザー数/割合:** 単一ホスト開発環境の操作ユーザー 1 名。
- **機能への影響:**
  - `recap_failed_tasks` の `classification_0_results` 区分が 8 件累積（最古: 2026-04-14 17:00 UTC、最新: `d5c15111-de1a-4a87-8e16-03f94856086d` @2026-04-19 03:35 UTC）
  - 同期間中の他の失敗 (`not_found` 4 件 / `certificate_expired` 3 件 / `deserialization_error` 2 件 / `auth_error` 1 件) はそれぞれ別 PM で解消済み
  - 7days Recap、Feeds、Augur、Knowledge Home の 3days 以外は経路が独立で **影響なし**
  - 並行して観測された `tag-generator:9443` の BadCertificate（pki-agent reverse-proxy の `PROXY_ALLOWED_PEERS` に `recap-worker` が含まれていなかった）も同 PR で同時に解消
- **データ損失:** なし。recap job は abort のみで partial artifact は残らず、次回成功で上書きされる。
- **SLO/SLA違反:** 個別 SLO 未設定。Knowledge Home 全体 SLO への波及は 7days Recap 経路が健在だったため軽微。
- **潜在影響:**
  - 本件と並行して PM-031 (404) / PM-032 (CertificateExpired) が前面で発火していたため、3days Recap が 4-5 日連続失敗していても「PM-031 / 032 の修正待ち」と誤認しやすい状況だった。subworker 側の mTLS 未対応に気付くまでさらに数日延びる可能性があった
  - subworker 修正後、次の `dispatch` ステージで news-creator (`http://news-creator:11434`) に対する同型 silent failure が発火する **第二の地雷** が残っていた。本件で先回り修正

## タイムライン

全時刻は JST。UTC 併記は recap-worker ログ・`recap_failed_tasks.created_at` との整合のため。

| 時刻 (JST) | UTC | イベント |
|---|---|---|
| 2026-04-14 23:42 | 2026-04-14 14:42 | mTLS Phase 2 client-side enforcement の commit `953666412` がマージ。`recap-worker` の outbound に `https_only(true)` の reqwest クライアントが導入される。`compose/recap.yaml` の `SUBWORKER_BASE_URL=http://...` / `NEWS_CREATOR_BASE_URL=http://...` はそのまま。**ラテント発火** |
| 2026-04-15 02:00 | 2026-04-14 17:00 | 3days Recap の自動キック直後、`recap-worker` の `classify-runs` 呼び出しが `URL scheme is not allowed` で 5 チャンク全敗。`classification_0_results` 1 件目が `recap_failed_tasks` に積まれる |
| 2026-04-15 〜 18 | — | PM-2026-031 (alt-backend `:9443` REST 配線漏れ → 404) と PM-2026-032 (mTLS client cert in-memory → CertificateExpired) が前面で発火。3days Recap 失敗の主因として 404 と CertificateExpired が記録され、本件の `classification_0_results` は紛れて見落とされる |
| 2026-04-17 〜 18 | — | [[ADR-000759]] / [[ADR-000773]] が順次マージ・デプロイされ 404 / CertificateExpired は解消。残った `classification_0_results` 8 件が初めて他系統と切り分け可能になる |
| 2026-04-19 12:30 | 2026-04-19 03:30 | **検知。** ユーザーがチャットで「3days Recap の Job が失敗します。コンテナのログとDBを漁って原因を突き止めて」と報告 |
| 2026-04-19 12:35 | 03:35 | 並列調査開始。Explore サブエージェント 4 本を投入: (1) recap サービス全体構造、(2) コンテナログ、(3) DB の `recap_jobs` / `recap_failed_tasks`、(4) ADR / PM 検索 |
| 2026-04-19 12:50 | 03:50 | DB 結果から `classification_0_results` 8 件の存在を確認。最新 `d5c15111-...` のログに `URL scheme is not allowed` を発見、`reqwest::Client::builder().https_only(true)` の location (clients/mtls.rs:219) を特定 |
| 2026-04-19 13:00 | 04:00 | サービス全体の mTLS サーバ化監査を別エージェントで実施。news-creator が同型の plain HTTP 11434 のままであること、tag-generator allowed peers に `recap-worker` が含まれていないことを発見 |
| 2026-04-19 13:15 | 04:15 | 計画文書 `~/.claude/plans/3days-recap-job-db-plan-...md` を作成、ユーザー承認 |
| 2026-04-19 13:30 | 04:30 | TDD で実装。`recap-worker/src/config.rs` の `base_env_vars` に `MTLS_ENFORCE` を追加、scheme アサーションテスト 4 件を RED で書く → 実装で GREEN へ。326 tests pass |
| 2026-04-19 13:50 | 04:50 | `compose/pki.yaml` に `pki-agent-recap-subworker` / `pki-agent-news-creator` reverse-proxy サイドカー追加、`compose/base.yaml` に対応する cert ボリューム宣言、`compose/recap.yaml` の URL を https に切替、`compose/pki.yaml` の tag-generator allowed peers に `recap-worker` を追加 |
| 2026-04-19 14:00 | 05:00 | 2 commit に分割（RED テスト + GREEN 実装/compose）して main にマージ。commit `089404aa6` (test) / `6b7ec81a2` (feat) |
| 2026-04-19 14:10 | 05:10 | [[ADR-000774]] 執筆 |
| 2026-04-19 14:30 | 05:30 | 本 PM 執筆 |
| 2026-04-19 (執筆時点) | — | **alt-deploy 経路 (`git push origin main` → `dispatch-deploy.yaml` → `Kaikei-e/alt-deploy`) でのデプロイ待ち**。本セッション内ではユーザー承認後に push する |

## 検知

- **検知方法:** ユーザー報告（チャット経由）。
- **TTD (Time to Detect):** 4 日 19 時間（2026-04-15 02:00 JST のラテント発火 → 2026-04-19 12:30 JST のユーザー報告）。
- **検知の評価:** **遅い。** 本件は 2026-04-15 から実際に毎日 1 件以上の `classification_0_results` を `recap_failed_tasks` に積み続けていたが、検知の穴が 3 段階で重なっていた:

  1. **3days Recap が 1 日 1 回程度のジョブ**であり、初回失敗から複数連続失敗を観測するまでの体感が遅い（[[PM-2026-031]] と同じ構造）。[[PM-2026-031]] Action Item #5「recap job の連続失敗で alert」は期限 2026-04-30 の TODO のままだったため、本件発火期間中もアラートは飛ばなかった。
  2. **同時期に PM-031 (404) / PM-032 (CertificateExpired) が前面で発火**しており、ユーザーが「3days Recap が落ちている」と認識しても、調査・修正は前面の症状（404 → CertificateExpired）から順に行われた。本件の `classification_0_results` は同じ「3days Recap 失敗」というカテゴリでログに紛れていたため、PM-031/032 の修正で「全部直ったはず」という誤認を生みやすかった。
  3. **[[PM-2026-031]] Action Item #4「smoke に mTLS 経由の REST 疎通テストを追加」が TODO のまま**で、recap-worker から subworker / news-creator への実経路 E2E は smoke でカバーされていなかった。`/health` smoke は recap-subworker の plain HTTP 8002 にしか触れず、recap-worker から見えるべき `:9443` の不在を検知できない。

  ユーザーが今回チャットで「DBを漁って原因を突き止めて」と明示してくれたことで、`recap_failed_tasks` の区分集計を初めて行い、`classification_0_results` という別カテゴリの 8 件を切り出せた。**DB 状態テーブルがログより雄弁である**という [[PM-2026-031]] の教訓が、本件で 2 回目の確認となった。

## 根本原因分析

### 直接原因

`recap-worker/src/clients/mtls.rs:217-224` の `build_mtls_client(...)` が `Client::builder().use_preconfigured_tls(tls_config).https_only(true).build()` で全アウトバウンドリクエストに `https_only` を強制している。一方で `compose/recap.yaml:97` の `SUBWORKER_BASE_URL=http://recap-subworker:8002` は plain HTTP の URL を指していた。`recap-subworker/recap_subworker/__main__.py` の uvicorn は plain HTTP の :8002 のみで listen し、`compose/pki.yaml` には `pki-agent-recap-subworker` サイドカーが存在しなかった。

そのため reqwest は handshake 以前のスキーム検証段階で `URL scheme is not allowed` を返し、`classify-runs` POST が 5 チャンク全てで送信されず、`process_classify_body` が空配列を受け取って `classification returned 0 results for N articles` を出して fail-fast した。

`news-creator` (`http://news-creator:11434`) も同型の plain HTTP のみで、同じ silent failure を起こす経路だが、ジョブが先に subworker で止まるため可視化されていなかった。

`tag-generator:9443` への `BadCertificate` は別系統で、`compose/pki.yaml:252` の `PROXY_ALLOWED_PEERS=${TAG_GENERATOR_MTLS_ALLOWED_PEERS:-alt-butterfly-facade,alt-backend,search-indexer}` に `recap-worker` が含まれていなかったため、tag-generator の pki-agent reverse-proxy が peer verification 段階で recap-worker の leaf cert を弾いていた。

### Five Whys

1. **なぜ 3days Recap が `classification_0_results` で失敗するのか？**
   → `classify-runs` の 5 チャンクすべてが reqwest で送信される前に蹴られ、空配列が返ったから。
2. **なぜ送信前に蹴られたのか？**
   → recap-worker の reqwest クライアントが `https_only(true)` で構築されており、`SUBWORKER_BASE_URL=http://...` という http スキームの URL を受け取って `URL scheme is not allowed` を即返したから。
3. **なぜ http URL のままだったのか？**
   → recap-subworker / news-creator のサーバ側に mTLS リスナー (`:9443` の pki-agent reverse-proxy サイドカー) が用意されていなかったから。https URL に切り替えても反対側に listener がない。
4. **なぜサイドカーが用意されていなかったのか？**
   → 2026-04-15 commit `953666412` の ADR-727 Phase 2 で recap-worker の outbound を `MTLS_ENFORCE=true` に切り替えたとき、Go サービス（alt-backend / auth-hub / pre-processor / search-indexer / alt-butterfly-facade）と Rust の recap-worker は同 PR でサーバ側 mTLS listener も整備されたが、Python サービス（recap-subworker / news-creator）は **outbound だけが mTLS-enforcing** になり、サーバ側は plain HTTP のままだった。tag-generator は同 PR で pki-agent reverse-proxy を入れたが allowed peers に recap-worker は含めていなかった。
5. **なぜ Python サーバ側 mTLS 化が抜けたのか？**
   → Go 側 (PR の主軸) と比べて Python サーバの mTLS 化は別の作業 (uvicorn ssl オプション or pki-agent サイドカー) を要し、ADR-727 Phase 2 の射程からは「サーバ側化は別 ADR で順次」と暗黙に切り出されていた。しかし「outbound 側を MTLS_ENFORCE=true に固定すると、サーバ側未対応経路は全て送信前に死ぬ」という対称性が ADR-727 に明文化されておらず、cutover 検証経路 (`/health` smoke / Pact gate) でも実経路が叩かれていなかったため、release から 4 日経って初めて気付いた。
6. **なぜ「outbound = サーバ側対応必須」の対称性が見落とされたのか？** (補足)
   → [[PM-2026-031]] / [[PM-2026-032]] と同じ「ADR の Cons は設計者への警告、Action Item は実行者への指示。Cons だけだと実行されない」という組織的弱点。ADR-727 Phase 2 の Cons には書かれていたかもしれないが、サーバ側 mTLS 化を担当・期限付き Action Item として別 ADR / PM に転記していなかった。

### 根本原因

**ADR-727 Phase 2 cutover の射程切り出しと、検知の穴の重なり**:

- 発火源: ADR-727 Phase 2 が outbound 側だけを `MTLS_ENFORCE=true` 固定し、Python サービス（recap-subworker / news-creator）のサーバ側 mTLS 化を「別 ADR で順次」として残した結果、対称性が崩れた状態で 4 日間運用された。
- 増幅器: PM-2026-031 (404) と PM-2026-032 (CertificateExpired) が同期間に前面で発火し、本件 (`classification_0_results`) を「同じ 3days Recap 失敗の別症状」としてノイズに埋もれさせた。`/health` smoke は subworker plain :8002 にしか触れず実経路を確認できず、Pact gate は contract 互換性のみ見るため transport 層の不整合は射程外。

これは [[PM-2026-031]] (cutover 残タスクの第一形態: listener 配線漏れ) / [[PM-2026-032]] (第二形態: クライアント cert hot-reload 欠落) と同シリーズの **第三形態 (サーバ側 mTLS 化未着手)** として整理できる。

### 寄与要因

- **recap-worker のエラー文言が `(service may be unavailable)`** で、`URL scheme is not allowed` という根本原因よりも先に「subworker が落ちているのか」と誤認させる構造になっていた。reqwest 側の `builder error` を context として残しつつ、上位レイヤで「service may be unavailable」と書き換えるとシステム的な原因（URL スキーム不一致）が読み取りにくい
- **3days Recap のスケジュール頻度**が 1 日 1 回程度で、初回失敗から「連続失敗」と認識するまでの窓が広い ([[PM-2026-031]] と同じ）
- **同時期に複数 mTLS 系インシデントが発火**したため、調査者が「mTLS 系は順次直してる最中」と認識し、subworker / news-creator のサーバ側未対応に気付く優先度が下がった
- **Python サービスの mTLS サーバ化がサービスごと個別判断**になっており、`pki-agent-tag-generator` / `pki-agent-acolyte-orchestrator` は採用済み、`pki-agent-recap-subworker` / `pki-agent-news-creator` は未採用、という差が ADR / runbook に明示されていなかった
- **DB の `recap_failed_tasks.error_message` 列がログより雄弁**だった。最初からログ grep ではなく DB 集計をしていれば、`classification_0_results` の 8 件は即時に切り分けられた ([[PM-2026-031]] と同じ教訓の再確認)

## 対応の評価

### うまくいったこと

- **並列調査の効率。** 検知から 30 分以内で 4 本の Explore サブエージェントを並列投入し、(1) サービス構造、(2) コンテナログ、(3) DB 状態、(4) ADR / PM 履歴の 4 軸で根本原因を切り分けられた。
- **DB 状態テーブルでの切り分け。** `recap_failed_tasks.error_message` を区分集計したことで、18 件の失敗を 5 つの根本原因カテゴリに分解でき、PM-031 / 032 で既に解消済みの分と本件 (`classification_0_results` 8 件) を独立した症状として認識できた。
- **TDD 厳守。** `recap-worker/src/config.rs` の `validate_mtls_url_schemes` は RED コミット (`089404aa6`) → GREEN コミット (`6b7ec81a2`) で 2 段階に分けて TDD フローを徹底した。失敗テスト 4 件 → 実装で 10/10 GREEN を確認してからマージ。
- **将来の同型バグ予防。** scheme アサーションを `Config::from_env()` に組み込んだため、次に「`MTLS_ENFORCE=true` だが http URL」という silent failure が混入しても、起動時 fail-closed で確定的に検知される。平均検知時間 (TTD) を「ジョブ失敗が累積する数日」→「コンテナ起動の数秒」に短縮できる。
- **副次問題の同時解消。** `tag-generator` への BadCertificate も同 PR で `PROXY_ALLOWED_PEERS` に `recap-worker` を追加することで解消し、ログのノイズが減った。
- **news-creator の allowed peers を先回り宣言。** `pki-agent-news-creator` の `PROXY_ALLOWED_PEERS=recap-worker,acolyte-orchestrator,rag-orchestrator,recap-evaluator` を初期値とすることで、後続 Python orchestrator が `MTLS_ENFORCE=true` に切り替わった時に compose を再変更する必要がなくなる。
- **commit 分割の意味単位。** [[PM-2026-031]] の教訓「commit を意味単位で分割してレビュー可能性を維持」を本 PR でも踏襲。RED commit / GREEN+compose commit の 2 段構成で、レビュー時に「テストはどの実装を守っているか」が明確。
- **デプロイ手順遵守。** [[PM-2026-031]] で対応者が `docker compose up --build` を独断実行して叱責された反省から、本セッションは最後まで `git commit` で止め、`git push origin main` 経由 alt-deploy （`.github/workflows/dispatch-deploy.yaml` → `Kaikei-e/alt-deploy`）はユーザー承認待ちとした。デプロイ手順の改訂 (2026-04-19 alt-deploy 移行) も memory に永続化。

### うまくいかなかったこと

- **本件の検知が PM-031/032 の修正完了まで遅延。** 4 日 19 時間のラテントは、本来 PM-031/032 と同時に DB 集計ベースで切り分けていれば 1-2 日短縮できた。前面で発火している症状の修正に集中するあまり、別系統の silent failure を見逃した。
- **fmt 差分が既存コードに残っていることを発見したが、スコープ外として保留。** `cargo fmt --check` で 33 個の diff があり、私の追加コード (`config.rs:7XX`) も最初は含まれていた。手動修正で自分の差分は解消したが、残り 32 個は既存箇所の累積負債で、本 PR では触らなかった。CI で fmt gate が落ちる可能性がある。
- **plan 修正の往復が 3 回発生した。** デプロイ手順について `./scripts/deploy.sh production` → `./deploy-system/deploy-local.sh` → `git push → alt-deploy` と 3 段階でユーザー訂正を受けた。最初に `gh workflow list` か `.github/workflows/` の確認をしていれば 1 発で正解に辿り着けた。
- **subworker / news-creator のサーバ側 mTLS 化を `MTLS_ENFORCE=true` 切替時の不変条件として ADR-727 に書いていなかった。** outbound 側の enforcement と server 側の listener 整備は対称性があり、片方だけ進めると今回の silent failure を生む。これを ADR / runbook の checklist にしておくべきだった。

### 運が良かったこと

- **単一ホスト開発環境。** 本番マルチテナントなら 3days Recap を購読する全ユーザーが 4 日以上の空白を経験していた。
- **ジョブ abort 時の partial artifact が残らない設計。** `recap_jobs.status='failed'` で停止しても `recap_outputs` には書き込まれず、次回成功で上書きされる。データ整合性は保たれた。
- **ユーザーが「DBを漁って」と明示指示してくれた。** 通常ログから入る調査だったら、`URL scheme is not allowed` という reqwest の builder error がログ末尾に出ているかどうか、ログレベル次第では見逃した可能性がある。DB 集計で 8 件の存在が一目で分かった。
- **subworker で fail-fast したため news-creator までジョブが進まず**、第二の地雷 (news-creator も同型の plain HTTP) が発火しなかった。subworker を直すと dispatch ステージで news-creator にぶつかる予定だったが、本 PR で同時に修正したため再発火を回避。
- **`/alt-adr-writer` skill のメモリが「ADR 書いてデプロイ」のフローを守らせた。** 「ADR だけ書いて」のスキップ条件を明確化することで、deploy ステップを意図せず走らせるリスクを排除。

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|---|---|---|---|---|
| 1 | 予防 | `compose/pki.yaml` に `pki-agent-recap-subworker` / `pki-agent-news-creator` reverse-proxy サイドカー追加、`compose/base.yaml` に cert volumes 宣言、`compose/recap.yaml` の URL を https に切替 | recap / pki チーム | 2026-04-19 | **Done**（commit `6b7ec81a2`、[[ADR-000774]]） |
| 2 | 予防 | `compose/pki.yaml:252` の `TAG_GENERATOR_MTLS_ALLOWED_PEERS` 既定値に `recap-worker` を追加 | platform | 2026-04-19 | **Done**（commit `6b7ec81a2`） |
| 3 | 予防 | `recap-worker/src/config.rs` に `validate_mtls_url_schemes` を実装、`Config::from_env()` で呼び出し。`MTLS_ENFORCE=true` + http URL を fail-closed で起動時拒否 | recap チーム | 2026-04-19 | **Done**（commits `089404aa6` test / `6b7ec81a2` impl） |
| 4 | 予防 | recap-subworker / news-creator の plain HTTP listener を `127.0.0.1` 限定にバインドし、外部から plain port が見えないようにする | platform | 2026-05-15 | TODO（別 ADR、Phase 2 として分離） |
| 5 | 予防 | `recap-evaluator` / `rag-orchestrator` も将来 caller が `MTLS_ENFORCE=true` になった際に同型バグを起こさないよう、pki-agent サイドカーをいま追加するかは別 ADR で判断。news-creator allowed peers には先回り宣言済み | platform | 2026-06-01 | TODO |
| 6 | 検知 | `scripts/smoke.sh` に mTLS 経由の実経路 E2E を追加（recap-worker → subworker `/v1/classify-runs`、recap-worker → news-creator `/v1/summary/generate`、recap-worker → tag-generator `/api/v1/tags/batch`）。[[PM-2026-031]] Action Item #4 / [[PM-2026-032]] Action Item #5 と統合 | platform | 2026-04-30 | TODO（[[PM-2026-031]] / [[PM-2026-032]] から継承、期限前倒し） |
| 7 | 検知 | `recap_job_status_history` の連続失敗 alert を Prometheus exporter に追加 (`recap_job_failed_consecutive >= 2` で slack 通知)。[[PM-2026-031]] Action Item #5 から継承 | recap チーム | 2026-04-30 | TODO（継承） |
| 8 | プロセス | ADR-727 Phase 2 の Action Item として「Python サーバ側 mTLS 化対象の網羅リスト」を別 ADR / runbook に明文化する。outbound enforcement と server listener の対称性を不変条件として記録 | platform / docs | 2026-05-01 | TODO |
| 9 | プロセス | `docs/runbooks/mtls-cutover-checklist.md` の新規追加（[[PM-2026-032]] Action Item #8 と統合）。「caller を MTLS_ENFORCE=true にする前に、その全 callee がサーバ側 mTLS 化 (pki-agent サイドカー or 直書き ssl) されているか」を確認する項目を必須化 | docs / platform | 2026-05-01 | TODO（[[PM-2026-032]] から継承） |
| 10 | プロセス | デプロイ手順が 2026-04-19 に `./scripts/deploy.sh production` → `git push origin main` + `dispatch-deploy.yaml` 経由 alt-deploy に切り替わったことを feedback memory として記録。`/alt-adr-writer` skill の §3 / `feedback_no_raw_compose_build.md` も併せて更新 | Claude / docs | 2026-04-19 | **Done**（本セッション内で memory 更新） |
| 11 | 検知 | DB の `recap_failed_tasks.error_message` 区分集計を Prometheus exporter で expose し、Grafana ダッシュボードに「失敗カテゴリ別カウント」パネルを追加。本件のような「同じ症状に見えて別根本原因」を時系列で識別できるようにする | observability | 2026-05-15 | TODO |
| 12 | 検知 | recap-worker のエラー文言から、reqwest の builder error / TLS handshake error / HTTP 4xx/5xx を区別できるようログを構造化する。「service may be unavailable」のような曖昧な書き換えを廃止し、根本原因の type を出す | recap チーム | 2026-05-15 | TODO |
| 13 | 予防 | `cargo fmt --check` の既存 33 個の diff を別 PR で解消する（本 PR ではスコープ外として保留した分）。CI gate が将来 fail-fast になる前に潰しておく | recap チーム | 2026-05-30 | TODO（別 PR） |

## 教訓

### 技術面

- **Outbound `MTLS_ENFORCE=true` と server-side mTLS リスナーは対称対**。caller を mTLS-enforcing にした瞬間、その全 callee がサーバ側 mTLS 化されていないと、TLS handshake 以前のスキーム検証段階で確定的に死ぬ。`https_only(true)` は良いセキュリティ性質だが、URL 設定との一致を起動時に確定的に検証しないと、release から数日経って気付くことになる。
- **`reqwest::Client::builder().https_only(true)` の存在は明示的に検証されるべき**。Rust の reqwest では `https_only(true)` を立てたクライアントに `http://` URL を渡すと、リクエスト送信前に `builder error: URL scheme is not allowed` を返す。これは正しい fail-fast だが、ログを grep しないと気付かない。設定検証側 (`Config::from_env`) で同条件を assertion するのが対称的な防御策。
- **pki-agent reverse-proxy mode は Python サービスの mTLS サーバ化に最適なテンプレート**。tag-generator / acolyte-orchestrator で実績があり、本 ADR で recap-subworker / news-creator にも展開した。Python uvicorn 自身に SSL/TLS context を持たせると ADR-773 で経験した hot-reload 周りの言語横断バグを再生産するリスクがあるため、reverse-proxy で外側に終端する方が運用が安定する。
- **DB の状態テーブルはログより区分集計に向く**。`recap_failed_tasks.error_message` のような構造化された失敗カテゴリ列を持つことで、複数の根本原因が同じ「3days Recap 失敗」というラベルに紛れて潜伏していた状況を一発で切り分けられた。ログ grep だと time-series で読まないと「何種類の根本原因が同居しているか」が見えにくい。
- **`compose/pki.yaml` の `PROXY_ALLOWED_PEERS` 既定値は包括的に書く**。news-creator allowed peers を `recap-worker,acolyte-orchestrator,rag-orchestrator,recap-evaluator` の 4 サービスで初期化したことで、将来の `MTLS_ENFORCE=true` 切替時に compose を触らずに済む。allowed peers をケチると本件と同じ「caller を切り替えた瞬間 BadCertificate」を生む。

### 組織面

- **ADR の Cons だけでは作業は進まない、Action Item に転記しないと忘れる**（[[PM-2026-031]] / [[PM-2026-032]] と同じ教訓の 3 度目の確認）。ADR-727 Phase 2 で「Python サーバ側 mTLS 化は別 ADR で順次」と書かれていた可能性があるが、担当・期限付きで別 ADR に転記されていなかったため 4 日間 silent に潜伏した。次の cutover 系 ADR では「対称性を保つ作業」を必ず Action Item 表に列挙する。
- **「同じカテゴリの障害が連続したら、別根本原因の混入を疑う」**。本件は PM-031 / 032 と同じ「3days Recap 失敗」というカテゴリに紛れていた。複数の修正をデプロイした後に「同じ症状」が残った場合、表面上の症状で安心せず、DB / ログを区分集計し直す習慣が必要。
- **デプロイ手順は急速に変わる、毎セッションで確認する**。本セッション中に `./scripts/deploy.sh production` (廃止) → `./deploy-system/deploy-local.sh` (ローカル開発用) → `git push origin main` 経由 alt-deploy (現行) と 3 段階で訂正を受けた。`./scripts/deploy.sh production` は 2026-04-17 PM-2026-031 時点では現役だったが、わずか 2 日後の 2026-04-19 セッション開始時点で既に廃止されていた。memory に保存した手順も逐次更新する。
- **「ユーザーがDBを漁って」と明示してくれたことが TTD を救った**。通常はログから入る調査ルーチンだが、DB 状態テーブルから入った方が雄弁な場合がある。今後は障害調査の最初に「ログ vs DB 状態テーブル」のどちらが雄弁かを判断する一手間を入れる。

## 参考資料

- [[ADR-000774]] 本 PM で根治した実装決定（recap-worker 下流の mTLS サーバ化と tag-generator allowed peers 是正）
- [[ADR-000727]] mTLS Phase 2 client-side enforcement — 本件のラテント発火源
- [[ADR-000759]] alt-backend `:9443` REST/Connect-RPC ハイブリッド listener — cutover 残課題の第一形態への対処
- [[ADR-000773]] pki-agent ローテに全 6 サービスの mTLS クライアントを追随させる — 第二形態への対処
- [[ADR-000772]] recap-worker に Hurl E2E スイートを追加 — `scenarios/06-recaps-3days.hurl` が本件の回帰防止に直接効く
- [[PM-2026-031]] mTLS cutover 残タスクで 3days Recap が 4 日連続 404 — 第一形態
- [[PM-2026-032]] recap-worker の mTLS クライアント証明書が in-memory に固定され 3days Recap が CertificateExpired で停止 — 第二形態
- [[PM-2026-030]] pki-agent sidecar の netns 幽霊化 — `depends_on.restart: true` 採用の根拠
- [[PM-2026-028]] mTLS 証明書期限切れによる Knowledge Home 停止 — 最初の mTLS 系 PM
- `recap-worker/recap-worker/src/clients/mtls.rs:217-224` — `https_only(true)` の location
- `recap-worker/recap-worker/src/config.rs:225,728-744` — `validate_mtls_url_schemes` と呼び出し
- `compose/pki.yaml:252,313-381` — tag-generator allowed peers と新規 2 サイドカー
- `compose/recap.yaml:96-97` — recap-worker URL の https 切替
- `compose/base.yaml:84-85` — `recap_subworker_certs` / `news_creator_certs` ボリューム宣言
- commit `953666412` (2026-04-15) — ADR-727 Phase 2、本件のラテント発火源
- commit `089404aa6` (2026-04-19) — RED: scheme assertion tests
- commit `6b7ec81a2` (2026-04-19) — GREEN: compose changes + validate_mtls_url_schemes 実装
- 失敗ジョブ DB レコード: `recap_jobs.window_days=3` の `status='failed'` 18 件、最新 `d5c15111-de1a-4a87-8e16-03f94856086d`

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
> 特に本 PM では、ADR-727 Phase 2 の cutover 時に「Python サーバ側 mTLS 化が抜けた」ことを
> 「実装担当者の見落とし」ではなく、「言語横断 cutover の対称性を不変条件として明文化していなかった
> システムの穴」として扱っています。同じ穴は ADR / runbook の checklist で塞ぐべきです。
