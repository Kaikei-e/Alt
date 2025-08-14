# レイヤー01: Foundation 概要

## 1. 責務

この`01-foundation`レイヤーは、Altプロジェクト全体の基盤(Foundation)となる、横断的な関心事を管理・デプロイする責務を負います。具体的には、セキュリティ、設定、ネットワーク、証明書管理など、アプリケーションが稼働するための前提条件を整備します。

## 2. 管理コンポーネント

`skaffold.yaml`で定義されている主要なHelmリリースは以下の通りです。

| Helmリリース名 | Chartパス | Namespace | 説明 |
| :--- | :--- | :--- | :--- |
| `cert-manager` | `charts/cert-manager` | `cert-manager` | クラスタ内のTLS証明書のライフサイクルを自動管理します。開発環境(`dev`プロファイル)ではCRDのインストールも行います。 |
| `common-config` | `charts/common-config` | `alt-config` | 複数のサービスで共有されるConfigMap、Namespace、リソースクォータなどを一元的に定義・管理します。 |
| `common-secrets-apps` | `charts/common-secrets-apps` | `alt-apps` | `alt-apps`ネームスペースに属するアプリケーション群が使用するデータベース接続情報やAPIキーなどの機密情報を管理します。 |
| `ca-issuer` | `charts/ca-issuer` | `cert-manager` | `cert-manager`が内部サービス間のmTLS通信などで使用する証明書を発行するための認証局(CA)を定義します。 |
| `network-policies` | `charts/network-policies` | `default` | プロジェクト全体のネットワークセキュリティの根幹をなすポリシー群を適用します。`default-deny`（デフォルトで全通信を拒否）を基本とし、必要なサービス間通信のみを明示的に許可するゼロトラストネットワークを構築します。 |

## 3. プロファイル

- **`dev` (デフォルト)**: ローカル開発環境(kindなど)向け。`cert-manager`のCRDをインストールし、開発用の`values.yaml`を適用します。
- **`prod`**: 本番環境向け。本番用の`values.yaml`を適用し、よりセキュアで安定した構成をデプロイします。

## 4. ビルドアーティファクト

- **`migrate`イメージ**: 他のレイヤーでも利用されるデータベースマイグレーション用のコンテナイメージをビルドします。

このレイヤーは、上位のレイヤー(Infrastructure, Core Servicesなど)が安全かつ安定して稼働するための土台を築く、極めて重要な役割を担っています。
