# ポストモーテム: recap-subworker の learning_machine artifacts 欠落で 3days Recap が 948 件分 classification 失敗

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-035 |
| 発生日時 | 2026-04-20 19:00:51 JST (手動キック分。潜在的なラテント発火は 2025-12-13 の `8235e8f0a` 以降、`classification_backend` デフォルトが `"learning_machine"` になった時点から、artifacts 配布経路が整備されていない環境では理論上の時限爆弾となっていた) |
| 検知日時 | 2026-04-20 19:00:51 JST — ユーザーが「Recap が死ぬ。`classification returned 0 results for 948 articles (service may be unavailable)`」と報告 |
| 復旧日時 | 2026-04-20 セッション内で [[ADR-000811]] の修正コミットを作成 (執筆時点で alt-deploy 経由のデプロイ反映待ち) |
| 影響時間 | 手動キック 1 回分。ユーザー体感としては 3days Recap ジョブ 1 件の結果喪失と、`result_count=0, elapsed_seconds=1516` (≒ 25 分) の待ち時間 |
| 重大度 | SEV-3 (Knowledge Home の 3days Recap 経路のみ、7days Recap / Feeds / Augur / Knowledge Home 本体は独立のため影響なし、単一ユーザー開発環境) |
| 作成者 | recap / platform チーム |
| レビュアー | — |
| ステータス | Draft |

## サマリー

2026-04-20 19:00:51 JST、3days Recap の手動キックが `classification returned 0 results for 948 articles (service may be unavailable)` で失敗した。エラーメッセージは [[PM-2026-033]] (mTLS URL スキーム不一致) のものと**完全に同文**だが、根本原因層は別。[[ADR-000774]] の対策 (`pki-agent-recap-subworker` / `pki-agent-news-creator` reverse-proxy サイドカー、compose の `https://...:9443`、`recap-worker/src/config.rs` の `validate_mtls_url_schemes`) は全てデプロイ済みでコード・稼働コンテナの両方で確認できており、TLS 層は健全だった。

真因は `recap-subworker` 側の学習機械モデル artifacts 欠落。`recap_subworker/infra/config.py:537-540` の `classification_backend` デフォルトが 2025-12-13 の commit `8235e8f0a` (「feat: Enhance configuration and data processing for learning machine」) 以降 `"learning_machine"` となっており、同バックエンドは `recap_subworker/learning_machine/artifacts/student/v0_ja|v0_en` を読み込む。しかし公開 OSS リポジトリ側 `.gitignore:44` がこのディレクトリ全体を除外しているため、`git clone` ベースのデプロイランナーには届かず、`compose/recap.yaml:224` の bind-mount で `/app/recap_subworker/learning_machine/artifacts/` として空のディレクトリが mount されていた。

`POST /v1/classify-runs` を受けると `recap_subworker/services/learning_machine_classifier.py:131-132` の `LearningMachineStudentClassifier.__init__` が `if self.model_ja is None and self.model_en is None: raise RuntimeError("At least one model (JA or EN) must be loaded")` を送出、worker pool init が `Failed to initialize classification worker pool within 300s` で落ち、5 チャンク × 3 リトライ全てで空配列を受け取って `classification returned 0 results for 948 articles (service may be unavailable)` で fail-fast した。`pki-agent-recap-subworker` サイドカーは upstream uvicorn が応答しない間 `http: proxy error: context canceled` を返しており、TLS 層の健全性を併せて示していた。

並行して `/app/data/*.joblib` (joblib バックエンド用のモデル) は `.gitignore:45` で除外されているが、別配布経路によりデプロイランナーの `recap-subworker/data/` 配下に実在し、`compose/recap.yaml` で個別ファイル bind-mount されコンテナ内 `/app/data/*.joblib` として読める状態だった (`docker inspect alt-recap-subworker-1` で確認)。つまり `joblib` バックエンドに切り替えるだけで即日復旧可能だった。

