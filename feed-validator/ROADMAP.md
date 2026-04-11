# F# Feed Validator 学習ロードマップ

## 概要

このドキュメントは、F# 10 を学習しながら feed-validator サービスを実装するためのガイドです。

---

## Phase 1: 環境構築とHello World

### 目標
- .NET 10 SDK のインストール
- F# プロジェクトの作成
- 基本的なビルド・実行の確認

### 手順

#### 1.1 .NET 10 SDK インストール

```bash
# Ubuntu/Debian
wget https://dot.net/v1/dotnet-install.sh
chmod +x dotnet-install.sh
./dotnet-install.sh --channel 10.0

# または公式サイトからダウンロード
# https://dotnet.microsoft.com/download/dotnet/10.0
```

#### 1.2 プロジェクト作成

```bash
cd feed-validator
dotnet new console -lang F# -n FeedValidator
```

#### 1.3 生成されるファイル

```
FeedValidator/
├── FeedValidator.fsproj    # プロジェクト定義
└── Program.fs              # エントリーポイント
```

#### 1.4 初回実行

```bash
cd FeedValidator
dotnet run
# => Hello from F#
```

### 学習ポイント
- `.fsproj` ファイルの構造（C# の `.csproj` とほぼ同じ）
- F# はファイル順序が重要（依存関係順に並べる）
- `dotnet` CLI の基本コマンド

### 確認課題
- [ ] `dotnet --version` で 10.x が表示される
- [ ] `dotnet run` で Hello World が表示される
- [ ] `dotnet build` でビルドが成功する

---

## Phase 2: ドメインモデルの定義

### 目標
- 判別共用体（Discriminated Union）の理解
- レコード型の理解
- Option 型の使い方

### ファイル構成

```
src/
└── Domain/
    ├── Feed.fs           # フィード関連の型
    └── ValidationError.fs # エラー型
```

### 2.1 判別共用体（Union Types）

```fsharp
// Domain/Feed.fs

/// フィード形式を表す判別共用体
type FeedFormat =
    | RSS2        // RSS 2.0
    | Atom        // Atom 1.0
    | JSONFeed    // JSON Feed 1.1
    | Unknown of string  // 不明な形式（理由を保持）
```

**学習ポイント**:
- 各ケースは異なる「形」を持てる（`Unknown` は string を持つ）
- パターンマッチで全ケースを網羅的に処理
- TypeScript の Union Types に似ているが、より強力

### 2.2 レコード型

```fsharp
/// フィードアイテム（記事）
type FeedItem = {
    Title: string
    Link: string option        // 省略可能
    Description: string option
    PublishedAt: System.DateTimeOffset option
    Author: string option
}

/// 正規化されたフィード
type NormalizedFeed = {
    Format: FeedFormat
    Title: string
    Link: string
    Description: string option
    Items: FeedItem list
    Language: string option
}
```

**学習ポイント**:
- レコードは不変（immutable）がデフォルト
- `option` は null の代わり（Some x または None）
- `list` は F# のイミュータブルリスト

### 2.3 エラー型

```fsharp
// Domain/ValidationError.fs

type ValidationError =
    | ParseError of message: string
    | MissingRequiredField of field: string
    | InvalidUrl of url: string
    | InvalidDate of value: string
    | TooManyItems of count: int * max: int  // タプル
```

**学習ポイント**:
- 名前付きフィールド（`of message: string`）で可読性向上
- タプル（`int * int`）で複数値を保持
- これが「不正な状態を表現不能にする」の基礎

### 2.4 Result 型

```fsharp
/// 検証結果（標準ライブラリの Result を使用）
type ValidationResult = Result<NormalizedFeed, ValidationError list>

// 使用例
let success: ValidationResult = Ok normalizedFeed
let failure: ValidationResult = Error [MissingRequiredField "title"]
```

### 確認課題
- [ ] 各型を定義してコンパイルが通る
- [ ] F# Interactive (`dotnet fsi`) で型を試せる
- [ ] `FeedFormat.RSS2` と `FeedFormat.Unknown "invalid"` の違いを理解

