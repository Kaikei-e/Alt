---
title: "PM-2026-036 recap-subworker joblib artefacts bind-mount が空ディレクトリ化し 3days/1day Recap が 8 日間 silent 失敗"
date: 2026-04-22
tags:
  - alt
  - postmortem
  - recap-subworker
  - recap-worker
  - docker-compose
  - silent-failure
  - classification
---

# ポストモーテム: recap-subworker の joblib artefacts bind-mount が空ディレクトリ化し、3days / 1day Recap が 8 日間 silent に失敗し続けた

## メタデータ

| 項目 | 内容 |
|------|------|
| インシデントID | PM-2026-036 |
| 発生日時 | 2026-04-14 02:00:05 JST (= 2026-04-14 17:00:05 UTC の自動バッチ初回失敗) |
| 検知日時 | 2026-04-22 (ユーザーが「3days Recap の Job が失敗し続けている」と報告) |
| 復旧日時 | 執筆時点 (2026-04-22) — コード修正は landed、deploy runner 側の host artefacts 復旧待ち |
| 影響時間 | 約 8 日間の連続失敗 (自動バッチ最低 8 回 + 手動キック複数) |
| 重大度 | SEV-3 (Knowledge Home 3days/1day Recap 経路のみ、7days Recap / Feeds / Augur / Knowledge Home 本体は独立で影響なし、単一ホスト開発環境) |
| 作成者 | recap / platform チーム |
| レビュアー | — |
| ステータス | Draft |

## サマリー

2026-04-14 02:00 JST 以降、3days Recap の自動バッチが **連続 8 日間** `classification returned 0 results for N articles (service may be unavailable)` で失敗し続けていた。同じ期間に 1day morning update パイプラインも `window_days=1` の recap として 11 件失敗。最後に成功したのは 2026-04-14 02:56 JST (= `2026-04-13 17:56:36 UTC`) で、以降 3days が 23 件、1day が 11 件、計 **34 ジョブが `recap_failed_tasks.stage=pipeline_execution` で累積**していた。`alt-recap-subworker-1` コンテナは `Up 39 hours (healthy)` を維持し、`/health` HTTP プローブは常時 200 を返していたため compose 外形監視では一切の異常が出なかった。

真因は docker-compose の file-scoped bind mount の仕様: `compose/recap.yaml:215-223` が `../recap-subworker/data/genre_classifier.joblib:/app/data/genre_classifier.joblib:ro` のように **個別ファイル単位**で 10 個の joblib / json を bind-mount していたが、deploy runner のホスト側 `<alt-deploy>/alt/recap-subworker/data/*.joblib` が 2026-04-14 頃に消失。docker は「missing host source file」を無視せずに**空ディレクトリを自動生成してその場所に mount** する。結果、コンテナ内 `/app/data/genre_classifier.joblib` が「サイズ 0 の空ディレクトリ」として見え、`Path.exists()` = True を通過、`joblib.load()` が `IsADirectoryError` を送出、classification worker pool init が 300s で timeout、5 チャンクとも空配列返却、recap-worker 側で `classification returned 0 results for N articles` として fail-fast。

本件は [[PM-2026-035]] の直系の後継。PM-035 では 2026-04-20 に `classification_backend` デフォルトを `"learning_machine"` → `"joblib"` に revert し、`_validate_learning_machine_artifacts` fail-closed validator を追加したが、**「joblib artefacts が deploy runner に届いている」を暗黙の前提としていた**。実際にはその 6 日前 (2026-04-14) から joblib artefacts も行方不明で、PM-035 の fix は silent に ineffective だった。エラー文言は PM-035 / [[PM-2026-033]] と completely identical (`classification returned 0 results for N articles ...`) で、3 度目の誤誘導が成立し得る状況だった。

恒久対策は 3 層: (1) `Settings._validate_joblib_artifacts` 追加で `classification_backend == "joblib"` かつ joblib パスが「ディレクトリ型」なら起動時 `ValidationError` で fail-closed、(2) `recap-subworker/recap_subworker/services/classifier.py:75` の `.exists()` → `.is_file()` 多層防御、(3) `compose/recap.yaml` の 10 個の file-scoped bind を named volume + busybox init container の 1 本に圧縮、`recap-subworker-artifacts-init` が host ディレクトリに最低 1 個の usable joblib model がなければ `exit 1` で起動前に落とす。

## 影響

