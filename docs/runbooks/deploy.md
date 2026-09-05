---
title: 手動デプロイ runbook (CDC Gate + c2quay whole-stack converge)
date: 2026-04-15
tags:
  - runbook
  - deploy
  - ci-cd
  - pact
---
# 手動デプロイ runbook — CDC Gate + c2quay whole-stack converge

単一ホスト Docker Compose 構成における **本番デプロイ手順**。現行の主経路は
**`main` への `git push` が自動発火**する: `.github/workflows/dispatch-deploy.yaml`
(public repo) が push ごとに private `alt-deploy` repo へ `repository_dispatch`
を飛ばし、`alt-builder` / `alt-prod` の 2 台の self-hosted runner がビルド・
Pact gate・E2E・本番反映を行う (ADR [[000763]]、runner の詳細は [[runner-setup]])。
`alt-deploy` 側のジョブ定義そのものは private repo にあり、本 repo には無い。

本 runbook が主に扱うのは、この repo に実在する **`scripts/deploy.sh`** —
単一ホスト上でローカルに全ステップを実行する手動 fallback 経路 (ADR
[[000763]] が "ひとまず現状のまま残す" と明記した既存経路)。緊急デプロイ・
CI 経路が使えないときの代替・ローカル検証に使う。

関連: [[pact-broker-ops]], [[mtls-cutover]], [[runner-setup]], ADR [[000740]], [[000741]], [[000763]]

## デプロイ経路

この repo にはローカルで完結する手動デプロイ経路が 2 つある:

1. **`scripts/deploy.sh`** (本 runbook が主に扱う経路) — build-free。
   既存イメージをそのまま使い、Pact gate → pki-agent leftover sweep →
   `c2quay deploy` だけを行う。
2. **`deploy-system/deploy-local.sh`** — `git fetch/pull --ff-only` →
   `docker compose build` (イメージを実際に再ビルドするのはこちらだけ) →
   `c2quay deploy --env production --config c2quay.yml` →
   `deploy-system/smoke-test.sh` (c2quay 自身の smoke ステップがカバーし
   ない補足チェック) の順。実行前に `export DOCKER_GROUP_ID=$(scripts/get-
   docker-gid.sh)` が必須 (`compose/logging.yaml` の変数展開がこれを要求
   するため、未設定だと `docker compose build` の手前でエラーになる)。
   `deploy-system/install.sh` で `deploy-system/systemd/alt-deploy.timer`
   (5 分ポーリング) を仕込めば自動実行もできる。

どちらも内部で呼ぶ `c2quay deploy` 自体は同じ経路 (14 pacticipant ぶんの
can-i-deploy gate → `docker compose up -d --wait --remove-orphans` →
smoke → record-deployment、`c2quay.yml` の `all_or_nothing: true` により
1 pacticipant でも gate に落ちれば全体を deploy しない)。

## TL;DR

```bash
# 前提ツール: pact-broker-cli (Rust 版、v0.6.3+) と c2quay がホストの PATH にあること
pact-broker-cli --version
c2quay --version

cd ~/alt
git pull origin main
./scripts/deploy.sh production

# First install only: no com.docker.compose.project=alt containers visible.
# matching pki-agent=0 on a running stack is a steady no-op and does not
# need this ACK. Wrong Docker context / rootless looks empty — do not
# set the ACK unless the host has never run project=alt.
# ALT_ACK_FRESH_INSTALL=1 ./scripts/deploy.sh production
```

`scripts/deploy.sh production` が以下を順に行う (失敗した時点で `set -euo
pipefail` により即中断、以降のステップは実行しない):

1. **`scripts/check-compose-variables.sh`** — `.env` の未設定変数を
   `docker compose config` の警告ではなく非ゼロ exit にして deploy 前に検知する
   (PM-2026-048: 空文字設定のまま 24h 稼働した事故の再発防止)
2. **`scripts/pact-check.sh --broker`** — pact を Broker に publish
3. **`scripts/retire-alt-pki-agent-leftovers.sh`** — project=`alt` の
   leftover `pki-agent-*` を stop/rm してゼロを確認 ([[pki-agent-recovery]])
4. **`c2quay deploy --env production --config c2quay.yml`** — `c2quay.yml`
   に登録された 14 pacticipant ぶんの can-i-deploy gate → `docker compose up -d
   --wait --remove-orphans` → `scripts/smoke.sh` → record-deployment
   (`scripts/deploy.sh` 冒頭コメントは「13 pacticipants」のままだが、
   `c2quay.yml` の registry は 14 件に増えており実態と食い違っている)

