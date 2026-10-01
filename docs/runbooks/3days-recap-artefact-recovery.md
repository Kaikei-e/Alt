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

recap-worker (`/opt/rustbert-cache`) および recap-subworker (`/var/lib/alt-recap-subworker-data`) のホスト側 artefact 欠落・権限不整合によるコンテナ起動停止・ジョブ失敗からの復旧手順。

## 前提と現状

- **recap-worker**: `/opt/rustbert-cache` に tokenizer + weights が populate されていない場合、`RECAP_WORKER_EMBEDDING_REQUIRED=true` により起動時 fail-closed で終了する ([[000827]])。
- **recap-subworker**: `/var/lib/alt-recap-subworker-data/` に joblib / json が不在の場合、compose の `create_host_path: false` により container create が refuse される ([[000825]])。また、Learning Machine モデル用の `/var/lib/alt-recap-subworker-artifacts/` も同様の契約を持つ。

## 関連

- [[000825]] recap-subworker の joblib artefact 欠落を Pydantic validator と named-volume-with-init-container で 2 層 fail-closed にする
- [[PM-2026-036-recap-subworker-joblib-bind-mount-empty-directory-8day-outage|PM-2026-036]] recap-subworker joblib artefacts bind-mount が空ディレクトリ化し、3days / 1day Recap が 8 日間 silent に失敗し続けた
- [[000811]] / [[PM-2026-035-recap-subworker-learning-machine-artifacts-missing|PM-2026-035]] — 対称な learning_machine validator の先例
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
2. **rust-bert AllMiniLmL12V2 model cache を populate** — tokenizer + model weights (約 130 MB) を deploy runner 上で一度だけダウンロード。これをスキップすると container 起動後に `Read-only file system (os error 30)` で embedding init が失敗し、subgenre-splitter が keyword-only fallback に落ちて 30 ジャンル taxonomy が 2 バケットに崩壊する ([[PM-2026-038-recap-worker-rustbert-cache-empty-silent-keyword-only-fallback|PM-2026-038]])。

具体的なパス・secrets 取り扱い・ワンライナー・冪等化・自動化はプライベートデプロイリポジトリの運用スクリプトで管理する。Alt 側から触るべき手順は以下 2 点:

- プライベートデプロイリポジトリ側の cache provisioning 手順（mkdir + chown 999:999 + chmod 0755）を冪等に実施。
- 同 populate 手順で現行 image の `recap-worker warmup` を rw bind で実行し cache を populate。

両者完了後に `uid=999 gid=999 mode=755` かつ `/opt/rustbert-cache` 配下に non-zero な `.ot` / `.json` が populate 済であることを確認する。

### 検証 (Alt 側 smoke)

Alt 側の smoke は compose 起動後に以下で成立する:

- `docker logs alt-recap-worker-1 --since 2m | grep 'Embedding service initialized successfully'` が 1 行以上出ること
- `RECAP_WORKER_EMBEDDING_REQUIRED=true` (compose デフォルト) で container が `healthy` を維持していること。populate 未完了なら `EmbeddingService::new()` が Err を返し `ComponentRegistry::build` が context 付きで bail、container が restart ループに入る (fail-closed / [[000827]])

### follow-up

- [[000827]] で recap-worker に `RECAP_WORKER_EMBEDDING_REQUIRED` フラグを追加し init 失敗時 fail-closed を実装。compose デフォルトは `true`。dev stack で populate を持たない環境は `.env` で `RECAP_WORKER_EMBEDDING_REQUIRED=false` を override すれば従来の degraded mode で起動可能。
- プライベートデプロイリポジトリ側の運用手順に `populate-cache` の運用詳細 (image sha の引き方、secrets マウント、ssh 経由の実行) を集約。Alt 側からは参照のみ。
- PM-2026-036 AI #5 の「.gitignore 除外パスの配布経路対応表 (distribution-paths)」作成タスクは未完了のまま残存 (backlog)。

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

recap-subworker にはこれと同じ fail-closed 契約 (`create_host_path: false` の directory-scoped bind, 欠落で container create を refuse) のもう 1 本の host artefact パスがある: Learning Machine の student/teacher モデル (`RECAP_SUBWORKER_ARTIFACTS_HOST_PATH:-/var/lib/alt-recap-subworker-artifacts`、5.4 GB、`/app/recap_subworker/learning_machine/artifacts` にマウント)。本 runbook が扱う `genre_classifier*` 系の joblib artefact とは別物なので、`classification returned 0 results` 以外の起動失敗 (learning machine 側のエラー) ではこちらの欠落も疑う。

## デプロイ実行

両ブロッカー解消後、[[deploy]] の手順に従ってデプロイパイプラインを実行し、コンテナの再起動とヘルスチェックの通過を確認する。

### 成功判定

- `recap-worker`: `/opt/rustbert-cache` がマウントされ、Embedding 初期化ログが出力されること
- `recap-subworker`: host artefact が認識され、コンテナが `healthy` 状態になること

## デプロイ後の検証

```bash
curl -X POST http://127.0.0.1:9005/v1/generate/recaps/3days \
  -H 'Content-Type: application/json' -d '{"genres":["ai"]}'
```

202 が返ったら DB を確認する:

```sql
SELECT job_id, status, last_stage, window_days, kicked_at
FROM recap_jobs
WHERE window_days = 3
ORDER BY kicked_at DESC LIMIT 5;
```

新しい `status='completed'` 行が出れば正常。`status='failed'` で `last_stage='dedup'` が続くなら `recap_failed_tasks` を確認:

```sql
SELECT job_id, stage, substr(error, 1, 200) AS error_head
FROM recap_failed_tasks
WHERE created_at > NOW() - INTERVAL '10 minutes'
ORDER BY created_at DESC;
```

### `classification returned 0 results for N articles` が再発した場合

Settings validator + classifier `is_file()` guard + compose directory-scoped bind (`create_host_path: false`) は「artefact が dir 型 / 欠落 / mount が空ディレクトリ化」の silent failure を防ぐための多層防御。再発時の確認ポイント:

| 症状 | 可能性の高い原因 | 確認コマンド |
|---|---|---|
| compose up が "bind source path does not exist" で failed | host `/var/lib/alt-recap-subworker-data/` 不在 | `ls -la /var/lib/alt-recap-subworker-data/` |
| recap-subworker が起動しない (validator panic) | `RECAP_SUBWORKER_GENRE_CLASSIFIER_MODEL_PATH_*` env が dir を指している | `docker compose logs recap-subworker` の冒頭に ValidationError |
| recap-subworker 起動するが classify-runs タイムアウト | recap-worker 側 dispatch の real-timeout (LLM 側の issue 等) | recap-worker の `recap_failed_tasks.error` |
| 上記以外で `classification returned 0 results ...` 発生 | mTLS 証明書や peer identity 不整合の可能性 | recap-worker / recap-subworker ログ照合（[[000978]]） |

## 緊急時連絡・エスカレーション

- recap 関連の障害対応は [[deploy]] / [[runner-setup]] を参照
- pact broker の異常は [[pact-broker-ops]] を先に参照
- bind mount ポリシーは [[compose-bind-mount-policy]] を参照