- **影響を受けたサービス:** `recap-subworker` の classification 経路 (内部呼び出し `/v1/classify-runs` に相当する分類パイプライン)。`/health` エンドポイントは uvicorn が応答するので外形監視上は「healthy」と見えた。`alt-recap-worker-1` は `/v1/generate/recaps/3days` 受付はできるが、pipeline 実行時に `dispatch` 段で空配列を受け取り `pipeline_execution` 段で fail。
- **影響を受けた画面:** Knowledge Home の 3days Recap セクション (8 日間全て stale)、Morning Update の 1day article group (11 件失敗分)。
- **影響を受けたユーザー数/割合:** 単一ホスト開発環境の操作ユーザー 1 名。マルチテナント本番であれば 3days/1day Recap を購読する全ユーザーに影響する設計。
- **機能への影響:**
  - **3days recap:** 2026-04-14 02:00 JST 以降の全自動バッチ (8 日 × 02:00 JST 分) + 手動キック複数 = DB 上で `recap_jobs WHERE window_days=3 AND status='failed'` が 23 件、`status='completed'` が 14 件 (いずれも 2026-04-13 以前の古いもの)。
  - **1day morning update:** 同期間の自動実行で `window_days=1 AND status='failed'` が 11 件、`completed` は 1 件のみ (古いもの)。
  - **7days recap:** 自動バッチ頻度が低く、かつ coarse genre classifier が fallback keyword 経路を経由する頻度が高いため、部分的に動作していた可能性が高い (明確なエラー確認なし)。本 PM ではスコープ外。
- **データ損失:** なし。失敗 recap は `recap_outputs` への書き込み前に abort しており、partial artifact は残っていない。成功すれば後続の `window_days=3` ジョブが上書き。
- **SLO/SLA違反:** 個別 SLO 未設定。
- **潜在影響:**
  - 本番マルチテナント運用であれば「Knowledge Home のトップ面が 8 日間古い」状態がユーザー可視の劣化となる。
  - PM-2026-035 の fail-closed validator は learning_machine backend のみカバーしており、joblib backend には届いていなかった。結果、PM-035 の修正 commit (`63aa6dee2`, 2026-04-20) が投入された後も本件は無症状 (ただし継続中) のまま 2 日間放置された。PM-035 が「これで silent failure は塞がった」と言い切っていた状態で、**塞ぎ漏れが直後に露見**したことは、cutover 系 PM シリーズ ([[PM-2026-031]] → [[PM-2026-032]] → [[PM-2026-033]] → [[PM-2026-035]] → 本 PM) の通算 5 本目の silent failure として記録すべき。

## タイムライン

全時刻は JST。UTC 併記は DB の `recap_jobs.kicked_at` (UTC) および `recap_failed_tasks.created_at` (UTC) との整合のため。

| 時刻 (JST) | UTC | イベント |
|---|---|---|
| 2026-04-14 02:56:36 | 2026-04-13 17:56:36 | `recap_jobs.job_id=137a4a66-...` (`window_days=3`) が **最後に status=completed** で完了。以後連続失敗の開始点。 |
| 2026-04-14 02:00:05 | 2026-04-13 17:00:05 | **wait, this is the UTC of the first FAILED run after the last success**: `recap_jobs.job_id=71ece724-...` (`window_days=3`) が 02:00 JST の自動バッチで kick、`classification returned 0 results for 959 articles` で失敗。**以降 8 日間、毎日 02:00 JST の自動バッチが同症状で失敗**。ラテント起点。 |
| 2026-04-14 (日付不明) | — | deploy runner ホスト側の `<alt-deploy>/alt/recap-subworker/data/*.joblib` 群が何らかの理由で **消失 or 初期化**。消失契機は本 PM 執筆時点で未確定 (alt-deploy runner 自体は別 Private リポジトリのため Alt 側で履歴追跡不可能)。 |
| 2026-04-14 以降 8 日間 | — | 各日 02:00 JST の自動 3days recap、および朝の 1day morning update が連続失敗。DB の `recap_failed_tasks.stage=pipeline_execution` に毎日 1-2 件ずつ累積。`alt-recap-subworker-1` は `Up … (healthy)` を維持、`/health` HTTP は 200 を返し続け、`docker compose ps` 上も異常なし。外形 smoke / Prometheus アラートは発火せず。 |
| 2026-04-20 19:00:51 | 2026-04-20 10:00:51 | [[PM-2026-035]] で記録された手動キック分。こちらは `learning_machine` backend の artifacts 欠落で失敗 (当時は default が `learning_machine`)。**同時に joblib artefacts も既に欠落していた**が、当時は `learning_machine` が default だったため joblib 経路には触れず、joblib 側の欠落は検知されなかった。 |
| 2026-04-20 (後半) | — | [[ADR-000811]] / [[PM-2026-035]] の修正 commit `63aa6dee2` が Alt main に landed: `classification_backend` default を `"joblib"` に revert + `_validate_learning_machine_artifacts` validator 追加。修正者の認識は「joblib artefacts は別配布機構で deploy runner に届いているので即日復旧」だった。 |
| 2026-04-21 (午前) | — | alt-deploy 経由の deploy が走り、新 image + 新設定が runner 反映。`classification_backend=joblib` になり、`learning_machine` validator は短絡 (backend が joblib なので skip)。ここから joblib backend で失敗が表出するフェーズに入ったが、error message が `classification returned 0 results for N articles ...` と PM-035 時代と完全同文のため、時系列でも外見上の変化なし。 |
| 2026-04-21 〜 2026-04-22 | — | Alt main 側は並行して別トピック (release-deploy の CDC 検証 / Ansible 移行 / libtorch SIGSEGV) の火消しに費やしており、本件は「[[PM-2026-033]] の再発か、PM-035 の fix が反映される前の残留か」と誤認しやすい状態で放置。[[PM-2026-035]] §「うまくいかなかったこと」で触れた「同文メッセージによる誤誘導」の 2 回目の発現。 |
| 2026-04-22 (検知) | — | ユーザーから「3days Recap の Job が失敗し続けている。DB やコンテナのログ、Job 履歴を徹底的に分析して、原因を突き止めて」との報告。調査開始。 |
| 2026-04-22 (並列調査) | — | Plan Context Loader + 2 並列 Agent (コード探索 + runtime 状態) を投入。30 分でコンテナ状態・DB 累積失敗件数・subworker traceback (`IsADirectoryError: Is a directory: 'data/genre_classifier.joblib'`) を取得。`docker exec alt-recap-subworker-1 ls -la /app/data/` で全 joblib / json エントリが **サイズ 0 の空ディレクトリ**であることを目視確認。 |
| 2026-04-22 (真因特定) | — | docker file-scoped bind mount の仕様: 「host source file が missing の場合、空ディレクトリを自動作成して mount target に置く」挙動を想起。`recap-subworker/data/` が `.gitignore` 除外 + deploy runner 側で消失 → 個別 file bind の 10 本が全て空ディレクトリで mount されていた。 |
| 2026-04-22 (計画策定) | — | Plan file 作成、ユーザー承認。AskUserQuestion で「Validator only / Validator + baked image / Validator + named volume with init container」の 3 択を提示、ユーザーが **named volume + init container** を選択。 |
| 2026-04-22 (TDD 実装) | — | RED テスト 7 件追加 (`tests/unit/test_classification_backend_validation.py` の `TestJoblibArtifactValidation`) + 2 件 (`tests/services/test_classifier_artifact_guard.py`) = 9 件。`_validate_joblib_artifacts` validator 実装、`classifier.py:75` の `.exists()` → `.is_file()` 多層防御、`compose/base.yaml` に named volume 追加、`compose/recap.yaml` に `recap-subworker-artifacts-init` busybox init service 追加 + `recap-subworker` の 10 本 bind を named volume 1 本に圧縮。 |
| 2026-04-22 (CI parity) | — | `uv run pytest tests/unit tests/services` 302 passed + 12 skipped、`uv run ruff check` / `uv run ruff format --check` 新規コード clean。`docker compose config` で compose ファイル構文検証。 |
| 2026-04-22 (ADR/PM 執筆) | — | [[ADR-000825]] (または連番に従う ADR) と本 PM を執筆。deploy runner 側の host artefact 復旧は user 作業として handoff (alt-deploy Private 領域)。 |

