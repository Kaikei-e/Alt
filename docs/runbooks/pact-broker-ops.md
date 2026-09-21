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
./scripts/deploy.sh production    # pact-check --broker → c2quay deploy → record-deployment

# Broker UI 認証
curl -u pact:$(cat secrets/pact_broker_basic_auth_password.txt) \
  http://localhost:9292/diagnostic/status/heartbeat
```

> **本番ゲートは単一ホストが唯一の真実ソース**。ADR-000740 は CI で
> `can-i-deploy` を回す設計だったが、OSS リポで機微情報を GitHub Actions
> に置かない方針に合わせ `deploy.sh` に移管済。現在は
> [c2quay](https://github.com/Kaikei-e/c2quay) が can-i-deploy と
> `docker compose up --wait` と record-deployment を担う。
> 詳細は [[deploy]] runbook を参照。

## 1. Broker 起動と認証

- `compose/pact.yaml` に basic-auth password secret mount + entrypoint 注入が入っている。
- secret file: `secrets/pact_broker_basic_auth_password.txt` (0644 で ruby UID 100 が読める必要あり)。
- DB password: `secrets/pact_db_password.txt`（Broker basic-auth とは分離。初回は `openssl rand -hex 32 > secrets/pact_db_password.txt`。既存 `pact_db_data` volume がある場合は init 時のパスワードと一致させるか `docker volume rm alt_pact_db_data` で再作成）。
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

## 3. can-i-deploy gate

`.github/workflows/release-gate.yaml` と `.github/workflows/deploy.yaml` はこの
repo に存在しない (退役済み)。can-i-deploy は現在 [[deploy]] が使う
`c2quay deploy` が内部で担う (`c2quay.yml` の `environments.production.services`
に登録された 14 pacticipant ぶん)。public repo 側に残る deploy 関連の
workflow は push-on-main で private `alt-deploy` へ `repository_dispatch`
するだけの `.github/workflows/dispatch-deploy.yaml` のみ。

can-i-deploy を単体で手動確認したいときは `pact-broker-cli` を直接叩ける:

```bash
# Broker に今の main を publish した直後
export PACT_BROKER_USERNAME=pact
export PACT_BROKER_PASSWORD=$(cat secrets/pact_broker_basic_auth_password.txt)
pact-broker-cli can-i-deploy \
  --pacticipant search-indexer \
  --version $(git rev-parse --short HEAD) \
  --to-environment production \
  --broker-base-url http://localhost:9292
