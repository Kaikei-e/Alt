---
title: Pact Broker 運用 runbook
date: 2026-04-15
tags:
  - runbook
  - testing
  - ci-cd
---
# Pact Broker 運用 runbook

ADR-000736 で Pact Broker が compose default profile に昇格してから、運用
ステップが増えた。ここに "起動・バックアップ・復旧・webhook 追加・failed
verify 調査" の手順を 1 本化する。

## TL;DR

```bash
# 常時起動 (compose up に含まれる)
docker compose -f compose/compose.yaml -p alt up -d pact-db pact-broker

# ローカルでの全 pact 検証
./scripts/pact-check.sh           # ファイルモード (Broker 不要)
./scripts/pact-check.sh --broker  # Broker モード (publish + can-i-deploy)

# デプロイ直前ゲート (本番ホスト専用・手動実行)
./scripts/pre-deploy-verify.sh    # heartbeat → pact-check --broker → can-i-deploy ×14

# 実際のデプロイ
./scripts/deploy.sh production    # gate → rolling recreate → smoke → record-deployment

# Broker UI 認証
curl -u pact:$(cat secrets/pact_broker_basic_auth_password.txt) \
  http://localhost:9292/diagnostic/status/heartbeat
```

> **本番ゲートは単一ホストが唯一の真実ソース**。ADR-000740 は CI で
> `can-i-deploy` を回す設計だったが、OSS リポで機微情報を GitHub Actions
> に置かない方針に合わせ、`pre-deploy-verify.sh` + `deploy.sh` に移管済。
> 詳細は [[deploy]] runbook を参照。

## 1. Broker 起動と認証

- `compose/pact.yaml` に basic-auth password secret mount + entrypoint 注入が入っている。
- secret file: `secrets/pact_broker_basic_auth_password.txt` (0644 で ruby UID 100 が読める必要あり)。
- 未認証アクセスは **401** (public read disabled)。これは意図的 — orphan reader で pact を盗み見るシナリオを閉じるため。

### 運用上の落とし穴

1. secret file の permission を `chmod 600` にすると broker コンテナが起動失敗 (`secret not mounted or not readable`)。`chmod 644` にすること。
2. pact-broker image の entrypoint は `/pact_broker/entrypoint.sh`。`/usr/local/bin/docker-entrypoint.sh` は **存在しない**。カスタム entrypoint を書く際は `cd /pact_broker && exec sh ./entrypoint.sh` にする。

## 2. CI branch protection 配線 (required status check)

`proto-contract.yaml` を blocking gate にする最後のピースは GitHub repo 側の "Require status checks to pass before merging" 配線。

```
Settings → Branches → Branch protection rules → main → Require status checks
に下記ジョブ名を列挙:

  - Proto & Contract Validation / Buf Lint & Breaking Change Detection
  - Proto & Contract Validation / Contract Conformance Tests (FE)
  - Proto & Contract Validation / Pact CDC Consumer Tests (Go)
  - Proto & Contract Validation / Pact CDC Consumer Tests (Rust)
  - Proto & Contract Validation / Pact CDC Consumer Tests (Python)
  - Proto & Contract Validation / Pact Publish & Provider Verification
```

**ジョブ名変更時は protection ルールの再登録を忘れない**。YAML 内 `name:` の
文字列変更は status check 名を変えるため、リネーム直後の最初の merge で gate
が効かない窓が空く。

## 3. can-i-deploy gate (deploy.yaml)

`release-gate.yaml` は `.github/workflows/deploy.yaml` の `needs: release-gate`
で必ず deploy の前段に入る。失敗時は deploy job 自体が skip される。

動作確認:

```bash
# Broker に今の main を publish した直後
export PACT_BROKER_PASSWORD=$(cat secrets/pact_broker_basic_auth_password.txt)
pact-broker can-i-deploy \
  --pacticipant search-indexer \
  --version $(git rev-parse --short HEAD) \
  --to-environment production \
  --broker-base-url http://localhost:9292 \
  --broker-username pact \
  --broker-password "$PACT_BROKER_PASSWORD"
```

`Computer says yes \o/` なら safe to deploy。failing matrix row は broker UI の
"Matrix" タブで可視化される。