## 検知

- **検知方法:** ユーザー報告 (チャット経由)。「3days Recap の Job が失敗し続けている」。
- **TTD (Time to Detect):** 約 **8 日間 (= 192 時間)**。本 PM シリーズの中で最長。
- **検知の評価:** **検知の穴が 4 段階で重なった silent failure**:
  1. **`/health` smoke だけでは検知不能** (PM-035 と同じ穴)。uvicorn 本体は起動し healthy、分類失敗は実データ処理時のみ発生。
  2. **docker 外形監視も検知不能**。`docker compose ps` は `Up (healthy)` を表示。`alt-recap-subworker-1` は `Up 39 hours (healthy)` を維持し続けていた。
  3. **[[PM-2026-033]] / [[PM-2026-035]] と error message が完全同文** (3 度目)。`classification returned 0 results for N articles (service may be unavailable)` が 3 PM 連続で同じ。過去 PM の教訓 AI #12 (recap-worker のエラー文言を upstream error kind 毎に区別) が未着手のため、識別が困難。
  4. **`recap_failed_tasks` の累積件数がダッシュボード化されていない**。DB には毎日失敗が 1-2 件ずつ積み上がっていたが、Prometheus / Grafana に expose されていないため、外部観測者には見えない。PM-2026-033 AI #11 (recap_failed_tasks.error_message 区分集計を Prometheus exposer で expose) が未着手のためこの穴が残っていた。

### 本来の検知ルート (仮想)

- PM-2026-033 AI #6 (mTLS 経由の実経路 E2E smoke) が完成していれば、毎日の CI run で `/v1/classify-runs` 実データ叩きが走り、起動後数秒で検知できた。未着手だったため届かず。
- PM-2026-035 で追加した `_validate_learning_machine_artifacts` は `classification_backend == "learning_machine"` でないと短絡する設計。joblib default に revert 後は validator 全経路が silent に短絡した。

## 根本原因分析

### 直接原因

`compose/recap.yaml:215-223` の 10 行の file-scoped bind mount:

```yaml
- ../recap-subworker/data/genre_classifier.joblib:/app/data/genre_classifier.joblib:ro
- ../recap-subworker/data/genre_classifier_ja.joblib:/app/data/genre_classifier_ja.joblib:ro
- ../recap-subworker/data/genre_classifier_en.joblib:/app/data/genre_classifier_en.joblib:ro
- ../recap-subworker/data/tfidf_vectorizer.joblib:/app/data/tfidf_vectorizer.joblib:ro
... (他 6 本)
```