```

> 認証は **環境変数で渡す** (`PACT_BROKER_USERNAME` / `PACT_BROKER_PASSWORD`)。
> `--broker-username` / `--broker-password` を `can-i-deploy` や
> `record-deployment` に付けると、値が正しくても 401 になる (この CLI の
> deployment 系 subcommand は flag 認証をサポートしない — `scripts/deploy.sh`
> と `scripts/pact-check.sh` がどちらも export だけで済ませているのはこの
> ため)。flag 形式が通るのは `publish` subcommand だけ。
>
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

## 6. Orphan pact (consumer はあるが provider が verify していない) 検出

Broker UI の "Pacticipants" → provider → Tab "Contract requiring verification"
に listed されている pact は verify 未実施。これを 0 に保つ。

自動化: `scripts/pact-broker-check-orphans.sh` (TODO、D7 完了時に配線)

## 6.5. Manual verification の publish (自動 provider verify が無い pact 向け)

自動化された provider verify を持たない pact (bridging registry に載る組み
合わせ) は、証跡ベースの manual verification record を Broker に post する
専用の leg で扱う: `scripts/pact-check.sh --publish-manual-verifications`。
これは bridging evidence を出す verification だけを実行したあと、
**`playbooks/publish-manual-verifications.yml` という Ansible playbook**
(`ansible-playbook -i localhost, -c local`) に委譲して Broker への実際の
post を行う — bridge registry・evidence gate・Broker への反復 POST・
credential 取り扱いはすべてこの playbook 側にある。したがって
`ansible-playbook` は、このリーグを走らせるどのホストにとっても
事実上の前提ツールになる ([[runner-setup]] の alt-builder ツール表を参照)。
`--dry-run` を付けると Broker に触れずに評価だけ表示する
(`-e bridge_dry_run=true` で `ansible-playbook --check` を回す)。
`--skip-manual-bridge` は、別の専用 leg が同じ playbook を既に走らせている
ことが分かっている呼び出し元向けに、この bridging block だけを飛ばす。

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

必要な PAT は **Kaikei-e/alt-deploy にスコープした fine-grained token**。
権限は **Contents: Read and write** + **Metadata: Read**。他 repo には
scope させない。

置き場所は `/etc/alt/secrets/gh_dispatch_pat.txt`、モードは **0600 のままでよい**。
同ディレクトリの compose 用 secret は 0644 だが、これは compose が
bind-mount する側の要件であってディレクトリ全体の規約ではない。
alt-deploy の deploy job は `docker compose config` が宣言した secret だけを
staging するので、compose が参照しないファイルの権限は deploy に影響しない
(以前はディレクトリを丸ごと glob していたため、この PAT を 0600 で置いた
だけで本番デプロイが落ちた。2026-07-31)。

> **Contents は write が必須。** `POST /repos/{owner}/{repo}/dispatches` は
> Contents の書き込み権限を要求する。Actions 権限では通らない。
> Actions:Read だけの token は `/actions/workflows` に 200 を返すのに
> dispatch は `403 Resource not accessible by personal access token` で
> 落ちる (2026-07-31 実測)。

#### ⚠️ `pact-broker-cli create-webhook` は使わない

Rust 版 CLI (0.6.3) の `--header` は**値を空白で分割し、各断片をヘッダ名として
登録する**。`--header "Authorization: Bearer <PAT>"` は次のように壊れる:

```
"authorization:" : "**********"   ← 値はマスクされる
"bearer"         : null
"github_pat_..." : null            ← PAT がヘッダ「名」になる
```

Broker はヘッダの**値**しかマスクしないため、**PAT が平文で保存され、webhook
定義を読める者全員に見える**。2026-07-31 にこれで token 1 本を失効させている。

加えて `--header` は 1 回しか指定できず、GitHub が要求する
`Content-Type: application/json` を Authorization と併記できない。CLI 既定の
`application/x-www-form-urlencoded` のまま送られ GitHub に拒否される。

(URL も `--url` ではなく位置引数。CLI を使わないので実害はないが、旧版の
手順をそのまま流すと最初にここで止まる。)

#### 登録 — Broker の REST API に直接 POST

ヘッダを JSON オブジェクトで渡すので分割は起きず、Content-Type も指定できる。

```python
#!/usr/bin/env python3
# 出力にリクエストボディ / レスポンスボディ / ヘッダ値を出さないこと。
import json, os, urllib.request
from base64 import b64encode
from pathlib import Path

BROKER = os.environ["PACT_BROKER_URL"]   # tailnet の :9292。登録先はどのホストでもよい (§下記参照)
DISPATCH_URL = "https://api.github.com/repos/Kaikei-e/alt-deploy/dispatches"
PROVIDERS = [
    "alt-backend", "alt-butterfly-facade", "auth-hub", "pre-processor",
    "search-indexer", "mq-hub", "rag-orchestrator", "recap-worker",
    "recap-subworker", "recap-evaluator", "news-creator", "tag-generator",
    "acolyte-orchestrator", "knowledge-sovereign",
]

token = Path("/etc/alt/secrets/gh_dispatch_pat.txt").read_text().strip()
pw = Path("/etc/alt/secrets/pact_broker_basic_auth_password.txt").read_text().strip()
auth = b64encode(f"pact:{pw}".encode()).decode()