途中で失敗しても **自動ロールバックは無い**。復旧は手動:
`git revert` → re-commit → 本スクリプトを再実行 (`scripts/deploy.sh` 冒頭コメント)。

## 0. 前提

| 条件 | 確認コマンド |
|------|---|
| `pact-broker-cli` が PATH | `pact-broker-cli --version` (v0.6.3+、`pact-check.sh --broker` の publish に使う) |
| `c2quay` が PATH | `c2quay --version` (`C2QUAY_BIN` 環境変数で上書き可) |
| Pact Broker が healthy | `curl -fsS -u pact:$(cat secrets/pact_broker_basic_auth_password.txt) http://localhost:9292/diagnostic/status/heartbeat` |
| secrets 配置 | `ls secrets/pact_broker_basic_auth_password.txt` |
| mTLS step-ca 稼働 | `docker compose -f compose/compose.yaml ps step-ca` → healthy |
| disk / loadavg 余裕 | `df -h` / `uptime` |
| (`deploy-system/deploy-local.sh` 使用時のみ) `DOCKER_GROUP_ID` | `export DOCKER_GROUP_ID=$(scripts/get-docker-gid.sh)` — 未設定だと `compose/logging.yaml` の変数展開で `docker compose build` の手前で fail する |

`PACT_BROKER_USERNAME` は `scripts/deploy.sh` が `pact` に固定して export
する。`PACT_BROKER_PASSWORD` は `secrets/pact_broker_basic_auth_password.txt`
から自動で読む。ファイルが読めない場合、`scripts/deploy.sh` 自身は空のまま
`pact-check.sh` に渡すが、`pact-check.sh` 側が同じファイルを自分でも再度
読みに行き、それも読めなければリテラル `pact` にフォールバックする
(`scripts/pact-check.sh:277-283`) — 本物の Broker に対しては 401 になる。

## 1. ADR → deploy

```bash
# ADR を書いて commit した直後 (main へ push すると同時に dispatch-deploy が自動発火する)
git log -1 --oneline
./scripts/deploy.sh production   # 手動 fallback 経路。主経路は 1. の push 自体
```

`c2quay.yml` の `environments.production` は `all_or_nothing: true` —
c2quay に 1 サービスだけを対象にした deploy モードは無い (この repo のどの
呼び出しにも `--service` フラグは登場しない。`scripts/deploy.sh:96` /
`deploy-system/deploy-local.sh:11` / `tests/scripts/test_deploy.sh:8` は
いずれも `--env` と `--config` のみ)。1 サービスだけを入れ替えたいときは
c2quay を経由せず compose を直接叩く (migrate の要否は次節を参照):

```bash
docker compose -f compose/compose.yaml -p alt up -d --force-recreate --no-deps knowledge-sovereign
```

## 2. DB マイグレーションが絡むとき

Postgres を持つサービスは、それぞれ専用の Atlas 製 one-shot マイグレータを
compose service として持つ (`migrate` / `acolyte-db-migrator` /
`rag-db-migrator` / `recap-db-migrator` / `pre-processor-db-migrator` /
`knowledge-sovereign-db-migrator`。ADR [[000871]] が数える Atlas migrator
Dockerfile は全部で 7 つ)。加えて ClickHouse には Atlas ではない別枠の
one-shot `clickhouse-migrator` (`compose/db.yaml`) があり、alt-deploy 側の
自動処理がロールの前に `docker compose run --rm clickhouse-migrator apply`
で適用する。**`--env production` という Atlas environment は存在しない** —
各 `atlas.hcl` が定義するのは `local` / `kubernetes` (compose 上の実行は
こちら。K8s 非依存だが命名は歴史的なまま) / 一部 `ci` / 一部 `default` で、
実行時の接続先は `DATABASE_URL` を `DB_HOST` / `DB_PORT` / `DB_USER` /
`DB_NAME` + secret ファイルから組み立てて `--url` フラグで渡す
(`migrations-atlas/docker/scripts/migrate.sh`)。

マイグレータは対応するアプリ本体の `depends_on: … condition:
service_completed_successfully` で先行実行が保証される (例: `alt-data-hub`
は `migrate` の完了を待つ)。**この保証が効くのはフルスタックの `up` だけ**
—「1 サービスだけ再作成する」ロール (`up -d --no-deps <svc>`) はマイグレータ
を一切起動しないので、新しい migration ファイルは名指しで適用しないと
deploy が成功したまま pending のまま残る (`compose/acolyte.yaml` の
`acolyte-db-migrator` コメントが明記する PM-2026-037 の形そのもの)。

