---
title: 3-day Recap ジョブ復旧 — recap-subworker artefact 配置と CI unblock 手順
date: 2026-04-22
tags:
  - runbooks
  - recap
  - recap-subworker
  - recap-worker
  - ci-cd
  - deploy
---

# 3-day Recap ジョブ復旧手順

[[PM-2026-036]] / [[000825]] の後処理。Alt main に 3-day recap の fail-closed 修正 (`sha 192bea225`) が landed した状態から、本番反映までの運用手順を 2 つのブロッカー込みで書き出す。

## 前提と現状

- **Alt main**: `473b98251` まで前進済み。recap-subworker に Settings validator + classifier `is_file()` guard を投入、compose は direct bind パターン (`/var/lib/alt-recap-subworker-data:/app/data:ro`) に改修済み。
- **alt-deploy release-deploy**: stat+assert 化で sudo 障壁は解消済み。
- **prod host の artefact**: `/var/lib/alt-recap-subworker-data/` 配下に joblib / json が必要。不在なら compose v2.24+ が container create を refuse して deploy job が fail する (これが設計どおりの fail-closed)。

この 2 つを解決して初めて 3-day recap が生きる。

## 関連

- [[000825]] recap-subworker の joblib artefact 欠落を Pydantic validator と named-volume-with-init-container で 2 層 fail-closed にする
- [[PM-2026-036]] recap-subworker joblib artefacts bind-mount が空ディレクトリ化し、3days / 1day Recap が 8 日間 silent に失敗し続けた
- [[000811]] / [[PM-2026-035]] — 対称な learning_machine validator の先例
- [[runner-setup]] self-hosted runner bootstrap の一般方針

## ブロッカー 1: `/opt/rustbert-cache` の passwordless sudo 依存

### 症状

release-deploy の `e2e (recap-worker)` job が以下で fail。

```
TASK [Ensure /opt/rustbert-cache exists with recap UID ownership]
fatal: [localhost]: FAILED!
    changed: false
    module_stderr: |-
        sudo: a password is required
    rc: 1
```

### 原因

recap-worker の staging 経路で `/opt/rustbert-cache` を host bind-mount する必要があるが、各 self-hosted runner の actions-runner ユーザは NOPASSWD sudo を持たない方針。CI step から privileged provisioning を試みると sudo プロンプトで停止する。**privileged provisioning は CI ではなく runner bootstrap で事前に完了させる** のが Alt の設計原則 ([[runner-setup]] §2.5 と同方針)。

### 復旧手順

`/opt/rustbert-cache` は **self-hosted deploy runner host 全体で共有される単一ディレクトリ**で、recap-worker container 内 `recap` user (uid/gid `999:999`, Dockerfile で pin 済) が読み取る。復旧は 2 段階:

1. **ディレクトリ provisioning** — mkdir + chown 999:999 + chmod 0755 を sudo で実施。
2. **rust-bert AllMiniLmL12V2 model cache を populate** — tokenizer + model weights (約 130 MB) を deploy runner 上で一度だけダウンロード。これをスキップすると container 起動後に `Read-only file system (os error 30)` で embedding init が失敗し、subgenre-splitter が keyword-only fallback に落ちて 30 ジャンル taxonomy が 2 バケットに崩壊する ([[PM-2026-038]])。

具体的なパス・secrets 取り扱い・ワンライナー・冪等化・自動化は本 public repo ではなく alt-deploy (Private) の運用ドライバで管理する ([[feedback_no_host_names_in_public]] 準拠)。Alt 側から触るべき API は以下 2 点だけ:

- `alt-deploy/scripts/recover-3days-recap.sh provision-cache --yes` — ステップ 1 を冪等に実施。
- `alt-deploy/scripts/recover-3days-recap.sh populate-cache --yes` — ステップ 2 で現行 image の `recap-worker warmup` subcommand を `rw` bind で実行し cache を populate。

両者完了後に `verify-cache` sub-command で `uid=999 gid=999 mode=755` かつ `/opt/rustbert-cache` 配下に non-zero な `.ot` / `.json` が populate 済であることを確認する。

### 検証 (Alt 側 smoke)

Alt 側の smoke は compose 起動後に以下で成立する:

- `docker logs alt-recap-worker-1 --since 2m | grep 'Embedding service initialized successfully'` が 1 行以上出ること
- `RECAP_WORKER_EMBEDDING_REQUIRED=true` (compose デフォルト) で container が `healthy` を維持していること。populate 未完了なら `EmbeddingService::new()` が Err を返し `ComponentRegistry::build` が context 付きで bail、container が restart ループに入る (fail-closed / [[000827]])

### follow-up