for provider in PROVIDERS:
    body = {
        "description": f"verify_pact dispatch for {provider}",
        "provider": {"name": provider},
        "enabled": True,
        "request": {
            "method": "POST",
            "url": DISPATCH_URL,
            "headers": {
                "Authorization": f"Bearer {token}",
                "Content-Type": "application/json",
                "Accept": "application/vnd.github+json",
            },
            # ${pactbroker.*} はリテラルのまま broker に渡す。
            # 発火時に broker が実値へ置換する。
            "body": {
                "event_type": "verify_pact",
                "client_payload": {
                    "provider": provider,
                    "providerVersion": "${pactbroker.providerVersionNumber}",
                    "pactUrl": "${pactbroker.pactUrl}",
                    "consumer": "${pactbroker.consumerName}",
                    "consumerVersion": "${pactbroker.consumerVersionNumber}",
                },
            },
        },
        "events": [{"name": "contract_requiring_verification_published"}],
    }
    req = urllib.request.Request(
        f"{BROKER}/webhooks", data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json",
                 "Authorization": f"Basic {auth}"}, method="POST")
    with urllib.request.urlopen(req, timeout=30) as resp:
        print(provider, json.loads(resp.read()).get("uuid"))
```

#### `${pactbroker.pactUrl}` は起動元ホストで展開される

Broker に `PACT_BROKER_BASE_URL` が設定されていないため、broker は自身の URL を
**リクエスト元のホストから推測する**。

webhook 定義には `${pactbroker.pactUrl}` が**リテラルのまま**保存され、実 URL は
**発火時**に解決される。つまり登録をどのホスト経由で行ったかは影響しない —
効くのは **webhook を発火させた側**のホストである。`127.0.0.1:9292` 経由で
`test-webhook` すると `pactUrl` が `http://127.0.0.1:9292/...` になり、GitHub
runner から取得できず `Failed to load pact` で落ちる
(2026-07-31 run 30597755105 がこれ)。CI が tailnet の `:9292` へ publish して
発火する通常経路では正しく解決される。

疎通確認は **runner から到達できるホスト (tailnet の `:9292`) 経由で行う**こと。
恒久対策は broker に `PACT_BROKER_BASE_URL` を明示し、発火元に依存させないこと。

#### 登録後の確認

**1 provider だけ登録して疎通を確認してから残りを流す。**

```bash
pact-broker-cli test-webhook --uuid <UUID>   # success: true を確認
```

`success: false` のとき broker はレスポンス詳細を記録しない
(`PACT_BROKER_WEBHOOK_HOST_WHITELIST` 未設定のため)。切り分けは同じ body /
header で GitHub に直接 POST して status を見るのが速い。

token が正しく格納されたかは必ず確認する。ヘッダ名が 3 つに分かれ、
authorization の値がマスクされていれば正常:

```bash
curl -s -u pact:"$PW" "$BROKER/webhooks/<UUID>" \
  | python3 -c 'import sys,json; print(list(json.load(sys.stdin)["request"]["headers"]))'
# => ['authorization', 'content-type', 'accept']
```

一覧は `_embedded.webhooks` ではなく **`_links."pb:webhooks"`** に入る。
`_embedded` を見ると登録済みでも常に 0 件に見える。

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

## 9.5. `record-deployment` の instance 識別子忘れによる恒久 block

### 症状

`can-i-deploy` が、とうに置き換わったはずの古い version 名を挙げて落ち続ける。
section 9 の stale-verification パターンと違い、対象 pact の再 verify を通し
ても直らない — verification 自体には何も問題が無い。

### 原因

複数インスタンスを区別するための `--application-instance` (Broker の古い
ドキュメント/issue では `--target` と呼ばれていた同じフィールド) を指定せず
`record-deployment` を実行すると、Broker はそのデプロイを instance 未指定の
まま `currently deployed` として記録する。以後 **別の `--application-instance`
を指定した** `record-deployment` は instance が異なるため supersede しない。
結果として最初の (instance 未指定の) デプロイ記録が `currently deployed` の
まま永久に残り、それ以降の全ての `can-i-deploy` がとうに存在しない旧
version と比較され続ける。

c2quay がこのスタックの `record-deployment` を内部で実行しているが、
`c2quay.yml` にも本 repo のどのスクリプトにも `--application-instance` の
指定は見当たらない — c2quay がデフォルトでどう扱っているかは c2quay 自身の
ソース (この repo にはベンダリングされていない) を確認するまで未検証。この
症状が出たらまずここを疑う。`pact-broker-cli` に deployment 一覧を返す
サブコマンドは無いので、詰まっている version の特定は `can-i-deploy` の
エラーメッセージ自体 (旧 version 名を名指しする) と Broker の web UI
(pacticipant → Environments タブ) で行う。

