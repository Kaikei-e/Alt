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

# 実際のデプロイ (本番ホスト専用・手動実行)
./scripts/deploy.sh production    # pact-check --broker → c2quay deploy → tts-speaker record-deployment

# Broker UI 認証
curl -u pact:$(cat secrets/pact_broker_basic_auth_password.txt) \
  http://localhost:9292/diagnostic/status/heartbeat
```

> **本番ゲートは単一ホストが唯一の真実ソース**。ADR-000740 は CI で
> `can-i-deploy` を回す設計だったが、OSS リポで機微情報を GitHub Actions
> に置かない方針に合わせ `deploy.sh` に移管済。現在は
> [c2quay](https://github.com/Kaikei-e/c2quay) が can-i-deploy ×15 と
> `docker compose up --wait` と record-deployment を担い、別ホストの
> tts-speaker だけ `scripts/record-remote-pacticipant.sh` が打刻する。
> 数は `c2quay.yml` の `production.services` 件数に追従する — pacticipant
> 追加 / 削除のたびに本文も更新する。
> 詳細は [[deploy]] runbook を参照。

> **新 pacticipant を `c2quay.yml` に足した直後** は §5.5 を実行しないと
> can-i-deploy gate が `unknown` で全 pacticipant を巻き込んで落ちる。
> [[000834]] 後に `knowledge-sovereign` を昇格させたときがこの経路にあたった。

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
pact-broker-cli can-i-deploy \
  --pacticipant search-indexer \
  --version $(git rev-parse --short HEAD) \
  --to-environment production \
  --broker-base-url http://localhost:9292 \
  --broker-username pact \
  --broker-password "$PACT_BROKER_PASSWORD"
```

> CLI は Rust 版 `pact-broker-cli` (ADR-000740 の Ruby gem 時代から更新済)。
> 未導入なら `curl -fsSL https://raw.githubusercontent.com/pact-foundation/pact-broker-cli/main/install.sh | sh` で入れる。

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

## 5.5 新しい pacticipant を deploy environment に bootstrap する

`c2quay.yml` の `production.services` に pacticipant を新規追加した直後 1 回だけ
実行する手順。`c2quay deploy` は **デプロイ後** に `record-deployment` を打つフロー
だが、新 pacticipant は production 環境にまだ deploy として記録されていないため
初回 can-i-deploy が `unknown` で落ち、`all_or_nothing: true` のおかげで他の全
pacticipant も同じ run でブロックされる (chicken-and-egg)。

`scripts/seed-pacticipant-deployment.sh` がこのブートストラップ専用ヘルパーで、
broker 側 `record-deployment` を 1 行だけ idempotent に書き込む:

```bash
# 例: ADR-000834 で knowledge-sovereign を pacticipant に昇格させた直後
PACTICIPANT=knowledge-sovereign \
VERSION=$(git rev-parse --short HEAD) \
ENVIRONMENT=production \
  ./scripts/seed-pacticipant-deployment.sh
```

実行後の動作確認:

```bash
pact-broker-cli can-i-deploy \
  --pacticipant <consumer-svc> --version $(git rev-parse --short HEAD) \
  --to-environment production
# Computer says yes に転じれば bootstrap 成功
```

注意:

- **VERSION は broker に publish 済みの SHA を選ぶ** (consumer pact published 経由
  でも provider verification 経由でも構わない)。`pact-broker-cli record-deployment`
  は当該 pacticipant version row が無いと拒否する。
- **2 回目以降は不要**。次回以降の `scripts/deploy.sh production` で c2quay 自身が
  `record-deployment` を打つので、production version は自動更新される。
- **`scripts/record-remote-pacticipant.sh` とは責務が違う**。あちらは別ホストで
  動く pacticipant (tts-speaker 等) が c2quay の手の届かないところに居る場合の
  恒常運用ループ。本スクリプトは「同ホスト稼働だが昇格直後で初回 gate を通せない」
  ケースの 1 回限りの bootstrap。

## 6. Orphan pact (consumer はあるが provider が verify していない) 検出