恒久対策として [[ADR-000811]] を作成し、`classification_backend` デフォルトを `"joblib"` に戻し、`Settings` に `@model_validator(mode="after")` で `_validate_learning_machine_artifacts` を追加した。`classification_backend == "learning_machine"` かつ JA/EN student ディレクトリが両方とも `Path.is_dir()` を満たさない場合、起動時に `ValidationError` で fail-closed する。これにより同型の「デフォルトと配布実態の不整合」が将来混入しても、コンテナ起動の数秒で確定検知できる。

本件は [[PM-2026-033]] と**同じエラー文言だが別根本原因**のケースとして記録する。PM-2026-033 の教訓「DB の `recap_failed_tasks.error_message` を区分集計して根本原因を切り分ける」は、メッセージ文字列だけで同型障害を同一視しない姿勢として引き続き有効である。

## 影響

- **影響を受けたサービス:** `recap-subworker` の classification 経路 (`/v1/classify-runs` エンドポイント)。`/health` エンドポイントは uvicorn が応答するので、外形監視上は「健康」と見えた。
- **影響を受けた画面:** Knowledge Home の 3days Recap セクション (当該キック分)。
- **影響を受けたユーザー数/割合:** 単一ホスト開発環境の操作ユーザー 1 名、手動キック 1 件。
- **機能への影響:**
  - 3days Recap ジョブ `65e63785-554a-457b-80fc-115ba3fa61b9` が `classification returned 0 results for 948 articles` で abort。`recap_outputs` への書き込みなし、次回成功で上書き可能。
  - 7days Recap / Feeds / Augur / Knowledge Home 本体は経路が独立で影響なし
- **データ損失:** なし。recap job は abort のみで partial artifact は残らず、次回成功で上書き。
- **SLO/SLA違反:** 個別 SLO 未設定。
- **潜在影響:**
  - 2025-12-13 の commit `8235e8f0a` 以降、本件は**ラテント状態で潜伏**していた。joblib → learning_machine のデフォルト切り替えが配布経路整備とセットで運用されておらず、`RECAP_CLASSIFICATION_BACKEND=joblib` を明示設定していた環境では表面化せず、デフォルトに依存していた環境で最初の classify-runs 呼び出しで初めて露見する設計。アラート/smoke/CI のどれもこれを捕捉できなかった
  - エラーメッセージが [[PM-2026-033]] と完全一致するため、「PM-2026-033 の再発」と誤認する可能性が高い。実際は別根本原因で、アクション (mTLS サイドカー整備) も別
  - 今回は `learning_machine_classifier.py:131-132` の `RuntimeError` 文言がログから追えたため切り分けできたが、`classification returned 0 results ...` の上位メッセージだけ見て判断していれば真因特定はさらに遅れた

## タイムライン

全時刻は JST。UTC 併記はコンテナログ `alt-recap-worker-1` との整合のため。