host 側の `<alt-deploy>/alt/recap-subworker/data/*.joblib` が 2026-04-14 頃に消失。docker は **「missing host source file → 空ディレクトリを自動生成して mount target に置く」** 挙動を持つため、コンテナ内 `/app/data/genre_classifier.joblib` が「サイズ 0 の空ディレクトリ」として実体化。

`recap_subworker/services/classifier.py:75` の既存ガード:

```python
if not self.model_path.exists():
    raise FileNotFoundError(f"Model not found at {self.model_path}")
self.model = joblib.load(self.model_path)
```

は `Path.exists()` が「空ディレクトリでも True を返す」ため短絡せず、`joblib.load()` が内部で `open(path, "rb")` を呼んで `IsADirectoryError: [Errno 21] Is a directory: 'data/genre_classifier.joblib'` を送出。worker pool 初期化は 300s 経過で timeout、5 チャンク全てで空配列返却、recap-worker 側で `classification returned 0 results for N articles` として fail-fast。

### Five Whys

1. **なぜ 3days Recap が 8 日連続で `classification returned 0 results for N articles` で失敗したか？**
   → recap-subworker 側の classification worker pool が 300s 以内に初期化できず、`/v1/classify-runs` が全チャンク空配列を返していたから。

2. **なぜ classification worker pool が初期化できなかったか？**
   → worker init で `joblib.load(self.model_path)` が `IsADirectoryError` を送出し、pool 全 worker が `init` 段で死んでいたから。

3. **なぜ `joblib.load()` が `IsADirectoryError` を送出したか？**
   → コンテナ内 `/app/data/genre_classifier.joblib` が「ファイルではなく空ディレクトリ」として実体化していたから。`Path.exists()` は True を返すので classifier.py の既存ガードは短絡しなかった。

4. **なぜコンテナ内パスが空ディレクトリだったか？**
   → docker-compose の file-scoped bind mount (`host/file:container/file`) は、host source file が存在しない場合、**silently に空ディレクトリを自動作成してそこに mount** する仕様。`compose/recap.yaml:215-223` が 10 本の file-scoped bind を使っていて、host 側 `<alt-deploy>/alt/recap-subworker/data/*.joblib` が 2026-04-14 に消失 → 10 本全てが空ディレクトリで mount された。

5. **なぜ [[PM-2026-035]] の修正 (2026-04-20) 後もこの失敗が検知されなかったか？**
   → PM-035 の `_validate_learning_machine_artifacts` validator は `classification_backend == "learning_machine"` の場合のみ走る。PM-035 で default を `joblib` に revert した瞬間、joblib 経路には validator が存在しない状態になった。加えて classifier.py の runtime guard は `Path.exists()` ベースで空ディレクトリを通してしまう。この 2 箇所に「joblib 経路の fail-closed」が穴として残っていた。

6. **なぜ [[PM-2026-035]] 時点で joblib 経路にも同等の validator を同時導入しなかったか？** (補足)
   → PM-035 の判断: 「joblib artefacts は別配布機構で deploy runner に届いているので、validator は learning_machine 側だけで十分」。この前提が暗黙であり、「joblib artefacts の配布機構」の実体 (alt-deploy の sync ステップのどれ / どこ) と健全性監視の不在を **docs/runbooks/ 配布経路対応表** に書き起こしていなかった。PM-035 AI #6 (配布経路対応表作成、期限 2026-05-01) がまさにその TODO だったが、期限前に本 PM 発生。

### 根本原因

**「file-scoped bind mount の missing-source 挙動 (空ディレクトリ自動生成) を docker が silent に吸収する仕様 × joblib 経路に fail-closed がない設計 × 配布経路対応表の不在」の三重失敗**:

- **仕様側**: docker-compose の file-scoped bind mount (`host-path:container-path`) において host-path が存在しない場合、docker は warning すら出さずに空ディレクトリを自動生成する。directory-scoped bind (`host-dir:container-dir`) ならエラーで起動失敗するが、file-scoped は silent。これが**空ディレクトリを経由した silent failure の物理的起点**。
- **設計側**: [[PM-2026-035]] で joblib 経路のガードを追加しなかった。`_validate_learning_machine_artifacts` は backend==learning_machine のみ。joblib 側は runtime `Path.exists()` ガードしかなく、空ディレクトリ入力を通してしまう。**PM-035 の修正は「片肺の fail-closed」だった**。
- **運用側**: `.gitignore` で除外されているパスがどこで配布されているかの対応表が `docs/runbooks/` に存在しない。PM-035 AI #6 で 2026-05-01 期限に設定されていたが未着手だった。

3 つが独立して存在している間は互いに救済しあう可能性があったが、2026-04-14 以降はこの救済が成立しなくなった (host artefacts 消失)。

### 寄与要因

