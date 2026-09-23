---
name: clean-architecture
description: Alt の Clean Architecture（REST → Usecase → Port → Gateway → Driver + Domain）の境界配置を決定し、層越境・逆依存を実装前に防ぐ。エンドポイントや RPC の追加、新規 usecase 実装、DB・HTTP・LLM 呼び出しの追加、「どこに書くべきか」の迷い、層をまたぐリファクタリングや差分レビュー時に、ユーザが「Clean Architecture」や「層」と言及しなくても起動する。リポジトリ全体の事後走査には layer-checker サブエージェント、追記型イベント・投影不変条件には immutable-design-guard、テスト駆動開発の順序付けには tdd-workflow を使用する。
---

# Clean Architecture (Alt Model)

Alt の全サービスは 5 つのレイヤー（REST, Usecase, Port, Gateway, Driver）と、それらを貫く Domain で構成される。

詳細リファレンス:
- [references/principles.md](references/principles.md) — Uncle Bob / Cockburn / Fowler らの基礎理論と Alt での具現化
- [references/service-map.md](references/service-map.md) — サービス別の実ディレクトリ名・リクエスト追跡パス・命名対応表
- [references/anti-patterns.md](references/anti-patterns.md) — 実際に観測されたアンチパターン、理由、解消例

## 1. レイヤー構造と依存モデル

REST（Handler と同義）は HTTP / Connect-RPC / キュー消費のエントリポイントを表す。Domain はエンティティ・値オブジェクト・ドメインエラーで構成され、全層の共通語彙として機能する（I/O を伴う処理パイプラインの 1 ステップではない）。

### 制御・データフロー vs ソースコード（import）依存

```
【実行時の制御・データフロー】
REST ════⇒ Domain ════⇒ Usecase ════⇒ Port ════⇒ Gateway ════⇒ Driver
(DTOパース)   (語彙流通)   (業務規則)   (抽象呼出)   (型写像)     (生I/O)

【ソースコード（import）依存 — 矢印は「import する」】
REST     → Usecase, Domain
Usecase  → Port, Domain
Port     → Domain
Gateway  → Port（実装する）, Driver, Domain
Driver   → 外部ライブラリのみ（Domain も import しない）
Domain   → なし（標準ライブラリのみ）
Composition Root → すべて（配線専用）
```

| レイヤー | 責務 | import してよい対象 | import 禁止対象 |
|---|---|---|---|
| REST | 入力パース、認証、ステータス/エラー変換 | Usecase, Domain | Gateway, Driver |
| Usecase | 単一の業務意図のオーケストレーション | Port, Domain | REST, Gateway, Driver, 生I/O |
| Port | Usecase が要求する外部ケイパビリティの抽象契約 | Domain のみ | すべての具象レイヤー |
| Gateway | Port の実装。腐敗防止層（ACL）、ドメイン ↔ Driver 型変換 | Port, Driver, Domain | REST, Usecase |
| Driver | 外部ライブラリを用いた生 I/O（SQL, HTTP, Redis, LLM） | 外部ライブラリのみ | Domain, Port（直接実装すると Gateway を飛ばすことになる）, Usecase, Gateway, REST |
| Domain | コア語彙、純粋なビジネスルール、不変条件、ドメインエラー | 標準ライブラリのみ | すべての層 |

Dependency Rule 上は内向きだが、Alt では Driver は自前の row 型だけを持ち、Domain との写像は Gateway が一手に担う（Driver が Domain 型を返すと Gateway の腐敗防止層（ACL）の責務が I/O コードに混入し、スキーマや API 変更が Domain 語彙へ直接漏洩するため、Alt では推奨ではなく違反として強制される）。

全層を import して具象を配線できる唯一の場所は Composition Root（`cmd/main`, `di/`, `bootstrap/`, `infra/container.py`）である。配線規則は [.claude/rules/di-wiring.md](../../rules/di-wiring.md)（ローカル専用ファイル: オプション依存は起動時に `*_enabled/*_disabled` をログし、未配線ブランチ突入時は panic すること。`if x == nil { return nil }` による握り潰しは禁止）を参照する。