Broker UI の "Pacticipants" → provider → Tab "Contract requiring verification"
に listed されている pact は verify 未実施。これを 0 に保つ。

自動化: `scripts/pact-broker-check-orphans.sh` (TODO、D7 完了時に配線)

## 7. Broker webhook (consumer 変更 → provider CI 自動実行)

### 7.1 なぜ必要か

Path-filtered CI matrix (`.github/workflows/docker-build.yaml`) は
**provider のソースが変わったとき**だけ provider image を rebuild し、
verification を再実行する。consumer 側だけの pact 変更 —
typical には新しい consumer が追加されたり、既存 consumer が
interaction を追加したり — だと provider の rebuild は走らず、
Broker 上の verification 結果は古いまま。`can-i-deploy` は
「既存の verification が green だから OK」と答えてしまい、
次の provider deploy で初めて実体と契約が乖離していることが
露見する。

Pact 公式は 2021-10 以降 `contract_requiring_verification_published`
webhook をこのケース専用に提供している。Broker が「この pact は
まだ provider main 側で verify されていない」と判定した瞬間に fire
する。

### 7.2 配線方法

対応 workflow は **alt-deploy** 側 (private):
`.github/workflows/verify-pact-on-demand.yaml` (`repository_dispatch`
type `verify_pact` を受信し、`scripts/pact-check.sh --publish-only
--services <provider>` を当該 provider SHA で走らせる)。

Broker 側は provider ごとに 1 度 webhook を登録する:

```bash
# Fine-grained PAT: Kaikei-e/alt-deploy に Contents:Read + Actions:Write
# のみ許可したものを $GH_TOKEN に置く。他 repo には scope させない。
export GH_TOKEN=...
for provider in alt-backend alt-butterfly-facade auth-hub pre-processor \
                search-indexer mq-hub rag-orchestrator \
                recap-worker recap-subworker recap-evaluator \
                news-creator tag-generator acolyte-orchestrator \
                knowledge-sovereign; do
  pact-broker-cli create-webhook \
    --request POST \
    --url "https://api.github.com/repos/Kaikei-e/alt-deploy/dispatches" \
    --header "Authorization: Bearer $GH_TOKEN" \
    --header "Accept: application/vnd.github+json" \
    --data "$(jq -nc --arg p "$provider" '{
      event_type: "verify_pact",
      client_payload: {
        provider:        $p,
        providerVersion: "\u0024{pactbroker.providerVersionNumber}",
        pactUrl:         "\u0024{pactbroker.pactUrl}",
        consumer:        "\u0024{pactbroker.consumerName}",
        consumerVersion: "\u0024{pactbroker.consumerVersionNumber}"
      }
    }')" \
    --provider "$provider" \
    --contract-requiring-verification-published \
    --broker-base-url "$PACT_BROKER_BASE_URL" \
    --broker-username "$PACT_BROKER_USERNAME" \
    --broker-password "$PACT_BROKER_PASSWORD"
done
```

`${pactbroker.*}` は Broker 側のテンプレート変数。JSON 文字列は
リテラル `${...}` のまま Broker に渡し、webhook 発火時に Broker が
実値を埋める。上の heredoc は `\u0024` で `$` をエスケープして
shell 展開を避ける。

### 7.3 期待する動作

1. consumer が pact publish → Broker が「provider main 未 verify」と判定
2. webhook fire → alt-deploy `verify-pact-on-demand.yaml` に dispatch
3. self-hosted runner が Alt を `providerVersion` SHA で checkout
4. `pact-check.sh --publish-only --services <provider>` が走る
5. verification result が Broker に publish される
6. 次の `can-i-deploy` query で正しい verdict が返る

## 8. Failed provider verification の調査フロー

1. broker UI → Pacticipants → 対象 provider → 最新 verification → "Failure"
2. 表示される matching error を確認。典型は:
   - **Header mismatch** — consumer が required header を落とした。consumer test の `.with_header(...)` 確認。
   - **Body mismatch** — response schema drift。provider 側の実装 diff を見る。
   - **Status mismatch** — e.g., 200 expected but 401。`ServiceAuthMiddleware` や peer-identity middleware の設定漏れ。
