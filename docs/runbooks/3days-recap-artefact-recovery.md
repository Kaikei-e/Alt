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

- **Alt main**: `192bea225` まで前進済み。recap-subworker に validator + named volume + init container を投入済み。
- **alt-deploy release-deploy**: 最新 run で `e2e (recap-worker)` が `sudo: a password is required` で fail し deploy 未実行。
- **recap-subworker 側の host artefact**: `<alt-deploy checkout>/alt/recap-subworker/data/*.joblib` 群が 2026-04-14 頃から欠落中 (本 runbook を書く時点でも未復旧、ただし named volume 化によって recap-subworker 起動は `recap-subworker-artifacts-init` で fail-fast するようになる)。

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

Alt `329e6dcad ci(e2e-hurl): warmup rust-bert cache on host before staging stack` に対応する alt-deploy 側 playbook が `become: true` で `/opt/rustbert-cache` を mkdir + chown しようとする。各 self-hosted runner の actions-runner ユーザは NOPASSWD sudo を持たない方針なので失敗する。**privileged provisioning は CI step ではなく runner bootstrap で事前に完了させる** のが Alt の設計原則 ([[runner-setup]] と同方針)。

### 復旧手順 (self-hosted deploy runner ホスト)

`/opt/rustbert-cache` は **host 全体で共有される単一ディレクトリ** (各 actions-runner ユーザ固有のパスではない)。runner host に sudo 可能なアカウントでログインし、以下のワンライナーを一度流すだけで足りる。`feedback_shell_paste_safe` 準拠。

**重要**: recap-worker Dockerfile で `recap` user の uid/gid は `999:999` に pin 済み (`d7153ad36`)。ホスト側 `/opt/rustbert-cache` は同 uid:gid で所有させる。

```bash
sudo mkdir -p /opt/rustbert-cache && sudo chown 999:999 /opt/rustbert-cache && sudo chmod 0755 /opt/rustbert-cache
```

冪等なので複数回流しても副作用なし。自動化版は alt-deploy (Private) 側の `scripts/recover-3days-recap.sh provision-cache --yes` を使う (ssh 越しの実行なら `RUNNER_HOST=<host> ./scripts/recover-3days-recap.sh provision-cache --yes`)。

### 検証

```bash
stat -c '%n uid=%u gid=%g mode=%a' /opt/rustbert-cache
```

`uid=999 gid=999 mode=755` と出れば OK。alt-deploy 側の `scripts/recover-3days-recap.sh verify-cache` も同等。

### follow-up (Alt / alt-deploy 側)

- alt-deploy `run-e2e-suite.yml` の該当タスクを `become: true` 依存から「存在確認 (state=directory, creates=skip existing)」のみに緩和し、provisioning 責務を `setup-runner.yml` 側に寄せる。
- [[runner-setup]] を更新し「`/opt/rustbert-cache` を 999:999 で事前作成」を Phase 1 bootstrap の checklist に追加する。

## ブロッカー 2: recap-subworker host artefact の復旧

### 症状

ブロッカー 1 を解消し deploy が通った直後、recap-subworker は新 compose (named volume + init container) で起動を試みるが、init container が以下で exit 1:

```
FATAL: no usable joblib model on the host
  checked: /src/genre_classifier_ja.joblib and /src/genre_classifier_en.joblib
  both are missing or zero-sized.
```

これは [[000825]] の設計どおり。silent 失敗していた 8 日間の潜伏を起動 1 秒以内の明示的 fail に置き換えた結果。

### 復旧手順 (deploy runner ホスト)

`<alt-deploy checkout>/alt/recap-subworker/data/` 配下に必要な artefact を再配置する。選択肢:

#### 選択肢 A: 既知動作状態の snapshot を運用チーム管理ストレージから再配置

内部運用文書に従い、過去の deploy runner snapshot (2026-04-13 時点の recap-subworker/data/ tarball 等) を `<alt-deploy checkout>/alt/recap-subworker/data/` に展開。**本番と同系譜のため推奨**。

#### 選択肢 B: training pipeline で再生成

`recap-subworker/recap_subworker/learning_machine/` の training パイプラインを実行して joblib を再生成する。時間がかかる (数時間オーダー) が再現性が高い。手順の詳細は recap-subworker 側 README を参照。

#### 選択肢 C: local dev 環境の snapshot (2026-01-31 mtime) を緊急用に流用

開発端末側には 2026-01-31 時点の `recap-subworker/data/*.joblib` が残っている可能性あり。古いが動作はする、緊急用の応急処置。運用に投入する際はバージョン差分のリスクを把握したうえで。

### 検証

ホスト側:

```bash
ls -la <alt-deploy checkout>/alt/recap-subworker/data/ | grep -E 'genre_classifier|tfidf_vectorizer|genre_thresholds|golden_classification'
```

最低限以下が **通常ファイル (非ゼロサイズ)** として並ぶこと:

- `genre_classifier.joblib` (deprecated 互換用)
- `genre_classifier_ja.joblib`
- `genre_classifier_en.joblib`
- `tfidf_vectorizer.joblib` / `_ja.joblib` / `_en.joblib`
- `genre_thresholds.json` / `_ja.json` / `_en.json`
- `golden_classification.json`

init container は `genre_classifier_ja.joblib` か `genre_classifier_en.joblib` のどちらか 1 つでも存在すれば通過するが、classifier 側は `_ja` / `_en` 両方あるのが想定。

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
- `e2e (recap-subworker)` — 現状は hurl suite 不在で short-circuit (exercise されていない)。将来 suite 整備後にここで init container が exercise される
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

今回の ADR の validator + init container は「artefact が dir 型 / 欠落」の silent failure を塞ぐためのもの。再発時の可能性は:

| 症状 | 可能性の高い原因 | 確認コマンド |
|---|---|---|
| recap-subworker コンテナが「Exited (1)」で上がらない | init container が exit 1 (artefact 欠落) | `docker compose logs recap-subworker-artifacts-init` |
| recap-subworker が起動しない (validator panic) | `RECAP_SUBWORKER_GENRE_CLASSIFIER_MODEL_PATH_*` env が dir を指している | `docker compose logs recap-subworker` の冒頭に ValidationError |
| recap-subworker 起動するが classify-runs タイムアウト | recap-worker 側 dispatch の real-timeout (LLM 側の issue 等) | recap-worker の `recap_failed_tasks.error` |
| 上記以外で `classification returned 0 results ...` 復活 | [[PM-2026-033]] 系の mTLS scheme drift 再発の可能性 | recap-worker / pki-agent-recap-subworker のログ照合 |

## ロールバック

名前空間化した変更なので、**名前空間単位で revert できる**。

### 最小ロールバック (validator だけ戻す)

`192bea225` の commit の中で、`recap-subworker/recap_subworker/infra/config.py` の `_validate_joblib_artifacts` だけを revert すれば起動時 fail-closed が消える。compose 側の名前空間 (named volume + init container) は維持。**非推奨**: silent failure に戻るだけで問題は解消しない。

### compose だけロールバック (named volume を file-scoped bind に戻す)

`96a2edc81` commit 単体を revert すれば、file-scoped bind + classifier.py の `is_file()` guard だけが残る構成に戻る。classifier.py の runtime guard が `FileNotFoundError` を投げるので worst case でも silent failure より早く露呈する。

### 全部ロールバック

`192bea225 → 96a2edc81 → 3b791ada2 → 87af119f1` を逆順で revert、またはまとめて `git revert 87af119f1..192bea225` 相当の reset。**非推奨**: [[PM-2026-036]] の silent failure に戻る。

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
