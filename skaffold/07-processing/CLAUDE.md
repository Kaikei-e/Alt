# レイヤー07: Processing 概要

## 1. 責務

この`07-processing`レイヤーは、Altプロジェクトにおける非同期のデータ処理パイプラインを構成するマイクロサービス群を管理・デプロイする責務を負います。RSSフィードの取得から、内容の解析、メタデータの付与（タグ生成）、コンテンツの変換・生成、検索インデックスへの登録、外部サービス連携の管理まで、一連のバックグラウンド処理を担当します。

## 2. 管理コンポーネント

`skaffold.yaml`で定義されている主要なHelmリリースは以下の通りです。これらはすべて`alt-processing`ネームスペースに集約され、データ処理パイプラインとして連携して動作します。

| Helmリリース名 | Chartパス | 説明 |
| :--- | :--- | :--- |
| `pre-processor` | `charts/pre-processor` | 登録されたRSSフィードを定期的に取得し、正規化などの初期処理を行うGo製サービス。パイプラインの起点となります。 |
| `pre-processor-sidecar` | `charts/pre-processor-sidecar` | `pre-processor`の補助的なタスク（例：Inoreader APIとの連携）をCronJobとして定期実行します。 |
| `search-indexer` | `charts/search-indexer` | 処理された記事データをMeiliSearchに投入し、全文検索を可能にするためのインデックスを作成するGo製サービス。 |
| `tag-generator` | `charts/tag-generator` | PythonベースのMLサービス。記事の内容を自然言語処理し、関連するキーワードをタグとして自動生成します。 |
| `news-creator` | `charts/news-creator` | LLM（大規模言語モデル）を利用して、記事の要約や関連コンテンツの生成など、高度なコンテンツ変換を行うサービス。 |
| `auth-token-manager` | `charts/auth-token-manager` | Inoreader APIなど、外部サービスのOAuth2アクセストークンを安全に管理し、定期的に更新するDeno/TypeScript製のサービス。 |

**注**: `rask-log-aggregator`は、Skaffoldの管理対象外であり、`docker-compose.yaml`を通じて管理されています。

## 3. ビルドアーティファクト

- 上記の各マイクロサービスのコンテナイメージ。
- **`shared-auth-go`, `shared-auth-python`**: 複数のGo/Pythonサービスで共通して利用される認証関連の処理をまとめたベースイメージ。コードの再利用性を高めています。

## 4. プロファイル

- **`dev` (デフォルト)**, **`staging`**, **`prod`**: 各環境向けの標準的なプロファイル。適用する`values.yaml`を切り替えます。
- **`schedule-mode`**: `pre-processor-sidecar`を通常の`CronJob`ではなく、常時稼働する`Deployment`としてデプロイするための特殊なプロファイル。デバッグや特定の運用シナリオで使用されることを想定しています。

## 5. デプロイ戦略と構成の変遷

- **インシデント駆動の改善**: `skaffold.yaml`内には「INCIDENT 82 FIX」や「INCIDENT 89 FIX」といったコメントが多数残されています。これは、過去に発生したインシデントへの対応を通じて、リポジトリ名の統一、ビルド方法の改善（Buildkitの利用）、タグ付けポリシーの変更など、構成が継続的に改善されてきたことを示しています。
- **ローカル開発の重視**: `dev`プロファイルでは`image.pullPolicy: "Never"`が徹底されており、ローカルでビルドした最新のイメージを使って開発サイクルを回すことが前提となっています。