- [[000827]] で recap-worker に `RECAP_WORKER_EMBEDDING_REQUIRED` フラグを追加し init 失敗時 fail-closed を実装。compose デフォルトは `true`。dev stack で populate を持たない環境は `.env` で `RECAP_WORKER_EMBEDDING_REQUIRED=false` を override すれば従来の degraded mode で起動可能。
- alt-deploy 側 runbook に `populate-cache` の運用詳細 (image sha の引き方、secrets マウント、ssh 経由の実行) を集約。Alt 側からは参照のみ。

## ブロッカー 2: recap-subworker host artefact の復旧

### 症状

deploy で recap-subworker を bring-up するとき、compose v2.24+ は directory-scoped bind mount の host source が不在だと container create を **refuse** する。つまり host 上の `/var/lib/alt-recap-subworker-data/` が無い状態で deploy が走ると、`docker compose up recap-subworker` が即 fail し、deploy job も赤になる。

もう 1 つの失敗形: host path は存在するが `*.joblib` / `*.json` が無い or 空ディレクトリ。この場合 container は起動するが classifier 初期化で `FileNotFoundError` or `IsADirectoryError` を投げ、recap-worker 側で `classification returned 0 results for N articles` として fail。Settings validator と classifier.py の `is_file()` guard が多層防御。

### 復旧手順 (prod host 上で直接)

Alt は single-machine 構成なので、`alt-prod` 役の self-hosted runner が走るホスト = 実際の prod ホスト。artefact は **deploy workspace (ephemeral) ではなく、そのホストの `/var/lib/alt-recap-subworker-data/` に直接**配置する。設計の背景と代替案の評価は [[000825]] addendum を参照。選択肢:

#### 選択肢 A: 既知動作状態の snapshot を運用チーム管理ストレージから再配置

内部運用文書に従い、過去の snapshot (2026-04-13 時点の tarball 等) を `/var/lib/alt-recap-subworker-data/` に展開。**本番と同系譜のため推奨**。

#### 選択肢 B: training pipeline で再生成

`recap-subworker/recap_subworker/learning_machine/` の training パイプラインを実行して joblib を再生成する。時間がかかる (数時間オーダー) が再現性が高い。手順の詳細は recap-subworker 側 README を参照。

#### 選択肢 C: local dev 環境の snapshot (2026-01-31 mtime) を緊急用に流用

開発端末側には 2026-01-31 時点の `recap-subworker/data/*.joblib` が残っている可能性あり。古いが動作はする、緊急用の応急処置。運用に投入する際はバージョン差分のリスクを把握したうえで。

#### 配置ワンライナー (prod host でログイン済前提)

```bash
cd <repo root> && tar -czf /tmp/recap-subworker-data.tar.gz -C recap-subworker data
```

```bash
sudo sh -c 'mkdir -p /var/lib/alt-recap-subworker-data && tar -xzf /tmp/recap-subworker-data.tar.gz -C /var/lib/alt-recap-subworker-data --strip-components=1 && chown -R 999:999 /var/lib/alt-recap-subworker-data && chmod -R u=rwX,go-rwx /var/lib/alt-recap-subworker-data'
```

### 検証

ホスト側:

```bash
ls -la /var/lib/alt-recap-subworker-data/ | grep -E 'genre_classifier|tfidf_vectorizer|genre_thresholds|golden_classification'
```

最低限以下が **通常ファイル (非ゼロサイズ)** として並ぶこと:

- `genre_classifier.joblib` (deprecated 互換用)
- `genre_classifier_ja.joblib`
- `genre_classifier_en.joblib`
- `tfidf_vectorizer.joblib` / `_ja.joblib` / `_en.joblib`
- `genre_thresholds.json` / `_ja.json` / `_en.json`
- `golden_classification.json`

classifier は `_ja` / `_en` 両方あるのが想定。片方だけでも起動はするが classification の片言語が static に空返しになる。

## デプロイ実行

両ブロッカー解消後、alt-deploy の最新 workflow run を再実行する (または新規 dispatch)。

### 手順

```
alt-deploy の Actions タブ → release-deploy workflow → Re-run all jobs
```

または `gh workflow run release-deploy.yaml -R Kaikei-e/alt-deploy` を自分の端末から明示的に叩く。`feedback_no_auto_push` / `feedback_no_auto_run_commands` 準拠で、この操作は **明示承認後に運用者が実行**。

### 成功判定

```
gh run view <run-id> -R Kaikei-e/alt-deploy
```

全 job が ✓ で、最後に `deploy` が成功していること。特に:

- `e2e (recap-worker)` — ブロッカー 1 解消後は `Ensure /opt/rustbert-cache ...` タスクが `changed=false` で通過
- `e2e (recap-subworker)` — 現状は hurl suite 不在で short-circuit (exercise されていない)。将来 suite 整備後はここで compose が real artefact を要求するため、staging 側は stub (`recap-pipeline-stub`) 経由で回避を続ける
- `deploy` — 各サービスの ansible docker_compose_v2 roll が per-service で完了

