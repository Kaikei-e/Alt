# meilisearch

_Last reviewed: September 5, 2026_

v1.27.0。全文検索 index。

## ポート

- 7700

## Health

- `/health`

## Volume

- `meili_data`

## Secrets

- `meili_master_key`

## 主要利用者

- `search-indexer` (write + query。Meilisearch に直接接続する唯一のサービス)
- `alt-backend` (search-indexer 経由で間接的に query。Meilisearch の secret は持たない)
- `recap-worker` (search-indexer 経由で `recaps` インデックスへ間接的に write)

## 注意

- index は **disposable**。記事 event log から再構築できる
- master key 変更時は search-indexer の secret を更新 (alt-backend は Meilisearch secret を持たない)

## Key ローテーション手順

`meili_search_key` (`secrets/meili_search_key.txt`) は独立した値ではなく、Meilisearch の Default Search API Key — **master key から導出される** ため、master key をローテーションすると自動的に値が変わる。旧 `meili_search_key.txt` を使い回すと search 専用キーが無効化され、read path が全滅する。

1. `meili_master_key.txt` を新しい値に更新し、`meilisearch` サービスを再作成して新 master key を反映
2. 新 master key で `GET /keys` を叩き、`name == "Default Search API Key"` の `key` を取得（値そのものは記録しない。取得直後にファイルへ書き込むのみ）
3. 取得した値を `secrets/meili_search_key.txt` へ書き込み、`meili_master_key.txt` と合わせてローカル開発環境およびデプロイ環境の各 `secrets/` ディレクトリに同期する（`secrets/` は gitignore 対象でチェックアウトごとに独立しているため、同期を忘れると片方だけ stale key のままになる）
4. 消費側を再作成して新 secret を読み込ませる: `meilisearch` → `search-indexer`（Meilisearch secret を持つのは search-indexer のみ。alt-backend は search-indexer 経由の間接利用なので secret 更新も再作成も不要）
5. `curl http://localhost:9300/v1/search?q=...` で search-indexer 経由の疎通を確認
