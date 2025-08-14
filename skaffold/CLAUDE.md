# Altプロジェクト Skaffold オーケストレーションガイド

## 1. はじめに

このドキュメントは、Altプロジェクトにおける`skaffold`ディレクトリの構成と、Skaffoldを利用したビルド・デプロイ戦略の全体像を解説するものです。プロジェクトは複数のレイヤーに分割されており、Skaffoldがそれらを統合的に管理することで、複雑なマイクロサービスアーキテクチャの効率的な運用を実現しています。

## 2. オーケストレーション戦略

Altプロジェクトのオーケストレーションは、以下の2つの主要な戦略に基づいています。

### 2.1. レイヤー化アーキテクチャ

Skaffoldの構成は、依存関係と関心事に基づいて複数のレイヤーに分割されています。デプロイは基本的に番号の若いレイヤーから順に行われます。

- **`01-foundation`**: プロジェクト全体の基盤。証明書管理(`cert-manager`)、ネットワークポリシー、共通設定などを提供します。
- **`02-infrastructure`**: 永続化層。PostgreSQL, ClickHouse, MeiliSearchといったステートフルなデータストアを管理します。
- **`04-core-services`**: 中核となるバックエンドAPI(`alt-backend`)と、関連するプロキシ(`envoy-proxy`)を管理します。
- **`05-auth-platform`**: 認証・認可基盤。Ory Kratosとカスタム認証サービス(`auth-service`)で構成されます。
- **`06-application`**: ユーザーが直接触れるUI(`alt-frontend`)と、外部からのリクエストを受け付けるIngress(`nginx-external`)を管理します。
- **`07-processing`**: 非同期のデータ処理パイプライン。RSSの取得、解析、タグ付け、コンテンツ生成などを行うマイクロサービス群で構成されます。
- **`08-operations`**: 運用タスク。データベースのバックアップやモニタリング関連のコンポーネントを管理します。(このドキュメントの自動生成対象外)

このレイヤー化により、各コンポーネントの依存関係が明確になり、レイヤー単位での独立した開発とデプロイが可能になっています。

### 2.2. プロファイルによる環境分離

Skaffoldの`profiles`機能を利用して、`dev`, `staging`, `prod`といった環境ごとの構成を管理しています。

```yaml
profiles:
  - name: prod
    deploy:
      helm:
        releases:
          - name: alt-frontend
            # ...
            valuesFiles: # ← プロファイルに応じて適用する設定ファイルを切り替え
              - charts/alt-frontend/values.yaml
              - charts/alt-frontend/values-production.yaml 
```

これにより、コードベースは単一のまま、適用する`values.yaml`やイメージリポジトリ、リソース要求などを環境ごとに柔軟に変更できます。

## 3. ビルド戦略

- **コンテナイメージの定義**: 各レイヤーの`skaffold.yaml`内の`build.artifacts`セクションで、ビルド対象のコンテナイメージとそのコンテキスト（Dockerfileの場所など）が定義されています。
- **ローカル開発の最適化**: `dev`プロファイルでは`push: false`や`image.pullPolicy: "Never"`が設定されており、ビルドしたイメージをコンテナリポジトリにプッシュすることなく、ローカルのDockerデーモンから直接Kubernetesクラスタ（kindなど）にロードすることで、開発サイクルを高速化しています。

## 4. デプロイ戦略

- **Helmによる宣言的デプロイ**: プロジェクトのすべてのコンポーネントはHelmチャートとして管理されています。Skaffoldは`deploy.helm.releases`セクションを通じて、これらのチャートを適切な順番と設定でクラスタにリリースします。
- **動的なイメージタグの注入**: Skaffoldはビルド時に生成したコンテナイメージのタグ（Gitコミットハッシュなど）を、`setValueTemplates`機能を使ってHelmの`values`に動的に注入します。これにより、常に正しいバージョンのイメージがデプロイされることが保証されます。

```yaml
# 例: 07-processing/skaffold.yaml
deploy:
  helm:
    releases:
      - name: pre-processor
        setValueTemplates:
          image.repository: "{{.IMAGE_REPO_kaikei_pre_processor}}"
          image.tag: "{{.IMAGE_TAG_kaikei_pre_processor}}" # ← ビルドしたイメージのタグがここに設定される
```

- **デプロイフックによる自動検証**: 多くのレイヤーで、デプロイの前後に`hooks`を利用して`kubectl`や`helm`コマンドが実行されます。これにより、デプロイ状況の確認や、デプロイ後のPodの起動検証が自動化され、デプロイプロセスの信頼性を高めています。

## 5. まとめ

Altプロジェクトでは、Skaffoldをオーケストレーションの中核に据え、**レイヤー化**、**プロファイル管理**、**Helmによる宣言的デプロイ**といった戦略を組み合わせることで、複雑なマイクロサービスアプリケーションのビルドとデプロイを体系的、効率的、かつ信頼性高く管理しています。