## デプロイ後の検証 (本番)

```
curl -X POST http://<recap-worker host>/v1/generate/recaps/3days \
  -H 'Content-Type: application/json' -d '{"genres":["ai"]}'
```

202 が返ったら DB を覗く:

```sql
SELECT job_id, status, last_stage, window_days, kicked_at
FROM recap_jobs
WHERE window_days = 3
ORDER BY kicked_at DESC LIMIT 5;
```

新しい `status='completed'` 行が出れば OK。`status='failed'` で `last_stage='dedup'` が続くなら `recap_failed_tasks` を確認:

```sql
SELECT job_id, stage, substr(error, 1, 200) AS error_head
FROM recap_failed_tasks
WHERE created_at > NOW() - INTERVAL '10 minutes'
ORDER BY created_at DESC;
```

### `classification returned 0 results for N articles` が再発した場合

今回の ADR の Settings validator + classifier `is_file()` guard + compose directory-scoped bind (v2.24+ missing-source-refuse) は「artefact が dir 型 / 欠落 / mount が silent に empty 化」の silent failure を塞ぐためのもの。再発時の可能性は:

| 症状 | 可能性の高い原因 | 確認コマンド |
|---|---|---|
| compose up が "bind source path does not exist" で failed | host `/var/lib/alt-recap-subworker-data/` 不在 | `ls -la /var/lib/alt-recap-subworker-data/` |
| recap-subworker が起動しない (validator panic) | `RECAP_SUBWORKER_GENRE_CLASSIFIER_MODEL_PATH_*` env が dir を指している | `docker compose logs recap-subworker` の冒頭に ValidationError |
| recap-subworker 起動するが classify-runs タイムアウト | recap-worker 側 dispatch の real-timeout (LLM 側の issue 等) | recap-worker の `recap_failed_tasks.error` |
| 上記以外で `classification returned 0 results ...` 復活 | [[PM-2026-033]] 系の mTLS scheme drift 再発の可能性 | recap-worker / pki-agent-recap-subworker のログ照合 |

## ロールバック

名前空間化した変更なので、**名前空間単位で revert できる**。

### 最小ロールバック (validator だけ戻す)

`192bea225` の commit の中で、`recap-subworker/recap_subworker/infra/config.py` の `_validate_joblib_artifacts` だけを revert すれば起動時 fail-closed が消える。compose 側の direct bind は維持。**非推奨**: silent failure に戻るだけで問題は解消しない。

### compose だけロールバック (direct bind を file-scoped bind に戻す)

`473b98251` (direct bind 移行) → `6931f7c8a` (env-override host path) → `96a2edc81` (init container) のいずれか 1 段階だけを revert することで中間状態に戻れる。いずれの中間形でも `is_file()` guard は残るので worst case でも silent failure より早く露呈する。

### 全部ロールバック

`87af119f1..473b98251` の一連 (test RED → fix GREEN → compose refactor → docs → env override → direct bind) を逆順で revert するか、まとめて reset。**非推奨**: [[PM-2026-036]] の silent failure に戻る。

## 再発防止チェックリスト

[[PM-2026-036]] の AI を optional chain として以下の順で着手:

- [ ] **AI #4 (本 runbook ブロッカー 2)**: deploy runner の `recap-subworker/data/` 復旧
- [ ] **AI #5 (期限 2026-04-30)**: `.gitignore` で除外されているパスの配布経路対応表を `docs/runbooks/distribution-paths.md` に新規作成
- [ ] **AI #6 (期限 2026-05-15)**: `/v1/classify-runs` 実データ smoke を `e2e/hurl/recap-subworker/` として新設 (現状 short-circuit)
- [ ] **AI #7 (期限 2026-05-15)**: `recap_failed_tasks.error_message` の区分集計を Prometheus exporter で expose
- [ ] **AI #8 (期限 2026-05-15)**: recap-worker 側の `classification returned 0 results ...` エラーを upstream error kind 毎に区別 (PM-033/035/036 で 3 PM 連続同文)
- [ ] **AI #10 (期限 2026-05-31)**: `docs/runbooks/compose-bind-mount-policy.md` で file-scoped bind 使用 policy を明文化
- [ ] **AI #11 (期限 2026-05-15)**: classifier.py の optional artefact 全 path (`tfidf` / `svd` / `scaler` / `thresholds`) を `.is_file()` に tighten

## 緊急時連絡・エスカレーション

- recap 関連の緊急事態は recap / platform チームへ
- pact broker の異常は [[pact-broker-ops]] を先に参照
- deploy pipeline の異常は [[deploy]] を先に参照
- runner 側の問題は [[runner-setup]] を先に参照