**なぜ制御フローと import 依存が逆転するのか（Dependency Inversion）**:
実行時の制御は Usecase から外界（Driver）へ向かうが、Usecase が Driver に依存すると外部技術（DB ドライバや外部 API 変更）の都合で業務ロジックが変更を強いられる。そこで Usecase 側に Port（抽象）を定義し、Gateway がその Port を実装して Driver を呼び出す。これによりソース依存は外側から内側の Port へ向かい、コア業務ロジックが外部技術の変更から保護される。

## 2. 配置判断フロー

新しいコードを書く際は、次の順序で質問を評価して配置を決定する:

1. **ドメインデータに対する純粋な計算・不変条件か？**
   → **Domain**。DB アクセスや外部通信を含まないエンティティや値オブジェクトのメソッドにする。
2. **単一のユーザ意図を達成するための手順・フロー調整か？**
   → **Usecase**。自身で I/O を行わず、必要な能力を Port のメソッドとして呼び出す。
3. **外界（データベース、外部 API、メッセージキュー、LLM）との対話か？**
   → **Port + Gateway + Driver**。Usecase が消費する Port を定義し、Gateway で外部型とドメイン型を相互変換し、Driver で生の通信を実行する。
4. **トランスポート形式の解釈、認証、HTTP/gRPC ステータスコードの割り当てか？**
   → **REST**。業務ロジックを含めず、DTO をドメイン値に変換して Usecase に委譲する。

## 3. 境界をまたぐデータモデル

Alt では 3 つのモデルを明確に分離する:

1. **Transport DTO**: JSON / Protobuf / クエリパラメータの構造体。REST 層が所有。
2. **Domain Model**: コア概念を表す純粋なモデル。シリアライズや永続化の都合を持たない。
3. **Driver Row / Payload**: SQL テーブル行（`pgx` 等）や外部 API レスポンスの構造体。Driver 層が所有。

**マッピング責務**:
- REST 層が Transport DTO ↔ Domain Model を変換する。
- Gateway 層が Domain Model ↔ Driver Row/Payload を変換する。
- Domain Model に `json:` や `db:` タグ、ORM 固有アノテーションを埋め込まない。外部形式の変更がドメインに波及するのを防ぐためである。

## 4. バリデーションの配置

- **形式的バリデーション（Syntactic）**: REST 層で行う。文字列長、必須フィールド、数値範囲、ページネーション上限など、単一リクエスト内で完結する構文検査。不正なリクエストを内側の層に入れずに弾く。
- **実質的バリデーション・不変条件（Semantic）**: Domain または Usecase で行う。エンティティの生成関数・値オブジェクトのコンストラクタで不変条件を強制し、状態遷移の妥当性や一意性制約を保証する。

## 5. エラー変換チェーン

下流のインフラ起因エラーを上位層に漏らさず、層ごとに適切な抽象度へ写像する:

1. **Driver**: 生の例外・エラー（`pgx.ErrNoRows`, HTTP 500 など）を返す。自前の Row 構造体や Driver 型で表現。
2. **Gateway**: Driver エラーを捕捉し、ドメインの sentinel error（例: `domain.ErrArticleNotFound`）に変換して Usecase に返す。
3. **Usecase**: ドメインエラーに基づきフローを制御し、必要に応じて上位に伝播する。
4. **REST**: ドメインエラーをトランスポートのステータスに変換する。Connect-RPC では `alt-backend/app/connect/errorhandler/error_handler.go` の標準に従い、内部詳細を秘匿して `{ ID, safe_message }` に正規化してクライアントへ返す（[docs/wiki/architecture/clean-architecture.md](../../../docs/wiki/architecture/clean-architecture.md)（ローカル専用ファイル: Connect-RPC ハンドラは error を `{ ID, safe_message }` に正規化し内部スタックや SQL 詳細を秘匿する）参照）。