```bash
# 状態だけ確認したい (書き込みなし)
docker compose -f compose/compose.yaml -p alt run --rm migrate status

# 名指しで適用する (フルスタック up を経由しない per-service ロールの後は必須)
docker compose -f compose/compose.yaml -p alt run --rm migrate                      # alt-db (command は既定で apply)
make recap-migrate                                                                  # recap-db
make acolyte-migrate                                                                # acolyte-db
docker compose -f compose/compose.yaml -p alt run --rm rag-db-migrator
docker compose -f compose/compose.yaml -p alt run --rm pre-processor-db-migrator
docker compose -f compose/compose.yaml -p alt run --rm knowledge-sovereign-db-migrator
docker compose -f compose/compose.yaml -p alt run --rm clickhouse-migrator apply
```

`recap-db-migrator` と `acolyte-db-migrator` だけは `make recap-migrate` /
`make acolyte-migrate` という専用ターゲットを持つ (Makefile)。残りは
compose service 名を直接 `run --rm … apply` する。

新しい migration ファイルを追加した PR では、`atlas.sum` を **開発時に
再生成して commit** すること。マイグレータ側の `ensure_hash_file`
(`migrations-atlas/docker/scripts/migrate.sh`) は `atlas.sum` が
**存在しない**場合のみ自動生成するので、古いまま commit すると `atlas
migrate validate` がハッシュ不一致で fail する。ホストに `atlas` を入れる
必要は無く、コンテナ化された Makefile ターゲットで完結する:

```bash
make migrate-hash           # alt-db (migrations-atlas/)
make recap-migrate-hash     # recap-db (recap-migration-atlas/)
make acolyte-migrate-hash   # acolyte-db (acolyte-migration-atlas/)
```

## 3. Broker が落ちているとき

Pact Broker が dead の場合 `pact-check.sh --broker` (deploy step 2) が
fail-fast し、以降のステップ (leftover sweep / c2quay deploy) は走らない。

```bash
docker compose -f compose/compose.yaml -p alt up -d pact-db pact-broker
# 数秒待って heartbeat 確認
curl -fsS -u pact:$(cat secrets/pact_broker_basic_auth_password.txt) \
  http://localhost:9292/diagnostic/status/heartbeat
./scripts/deploy.sh production
```

## 4. 検証チェックリスト

デプロイ完了時に以下を 1 回ずつ確認する (`scripts/smoke.sh` が c2quay の
`deploy.smoke.command` として同じ 4 エンドポイントを自動で叩くが、手動でも
再確認する):

```bash
curl -fsS http://localhost/health
curl -fsS http://localhost:9000/v1/health
curl -fsS http://localhost:9250/health
curl -fsS http://localhost:7700/health

# Broker matrix で本番 version が記録されたか
curl -s -u pact:$(cat secrets/pact_broker_basic_auth_password.txt) \
  "http://localhost:9292/matrix?q[][pacticipant]=alt-backend&latestby=cvp" | jq '.matrix[0]'
```

## 5. CI との関係

- `.github/workflows/release-gate.yaml` と `.github/workflows/deploy.yaml` は
  存在しない (退役済み)。public repo 側に残る deploy 関連 workflow は
  push-on-main で private `alt-deploy` へ dispatch するだけの
  `.github/workflows/dispatch-deploy.yaml` のみ。本番 gate・build・E2E・
  record-deployment は private `alt-deploy` repo の pipeline
  (self-hosted runner 上) が担う (ADR [[000763]]、[[runner-setup]])。
- PR 時の契約チェック (`proto-contract.yaml`) は引き続き走り、**共有 Broker
  にパブリッシュしない file-based pact-check** のみを行う。

## テスト

`tests/scripts/` 配下に deploy 系スクリプトの挙動テストがある:
`test_deploy.sh` が `scripts/deploy.sh` の 4 ステップ chain を、
`test_deploy_local.sh` が `deploy-system/deploy-local.sh` の
`DOCKER_GROUP_ID` fail-fast guard を、`test_smoke.sh` が `scripts/smoke.sh`
をそれぞれ docker/curl スタブでカバーする。ゲートを変えたら必ず実行すること:

```bash
bash tests/scripts/run.sh
```