3. 対応 PR では: consumer / provider のどちらを先に修正するかを決める。原則は "consumer が真実、provider が追従"。例外は auth hardening (PM-2026-025 型)。
4. 修正 + `./scripts/pact-check.sh` green → broker publish → CI green。

## 9. Stale verification による can-i-deploy 誤 block の解消

### 症状

release-deploy.yaml の `gate (<svc>)` が `can-i-deploy` で以下のように落ちる:

```
The verification for the pact between the version of <consumer> currently in production (<old-sha>)
and version <new-sha> of <provider> failed
❌ Computer says no
```

ただし:

- 該当 pact file 自体は main で動いている
- 該当 provider の最新コードでは provider test が pass する
- 原因は **broker 上の古い verification-results record** のまま（典型: 過去 deploy で provider が一時的に失敗した痕跡）

2026-04-20 の release-deploy run 24643555145 で `alt-backend × recap-worker@prod` が verification-results/1858 failure のまま残って deploy 全体を block した事例あり。

### Primary: 再 verify で matrix を上書き（**原則これだけ**）

Pact Broker matrix は **「同一 provider-version + pact-version」の最新 verification** を使う。provider 側を **実際に再 verify** して success record を POST すれば、古い failure が latest から押し出される。

```bash
# 1. 対象 pact-version の publish-verification-results URL を取得
PACT_URL="${PACT_BROKER_BASE_URL}/pacts/provider/<P>/consumer/<C>/pact-version/<PV>"
PUBLISH_URL=$(curl -fsS -u "pact:${PACT_BROKER_PASSWORD}" "$PACT_URL" \
  | jq -r '._links."pb:publish-verification-results".href')

# 2. 現行 prod の provider container で実テストを回す
cd alt-backend/app
go test ./pact_verifier/... -v

# 3. 結果を success record として POST（provider test が pass した前提）
curl -fsS -u "pact:${PACT_BROKER_PASSWORD}" \
  -X POST -H 'Content-Type: application/json' \
  -d "$(jq -n --arg v "$CURRENT_PROD_SHA" \
    '{success:true,providerApplicationVersion:$v,verifiedBy:{implementation:"manual-reverify",version:"1.0.0"}}')" \
  "$PUBLISH_URL"

# 4. can-i-deploy を再実行
pact-broker-cli can-i-deploy --pacticipant <P> --version <new-sha> --to-environment production
```

### Secondary（真に force-override 必要な例外経路のみ）

⚠️ **これは production gate を人間の主張で override する A08 Integrity Failure 相当のリスクを持つ**。使用時は次の 3 条件全てを満たすこと:

1. **2 人承認**: Linear issue + 別エンジニアの approve コメント
2. **`--build-url` に Linear issue URL を固定**（自由文字列禁止）
3. **監査ログ**: 実行後に slack #prod-audit へ invalidation URL + 理由 + approver 2 名を post

```bash
# ⚠️ PRIMARY の再 verify が技術的に不可能な場合にのみ
pact-broker-cli create-or-update-verification \
  --pact-url "$PACT_URL" \
  --provider-version "$CURRENT_PROD_SHA" \
  --success true \
  --build-url "https://linear.app/<org>/issue/<INC-NNNN>"
```

将来の予防策（backlog）:

- `scripts/pact-invalidate.sh` wrapper を作り、`PACT_ALLOW_FORCE_SUCCESS=true` + Linear URL 正規表現を CLI 側で強制（default refused）
- `record-deployment` model に全面移行（[[000740]] の superseding）して tags-based 判定箇所を削除すれば、stale failure が can-i-deploy の判断対象から自然に落ちる

## 参考

- [[000591]] Pact CDC 全面展開
- [[000735]] search-indexer consumer の X-Service-Token 強制
- [[000736]] Pact CDC 残ギャップ埋めと can-i-deploy gate
- [[PM-2026-025]] acolyte empty section incident
- Pact 公式: https://docs.pact.io/pact_broker/can_i_deploy
- Pact 公式: https://docs.pact.io/pact_broker/webhooks
