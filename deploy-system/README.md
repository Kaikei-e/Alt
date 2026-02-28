# Alt Deploy System

ランタイムマシンで Git push を検知し、Docker Compose サービスを自動更新するデプロイシステム。

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
# デフォルトスタックをデプロイ
./deploy-system/deploy-local.sh

# 特定スタックのみデプロイ
./deploy-system/deploy-local.sh core workers

# 全スタックデプロイ
./deploy-system/deploy-local.sh --all
```

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

```bash
# ローカル実行
./deploy-system/smoke-test.sh

# リモートホスト指定
ALT_RUNTIME_HOST=<YOUR_RUNTIME_IP> ./deploy-system/smoke-test.sh
```

### altctl deploy コマンド

```bash
# ワンコマンドデプロイ (git pull + build + up + smoke test)
altctl deploy

# 特定スタックのみ
altctl deploy core

# キャッシュなしでビルド
altctl deploy --no-cache
```

## 動作フロー

1. `git fetch origin main` で最新状態チェック
2. 差分がある場合のみ `git pull --ff-only`
3. `altctl up --build` でリビルド & 再起動
4. `smoke-test.sh` でヘルスチェック
5. 結果をログファイルに記録