| 時刻 (JST) | UTC | イベント |
|---|---|---|
| 2025-12-13 12:27 | 2025-12-13 03:27 | commit `8235e8f0a` (「feat: Enhance configuration and data processing for learning machine」) がマージ。`recap_subworker/infra/config.py` の `classification_backend` デフォルトが `"learning_machine"` に。artifacts 配布経路は整備されず。**ラテント発火開始** |
| 2026-04-20 (詳細時刻不明) | — | `recap-subworker` コンテナが `ghcr.io/kaikei-e/alt-recap-subworker:sha-1018dc8` でデプロイ開始 (`docker inspect alt-recap-subworker-1` の `StartedAt: 2026-04-20T01:33:35Z`)。`/health` は通るが `/v1/classify-runs` の lazy init がまだ走っていない状態 |
| 2026-04-20 18:35 | 2026-04-20 09:35 | `recap-subworker` ログに `LearningMachineStudentClassifier.__init__` の `RuntimeError: At least one model (JA or EN) must be loaded` が出現 (恐らく前段のキック分)。classify-runs は失敗している |
| 2026-04-20 19:00:51 | 2026-04-20 10:00:51 | ユーザーが 3days Recap を手動キック。`recap-worker` が 5 チャンク (text_count=200 × 5 ≒ 1000 件) を subworker に送信するが、subworker の classifier 初期化が `Failed to initialize classification worker pool within 300s` で失敗 → 全チャンク空配列 → `result_count=0, elapsed_seconds=1516` → `classification returned 0 results for 948 articles (service may be unavailable)` で abort |
| 2026-04-20 (報告時刻) | — | ユーザーがチャットで「Recap が死ぬ。classification returned 0 results for 948 articles (service may be unavailable)」と報告 |
| 2026-04-20 (調査開始) | — | Plan Context Loader で関連 ADR / PM を収集。[[PM-2026-033]] と [[ADR-000774]] に同文エラーメッセージを発見。一方で 2 並列 Explore エージェントで「コード・ログ・コンテナ状態」を確認 |
| 2026-04-20 (切り分け) | — | 確認結果: compose/pki/config 全て ADR-000774 修正済み → 本件は再発ではない。稼働コンテナの `alt-pki-agent-recap-subworker-1` ログは `context canceled` (TLS 健全) → 真因は recap-subworker 内部 |
| 2026-04-20 (真因特定) | — | `docker logs alt-recap-subworker-1` から `RuntimeError: At least one model (JA or EN) must be loaded` と `LearningMachineStudentClassifier.__init__` のスタックトレースを発見。`docker exec alt-recap-subworker-1 ls /app/recap_subworker/learning_machine/artifacts/` が空であることを確認。`.gitignore:44` が同ディレクトリを除外していることを確認 |
| 2026-04-20 (計画策定) | — | 計画ファイル `~/.claude/plans/velvety-hatching-boole.md` を作成、ユーザー承認を経て R2 (config default を joblib に戻し + 起動時 fail-closed validator 追加) を決定。R1 (artifacts 配置) / R3 (配布経路整備) は不採用 |
| 2026-04-20 (TDD 実装) | — | RED テスト 5 件 (`tests/unit/test_classification_backend_validation.py`) → config.py を編集してデフォルト変更 + `@model_validator` 追加 → GREEN 確認 (5/5 pass)。広範囲テスト 295/295 + 12 skipped |
| 2026-04-20 (CI parity) | — | `uv run ruff check` / `uv run ruff format --diff` (新規コードに追加差分なし、既存 fmt 債務はスコープ外) / `uv run pyrefly check recap_subworker/infra/config.py` (0 errors) |
| 2026-04-20 (ADR/PM 執筆) | — | [[ADR-000811]] と本 PM を執筆 |
| 2026-04-20 (執筆時点) | — | **alt-deploy 経路 (`git push origin main` → `dispatch-deploy.yaml` → `Kaikei-e/alt-deploy`) でのデプロイ待ち**。ユーザー承認後に push 予定 |

## 検知

- **検知方法:** ユーザー報告 (チャット経由)。
- **TTD (Time to Detect):** 不定。本件は 2025-12-13 以降ラテントで潜伏しており、その間に `learning_machine` バックエンドでキックされたジョブが実際に何件失敗していたかは recap_failed_tasks を深く追わないと確定できない。ただし今回キックしたジョブは即時失敗 (開始から abort まで 25 分)。
- **検知の評価:** **検知の穴が 3 段階で重なっていた**:
  1. **`/health` smoke だけでは検知不能**: uvicorn 自体は起動していたので `/health` は 200 を返す。分類エンドポイントの lazy init 失敗は最初の classify-runs 呼び出しまで潜む。
  2. **エラーメッセージが [[PM-2026-033]] と同文**: `classification returned 0 results for N articles (service may be unavailable)` が identical。一瞬「PM-2026-033 の再発か」と誤誘導される。
  3. **[[PM-2026-033]] Action Item #6 (mTLS 経由の実経路 E2E 追加) が未完了**: 仮に実装されていても、本件は TLS 層の障害ではないので検知対象外だった可能性が高い。`/v1/classify-runs` を実データで叩く E2E を smoke / CI に入れていれば検知できた。
  4. **Action Item #12 (エラー文言を reqwest builder error / TLS handshake / HTTP 4xx/5xx で区別) も未完了**: 今回のように「service may be unavailable」の曖昧メッセージが誤誘導する問題は [[PM-2026-033]] で既に指摘されていた。本件もその続編。

