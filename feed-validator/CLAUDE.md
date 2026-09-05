# Feed Validator Service

F# 10 **学習プロジェクト**（`README.md` / `ROADMAP.md` 参照）。将来的に RSS/Atom/JSON Feed
検証・正規化サービスとして実装することを目標にした設計仕様であり、以下の Domain/Usecase/
Gateway/Handler・API・NuGet 依存・alt-backend 統合は **すべて ROADMAP.md の Phase 1〜8 の
到達目標（未実装）**。

## 実装状況 (2026-09-05 時点)

- 実際に存在するコードは `FeedValidator/Program.fs` の `printfn "Hello from F#"` のみ（ROADMAP Phase 1）
- `FeedValidator.fsproj` に `Compile Include="Program.fs"` 以外のファイルは無く、Giraffe / FSharp.Data など
  NuGet パッケージへの参照もまだ無い
- `src/Domain/`, `src/Usecase/`, `src/Gateway/`, `src/Handler/`, `FeedValidator.Tests/` ディレクトリは未作成
- `Dockerfile` は存在せず、`compose/*.yaml` のどこにも `feed-validator` サービス定義は無い（起動できない）
- alt-backend 側には `port/feed_validator_port.go` / `gateway/feed_validator_gateway.go` は存在しない。
  alt-backend の feed 登録バリデーションは無関係の `alt-backend/app/validation/feed_validator.go`
  （SSRF 対策など入力形状チェックのみを行う、この F# サービスとは繋がっていない Go コード）が担う
- 以下のセクションはこのサービスが実装された **場合の** 仕様・設計であり、現状のコードを説明するものではない

---

## アーキテクチャ

```
Handler → Usecase → Domain
   │
   └── Gateway（外部URL取得時のみ）
```

### レイヤー責務

| レイヤー | 責務 | ファイル |
|---------|------|---------|
| Domain | 型定義、ビジネスルール | `src/Domain/*.fs` |
| Usecase | 検証・正規化ロジック | `src/Usecase/*.fs` |
| Gateway | 外部HTTP通信 | `src/Gateway/*.fs` |
| Handler | HTTPルーティング | `src/Handler/*.fs` |

---

## ディレクトリ構成

```
feed-validator/
├── CLAUDE.md                 # このファイル
├── ROADMAP.md               # 学習ロードマップ
├── FeedValidator/
│   ├── FeedValidator.fsproj
│   ├── Program.fs
│   └── src/
│       ├── Domain/
│       │   ├── Feed.fs          # フィード型定義
│       │   └── ValidationError.fs
│       ├── Usecase/
│       │   ├── Parser.fs        # パース処理
│       │   ├── Validator.fs     # 検証ロジック
│       │   └── Normalizer.fs    # 正規化処理
│       ├── Gateway/
│       │   └── HttpGateway.fs   # 外部フィード取得
│       └── Handler/
│           └── Routes.fs        # Giraffe ルート
├── FeedValidator.Tests/
│   ├── FeedValidator.Tests.fsproj
│   ├── DomainTests.fs
│   ├── ValidatorTests.fs
│   └── IntegrationTests.fs
├── samples/
│   ├── rss2.xml
│   ├── atom.xml
│   └── jsonfeed.json
└── Dockerfile
```

---

## API エンドポイント

### GET /health

ヘルスチェック。

**レスポンス**:
```json
{
  "status": "ok",
  "service": "feed-validator"
}
```

### POST /v1/validate

フィードコンテンツを検証・正規化。

**リクエスト**:
```http
POST /v1/validate
Content-Type: application/xml

<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title>Example Feed</title>
    ...
  </channel>
</rss>
```

**成功レスポンス** (200):
```json
{
  "format": "RSS2",
  "title": "Example Feed",
  "link": "https://example.com",
  "description": "An example feed",
  "items": [
    {
      "title": "Article 1",
      "link": "https://example.com/1",
      "description": "...",
      "publishedAt": "2025-01-01T00:00:00Z",
      "author": "John Doe"
    }
  ],
  "language": "en"
}
```