### 復旧

1. `can-i-deploy` の失敗メッセージが名指しする古い version を確認する
   (Broker web UI の pacticipant → Environments タブでも同じ行が見える)。

2. その version の deployment を undeploy する (最初の record-deployment が
   `--application-instance` を指定していなかった場合、undeploy 側も指定しない
   ことで同じ行にマッチさせる):

   ```bash
   export PACT_BROKER_USERNAME=pact
   export PACT_BROKER_PASSWORD=$(cat secrets/pact_broker_basic_auth_password.txt)
   pact-broker-cli record-undeployment \
     --pacticipant <P> \
     --environment production \
     --broker-base-url http://localhost:9292
   ```

3. `can-i-deploy` を再実行して matrix が新しい version を見ているか確認する。
4. 恒久策として、以後の `record-deployment` / `record-undeployment` には
   常に同じ `--application-instance` を明示して渡す（未指定のままにしない）。

## 9.6. Lockstep breaking migration による provider verification デッドロック

### 症状

release-deploy.yaml の `pact-publish-provider (<P>)` が落ち、ログの `Failures:` が
**すべて `currently deployed to production` で選択された pact に対するもの**で、
同じ consumer の `latest version ... from the main branch` の pact は全て PASS する。

section 9 は can-i-deploy が古い *verification record* で落ちるケース、
section 9.5 は `--application-instance` 未指定で deployment 行が固着するケース。
本節はそのどちらでもなく、**deployment 記録も verification 記録も正しいまま**落ちる。

### 原因

デプロイ側パイプラインでは provider 検証がリリースの序盤に走るのに対し、
`record-deployment` は **ロールが完了した最終段でしか走らない**。一方 provider 検証は
`DeployedOrReleased: true` selector により **前回リリースの consumer version** を
相手に走る。

consumer と provider を同一コミットで同時に変える lockstep な破壊的変更では、
「前回の consumer が新しい provider を満たさない」のは定義上の帰結であり、
そのリリースは record-deployment に到達できない = 永久に通らない。

[[PM-2026-053]] の根本原因表 #3 に「鶏卵デッドロック」として記録済み。本節は
そのアクションアイテム #10（手動 record-deployment 手順の文書化）に対応する。

### まず分類する（ここを飛ばさない）

落ちた interaction を 2 つに仕分ける。

- **A. 実在する非互換** — 旧 consumer が新 provider に送るリクエストが本当に
  弾かれる。ロール順を誤ればデプロイ窓で実害が出る。
- **B. テストフィクスチャの虚構** — provider 側がスタブで、契約が実装と無関係に
  書かれていた / スタブが本番と違う直列化をしていた等。本番は最初から壊れていない。

section 9 の Secondary（verification を success として POST する force-override）は
**本節では使ってはならない**。検証は正しく失敗しており、成功を主張するのは
A08 Integrity Failure そのものになる。

### 復旧手順

原則は **consumer を先に出す**。新 consumer は旧 provider に対しても動く
（追加したヘッダ / パラメータは旧 provider から見れば未知で無視される）が、
逆は成り立たない。

1. 分類 A の consumer を先にロールする。イメージは失敗した run の build ジョブが
   既に GHCR へ push している（タグ `sha-<short>`）。

   ```bash
   # 本番ホストで
   docker compose -f compose/compose.yaml -p alt up -d --no-deps <consumer...>
   ```

2. ロールできた consumer を broker に記録する。**`--application-instance` は
   付けない** — release-deploy.yaml の record-deployment が未指定で記録している
   ので、付けると別行になって supersede しない（section 9.5）。

   ```bash
   export PACT_BROKER_USERNAME=pact
   export PACT_BROKER_PASSWORD=$(cat secrets/pact_broker_basic_auth_password.txt)
   export SHA=<new-sha>
   for p in <consumer...>; do
     pact-broker-cli record-deployment \
       --pacticipant "$p" \
       --version "$SHA" \
       --environment production \
       --broker-base-url http://localhost:9292
   done
   ```

   `--broker-password` は 401 になる。password は env 経由のみ。