## 根本原因分析

### 直接原因

`recap_subworker/infra/config.py:537-540` の `classification_backend: Literal["joblib", "learning_machine"] = Field("learning_machine", ...)` が 2025-12-13 の commit `8235e8f0a` 以降デフォルト `"learning_machine"` となっていた。本バックエンドは `recap_subworker/learning_machine/artifacts/student/{v0_ja, v0_en}` を `Path.exists()` でチェックして `StudentDistilBERT.from_pretrained(...)` でロードする (`learning_machine_classifier.py:105-129`)。しかし:

- **OSS リポジトリで除外**: `.gitignore:44` が `recap-subworker/recap_subworker/learning_machine/artifacts/` 全体を除外。`git ls-files` も空。
- **配布経路不整備**: `.gitignore:45` で同様に除外されている `recap-subworker/data/*.joblib` は別配布機構で deploy ランナーに届いているが (`docker inspect` で検証)、`learning_machine/artifacts/` は同機構の射程に入っていない。
- **bind-mount で空を上書き**: `compose/recap.yaml:224` の `../recap-subworker/recap_subworker/learning_machine/artifacts:/app/recap_subworker/learning_machine/artifacts:ro` により、コンテナ内 `/app/recap_subworker/learning_machine/artifacts/` は host 側の空ディレクトリで上書きされる。

結果、`POST /v1/classify-runs` 受付時に `LearningMachineStudentClassifier.__init__` が `if self.model_ja is None and self.model_en is None: raise RuntimeError(...)` (`learning_machine_classifier.py:131-132`) を投げ、classification worker pool 初期化が失敗、全チャンク空配列で fail-fast。

### Five Whys

1. **なぜ 3days Recap が `classification returned 0 results for 948 articles` で失敗したか？**
   → recap-subworker の `classify-runs` が空配列を返したから。
2. **なぜ subworker が空配列を返したか？**
   → `LearningMachineStudentClassifier.__init__` が `RuntimeError: At least one model (JA or EN) must be loaded` を投げて classification worker pool が初期化できなかったから。
3. **なぜ classifier がモデルをロードできなかったか？**
   → `/app/recap_subworker/learning_machine/artifacts/student/{v0_ja, v0_en}` が `Path.exists()` = False で、JA/EN どちらのモデルも None のままだったから。
4. **なぜ artifacts ディレクトリが空だったか？**
   → `.gitignore:44` が当該ディレクトリ全体を除外しており、公開 OSS リポジトリの `git clone` ベースのデプロイチェックアウトには存在しない。同じく除外されている `data/*.joblib` は別配布機構で届いているが、`learning_machine/artifacts/` はその機構の射程外。
5. **なぜ `classification_backend` デフォルトが `learning_machine` なのに配布経路が整備されていなかったか？**
   → 2025-12-13 の commit `8235e8f0a` で「feat: Enhance configuration and data processing for learning machine」として `learning_machine` バックエンドの設定を追加した時点では、artifacts 配布経路が整備される前提だったが、その後 4 ヶ月間、実際の本番配布経路 (Git LFS / S3 / named volume 等) への着手がなかった。デフォルト値と実態のズレが放置された。
6. **なぜズレが 4 ヶ月間検知されなかったか？** (補足)
   → `/health` smoke は uvicorn 本体しか確認せず、分類エンドポイントの lazy init 失敗は captured できない。`learning_machine_classifier.py:131-132` の `RuntimeError` は最初の classify-runs 呼び出しまで顕在化しない lazy design。さらに [[PM-2026-033]] Action Item #6 (実経路 E2E smoke) が TODO のまま、Action Item #11 (DB の `recap_failed_tasks` 区分集計を Prometheus 化) も TODO のままで、ラテント潜伏の仕組みが整っていた。

