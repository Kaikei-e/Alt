# nginxログ解析ツール

SvelteKit（`/sv`）へのリクエストのパフォーマンスを分析するためのツールです。

## 使用方法

### 基本的な使用方法

```bash
# デフォルト設定で実行（直近1000行を分析）
./scripts/analyze-nginx-logs.sh
```

### 環境変数によるカスタマイズ

```bash
# 分析するログ行数を変更
LOG_LINES=5000 ./scripts/analyze-nginx-logs.sh

# 遅いリクエストの閾値を変更（デフォルト: 1.0秒）
THRESHOLD=2.0 ./scripts/analyze-nginx-logs.sh

# nginxコンテナ名を指定（デフォルト: nginx）
NGINX_CONTAINER=my-nginx ./scripts/analyze-nginx-logs.sh
```

### 例

```bash
# 直近5000行を分析し、2秒以上のリクエストを警告
LOG_LINES=5000 THRESHOLD=2.0 ./scripts/analyze-nginx-logs.sh
```

## 出力内容

スクリプトは以下の情報を出力します：

1. **統計情報**
   - 平均リクエストタイム
   - 中央値リクエストタイム
   - 最大/最小リクエストタイム

2. **アップストリーム統計**
   - 平均アップストリーム応答タイム
   - 平均アップストリーム接続タイム

3. **遅いリクエスト**
   - 閾値以上のリクエストの一覧
   - トップ10の遅いリクエスト

4. **ステータスコード別統計**
   - 各ステータスコードの出現回数と割合

5. **エンドポイント別統計**
   - エンドポイントごとの平均レスポンスタイムとリクエスト数

## nginx設定の変更

ログ解析を有効にするには、nginx設定をリロードする必要があります：

```bash
# nginx設定をテスト
docker compose exec nginx nginx -t

# nginx設定をリロード（ダウンタイムなし）
docker compose exec nginx nginx -s reload

# または、コンテナを再起動
docker compose restart nginx
```

## ログフォーマット

nginxのログフォーマットには以下のタイミング情報が含まれます：

- `rt` - リクエスト処理時間（秒）
- `uct` - アップストリームへの接続時間（秒）
- `uht` - アップストリームからのヘッダー受信時間（秒）
- `urt` - アップストリームの応答時間（秒）

## トラブルシューティング

### コンテナが見つからない場合

```bash
# 利用可能なコンテナを確認
docker ps --format '{{.Names}}'

# コンテナ名を指定して実行
NGINX_CONTAINER=alt-nginx ./scripts/analyze-nginx-logs.sh
```

### ログが見つからない場合

- nginxコンテナが起動していることを確認
- `/var/log/nginx/access.log`が存在することを確認
- `/sv`パスへのリクエストが実際に発生していることを確認

### ログフォーマットが正しくない場合

`nginx/nginx.conf`の`log_format`ディレクティブを確認し、以下のフィールドが含まれていることを確認：

```
rt=$request_time uct=$upstream_connect_time uht=$upstream_header_time urt=$upstream_response_time
```

## パフォーマンス最適化のヒント

スクリプトの出力を基に、以下の点を確認してください：

1. **アップストリーム接続時間が長い場合**
   - DNS解決の問題（`resolver 127.0.0.11 valid=10s`の設定を確認）
   - ネットワーク遅延
   - アップストリームサーバーの負荷

2. **アップストリーム応答時間が長い場合**
   - SvelteKitアプリケーションの処理時間
   - バックエンドAPIへのリクエスト遅延
   - データベースクエリの最適化

3. **リクエストタイムが長いがアップストリーム応答時間が短い場合**
   - nginxのバッファリング設定
   - クライアントへの転送時間
   - ネットワーク帯域幅

