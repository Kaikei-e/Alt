---
title: 手動デプロイ runbook (Rolling-Recreate + CDC Gate)
date: 2026-04-15
tags:
  - runbook
  - deploy
  - ci-cd
  - pact
---
# 手動デプロイ runbook — Rolling-Recreate + CDC Gate

単一ホスト Docker Compose 構成で Blue-Green も Argo Rollouts も使えない環境
における **本番デプロイ手順**。デプロイは **ADR を書いた直後に人間が手動で
叩く** 運用に統一している。CI 自動発火はしない。

関連: [[pact-broker-ops]], [[mtls-cutover]], ADR [[000740]], [[000741]]

## TL;DR

```bash
# 初回のみ: broker CLI (Rust 版、pact_broker-client gem の後継) をホストに入れる
curl -fsSL https://raw.githubusercontent.com/pact-foundation/pact-broker-cli/main/install.sh | sh
# ダウンロードされたバイナリを PATH 上のディレクトリに置く (例: ~/.local/bin)
pact-broker-cli --version   # → 0.6.3 以降

cd ~/alt
git pull origin main
./scripts/deploy.sh production
```

`scripts/deploy.sh` が以下を順に行う:

1. **pre-deploy Pact gate** (`scripts/pre-deploy-verify.sh`)
2. **レイヤ順 rolling recreate** — サービスを 1 つずつ `--no-deps --force-recreate`
3. **global smoke** (nginx / alt-backend / bff / meilisearch)
4. **`pact-broker-cli record-deployment`** × 15 pacticipants (`c2quay.yml` `production.services` の件数に追従)

途中で失敗すると直前 SHA に自動でロールバックし、record-deployment は打刻されない。
record-deployment が 1 件でも失敗した場合は exit 12 で終了 (broker matrix と現実が乖離しないよう構造的に fail-fast)。

## 0. 前提

| 条件 | 確認コマンド |
|------|---|
| `pact-broker-cli` が PATH | `pact-broker-cli --version` (v0.6.3+) |
| Pact Broker が healthy | `curl -fsS -u pact:$(cat secrets/pact_broker_basic_auth_password.txt) http://localhost:9292/diagnostic/status/heartbeat` |
| secrets 配置 | `ls secrets/pact_broker_basic_auth_password.txt` |
| mTLS step-ca 稼働 | `docker compose -f compose/compose.yaml ps step-ca` → healthy |
| disk / loadavg 余裕 | `df -h` / `uptime` |

> CLI は `PACT_BROKER_BIN` 環境変数で上書き可能 (例: Docker イメージ
> `pactfoundation/pact-broker-cli:latest-debian` を薄いラッパで呼びたい場合)。

## 1. ADR → deploy

```bash
# ADR を書いて commit した直後
git log -1 --oneline
./scripts/deploy.sh production
```

ADR が無い通常のドキュメント変更だけの場合は `--only` で当該サービスだけ入れ替える:

```bash
./scripts/deploy.sh --only alt-butterfly-facade production
```

## 2. フラグ

| フラグ | 用途 |
|---|---|
| `--dry-run` | `docker compose up` も `record-deployment` も打たず、順序だけ echo |
| `--skip-verify` | Pact gate をスキップ (**緊急時のみ**。Broker 障害など) |
| `--only <svc>` | 1 サービスだけ入れ替え。gate は全 14 参加者ぶん走る |
| `--no-record` | smoke まで通るが Broker への record-deployment を打たない (検証用) |

## 3. レイヤ順序 (`scripts/_deploy_lib.sh:DEFAULT_LAYERS`)

1. **Infra** `step-ca`
2. **Auth** `kratos`, `auth-hub`
3. **Core** `alt-backend`, `search-indexer`, `mq-hub`, `pre-processor`
4. **Workers/AI** `news-creator`, `tag-generator`, `recap-*`, `acolyte-orchestrator`, `rag-orchestrator`, `tts-speaker`
5. **Edge** `alt-butterfly-facade`, `alt-frontend-sv`, `nginx`

DB (`db`, `kratos-db`, `pre-processor-db`, `rag-db`, `meilisearch`, `clickhouse`, `pact-db`) は **recreate 対象外**。schema 変更は Atlas (`cd migrations-atlas && atlas migrate apply`) で単独実行する (後述)。

各サービスごとに `docker compose ps` の healthcheck を最大 120s 待ち、`healthy` になってから次のレイヤへ進む。

## 4. ロールバック

`scripts/deploy.sh` は各回の開始時に `.deploy-prev` に直前 SHA を記録する。
layered recreate / global smoke のどこかで失敗すると、自動で:

```bash
git checkout <PREV_COMMIT> -- compose/
docker compose -f compose/compose.yaml up -d --remove-orphans
```

に相当する処理を実行する。Broker 側には **record-deployment を打たない**
ので matrix は直前の production version のままで整合が保たれる。

手動でもう一段戻したい場合:

```bash
PREV=$(awk -F= '/^PREV_COMMIT=/{print $2}' .deploy-prev)
git checkout $PREV -- compose/
docker compose -f compose/compose.yaml up -d --force-recreate --remove-orphans
```

## 5. DB マイグレーションが絡むとき

rolling recreate 自体は DB を触らない。schema 変更ありの PR は:

```bash
# 1) 事前に migration hash を整える (未実施なら)
cd migrations-atlas && atlas migrate hash

# 2) apply
atlas migrate apply --env production

# 3) そのあと通常 deploy
cd ~/alt && ./scripts/deploy.sh production
```

順序を必ず **migrate → deploy** にする。アプリ側が新スキーマを期待する
まま旧スキーマで起動すると healthcheck が通らず自動ロールバックで戻される。

## 6. Broker が落ちているとき

Pact Broker が dead の場合 `pre-deploy-verify.sh` が fail-fast する。

- **Broker だけを復旧**: `docker compose -f compose/compose.yaml up -d pact-db pact-broker` → 数秒待って heartbeat 確認
- **Broker 修復を待たず緊急デプロイ**: `./scripts/deploy.sh --skip-verify production` (運用ログに理由を残す)

## 7. 検証チェックリスト

デプロイ完了時に以下を 1 回ずつ確認する:

```bash
# global smoke (deploy.sh も叩くが手動で再確認)
curl -fsS http://localhost/health
curl -fsS http://localhost:9000/v1/health
curl -fsS http://localhost:9250/health
curl -fsS http://localhost:7700/health

# Broker matrix で本番 version が記録されたか
curl -s -u pact:$(cat secrets/pact_broker_basic_auth_password.txt) \
  "http://localhost:9292/matrix?q[][pacticipant]=alt-backend&latestby=cvp" | jq '.matrix[0]'

# 直近の deploy state
cat .deploy-prev .deploy-current 2>/dev/null || true
```

## 8. CI との関係

- `.github/workflows/release-gate.yaml` と `.github/workflows/deploy.yaml` は
  本計画で **退役**。本番 gate と record-deployment はローカル単一ホストが
  唯一の真実ソース。OSS リポに本番 Broker 資格情報を秘匿する必要がなくなる。
- PR 時の契約チェック (`proto-contract.yaml`) は引き続き走り、**共有 Broker
  にパブリッシュしない file-based pact-check** のみを行う。

## テスト

`tests/scripts/` 配下に `scripts/deploy.sh` と `scripts/pre-deploy-verify.sh`
の挙動テストがある。ゲートを変えたら必ず実行すること:

```bash
bash tests/scripts/run.sh
```