### 根本原因

**「設定デフォルトと artifacts 配布経路の非対称性」**:

- 発火源: 2025-12-13 に `classification_backend` デフォルトを `"learning_machine"` に変更したが、同バックエンドが要求する artifacts の配布経路を同時に整備しなかった。設定と実態のズレがラテントで残存した。
- 増幅器: (1) lazy init 設計により `/health` smoke で事前検知できない、(2) エラーメッセージが [[PM-2026-033]] と完全一致するため誤誘導、(3) [[PM-2026-033]] の Action Item #6 / #11 / #12 が未完のため補助的な検知手段が存在しなかった。

これは [[PM-2026-031]] → [[PM-2026-032]] → [[PM-2026-033]] と続く「設定と実態の非対称を検知できない silent failure シリーズ」の派生形。mTLS cutover 側が主軸だった PM-031 〜 033 と違い、本件は ML artifacts 配布側の非対称だが、「cutover 時に片側だけ変更される」という構造は同じ。

### 寄与要因

- **エラーメッセージの誤誘導**: recap-worker 側の `classification returned 0 results for N articles (service may be unavailable)` は reqwest / TLS / HTTP 層の問題に聞こえるが、実際は subworker 内の Python 例外。メッセージを構造化して「upstream service error」「upstream HTTP error」「upstream timeout」「invariant violated in classifier」等を区別していれば、真因特定が速かった。[[PM-2026-033]] Action Item #12 (同内容) が継承課題として残る。
- **lazy init 設計**: `_CLASSIFIER = LearningMachineStudentClassifier(...)` (`classification_worker.py:61-67`) は最初の classify-runs 呼び出し時にのみ実行される。起動時の `/health` では触れないため、artifacts 欠落は実運用キックまで発覚しない。Eager init に切り替えれば起動時に落ちるが、`joblib` バックエンドには影響させたくないので Pydantic validator 層で解決するのが最小侵襲。
- **`.gitignore` と配布機構の対応表が不在**: OSS 側で除外されたパスのうち、どれが別配布機構で deploy 側に届いているか (e.g. `recap-subworker/data/*.joblib`) / 届いていないか (`learning_machine/artifacts/`) の対応表がリポジトリ内になく、新バックエンド導入時に「artifacts 配布も同時に整備する」というチェックが走らなかった。
- **E2E / CDC テストが本分類経路を実データで叩いていない**: [[ADR-000772]] で recap-worker の Hurl E2E スイート (`scenarios/06-recaps-3days.hurl`) があるが、classification 経路を実モデルで走らせていない可能性が高い (要確認)。`/v1/classify-runs` を実データで叩く smoke を追加すれば、artifacts 欠落を起動後数秒で検知できる。

## 対応の評価

### うまくいったこと

- **並列調査の効率。** Plan Context Loader による ADR/PM 検索と 2 並列 Explore エージェント (コード探索 + DB/ログ) の投入で、30 分以内に「これは PM-2026-033 の再発ではない」と切り分けできた。[[PM-2026-033]] の教訓「同じカテゴリの障害が連続したら別根本原因の混入を疑う」を今回は早期に発動。
- **ADR / PM の事前読み込み。** [[ADR-000774]] / [[PM-2026-033]] を最初に読んでいたため「同文メッセージ = 同一原因」という短絡を回避できた。
- **稼働コンテナでの現実確認。** `docker ps` / `docker inspect` / `docker exec ls` で実際のマウント状態と artifacts 不在を目視確認し、仮説ではなく観測事実で真因を確定した。
- **公開境界の明示化。** ユーザーから「Alt は OSS、alt-deploy は Private、artifacts は公開できない」との前提を受けて、プランから R1 (rsync) と R3 (Git LFS) を early exclude。OSS 側でできる最小侵襲修正 (R2) に絞り込めた。
- **TDD 厳守。** 5 件の RED テストで「デフォルトが joblib」「学習機械バックエンド + 両パス欠落で ValidationError」「JA のみ / EN のみ accept」「joblib は validator 通過」を確定。実装後 5/5 GREEN → 広範囲 295/295 regression-free を確認。
- **fmt 債務をスコープ外として扱う規律。** PM-2026-033 §「うまくいかなかったこと」で言及されていた既存 fmt 差分 (33 個) を本 PR で一緒に潰したくなる誘惑を退け、新規追加部分だけ ruff format に準拠。PR の責務が明確。
- **公開境界の違反を回避。** artifacts を git に含める / Git LFS で公開する案を早期に却下し、OSS 境界を維持。