3. release-deploy を再 dispatch する。provider 検証の `DeployedOrReleased` が
   新しい version を指すようになり、分類 A / B とも解消する。

4. 残りの provider はパイプラインの deploy ジョブに任せる。

### やってはいけないこと

- **まだロールしていない version を record-deployment しない。** 検証は通るが
  デプロイ窓の非互換は消えていないので、gate を通した意味が無くなる。
- **selector から `DeployedOrReleased` を外さない。** これはデプロイ窓の非互換を
  検出する唯一の仕掛けで、外すと本節の症状が「静かな本番障害」に変わる。
- **provider 側の検証を緩めて緑にしない。** テナント分離や認証を optional に
  戻すのは一時的なセキュリティ後退であり、契約の問題を本番の問題に移し替えるだけ。

### interaction の追加 / 改名も lockstep 変更である

consumer pact に interaction を追加したり、パスや description を変えると、Broker の
`contract_requiring_verification_published` webhook が発火し、provider 側を
**既にデプロイ済みの version** で checkout して検証しにくる。provider 検証は
スタブサーバに対して走るため、新しいパスは旧 version のスタブに存在せず
**404 / `text/plain`** になり、Broker に failure verdict が記録されて
can-i-deploy が赤くなる。

したがって interaction の追加 / 改名は、**provider 側のルートを先に出してから**
consumer を変える。consumer 単独の「契約の書き直し」は、たとえ実装に近づける
方向の修正であっても安全ではない。

### 実例 (2026-09-21)

1 リリースで 4 つの provider の inbound 認証 / テナント分離を同時に強化した結果、
契約検証が 5 グループ失敗した。いずれも「本番稼働中の consumer がその資格情報を
持っていない」であり、リグレッションは 1 件も無かった。

| provider | 強化コミット | 影響 consumer | 検証の相手 | 分類 |
|---|---|---|---|---|
| knowledge-sovereign | `a19597d6c` | alt-backend / rag-orchestrator | 実サーバ | **A** |
| recap-subworker | `3dda95fd2` | recap-worker | 実アプリ router | **A** |
| search-indexer | `1332fe974` | rag-orchestrator / acolyte-orchestrator | 実サーバ | **A** |
| search-indexer | `1332fe974` | alt-backend | 実サーバ | B — `estimatedTotalHits` は proto が当初から `int64` で本番は常に文字列を返していた。旧 provider スタブが `encoding/json` + `int` で本番と食い違っていただけ |
| alt-backend | `13673a3fa` | alt-butterfly-facade | スタブ | B — 対象の `GetOverview` は proto に存在しない RPC だった |

`gate` と `e2e` の失敗はすべて gate artifact 欠損による派生で、独立した原因は無い。
**分類の当たりを付けるには、まず provider 検証が実サーバを立てているのかスタブなのかを
確認する**。この repo では両方の流儀が混在しており、そこを取り違えると結論が反転する。

ロール順は以下。

```
wave 1  alt-butterfly-facade  rag-orchestrator  acolyte-orchestrator  recap-worker
wave 2  alt-backend
wave 3  search-indexer  knowledge-sovereign  recap-subworker
```

`alt-backend` が wave 2 なのは、alt-butterfly-facade に対しては provider、
search-indexer と knowledge-sovereign に対しては consumer という両義性があるため。

新 consumer は旧 provider に対して全ケース安全だった（旧 provider には interceptor が
無く余分な `Authorization` を無視する。search-indexer の `user_id` も旧実装では任意
パラメータとして受理されていた）。逆順にすると `/trail` の描画、Trail 検索、記事取り込みの
イベント追記、outbox worker、日次 recap のクラスタリングが実際に 401 / 400 で落ちる。

## 参考

- [[000591]] Pact CDC 全面展開
- [[000735]] search-indexer consumer の X-Service-Token 強制
- [[000736]] Pact CDC 残ギャップ埋めと can-i-deploy gate
- [[PM-2026-025]] acolyte empty section incident
- Pact 公式: https://docs.pact.io/pact_broker/can_i_deploy
- Pact 公式: https://docs.pact.io/pact_broker/webhooks
