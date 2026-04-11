# 記事HTML→テキスト移行スクリプト

既存の`articles`テーブルに保存されているHTMLデータを、テキスト抽出済みデータに移行する高効率スクリプトです。

## 特徴

- **高効率**: 並列処理（デフォルト8ワーカー）とバッチ更新で高速化
- **安全**: ドライランモードで事前確認可能
- **進捗表示**: tqdmによるリアルタイム進捗表示
- **エラーハンドリング**: 抽出失敗時も元のデータを保持
- **統計情報**: 削減率、処理速度などの詳細統計を表示

## 前提条件

### 依存関係のインストール

```bash
cd scripts
pip install -r requirements.txt
```

必要なライブラリ:
- `psycopg2-binary`: PostgreSQL接続
- `beautifulsoup4`: HTMLパース
- `lxml`: 高速HTMLパーサー
- `readability-lxml`: 記事抽出
- `tqdm`: 進捗表示

## 使用方法

### 基本的な使い方

```bash
# 環境変数から接続情報を取得
./migrate_articles_to_text.sh

# または直接Pythonスクリプトを実行
python3 migrate_articles_to_text.py
```

### オプション

```bash
python3 migrate_articles_to_text.py \
  --batch-size 1000 \      # バッチサイズ（デフォルト: 1000）
  --workers 8 \             # 並列処理数（デフォルト: 8）
  --limit 100 \             # 処理する記事数の上限（テスト用）
  --dry-run \               # ドライランモード（実際には更新しない）
  --dsn "postgresql://..."  # 接続文字列を直接指定
```

### 環境変数

以下の環境変数から接続情報を取得します:

- `DB_HOST`: データベースホスト（デフォルト: localhost）
- `DB_PORT`: データベースポート（デフォルト: 5432）
- `DB_USER`: データベースユーザー（デフォルト: devuser）
- `DB_PASSWORD`: データベースパスワード
- `DB_PASSWORD_FILE`: パスワードファイルのパス（`DB_PASSWORD`より優先）
- `DB_NAME`: データベース名（デフォルト: devdb）

### 実行例

#### 1. ドライランで確認

```bash
python3 migrate_articles_to_text.py --dry-run --limit 100
```

#### 2. 小規模テスト

```bash
python3 migrate_articles_to_text.py --limit 1000
```

#### 3. 本番移行

```bash
python3 migrate_articles_to_text.py --batch-size 2000 --workers 16
```

## 処理内容

1. **HTML検出**: `content LIKE '<%'` でHTMLを含む記事を検出
2. **テキスト抽出**: Goの`html_parser.ExtractArticleText`と同じロジックでテキスト抽出
   - Next.js `__NEXT_DATA__` の処理
   - 不要要素の除去（iframe、embed、ソーシャルメディア、コメントなど）
   - readability-lxmlによる記事抽出
   - パラグラフ抽出
3. **バッチ更新**: バッチサイズごとに一括更新
4. **統計表示**: 処理結果の詳細統計を表示

## 出力例

```
データベースに接続中...
HTMLを含む記事数を取得中...
処理対象記事数: 1,500件
処理中: 100%|████████████| 1500/1500 [00:45<00:00, 33.2件/s, 更新=1485, 失敗=15, 削減率=97.2%]

============================================================
移行完了
============================================================
処理記事数: 1,500件
更新成功: 1,485件
更新失敗: 15件
処理時間: 45.23秒
処理速度: 33.2件/秒
元のサイズ: 342.15MB
抽出後サイズ: 5.23MB
削減率: 98.47%
```

## 注意事項

- **バックアップ**: 実行前にデータベースのバックアップを取得してください
- **ドライラン**: 初回実行時は`--dry-run`で動作確認を推奨
- **パフォーマンス**: `--workers`をCPUコア数に合わせて調整してください
- **メモリ**: 大量データ処理時は`--batch-size`を調整してください

## トラブルシューティング

### 接続エラー

```bash
# 接続文字列を直接指定
python3 migrate_articles_to_text.py --dsn "postgresql://user:pass@host:port/dbname"
```

### 依存関係エラー

```bash
pip install --upgrade -r requirements.txt
```

### メモリ不足

```bash
# バッチサイズを小さく
python3 migrate_articles_to_text.py --batch-size 500 --workers 4
```