### うまくいかなかったこと

- **ラテント 4 ヶ月**。2025-12-13 のデフォルト変更から 2026-04-20 の実害検知まで、約 4 ヶ月のラテント期間があった。この間、デフォルトに依存していたキックが何件失敗していたかは `recap_failed_tasks` の `classification_0_results` 区分集計を広範に行えば確定できるが、[[PM-2026-033]] Action Item #11 の Prometheus 化が未完のため事後追跡コストが高い。
- **エラーメッセージ問題は [[PM-2026-033]] の Action Item #12 として記録されていたが未着手**。同じ曖昧メッセージに 2 度誤誘導されている。次回もっと直接的に取り組むべき。
- **lazy init の検知困難性を設計時に想定していなかった**。本来は `classification_backend == "learning_machine"` なら起動時に eager init する / または本 ADR で追加したような Pydantic validator で検証する設計を、2025-12-13 時点でセットで入れるべきだった。
- **配布経路対応表の不在を 4 ヶ月放置**。`.gitignore` で除外されているパスについて、どれが本番配布機構に乗っているかの対応表 (docs/runbooks/ 配下など) を作っていなかった。本 ADR でも対応表自体は作らず、バリデータで守るだけ。

### 運が良かったこと

- **単一ホスト開発環境。** 本番マルチテナントなら 3days Recap を購読する全ユーザーが影響を受ける状況だった。
- **ユーザーが即座に報告した。** 障害発生 (19:00:51 JST) から数分内に「Recap が死ぬ。(エラー原文)」と明示報告があり、調査開始が速かった。
- **joblib バックエンドの artifacts が deploy ランナーに配布済みだった。** `.gitignore:45` で同様に除外されているにもかかわらず、別配布機構で `recap-subworker/data/*.joblib` は届いていた。そのため「デフォルトを `joblib` に戻せば即復旧」という R2 が成立。
- **エラーメッセージが [[PM-2026-033]] と同文だったおかげで、PM-033 を先に読んで除外検証したことで「同文 ≠ 同根本原因」という確信を得てから真因調査に入れた。** 逆に同文でなければ古い ADR を参照せず、ADR-000774 が `validate_mtls_url_schemes` で同パターンを既に導入していることも見逃していた可能性がある。

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|---|---|---|---|---|
| 1 | 予防 | `recap_subworker/infra/config.py:537-540` の `classification_backend` デフォルトを `"learning_machine"` → `"joblib"` に戻す | recap チーム | 2026-04-20 | **Done** (本 PM の修正コミット、[[ADR-000811]]) |
| 2 | 予防 | `Settings` に `@model_validator(mode="after")` で `_validate_learning_machine_artifacts` を追加。`classification_backend == "learning_machine"` かつ JA/EN student ディレクトリが両方 `is_dir()` = False なら起動時 `ValidationError` で fail-closed | recap チーム | 2026-04-20 | **Done** (本 PM の修正コミット、[[ADR-000811]]) |
| 3 | 予防 | [[PM-2026-033]] Action Item #12 (recap-worker のエラー文言を reqwest builder error / TLS handshake error / HTTP 4xx/5xx / upstream domain error で区別) を着手。今回の「service may be unavailable」誤誘導は同内容の 2 回目の発現 | recap チーム | 2026-05-15 | TODO ([[PM-2026-033]] から継承、優先度上げ) |
| 4 | 検知 | [[PM-2026-033]] Action Item #6 (mTLS 経由の実経路 E2E を smoke に追加) の射程を広げ、`/v1/classify-runs` を実データで叩いて `classification_backend` ごとの正常動作を確認する E2E を追加。artifacts 欠落 silent failure を起動後数秒で検知できるようにする | platform | 2026-05-15 | TODO ([[PM-2026-033]] から継承、射程拡張) |
| 5 | 検知 | [[PM-2026-033]] Action Item #11 (DB の `recap_failed_tasks.error_message` 区分集計を Prometheus exporter で expose) を着手。本件のような「長期ラテント + 同文メッセージで別根本原因」を時系列で識別できるようにする | observability | 2026-05-15 | TODO ([[PM-2026-033]] から継承、優先度上げ) |
| 6 | プロセス | `docs/runbooks/` に「`.gitignore` で除外されているパスの配布経路対応表」を新規作成。`recap-subworker/data/*.joblib`、`recap-subworker/recap_subworker/learning_machine/artifacts/`、他の類似データディレクトリについて「OSS で除外、Private 経路で配布あり / なし、配布機構 (alt-deploy / Git LFS 等) の種類」を表形式で明記 | docs / platform | 2026-05-01 | TODO |
| 7 | プロセス | 「新しい外部データ依存を追加する ADR のチェックリスト」を `docs/runbooks/` または ADR テンプレート更新で明文化。`(a) 設定フィールドの追加, (b) 配布経路の整備 (Private mechanism), (c) 起動時 fail-closed バリデータ, (d) CI/smoke での疎通確認` の 4 点を同一 PR か対応する後続 PR でまとめて完成させる運用 | docs / platform | 2026-05-15 | TODO |
| 8 | 予防 | 本件と同型の「デフォルト値と外部依存の非対称」が他バックエンド (e.g. `model_backend='ollama-remote'` with no OLLAMA_EMBED_URL set, `graph_build_enabled=true` with no graph store) にも潜んでいないかの audit。同様に `@model_validator` で起動時 fail-closed を必要箇所に追加 | recap チーム | 2026-05-30 | TODO |
| 9 | 将来 | `learning_machine` バックエンドを本番で運用したい判断になった場合、alt-deploy (Private) の配布機構に `learning_machine/artifacts/` を追加する経路を別 ADR で設計する (`R3`)。本 PM と [[ADR-000811]] では dev/eval 用途に留め、本番は `joblib` 固定 | ML / platform | 未定 | 保留 (明示判断があれば再開) |