## 4. バックアップと復旧

`compose/backup.yaml` の Restic スケジュールに `pact_db_data` volume と
`pact-db` の論理 dump (`pg_dump -U pact pact`) を追加済 (ADR-000736)。

- 毎時: `pg_dump` (`scripts/backup/backup-all.sh --pg-only`)
- 日次 03:00: Restic で dump + volume を Restic repo に commit + prune
- 日次 05:00: オフサイト同期 (`sync-offsite.sh`)
- 週次: `restore-verify.sh` が per-DB 復元を dry-run

### 手動復旧手順 (四半期 1 回は訓練で実行する)

```bash
# 1. pact-broker と pact-db を停止
docker compose -f compose/compose.yaml -p alt stop pact-broker pact-db

# 2. 最新 dump を特定
ls -lt backups/postgres/pact-db-*.dump | head -1
DUMP=backups/postgres/pact-db-YYYYMMDD_HHMMSS.dump

# 3. volume を wipe + db 再起動
docker volume rm alt_pact_db_data
docker compose -f compose/compose.yaml -p alt up -d pact-db
# pg_isready になるまで待つ

# 4. restore
docker exec -i alt-pact-db-1 pg_restore -U pact -d pact --clean --if-exists < "$DUMP"

# 5. broker 起動 + 動作確認
docker compose -f compose/compose.yaml -p alt up -d pact-broker
./scripts/pact-check.sh --broker
```

## 5. 新しい consumer を broker に登録する

1. consumer test を `<svc>/app/driver/contract/` に書く (Go) or `<svc>/tests/contract/` (Python/Rust)。
2. pact JSON が `<svc>/pacts/<consumer>-<provider>.json` に書き出されることを確認。
3. `./scripts/pact-check.sh --broker` を走らせると自動的に broker に publish される。
4. 対応する provider の `provider_test.go` / `test_provider_verification.py` に pact file path を追加し、`PactFiles` or broker selector で拾うようにする。
5. PR に `./scripts/pact-check.sh` 15/N passed の出力を貼る。

## 6. Orphan pact (consumer はあるが provider が verify していない) 検出

Broker UI の "Pacticipants" → provider → Tab "Contract requiring verification"
に listed されている pact は verify 未実施。これを 0 に保つ。

自動化: `scripts/pact-broker-check-orphans.sh` (TODO、D7 完了時に配線)

## 7. Broker webhook (consumer 変更 → provider CI 自動実行)

Phase E3 では未配線。配線時は:

```bash
pact-broker create-webhook \
  --request POST \
  --url https://api.github.com/repos/Kaikei-e/Alt/actions/workflows/proto-contract.yaml/dispatches \
  --header "Authorization: token $GH_TOKEN" \
  --data '{"ref":"main"}' \
  --provider <provider-name> \
  --contract-requiring-verification-published
```

これで consumer の main pact が更新された瞬間に provider 側 CI が再起動される。

## 8. Failed provider verification の調査フロー

1. broker UI → Pacticipants → 対象 provider → 最新 verification → "Failure"
2. 表示される matching error を確認。典型は:
   - **Header mismatch** — consumer が required header を落とした。consumer test の `.with_header(...)` 確認。
   - **Body mismatch** — response schema drift。provider 側の実装 diff を見る。
   - **Status mismatch** — e.g., 200 expected but 401。`ServiceAuthMiddleware` や peer-identity middleware の設定漏れ。
3. 対応 PR では: consumer / provider のどちらを先に修正するかを決める。原則は "consumer が真実、provider が追従"。例外は auth hardening (PM-2026-025 型)。
4. 修正 + `./scripts/pact-check.sh` green → broker publish → CI green。

## 参考

- [[000591]] Pact CDC 全面展開
- [[000735]] search-indexer consumer の X-Service-Token 強制
- [[000736]] Pact CDC 残ギャップ埋めと can-i-deploy gate
- [[PM-2026-025]] acolyte empty section incident
- Pact 公式: https://docs.pact.io/pact_broker/can_i_deploy
- Pact 公式: https://docs.pact.io/pact_broker/webhooks
