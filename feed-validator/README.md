# Feed Validator

F# 10 学習プロジェクト - RSS/Atom/JSON Feed 検証サービス

## ドキュメント構成

| ファイル | 内容 |
|----------|------|
| [ROADMAP.md](./ROADMAP.md) | 学習ロードマップ（Phase 1〜8） |
| [CLAUDE.md](./CLAUDE.md) | サービス仕様・API・アーキテクチャ |
| [FSHARP_REFERENCE.md](./FSHARP_REFERENCE.md) | F# クイックリファレンス |

## サンプルファイル

```
samples/
├── rss2.xml                    # 有効な RSS 2.0
├── atom.xml                    # 有効な Atom 1.0
├── jsonfeed.json               # 有効な JSON Feed 1.1
├── invalid_rss_missing_title.xml  # 無効: title 欠落
├── invalid_rss_bad_url.xml        # 無効: 不正なURL
└── invalid_malformed.xml          # 無効: 不正なXML構造
```

## クイックスタート

### 1. 環境準備

```bash
# .NET 10 SDK インストール確認
dotnet --version  # 10.x.x

# プロジェクト作成
dotnet new console -lang F# -n FeedValidator
cd FeedValidator

# 依存関係追加
dotnet add package Giraffe
dotnet add package FSharp.Data
```

### 2. 最初の一歩

```bash
# Hello World 実行
dotnet run

# F# Interactive で実験
dotnet fsi
```

### 3. 学習の進め方

1. **ROADMAP.md** の Phase 1 から順に進める
2. 各 Phase の「確認課題」をクリアしてから次へ
3. 詰まったら **FSHARP_REFERENCE.md** を参照
4. **samples/** のファイルでテスト

## 学習ポイント

### なぜ F# か

1. **型安全性**: 判別共用体で「不正な状態を表現不能に」
2. **パイプライン**: `|>` でデータ変換を左から右へ読める
3. **イミュータブル**: デフォルト不変でバグを減らす
4. **簡潔**: 少ないコードで意図を明確に表現

### このプロジェクトで学べること

| 概念 | 適用箇所 |
|------|----------|
| 判別共用体 | FeedFormat, ValidationError |
| レコード型 | FeedItem, NormalizedFeed |
| Option 型 | 省略可能フィールド |
| Result 型 | 検証結果 |
| パイプライン | 検証フロー |
| 型プロバイダー | XML/JSON パース |
| async/task | HTTP 処理 |

## 参考資料

### 必読
- [F# for Fun and Profit](https://fsharpforfunandprofit.com/) - 総合学習サイト
- [Domain Modeling Made Functional](https://pragprog.com/titles/swdddf/) - DDD + F# 書籍

### 公式
- [F# Documentation](https://learn.microsoft.com/en-us/dotnet/fsharp/)
- [What's new in F# 10](https://learn.microsoft.com/en-us/dotnet/fsharp/whats-new/fsharp-10)

### フレームワーク
- [Giraffe](https://giraffe.wiki/) - Web フレームワーク
- [FSharp.Data](https://fsprojects.github.io/FSharp.Data/) - 型プロバイダー