## 教訓

### 技術面

- **エラーメッセージの完全一致 ≠ 同一根本原因**。本件は `classification returned 0 results for N articles (service may be unavailable)` が [[PM-2026-033]] と identical だったが、原因層は TLS scheme validation → Python `RuntimeError` と完全に別。メッセージ文字列だけで一次切り分けすると、修正済みのはずの問題を「再発」と誤判断するリスクがある。DB / ログ / コンテナ状態を観測事実ベースで突き合わせる姿勢が必須。
- **lazy init は silent failure の温床**。`_CLASSIFIER = LearningMachineStudentClassifier(...)` のような lazy init は初回呼び出しまで失敗を隠す。`/health` smoke を通すためにプロセス自体は生きているので、外形監視でも検知できない。Pydantic validator で eager な起動時バリデーションを入れるのが最小侵襲の対策。
- **公開 OSS と Private 配布機構の非対称は暗黙知になりやすい**。`.gitignore` で除外されているパスのうち、どれが別配布機構で deploy 側に届いているかは見えにくい。対応表を `docs/runbooks/` に置くことで、次のバックエンド追加時に「artifacts 配布も対応させる」チェックが走るようにする。
- **`Path.is_dir()` vs `Path.exists()` の同意。** 本 PR の validator は `is_dir()` を使い、既存 classifier は `exists()` を使う。空ディレクトリが存在するケースでは前者は True、後者は True (ディレクトリそのものは存在するので)。今回は「ディレクトリが空でも通過」で構わない (classifier 側の `RuntimeError` が守る多層防御) という判断をした。将来 stricter にしたくなったら `(dir / "config.json").is_file()` 等に上げる余地を残してある。