## 6. Port の設計指針

- **論理的に Usecase が所有する**: Port はサービスの `port/` パッケージに配置され、その形状は Usecase が何を必要とするかによって決まる。一部のサービスでは `usecase/` 配下にポートを同居（colocate）させる（例: `rag-orchestrator`）。実装側である Gateway/Driver パッケージ内に Port を定義してはならない。
- **狭いインターフェース（ISP）**: 1 つの Usecase が必要とする 1〜3 メソッド程度に絞る。巨大な fat interface はテスト時のモック肥大化と無用な結合を招く。
- **技術ではなくケイパビリティで命名**: `PostgresRepository` や `RedisCache` ではなく、`ArticleFinder`, `UserAuthenticator` のように業務能力で命名する。
- **コンストラクタは具象型を返し、引数はインターフェースで受ける**: "Accept interfaces, return structs"。
- **トランザクション境界**: Usecase から `*sql.Tx` などの生ハンドルを扱わない。トランザクションは Port の Unit of Work 抽象、または単一の完結した業務操作（ケイパビリティ RPC）として Gateway/Driver 内部で完結させる。

## 7. レイヤーごとのテスト戦略

テスト駆動開発の全体順序（E2E → CDC → Unit）は [.claude/skills/tdd-workflow/SKILL.md](../tdd-workflow/SKILL.md) を参照する。

- **Domain**: 外部依存ゼロの純粋なユニットテスト。モック不要。
- **Usecase**: Port のインメモリフェイク（またはスタブ）を注入してテストする。すべての呼び出しを検証する過剰なモック化（mock-of-everything）を避け、入出力と状態変化をテストする。
- **Gateway / Driver**: 実際の DB やモックサーバ（`testcontainers`, `httptest`）を用いた統合テスト。型マッピングと生 I/O を検証する。
- **REST**: ハンドラの入力パース、バリデーション、エラー変換、ステータスコードをテストする。
- **サービス間境界**: Pact による CDC（Consumer-Driven Contracts）で合意を固定する。

## 8. 実践的例外ルール

- **レイヤーの縮退・省略**: サービスの `CLAUDE.md` や ADR に明記されている場合にのみ許容される（例: `knowledge-sovereign/app/CLAUDE.md` は handler/ と usecase/ が driver/sovereign_db を直接 import する構成を明記している（Port/Gateway なし、Handler→Driver 直結もある））。こうした形状は文書化が必須であり、外から内への依存原則および Driver から Usecase への逆 import 禁止規則は依然として有効である。
- **過剰な抽象化の回避**: 単一の単純な読み取りで将来の拡張予定がない処理に、推測で多重の抽象や未使用インターフェースを導入しない。

## 9. 完了前チェックリスト

変更を終える前に以下を確認する:

1. [ ] 新規ロジックが [2. 配置判断フロー](#2-配置判断フロー) の適切な層に置かれているか。
2. [ ] Driver やインフラの型（`sql.DB`, `httpx`, `otel` 等）が Usecase や Domain に漏れていないか。
3. [ ] Domain モデルに `json:` や `db:` タグを付けていないか。
4. [ ] Port は論理的に Usecase 側で狭く定義され、Composition Root で配線されているか（[.claude/rules/di-wiring.md](../../rules/di-wiring.md)）。
5. [ ] レイヤー検証スクリプトを実行し、VIOLATION がゼロであることを確認したか（Go および Python のみ対応。Rust / TS の変更は layer-checker サブエージェントを使用する。WARN 行（usecase の otel import のみ。driver→domain は VIOLATION）はレビューのための注意喚起であり失敗ではない）:
   ```bash
   bash .claude/skills/clean-architecture/scripts/check_layers.sh <service-dir>
   ```
6. [ ] 複数層にまたがる変更やリファクタリングでは、走査用サブエージェント `layer-checker` を実行したか。
