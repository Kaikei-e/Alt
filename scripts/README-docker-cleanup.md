# Docker Disk Space Management

このディレクトリには、Dockerのディスク使用量を自動的に管理するスクリプトが含まれています。

## 概要

Dockerは時間の経過とともに大量のディスクスペースを使用する可能性があります：
- ビルドキャッシュ
- 未使用のイメージ
- 停止したコンテナ
- 未使用のボリューム

これらのスクリプトは、ディスク使用量が設定された制限（デフォルト: 100GB）を超えた場合に自動的にクリーンアップを実行します。

## ファイル

- `docker-cleanup.sh` - メインのクリーンアップスクリプト
- `docker-cleanup.service` - systemdサービスファイル
- `docker-cleanup.timer` - systemdタイマーファイル（定期実行用）
- `update-docker-daemon-config.sh` - Dockerデーモン設定の更新スクリプト

## 使用方法

### 1. 手動でクリーンアップを実行

```bash
# デフォルトの制限（100GB）でクリーンアップ
make docker-cleanup

# または直接スクリプトを実行
./scripts/docker-cleanup.sh

# カスタム制限を設定して実行
MAX_DOCKER_SIZE_GB=50 ./scripts/docker-cleanup.sh
```

### 2. 自動クリーンアップを有効化（systemd timer）

```bash
# systemd timerをインストール（毎時実行）
make docker-cleanup-install

# ステータスを確認
make docker-cleanup-status

# ログを確認
sudo journalctl -u docker-cleanup.service -f
```

### 3. 自動クリーンアップを無効化

```bash
make docker-cleanup-uninstall
```

### 4. 現在のディスク使用量を確認

```bash
make docker-disk-usage
```

## 設定

### 環境変数

- `MAX_DOCKER_SIZE_GB` - Dockerが使用できる最大ディスク容量（GB、デフォルト: 100）
- `DOCKER_ROOT_DIR` - Dockerのルートディレクトリ（デフォルト: `/var/lib/docker`）
- `LOG_FILE` - ログファイルのパス（デフォルト: `/var/log/docker-cleanup.log`）

### Dockerデーモン設定の更新

Dockerデーモンのビルドキャッシュ設定を更新する場合：

```bash
sudo ./scripts/update-docker-daemon-config.sh

# カスタム設定で実行
MAX_BUILD_CACHE_GB=30 KEEP_DURATION=12h sudo ./scripts/update-docker-daemon-config.sh

# Dockerデーモンを再起動
sudo systemctl restart docker
```

## クリーンアップの動作

### 通常のクリーンアップ（制限内の場合）

- 停止したコンテナを削除
- 24時間以上古い未使用イメージを削除
- 未使用のボリュームを削除
- 24時間以上古いビルドキャッシュを削除
- 未使用のネットワークを削除

### 積極的なクリーンアップ（制限超過時）

- すべての停止したコンテナを削除
- すべての未使用イメージを削除
- すべてのビルドキャッシュを削除
- 未使用のボリュームを削除（注意: データが失われる可能性があります）
- 未使用のネットワークを削除

## 注意事項

⚠️ **重要**: 積極的なクリーンアップは、未使用のボリュームも削除します。重要なデータが含まれている可能性があるため、ボリュームのバックアップを取ることを推奨します。

⚠️ **ボリュームの削除**: `docker volume prune`は、現在どのコンテナからも使用されていないすべてのボリュームを削除します。重要なデータが含まれている可能性があるため、慎重に使用してください。

## トラブルシューティング

### ログの確認

```bash
# クリーンアップスクリプトのログ
sudo tail -f /var/log/docker-cleanup.log

# systemdサービスのログ
sudo journalctl -u docker-cleanup.service -f
```

### 手動でのディスク使用量確認

```bash
# Dockerのディスク使用量
docker system df

# 詳細な内訳
docker system df -v

# ファイルシステムの使用量
df -h /var/lib/docker
```

### 問題が発生した場合

1. ログを確認してエラーメッセージを確認
2. Dockerデーモンが正常に動作しているか確認: `sudo systemctl status docker`
3. 手動でクリーンアップを実行して問題を再現
4. 必要に応じて、より保守的な設定に変更（例: `MAX_DOCKER_SIZE_GB=150`）

## 定期実行のスケジュール

systemd timerは以下のスケジュールで実行されます：

- **毎時**: 通常のメンテナンスクリーンアップ
- **毎日**: より積極的なクリーンアップ
- **起動時**: システム起動5分後に実行

タイマーの設定を変更する場合は、`docker-cleanup.timer`ファイルを編集してください。