### 組織面

- **「デフォルト値と外部依存の対称性」を不変条件として明文化**する。設定追加 PR には「デフォルトが依存する外部状態 (artifacts / env vars / listener / cert) の配布・存在が、同 PR か追跡 ADR で保証されているか」をレビュー項目に組み込む。これは [[PM-2026-031]] / [[PM-2026-032]] / [[PM-2026-033]] と同じ「cutover 残タスクが silent に潜伏する」系の教訓の再適用。
- **Action Item の継承は優先度上げで**。今回のエラーメッセージ誤誘導は [[PM-2026-033]] Action Item #12 で既に指摘されていた。同類の AI が 2 度以上の障害で現れた場合、次の PM では期限前倒し・担当明確化で優先度を上げる運用を徹底。
- **公開境界を先にユーザーに確認する**。本セッションで当初 R3 (Git LFS) を検討したが、ユーザーから「artifacts は公開できない、Alt は OSS で alt-deploy は Private」との前提を明示されて初めて R3 を除外できた。今後はプラン初手で「この変更は公開境界にまたがるか?」を確認し、境界違反を含む案は early exclude する。

## 参考資料

- [[ADR-000811]] 本 PM で決定した設計変更 (classification_backend default 切替 + fail-closed validator)
- [[ADR-000774]] recap-worker 下流を pki-agent reverse-proxy で mTLS サーバ化 — 同文エラーメッセージを生んだ先例、本 PM の切り分けで参照
- [[PM-2026-033]] recap-subworker / news-creator の mTLS サーバ側未対応で 3days Recap が 5 日連続失敗 — 同文 different-root-cause の直接先例
- [[PM-2026-031]] / [[PM-2026-032]] — cutover 残タスクが silent に潜伏するシリーズ
- [[ADR-000727]] mTLS Phase 2 client-side enforcement — `validate_mtls_url_schemes` のパターン導入元、本 PM の validator と同方針
- `recap-subworker/recap_subworker/infra/config.py:537-540` (default) / 同ファイル `_validate_learning_machine_artifacts` (validator)
- `recap-subworker/recap_subworker/services/learning_machine_classifier.py:131-132` 既存 `RuntimeError` 地点 (多層防御として残存)
- `recap-subworker/tests/unit/test_classification_backend_validation.py` 本 PM の TDD テスト (5 件)
- `compose/recap.yaml:224` bind-mount 行 (本 PR では変更なし、R3 スコープ)
- `.gitignore:44,45` artifacts / data 除外行 (本 PR では変更なし、R3 スコープ)
- commit `8235e8f0a` (2025-12-13) — `classification_backend` デフォルトを `"learning_machine"` にした commit (ラテント発火源)
- docker コンテナ観測: `alt-recap-subworker-1` `StartedAt: 2026-04-20T01:33:35Z`、`alt-recap-worker-1` `Up 24 hours (healthy)` (ADR-000774 の修正が既に稼働中であることの証跡)
- 失敗ジョブ: `recap_jobs.id=65e63785-554a-457b-80fc-115ba3fa61b9` (`window_days=3`, `result_count=0`, `elapsed_seconds=1516`)

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
> 特に本 PM では、2025-12-13 の classification_backend デフォルト変更時に「artifacts 配布経路を同時整備する」という対称性が
> 不変条件として明文化されていなかったことを「実装担当者の見落とし」ではなく、「公開 OSS / Private 配布機構の非対称を
> 検知する仕組みがシステム側に無かった穴」として扱っています。同じ穴は docs/runbooks/ の配布経路対応表と
> ADR チェックリストで塞ぐべきです。