### 参考資料
- [F# for Fun and Profit - Discriminated Unions](https://fsharpforfunandprofit.com/posts/discriminated-unions/)
- [F# for Fun and Profit - Records](https://fsharpforfunandprofit.com/posts/records/)

---

## Phase 3: パイプライン処理の実装

### 目標
- パイプライン演算子 `|>` の習得
- 高階関数（map, bind, filter）の理解
- Result 型でのエラーハンドリング

### ファイル構成

```
src/
└── Usecase/
    ├── Parser.fs         # パース処理
    ├── Validator.fs      # 検証ロジック
    └── Normalizer.fs     # 正規化処理
```

### 3.1 パイプライン演算子

```fsharp
// 従来の書き方（ネスト）
let result1 = normalize(validate(parse(content)))

// パイプライン（左から右へ読める）
let result2 =
    content
    |> parse
    |> validate
    |> normalize
```

### 3.2 Result との組み合わせ

```fsharp
module Usecase.Validator

open Domain.Feed
open Domain.ValidationError

/// フォーマット検出
let detectFormat (content: string) : FeedFormat =
    if content.TrimStart().StartsWith("<?xml") then
        if content.Contains("<rss") then RSS2
        elif content.Contains("<feed") then Atom
        else Unknown "XML but not RSS/Atom"
    elif content.TrimStart().StartsWith("{") then
        JSONFeed
    else
        Unknown "Unrecognized format"

/// タイトル検証
let validateTitle (feed: ParsedFeed) : Result<ParsedFeed, ValidationError> =
    if String.IsNullOrWhiteSpace(feed.Title) then
        Error (MissingRequiredField "title")
    else
        Ok feed

/// URL検証
let validateLink (feed: ParsedFeed) : Result<ParsedFeed, ValidationError> =
    match System.Uri.TryCreate(feed.Link, System.UriKind.Absolute) with
    | true, _ -> Ok feed
    | false, _ -> Error (InvalidUrl feed.Link)
```

### 3.3 Result.bind でチェーン

```fsharp
/// 検証パイプライン
let validate (parsed: ParsedFeed) : Result<ParsedFeed, ValidationError list> =
    parsed
    |> validateTitle
    |> Result.bind validateLink
    |> Result.bind validateItems
    |> Result.mapError List.singleton  // 単一エラーをリストに
```

**学習ポイント**:
- `Result.bind`: Ok なら次へ、Error ならそのまま返す
- `Result.map`: Ok の中身を変換
- `Result.mapError`: Error の中身を変換

### 3.4 複数エラーの収集（Applicative スタイル）

```fsharp
/// 複数の検証を並列実行し、全エラーを収集
let validateAll (feed: ParsedFeed) : Result<ParsedFeed, ValidationError list> =
    let errors =
        [ validateTitle feed
          validateLink feed
          validateItems feed ]
        |> List.choose (function Error e -> Some e | Ok _ -> None)

    if List.isEmpty errors then
        Ok feed
    else
        Error errors
```

### 確認課題
- [ ] `|>` を使って3段階以上のパイプラインを書ける
- [ ] `Result.bind` と `Result.map` の違いを説明できる
- [ ] エラーを収集するパターンを実装できる

### 参考資料
- [F# for Fun and Profit - Railway Oriented Programming](https://fsharpforfunandprofit.com/rop/)
- [F# for Fun and Profit - Map and Bind](https://fsharpforfunandprofit.com/posts/elevated-world/)

---

## Phase 4: XML/JSON パース

### 目標
- FSharp.Data を使った型プロバイダー
- XML パース（RSS/Atom）
- JSON パース

### 依存関係の追加

```xml
<!-- FeedValidator.fsproj に追加 -->
<ItemGroup>
  <PackageReference Include="FSharp.Data" Version="6.4.0" />
</ItemGroup>
```

```bash
dotnet add package FSharp.Data
```

### 4.1 型プロバイダーとは

F# の強力な機能。サンプルデータから型を自動生成。

```fsharp
open FSharp.Data

// サンプルXMLから型を生成
type RssFeed = XmlProvider<"samples/rss2.xml">

// 使用時は型安全にアクセス
let feed = RssFeed.Parse(content)
let title = feed.Channel.Title  // string型として推論
```

### 4.2 RSS 2.0 パース

```fsharp
module Usecase.Parser

open FSharp.Data
open Domain.Feed

// サンプルからRSS型を生成
type RssProvider = XmlProvider<"""
<rss version="2.0">
  <channel>
    <title>Sample</title>
    <link>https://example.com</link>
    <item>
      <title>Article</title>
      <link>https://example.com/1</link>
    </item>
  </channel>
</rss>
""">

let parseRss (content: string) : Result<NormalizedFeed, ValidationError> =
    try
        let rss = RssProvider.Parse(content)
        let items =
            rss.Channel.Items
            |> Array.map (fun item -> {
                Title = item.Title
                Link = item.Link |> Option.ofObj
                Description = item.Description |> Option.ofObj
                PublishedAt = None  // pubDate のパースは別途
                Author = None
            })
            |> Array.toList

        Ok {
            Format = RSS2
            Title = rss.Channel.Title
            Link = rss.Channel.Link
            Description = rss.Channel.Description |> Option.ofObj
            Items = items
            Language = rss.Channel.Language |> Option.ofObj
        }
    with ex ->
        Error (ParseError ex.Message)
```

### 4.3 Atom パース

```fsharp
type AtomProvider = XmlProvider<"samples/atom.xml">

let parseAtom (content: string) : Result<NormalizedFeed, ValidationError> =
    try
        let atom = AtomProvider.Parse(content)
        // ... Atom 固有の処理
        Ok { ... }
    with ex ->
        Error (ParseError ex.Message)
```

### 4.4 JSON Feed パース

```fsharp
open FSharp.Data

type JsonFeedProvider = JsonProvider<"samples/jsonfeed.json">

let parseJsonFeed (content: string) : Result<NormalizedFeed, ValidationError> =
    try
        let jf = JsonFeedProvider.Parse(content)
        Ok {
            Format = JSONFeed
            Title = jf.Title
            Link = jf.HomePageUrl
            Description = jf.Description |> Option.ofObj
            Items = jf.Items |> Array.map ... |> Array.toList
            Language = None
        }
    with ex ->
        Error (ParseError ex.Message)
```

### 確認課題
- [ ] FSharp.Data をインストールできる
- [ ] 型プロバイダーでサンプルから型を生成できる
- [ ] RSS/Atom/JSON Feed それぞれパースできる

### 参考資料
- [FSharp.Data Documentation](https://fsprojects.github.io/FSharp.Data/)
- [XML Type Provider](https://fsprojects.github.io/FSharp.Data/library/XmlProvider.html)

---

## Phase 5: Giraffe Web API

### 目標
- ASP.NET Core + Giraffe のセットアップ
- HttpHandler の理解
- JSON レスポンスの返却

### 依存関係

```xml
<ItemGroup>
  <PackageReference Include="Giraffe" Version="7.0.0" />
  <PackageReference Include="Microsoft.AspNetCore.App" />
</ItemGroup>
```

### 5.1 Giraffe の基本概念

```fsharp
// HttpHandler = HttpContext -> Task<HttpContext option>
// 成功時は Some ctx、失敗（次のハンドラへ）は None

let helloHandler : HttpHandler =
    fun next ctx ->
        task {
            return! text "Hello, World!" next ctx
        }
```

### 5.2 ルーティング

```fsharp
// Handler/Routes.fs

open Giraffe

let webApp : HttpHandler =
    choose [
        GET >=> route "/health" >=> healthHandler
        POST >=> route "/v1/validate" >=> validateHandler
        POST >=> route "/v1/validate/url" >=> validateUrlHandler
        RequestErrors.NOT_FOUND "Not Found"
    ]
```

**演算子の意味**:
- `>=>`: HttpHandler の合成（Kleisli composition）
- `choose`: 最初に成功したハンドラを使用

### 5.3 ハンドラ実装

```fsharp
open Giraffe
open Microsoft.AspNetCore.Http

/// ヘルスチェック
let healthHandler : HttpHandler =
    json {| status = "ok"; service = "feed-validator" |}

/// リクエストボディからフィードを検証
let validateHandler : HttpHandler =
    fun next ctx ->
        task {
            let! body = ctx.ReadBodyFromRequestAsync()
            let result = Usecase.Validator.validateFeed body

            match result with
            | Ok feed ->
                return! json feed next ctx
            | Error errors ->
                ctx.SetStatusCode 400
                return! json {| errors = errors |} next ctx
        }
```

### 5.4 Program.fs（エントリーポイント）

```fsharp
open Microsoft.AspNetCore.Builder
open Microsoft.Extensions.DependencyInjection
open Microsoft.Extensions.Hosting
open Giraffe

[<EntryPoint>]
let main args =
    let builder = WebApplication.CreateBuilder(args)

    builder.Services
        .AddGiraffe() |> ignore

    let app = builder.Build()

    app.UseGiraffe(Handler.Routes.webApp)

    app.Run()
    0
```

### 5.5 実行とテスト

```bash
# 起動
dotnet run

# 別ターミナルでテスト
curl http://localhost:5000/health

curl -X POST http://localhost:5000/v1/validate \
  -H "Content-Type: application/xml" \
  -d '<rss version="2.0"><channel><title>Test</title></channel></rss>'
```

### 確認課題
- [ ] Giraffe でサーバーが起動する
- [ ] `/health` エンドポイントが動作する
- [ ] `/v1/validate` でフィードを検証できる

### 参考資料
- [Giraffe Documentation](https://giraffe.wiki/)
- [Giraffe Samples](https://github.com/giraffe-fsharp/Giraffe/tree/master/samples)

---

## Phase 6: テスト

### 目標
- xUnit + FsUnit でユニットテスト
- TDD サイクル（Red → Green → Refactor）

### 依存関係

```bash
dotnet new xunit -lang F# -n FeedValidator.Tests
cd FeedValidator.Tests
dotnet add reference ../FeedValidator/FeedValidator.fsproj
dotnet add package FsUnit.xUnit
```

### 6.1 ドメインモデルのテスト

```fsharp
// tests/DomainTests.fs

module DomainTests

open Xunit
open FsUnit.Xunit
open Domain.Feed

[<Fact>]
let ``FeedFormat.RSS2 should be distinct from Atom`` () =
    RSS2 |> should not' (equal Atom)

[<Fact>]
let ``FeedItem with None values should be valid`` () =
    let item = {
        Title = "Test"
        Link = None
        Description = None
        PublishedAt = None
        Author = None
    }
    item.Title |> should equal "Test"
```

### 6.2 バリデーションのテスト

```fsharp
// tests/ValidatorTests.fs

module ValidatorTests

open Xunit
open FsUnit.Xunit
open Usecase.Validator
open Domain.ValidationError

[<Fact>]
let ``detectFormat should return RSS2 for valid RSS`` () =
    let content = """<?xml version="1.0"?><rss version="2.0">...</rss>"""
    detectFormat content |> should equal RSS2

[<Theory>]
[<InlineData("")>]
[<InlineData("   ")>]
let ``validateTitle should fail for empty title`` (title: string) =
    let feed = { Title = title; Link = "https://example.com"; Items = [] }
    validateTitle feed |> should be (ofCase <@ Error @>)

[<Fact>]
let ``validateLink should fail for invalid URL`` () =
    let feed = { Title = "Test"; Link = "not-a-url"; Items = [] }
    match validateLink feed with
    | Error (InvalidUrl url) -> url |> should equal "not-a-url"
    | _ -> failwith "Expected InvalidUrl error"
```

### 6.3 統合テスト

```fsharp
// tests/IntegrationTests.fs

module IntegrationTests

open Xunit
open FsUnit.Xunit
open System.IO

[<Fact>]
let ``Full validation pipeline should work for valid RSS`` () =
    let content = File.ReadAllText("../samples/rss2.xml")
    let result = Usecase.Validator.validateFeed content

    match result with
    | Ok feed ->
        feed.Format |> should equal RSS2
        feed.Items |> should not' (be Empty)
    | Error errors ->
        failwithf "Expected success but got errors: %A" errors
```

### 6.4 テスト実行

```bash
cd FeedValidator.Tests
dotnet test

# 詳細出力
dotnet test --logger "console;verbosity=detailed"

# 特定のテストのみ
dotnet test --filter "FullyQualifiedName~ValidatorTests"
```

### 確認課題
- [ ] テストプロジェクトが作成できる
- [ ] 全テストが通る
- [ ] TDD で新機能を追加できる

### 参考資料
- [FsUnit Documentation](https://fsprojects.github.io/FsUnit/)
- [Testing F# Code](https://fsharpforfunandprofit.com/posts/low-risk-ways-to-use-fsharp-at-work-3/)

---

## Phase 7: Docker 化

### 目標
- マルチステージビルド
- Docker Compose 統合

### 7.1 Dockerfile

```dockerfile
# Build stage
FROM mcr.microsoft.com/dotnet/sdk:10.0-alpine AS build
WORKDIR /src

# 依存関係の復元（キャッシュ効率化）
COPY *.fsproj .
RUN dotnet restore

# ビルド
COPY . .
RUN dotnet publish -c Release -o /app/publish

# Runtime stage
FROM mcr.microsoft.com/dotnet/aspnet:10.0-alpine
WORKDIR /app

# 非rootユーザーで実行
RUN adduser -D appuser
USER appuser

COPY --from=build /app/publish .
EXPOSE 9700

ENTRYPOINT ["dotnet", "FeedValidator.dll"]
```

### 7.2 compose/feed-validator.yaml

```yaml
services:
  feed-validator:
    build:
      context: ../feed-validator/FeedValidator
      dockerfile: Dockerfile
    container_name: alt-feed-validator
    environment:
      - ASPNETCORE_ENVIRONMENT=Production
      - ASPNETCORE_URLS=http://+:9700
    ports:
      - "9700:9700"
    networks:
      - alt-network
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:9700/health"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 10s

networks:
  alt-network:
    external: true
```

### 7.3 ビルドと実行

```bash
# ビルド
docker compose -f compose/feed-validator.yaml build

# 起動
docker compose -f compose/feed-validator.yaml -p alt up -d

# ログ確認
docker compose -f compose/feed-validator.yaml -p alt logs -f

# テスト
curl http://localhost:9700/health
```

### 確認課題
- [ ] Docker イメージがビルドできる
- [ ] コンテナが起動する
- [ ] ヘルスチェックが通る

---

## Phase 8: alt-backend 統合（将来）

### 目標
- Go からの HTTP 呼び出し
- Port/Gateway パターンでの統合

### 8.1 Port インターフェース

```go
// alt-backend/app/port/feed_validator_port.go

package port

type FeedValidatorPort interface {
    ValidateFeed(ctx context.Context, content string) (*ValidationResult, error)
    ValidateFeedURL(ctx context.Context, url string) (*ValidationResult, error)
}
```

### 8.2 Gateway 実装

```go
// alt-backend/app/gateway/feed_validator_gateway.go

type FeedValidatorGateway struct {
    baseURL string
    client  *http.Client
}

func (g *FeedValidatorGateway) ValidateFeed(ctx context.Context, content string) (*ValidationResult, error) {
    req, _ := http.NewRequestWithContext(ctx, "POST", g.baseURL+"/v1/validate", strings.NewReader(content))
    req.Header.Set("Content-Type", "application/xml")

    resp, err := g.client.Do(req)
    // ... レスポンス処理
}
```

---

## チェックリスト（全体）

### Phase 1: 環境構築
- [ ] .NET 10 SDK インストール
- [ ] F# プロジェクト作成
- [ ] Hello World 実行

### Phase 2: ドメインモデル
- [ ] FeedFormat 判別共用体
- [ ] FeedItem / NormalizedFeed レコード
- [ ] ValidationError 型
- [ ] ドメインテスト

### Phase 3: パイプライン
- [ ] パイプライン演算子 `|>` の使用
- [ ] Result 型でのエラーハンドリング
- [ ] 検証ロジックの実装

### Phase 4: パース
- [ ] FSharp.Data インストール
- [ ] RSS2 パーサー
- [ ] Atom パーサー
- [ ] JSON Feed パーサー

### Phase 5: Web API
- [ ] Giraffe セットアップ
- [ ] /health エンドポイント
- [ ] /v1/validate エンドポイント

### Phase 6: テスト
- [ ] xUnit + FsUnit セットアップ
- [ ] ドメインテスト
- [ ] ユースケーステスト
- [ ] 統合テスト

### Phase 7: Docker
- [ ] Dockerfile 作成
- [ ] Docker Compose 設定
- [ ] ローカル動作確認

### Phase 8: 統合（オプション）
- [ ] alt-backend Port/Gateway
- [ ] フィード登録フローへの組み込み

---

## 参考リンク

### F# 学習
- [F# for Fun and Profit](https://fsharpforfunandprofit.com/) - 必読
- [Domain Modeling Made Functional](https://pragprog.com/titles/swdddf/) - DDD + F#
- [F# Cheat Sheet](https://dungpa.github.io/fsharp-cheatsheet/)

### フレームワーク
- [Giraffe Wiki](https://giraffe.wiki/)
- [FSharp.Data](https://fsprojects.github.io/FSharp.Data/)

### F# 10 新機能
- [Introducing F# 10](https://devblogs.microsoft.com/dotnet/introducing-fsharp-10/)
- [What's new in F# 10](https://learn.microsoft.com/en-us/dotnet/fsharp/whats-new/fsharp-10)