- **ラテント 8 日 + エラー文言 3 度目の identical 誤誘導**。`classification returned 0 results for N articles (service may be unavailable)` は [[PM-2026-033]] (mTLS), [[PM-2026-035]] (learning_machine artefacts), そして本 PM (joblib bind-mount empty dir) の 3 root cause で完全一致。[[PM-2026-033]] AI #12 (エラー文言区分) が未着手のまま 2 つの後続 PM で同症状が出た。
- **`Path.exists()` のセマンティクス**。`exists()` は「空ディレクトリでも True」なので model ロード用 guard として誤り。`is_file()` が適切。classifier.py は現状でも `self.tfidf_path.exists()` / `self.svd_path.exists()` / `self.scaler_path.exists()` / `self.thresholds_path.exists()` を使っており、同様の path-type confusion が潜む可能性がある (本 PM では model_path だけ修正、他は optional fallback があるので即問題は起きないが debt として残る)。
- **recap-subworker の `/health` が分類経路の健全性を含まない**。PM-2026-033 / PM-2026-035 で同じ穴が指摘されていたが、healthcheck を分類 smoke まで広げる AI が着手されていなかった。
- **DB メトリクスの欠落**。`recap_failed_tasks` テーブルには 8 日分の失敗が累積していたが、Prometheus にも Grafana にも expose されていない。PM-2026-033 AI #11 が未着手。
- **docker-compose の挙動に対する集団的認知不足**。file-scoped bind mount の missing-source 挙動は docker 公式 doc に明記されているが、チーム内でこの挙動が「silent failure の footgun」として共有されていなかった。directory-scoped bind ならエラーになることとの非対称は、知らないと気付きにくい。

## 対応の評価

### うまくいったこと

- **並列 Agent 調査が高速に決着**。Plan Context Loader + 2 並列 Agent (architecture + runtime state) を同時投入、runtime Agent が `docker exec ls -la /app/data/` と `recap_failed_tasks` 集計と `IsADirectoryError` traceback を 30 分以内に揃えて返してきた。architecture Agent の報告と突き合わせて真因 (docker file-scoped bind の空ディレクトリ自動生成) を確定できた。
- **過去 PM シリーズの参照。** [[PM-2026-035]] / [[PM-2026-033]] / [[PM-2026-031]] / [[PM-2026-032]] の「error message 同文 ≠ 同 root cause」教訓を直接適用。3 度目の同文でも「再発」と短絡せずに DB / コンテナ状態を一次観測から立ち上げられた。
- **ユーザーとの設計選択の対話 (`AskUserQuestion`)**。3 択 (Validator only / Validator + baked image / Validator + named volume with init container) を提示しユーザー選択を得た。結果として named volume + init container を採用し、**docker の file-scoped bind の仕様そのものを迂回する構造**を得た。
- **TDD 厳守の 9 件 RED-then-GREEN**。`TestJoblibArtifactValidation` 7 件 + `TestModelPathGuard` 2 件が全て一度 RED を確認してから実装 commit に進んだ。広範囲 regression (302 unit tests) も zero regression。
- **CI parity local 完走**。`feedback_ci_parity_local.md` に従い `ruff check` / `ruff format --check` / `pytest` / `docker compose config` を手元で通してから handoff。
- **secondary issue (recap-worker SIGSEGV) を scope に引きずり込まなかった**。docs/daily/2026-04-21.md で追跡中の libtorch SIGSEGV は staging only の別件と判定。prod recap-worker は `Up 2 days (healthy)` で無関係。混ぜると diff が荒れて review しにくくなる判断で分離維持。
- **Pydantic V1→V3 互換を崩さず validator 追加**。`@model_validator(mode="after")` が既存 `_validate_learning_machine_artifacts` と同パターンで、validator 層の一貫性維持。

### うまくいかなかったこと

- **TTD が 8 日 (最長)**。過去 PM シリーズの中で最長の silent failure 期間。PM-2026-035 の「手動キック 1 回分」から桁が 2 つ上がっている。PM-035 時点では joblib 経路を疑わずに「学習機械だけ守ればよい」と判断したことが直接的原因。
- **PM-2026-035 の AI #6 (配布経路対応表) が未着手のまま後続 PM を生んだ**。期限 2026-05-01 だが、本 PM (2026-04-22) がそれより先に発生。AI 期限設定が甘かった / AI の優先度が他のトピック (release-deploy / libtorch SIGSEGV) に追い越された。
- **PM-2026-033 AI #11 / #12 / AI #6 の継承 TODO が 3 本 PM 連続で未完了のまま**。AI 継承の優先度上げを PM-035 でも宣言したが、実働に落ちていない。
- **PM-035 で「joblib backend に validator を追加するかどうか」を議論していない**。当時の焦点は「learning_machine 側の fail-closed」で、joblib 側の対称性検討が議題に上がっていない。PM 執筆時に「両 backend の fail-closed 整合」をレビュー項目に含めていれば、この穴は早期に塞げた可能性がある。
- **host artefact 消失の根本原因 (いつ / なぜ / どうやって消えたか) が特定できていない**。alt-deploy は Private リポジトリのため Alt 側からは追跡不可。これは「組織境界をまたぐ調査ができない」問題として残る。

### 運が良かったこと

