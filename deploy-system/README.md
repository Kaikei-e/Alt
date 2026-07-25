# Alt Deploy System

ランタイムマシンで Git push を検知し、Docker Compose サービスを自動更新するデプロイシステム。

デプロイの実行パスは **c2quay のみ**（Pact ゲート必須、PM-2026-031）。`altctl` は
開発者ローカルのライフサイクル操作（`up`/`down`/`rebuild`/`doctor`）専用で、
`altctl deploy` コマンドは削除済み。

## 構成

```
deploy-system/
  deploy-local.sh          # メインデプロイスクリプト
  smoke-test.sh            # ヘルスチェックスクリプト
  install.sh               # systemd タイマーのインストール
  systemd/
    alt-deploy.service     # systemd サービス定義
    alt-deploy.timer       # 5分ポーリングタイマー
```

## 使い方

### 手動デプロイ

```bash
# 全サービスをビルドしてデプロイ (c2quay 経由)
./deploy-system/deploy-local.sh

# 特定サービスのみ再ビルド (デプロイ自体は c2quay.yml の全サービスが対象)
./deploy-system/deploy-local.sh alt-backend search-indexer

# 全サービスビルド (--all は下位互換のため残しているだけで無指定と同じ)
./deploy-system/deploy-local.sh --all
```

`c2quay` バイナリが `PATH` 上に見つからない場合はエラーで即座に終了する。
`make install-c2quay` でインストールすること。

### 自動デプロイ (systemd timer)

```bash
# インストール (初回のみ)
./deploy-system/install.sh

# ステータス確認
systemctl status alt-deploy.timer
systemctl list-timers alt-deploy*

# ログ確認
journalctl -u alt-deploy.service -f

# 停止
sudo systemctl stop alt-deploy.timer
```

### スモークテスト

`smoke-test.sh` は c2quay の smoke ステップ（`scripts/smoke.sh`）が
カバーしていない補足チェックのみを行う（現状: frontend-sv への直接アクセス）。
c2quay 自体のスモークテストと二重実行しないよう、重複するチェックは
`smoke-test.sh` 側から削除済み。

```bash
# ローカル実行
./deploy-system/smoke-test.sh

# リモートホスト指定
ALT_RUNTIME_HOST=<YOUR_RUNTIME_IP> ./deploy-system/smoke-test.sh
```

## 動作フロー

1. `git fetch origin main` で最新状態チェック
2. 差分がある場合のみ `git pull --ff-only`
3. `docker compose -f compose/compose.yaml -p alt build` でイメージをビルド
   （c2quay は意図的に build-free / `pull: never` なので、ビルドはここで行う）
4. `c2quay deploy --env production --config c2quay.yml` でデプロイ
   - Pact `can-i-deploy` ゲート（`gate_only` サービスを含む全 pacticipant、
     `all_or_nothing: true`）
   - `docker compose up -d --wait --remove-orphans`
   - スモークテスト（`c2quay.yml` の `deploy.smoke` = `scripts/smoke.sh`）
   - 各 pacticipant の `record-deployment`
5. `smoke-test.sh` で c2quay がカバーしない補足ヘルスチェックを実行
6. 結果をログファイルに記録