**エラーレスポンス** (400):
```json
{
  "errors": [
    { "type": "MissingRequiredField", "field": "title" },
    { "type": "InvalidUrl", "url": "not-a-valid-url" }
  ]
}
```

### POST /v1/validate/url

URLからフィードを取得して検証。

**リクエスト**:
```json
{
  "url": "https://example.com/feed.xml"
}
```

**レスポンス**: `/v1/validate` と同じ

---

## ドメインモデル

### FeedFormat（判別共用体）

```fsharp
type FeedFormat =
    | RSS2        // RSS 2.0
    | Atom        // Atom 1.0
    | JSONFeed    // JSON Feed 1.1
    | Unknown of string
```

### NormalizedFeed（レコード型）

```fsharp
type NormalizedFeed = {
    Format: FeedFormat
    Title: string
    Link: string
    Description: string option
    Items: FeedItem list
    Language: string option
}
```

### FeedItem（レコード型）

```fsharp
type FeedItem = {
    Title: string
    Link: string option
    Description: string option
    PublishedAt: DateTimeOffset option
    Author: string option
}
```

### ValidationError（判別共用体）

```fsharp
type ValidationError =
    | ParseError of message: string
    | MissingRequiredField of field: string
    | InvalidUrl of url: string
    | InvalidDate of value: string
    | TooManyItems of count: int * max: int
```

---

## 検証パイプライン

```
入力コンテンツ
    │
    ▼
┌─────────────────┐
│ detectFormat    │  フォーマット判定
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ parse           │  XML/JSON パース
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ validate        │  必須フィールド・URL検証
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ normalize       │  正規化（日付フォーマット等）
└────────┬────────┘
         │
         ▼
    NormalizedFeed
```

**F# コード**:
```fsharp
let validateFeed (content: string) : Result<NormalizedFeed, ValidationError list> =
    content
    |> detectFormat
    |> Result.bind parse
    |> Result.bind validate
    |> Result.bind normalize
```

---

## 依存関係

### NuGet パッケージ

| パッケージ | バージョン | 用途 |
|-----------|-----------|------|
| Giraffe | 7.0.0 | Web フレームワーク |
| FSharp.Data | 6.4.0 | XML/JSON パース |
| FsUnit.xUnit | latest | テスト |

### .fsproj 例

```xml
<Project Sdk="Microsoft.NET.Sdk.Web">
  <PropertyGroup>
    <TargetFramework>net10.0</TargetFramework>
  </PropertyGroup>

  <ItemGroup>
    <PackageReference Include="Giraffe" Version="7.0.0" />
    <PackageReference Include="FSharp.Data" Version="6.4.0" />
  </ItemGroup>

  <!-- F# はファイル順序が重要 -->
  <ItemGroup>
    <Compile Include="src/Domain/ValidationError.fs" />
    <Compile Include="src/Domain/Feed.fs" />
    <Compile Include="src/Usecase/Parser.fs" />
    <Compile Include="src/Usecase/Validator.fs" />
    <Compile Include="src/Usecase/Normalizer.fs" />
    <Compile Include="src/Gateway/HttpGateway.fs" />
    <Compile Include="src/Handler/Routes.fs" />
    <Compile Include="Program.fs" />
  </ItemGroup>
</Project>
```

---

## コマンド

### 開発

```bash
# プロジェクト作成（初回のみ）
dotnet new console -lang F# -n FeedValidator

# 依存関係追加
dotnet add package Giraffe
dotnet add package FSharp.Data

# ビルド
dotnet build

# 実行
dotnet run

# ウォッチモード（変更検知で自動再起動）
dotnet watch run
```

### テスト