- **単一ホスト開発環境**。本番マルチテナントであれば 8 日間の Knowledge Home トップ面 stale は可視 UX 劣化に直結した。
- **ユーザーの直接問い合わせ**。外形監視・アラートが全く発火しない状態だったので、ユーザー報告が唯一の検知経路だった。報告がもう数日遅ければ失敗件数がさらに累積していた。
- **local dev 環境には `recap-subworker/data/*.joblib` が残存 (2026-01-31 mtime)**。リポジトリチェックアウト配下の `recap-subworker/data/` で実在を確認済み。これにより「host 側 artefact の source of truth はどのような内容か」のリファレンスが得られ、復旧手順の design に幅を持てた。
- **`.gitignore` による除外が直接の原因ではなかった**。`.gitignore` は OSS 境界維持のために必要な設計。本 PM の穴は「除外されたパスの配布実態を rigorously 監視する仕組みの欠落」であり、`.gitignore` 自体は妥当。

## アクションアイテム

| # | カテゴリ | アクション | 担当 | 期限 | ステータス |
|---|---|---|---|---|---|
| 1 | 予防 | `recap-subworker/recap_subworker/infra/config.py` に `_validate_joblib_artifacts` `@model_validator(mode="after")` を追加。`classification_backend == "joblib"` かつ 7 本の joblib path (`genre_classifier_model_path` / `_ja` / `_en`、`tfidf_vectorizer_path_ja` / `_en`、`genre_thresholds_path_ja` / `_en`) のいずれかが `Path.is_dir()` = True ならば `ValidationError` で起動時 fail-closed | recap チーム | 2026-04-22 | **Done** (本 PR、[[ADR-000825]]) |
| 2 | 予防 | `recap-subworker/recap_subworker/services/classifier.py:75` の既存ガードを `Path.exists()` → `Path.is_file()` に tighten。validator が bypass された場合の多層防御 | recap チーム | 2026-04-22 | **Done** (本 PR) |
| 3 | 予防 | `compose/recap.yaml` の 10 本の file-scoped bind mount を、named volume `recap_subworker_artifacts` + busybox `recap-subworker-artifacts-init` one-shot populator + `service_completed_successfully` gate に置き換え。host ディレクトリに最低 1 個の usable joblib model が無ければ init container が `exit 1` で落ち、recap-subworker は起動自体しない | platform チーム | 2026-04-22 | **Done** (本 PR、`compose/base.yaml` + `compose/recap.yaml`) |
| 4 | プロセス | `<alt-deploy>/alt/recap-subworker/data/` ディレクトリを復旧する。候補: (a) 2026-01-31 mtime の local dev 側 artefacts をコピー (古いが動作する)、(b) `recap-subworker/recap_subworker/learning_machine/` の training pipeline で再生成、(c) 長期的には bind mount → baked image or named volume 配布。**本件 AI は alt-deploy (Private) 領域のためユーザー手動作業**。完了後 `docker compose up -d --force-recreate recap-subworker-artifacts-init recap-subworker` で 3days recap 復旧確認 | user / ops | 2026-04-23 | TODO (本 PR 反映後のブロッキング AI) |
| 5 | プロセス | [[PM-2026-035]] AI #6 を本 PM に継承。`docs/runbooks/` に「`.gitignore` で除外されているパスの配布経路対応表」を新規作成。最低限: `recap-subworker/data/*.joblib` / `recap-subworker/recap_subworker/learning_machine/artifacts/` / その他類似パスについて、OSS 側で除外されていることと、Private 配布機構 (alt-deploy) での配布有無・機構種別・健全性監視の有無を表形式で明記。**期限を 2026-05-01 → 2026-04-30 に前倒し**、PM-035 からの 2 回目の継承として優先度 P1 | docs / platform | 2026-04-30 | TODO (PM-035 から継承、期限前倒し) |
| 6 | 検知 | [[PM-2026-033]] AI #6 を 3 PM 連続で継承。`/v1/classify-runs` を実データで叩く smoke test を e2e suite (Hurl `e2e/hurl/recap-worker/06-trigger-and-poll-3days.hurl` の拡張 or 新規 smoke) に追加し、起動後数秒で artefact 欠落を検知できるようにする。**期限 2026-05-15 のまま、ただし担当を platform から recap チームに変更して優先度上げ** | recap チーム | 2026-05-15 | TODO (PM-033, PM-035 から継承、3 度目) |
| 7 | 検知 | [[PM-2026-033]] AI #11 を 3 PM 連続で継承。`recap_failed_tasks.error_message` を Prometheus exporter で expose (区分ラベル: `classification_0_results` / `dispatch_timeout` / `persist_error` etc)。Grafana で window_days 毎の時系列で可視化。**期限 2026-05-15 のまま、担当 observability** | observability | 2026-05-15 | TODO (PM-033, PM-035 から継承、3 度目) |
| 8 | 検知 | [[PM-2026-033]] AI #12 を 3 PM 連続で継承。recap-worker 側の `classification returned 0 results for N articles (service may be unavailable)` エラー文言を、upstream error 種別ごとに区別 (upstream_service_error / upstream_http_error / upstream_timeout / classification_worker_init_failure / classification_empty_result)。**期限 2026-05-15 のまま、3 PM 連続同症状への対策として最優先** | recap チーム | 2026-05-15 | TODO (PM-033, PM-035 から継承、3 度目) |
| 9 | 予防 | recap-subworker の healthcheck を `/health` (uvicorn liveness) から `/health/ready` (classifier worker pool init 完了を含む readiness) に変更する検討。`classify-runs` を実データで叩くと無視できないコストがあるため、軽量な「classifier が 1 dummy 入力を processingable か」判定を `/health/ready` に実装する案 | recap チーム | 2026-05-31 | TODO |
| 10 | プロセス | **docker-compose の file-scoped bind mount 使用 policy を `docs/runbooks/compose-bind-mount-policy.md` として明文化**。原則「directory-scoped bind か named volume のみ許可」「file-scoped bind は特別な理由がある場合のみ、かつ init container などで source の事前検証を義務化」を宣言。既存 compose ファイルの file-scoped bind を grep で列挙して見直す audit も同時実施 | platform | 2026-05-31 | TODO (本 PM で初登場、他の service にも潜む可能性が高い) |
| 11 | 予防 | classifier.py 内の他の optional artefact load (`self.tfidf_path.exists()` / `self.svd_path.exists()` / `self.scaler_path.exists()` / `self.thresholds_path.exists()`) を `.is_file()` に一括 tighten する debt 処理。本 PR では model_path のみ修正 (即時 fatal なのは model のみで他は warn 経路のため) | recap チーム | 2026-05-15 | TODO |
| 12 | 予防 | PM-035 の AI #8 (「他バックエンドにも同型の silent failure audit」) を本 PM で再確認。`model_backend='ollama-remote'` without `OLLAMA_EMBED_URL`、`graph_build_enabled=true` without graph store、他の external dependency 系を一覧化して `@model_validator` で fail-closed 化 | recap チーム | 2026-05-30 | TODO (PM-035 から継承、未着手) |

