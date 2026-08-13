---
name: clean-architecture
description: Alt の Clean Architecture レイヤ規約（Handler → Usecase → Port → Gateway → Driver）に沿って、書こうとしている実装をどの層に置くかを判断し、層の逆依存・層越境を書く前に防ぐ。handler / rest / usecase / port / gateway / driver 配下を新規実装・リファクタするとき、ハンドラに fetch や SQL を足すような層をまたぐ変更に入るときに、ユーザが「Clean Architecture」や「層」に触れなくても使う。既に書かれたコードを横断的に走査して違反を列挙したいときは layer-checker サブエージェントを使う（このスキルは書く前の配置判断、layer-checker は書いた後の検出）。
paths:
  - "**/handler/**"
  - "**/rest/**"
  - "**/usecase/**"
  - "**/port/**"
  - "**/gateway/**"
  - "**/driver/**"
---

# Clean Architecture Layers

依存の向きは CLAUDE.md にある通り Handler → Usecase → Port → Gateway → Driver。
このスキルは「この処理をどの層に書くか」を決めるための責務表と、Alt で実際に繰り返された誤配置を扱う。

## 各層の責務

| 層（ディレクトリ） | 置くもの | 依存してよい先 |
|---|---|---|
| Handler (`rest/`, `handler/`) | HTTP / gRPC のエントリポイント、入力バリデーション、レスポンス整形 | Usecase, Port |
| Usecase (`usecase/`) | ビジネスロジックの組み立て。外部依存を直接持たない | Port のみ |
| Port (`port/`) | インタフェース定義（契約）のみ | 何にも依存しない |
| Gateway (`gateway/`) | 腐敗防止層。外部サービスのモデルを内部モデルに写像 | Port, Driver |
| Driver (`driver/`) | DB・API・外部連携の実装 | 外部ライブラリ |

## Alt で繰り返し出た誤配置

2026-07 の全リポジトリレビューで実際に見つかったパターン。変更を終える前に自分の差分を照らす。

- **Handler が Driver の仕事をしている**: REST / RPC ハンドラの中に HTTP fetch、SSRF 検証、直接の DB 呼び出しが書かれる（3 つのハンドラに ~600 行が重複した実例）。ハンドラは検証して Usecase に委譲してレスポンスを整えるだけ
- **Driver が Service / Usecase を import する**（逆依存）: `driver/` から `service/` や `usecase/` を参照すると依存の向きが反転する
- **Usecase がインフラを import する**: `usecase/` から `otel` / `httpx` / `asyncpg` / redis クライアント / `driver/` を直接参照している。Port インタフェース越しにする
- **抽出せずに層をまたいで複製する**: 同じロジックが複数のハンドラに貼られているのは、それが一段下の層に属しているサイン。Usecase に抽出する
- 層をまたぐ循環依存

## 既存コードの走査

リポジトリ全体やサービス単位で違反を洗い出すときは `layer-checker` サブエージェントを起動する。
検出用の grep とレポート形式はそちらが持っている。ここでは重複させない。