```bash
# テストプロジェクト作成
dotnet new xunit -lang F# -n FeedValidator.Tests
cd FeedValidator.Tests
dotnet add reference ../FeedValidator/FeedValidator.fsproj
dotnet add package FsUnit.xUnit

# テスト実行
dotnet test

# 詳細出力
dotnet test --logger "console;verbosity=detailed"

# 特定テストのみ
dotnet test --filter "ValidatorTests"
```

### Docker

```bash
# ビルド
docker build -t feed-validator .

# 実行
docker run -p 9700:9700 feed-validator

# Compose で起動
docker compose -f compose/feed-validator.yaml -p alt up -d

# ログ
docker compose -f compose/feed-validator.yaml -p alt logs -f
```

---

## F# Tips

### ファイル順序

F# はファイルの順序が重要。依存される側を先に記述。

```
1. Domain/ValidationError.fs  ← 他から参照される
2. Domain/Feed.fs             ← ValidationError を使う
3. Usecase/Parser.fs          ← Domain を使う
4. ...
```

### パイプライン演算子

```fsharp
// ネスト（読みにくい）
let result = func3(func2(func1(x)))

// パイプライン（左から右へ）
let result = x |> func1 |> func2 |> func3
```

### Option と Result

```fsharp
// Option: 値があるかないか
let maybeValue: string option = Some "hello"
let noValue: string option = None

// Result: 成功か失敗か
let success: Result<int, string> = Ok 42
let failure: Result<int, string> = Error "something went wrong"
```

### パターンマッチ

```fsharp
match feedFormat with
| RSS2 -> parseRss content
| Atom -> parseAtom content
| JSONFeed -> parseJsonFeed content
| Unknown reason -> Error (ParseError reason)
```

---

## 統合（alt-backend）

**注意**: 以下は統合する場合の設計案であり、alt-backend 側にこのファイル・型は存在しない。
現状 alt-backend の feed 登録時バリデーションは `alt-backend/app/validation/feed_validator.go`
(`FeedRegistrationValidator`) が行っており、この F# サービスへの依存は無い。

### Port インターフェース (案、未実装)

```go
// 想定パス: alt-backend/app/port/feed_validator_port.go (未作成)
type FeedValidatorPort interface {
    ValidateFeed(ctx context.Context, content string) (*ValidationResult, error)
}
```

### Gateway 実装 (案、未実装)

```go
// 想定パス: alt-backend/app/gateway/feed_validator_gateway.go (未作成)
type FeedValidatorGateway struct {
    baseURL string
    client  *http.Client
}

func (g *FeedValidatorGateway) ValidateFeed(ctx context.Context, content string) (*ValidationResult, error) {
    resp, err := g.client.Post(g.baseURL+"/v1/validate", "application/xml", strings.NewReader(content))
    // ...
}
```

### 使用想定箇所 (未実装)

- feed 登録時の検証パス（alt-backend の実際のレイヤ構成は現行の Clean Architecture 実装を確認すること — `usecase/rss_usecase.go` という単一ファイルは現状存在しない）
- 既存の検証ロジックと並行運用可能（feature flag）という前提もこのサービスが実装されてからの話

---

## トラブルシューティング

### ビルドエラー: ファイル順序

```
error FS0039: The type 'FeedFormat' is not defined
```

→ `.fsproj` の `<Compile Include="..." />` の順序を確認。依存される側を先に。

### 型プロバイダーエラー

```
error FS3033: The type provider 'FSharp.Data.XmlProvider' reported an error
```

→ サンプルファイルのパスが正しいか確認。相対パスはプロジェクトルートから。

### Giraffe ルーティングが動かない

```fsharp
// NG: スラッシュなし
route "health"

// OK: スラッシュあり
route "/health"
```

---

## 参考資料

- [ROADMAP.md](./ROADMAP.md) - 学習ステップ詳細
- [F# for Fun and Profit](https://fsharpforfunandprofit.com/)
- [Giraffe Wiki](https://giraffe.wiki/)
- [FSharp.Data](https://fsprojects.github.io/FSharp.Data/)