## 教訓

### 技術面

- **docker-compose file-scoped bind mount は missing-source を silent に空ディレクトリ化する**。この挙動は docker 公式 doc に明記されているが、チーム内で「silent failure の footgun」として共有されていなかった。本 PM を起点に `docs/runbooks/compose-bind-mount-policy.md` で明文化する。代替策は (a) directory-scoped bind (`host-dir:container-dir`) にする、(b) named volume + init container で populate する、(c) baked image で artefact を COPY する。(a) は「missing なら起動失敗」で fail-fast。(b)(c) は「配布実態と compose の乖離をチェックする一次関門」を追加できる。本 PM では (b) を採用。
- **`Path.exists()` は model ロード用 guard には不適切**。空ディレクトリでも True を返すため、`joblib.load()` / `open()` の前段ガードとしては `is_file()` を使う。本 PM で `classifier.py:75` を修正。他の同型 guard (tfidf / svd / scaler / thresholds) も AI #11 で一括 tighten。
- **Pydantic `@model_validator(mode="after")` は「起動時 fail-closed」の第一候補**。lazy init を避け、artefact 欠落を container start で検知する設計。[[ADR-000727]] で `validate_mtls_url_schemes` 導入、[[ADR-000811]] で `_validate_learning_machine_artifacts` 追加、本 PM で `_validate_joblib_artifacts` 追加 — 同パターンの 3 例目。設定層で守れる invariant はすべて validator で守る。
- **Named volume + init container は「file-scoped bind の silent failure 回避パターン」として普遍性が高い**。busybox の `cp -r /src/. /dst/` + `test -s` で source の事前検証を挟むだけで、host ディレクトリの健全性を gateway 化できる。他 service でも類似パターンの導入を検討する AI #10 を設定。

### 組織面

- **AI の継承は期限前倒しで、3 回目は最優先扱い**。PM-2026-033 で設定された AI #6 / #11 / #12 は PM-2026-035 で 1 回継承、本 PM で 2 回継承。**同じ AI が 3 PM 連続で未着手のまま次の PM を生む**事態は、AI 期限設定 / 優先度 / 担当割り当てに制度的な穴があることを示す。次の PM で同 AI が再登場したら、**全体の作業停止してでも着手**する運用ルールを検討する。
- **「片肺の fail-closed」は silent failure の再発を招く**。PM-2026-035 で learning_machine 経路に validator を追加したが、joblib 経路は穴のまま放置された。設定 validator を追加するときは「backend の全選択肢で対称な fail-closed を入れる」を PR チェック項目にする。本 PM の `_validate_joblib_artifacts` + `_validate_learning_machine_artifacts` の 2 本立てが対称性を満たす。
- **PM 執筆時に「前 PM の残タスクが本 PM の root cause に寄与していないか」を必ずチェック**する。本 PM は PM-2026-035 の「joblib 経路 validator が無い」+「配布経路対応表 AI が未着手」が直接的な寄与要因。PM テンプレートに「継承 AI の impact assessment」セクション追加を検討 (AI #10 の次候補)。
- **Private 領域 (alt-deploy) の状態変化を Alt 側から観測する仕組みが無いことが、本 PM の 8 日間 TTD の構造的原因**。組織境界をまたぐ監視は難しいが、最低限 deploy runner 側に「artefact 存在チェック cron」を置く選択肢はある。AI #5 の配布経路対応表の中で同時に議論する。
- **error message の完全一致が 3 PM 連続で生じた**。ユーザー報告と復旧担当者の初手が「前 PM の再発」と誤誘導されるリスクが 3 連続で実在した。AI #8 (エラー文言区分) を最優先で着手。

## 参考資料

### 本 PM の修正

- [[ADR-000825]] 本 PM で決定した設計変更 (joblib validator + named volume + init container)
- `recap-subworker/recap_subworker/infra/config.py` — `_validate_joblib_artifacts` `@model_validator(mode="after")` 追加
- `recap-subworker/recap_subworker/services/classifier.py:75` — `.exists()` → `.is_file()` ガード tighten
- `recap-subworker/tests/unit/test_classification_backend_validation.py` — `TestJoblibArtifactValidation` 7 件追加
- `recap-subworker/tests/services/test_classifier_artifact_guard.py` — `TestModelPathGuard` 2 件新規
- `compose/base.yaml` — named volume `recap_subworker_artifacts` 追加
- `compose/recap.yaml` — `recap-subworker-artifacts-init` busybox init service 追加、`recap-subworker` の 10 本 file-scoped bind を named volume 1 本に圧縮、`depends_on: recap-subworker-artifacts-init: condition: service_completed_successfully` 追加

### 関連 PM / ADR

- [[PM-2026-035]] recap-subworker learning_machine artifacts 欠落で 3days Recap が 948 件分 classification 失敗 — 本 PM の直系前 PM、片肺の fail-closed を残した直接の寄与要因
- [[PM-2026-033]] recap-subworker / news-creator mTLS サーバ側未対応で 3days Recap が 5 日連続失敗 — error message 3 連続同文の起点
- [[PM-2026-032]] mTLS client cert stale in-memory で 3days Recap 失敗
- [[PM-2026-031]] mTLS cutover 残タスクで acolyte 502 / 3days Recap 404
- [[ADR-000811]] classification_backend default を joblib に revert + learning_machine validator 追加 — 本 PM で joblib 側にも同パターン適用
- [[ADR-000774]] recap-worker 下流 pki-agent reverse-proxy mTLS サーバ化 — validator 前史 (`validate_mtls_url_schemes`)
- [[ADR-000727]] mTLS Phase 2 client-side enforcement — validator パターンの導入元

### 観測証跡

- DB snapshot (runtime Agent より):
  ```
   window_days |  status   | count
  -------------+-----------+-------
             1 | completed |     1
             1 | failed    |    11
             3 | completed |    14
             3 | failed    |    23
  ```
- 最後の成功 3day job: `137a4a66-...` / kicked_at = `2026-04-13 17:56:36 UTC`
- 最後の失敗 3day job 代表: `658a22e4-...` / kicked_at = `2026-04-21 17:00:05 UTC` / error = `classification returned 0 results for 994 articles`
- コンテナ状態: `alt-recap-subworker-1 Up 39 hours (healthy)` / `alt-recap-worker-1 Up 2 days (healthy)`
- コンテナ内 `/app/data/` のエントリが全て `drwxr-xr-x 0 root root 0 Apr 20 13:15 <name>/` (空ディレクトリ)
- subworker traceback: `IsADirectoryError: [Errno 21] Is a directory: 'data/genre_classifier.joblib'` at `classifier.py:79` (pre-fix) → `RuntimeError: Failed to initialize classification worker pool within 300s`
- host path (deploy runner): `<alt-deploy>/alt/recap-subworker/data/genre_classifier.joblib` — **missing**

### 外部資料

- docker-compose bind mount spec: file-scoped bind の missing-source 挙動 (公式 doc)
- PyTorch `joblib.load()` 実装: `open(path, "rb")` を呼ぶので path が directory だと `IsADirectoryError`

---

> **Blameless Postmortem の原則:** このドキュメントは個人の過失を追及するためではなく、
> システムの脆弱性とプロセスの改善機会を特定するために作成されています。
> 「誰が悪いか」ではなく「システムのどこが改善できるか」に焦点を当ててください。
> 特に本 PM では、[[PM-2026-035]] の修正時に「joblib 経路にも同等の fail-closed validator を対称に入れる」
> というレビュー項目が仕組み化されていなかったことを「前 PM 担当者の見落とし」ではなく、
> **「backend の全選択肢で fail-closed 対称性を担保する PR レビュー機構が組織にまだ存在しなかった」** 穴として扱っています。
> 同じ穴は AI #1 + AI #10 (compose bind-mount policy) + AI #12 (他 backend audit) で塞ぐべきです。
> 加えて、docker-compose file-scoped bind の silent 挙動自体はツール側の仕様なので、
> 「この仕様を知らなかった担当者のミス」ではなく「ツール挙動を runbook で明文化していなかった組織の穴」として扱います。
