---
name: bp-typescript
description: TypeScript ベストプラクティス。型安全性とコード品質を保つための規約。
  TRIGGER when: .ts または .tsx ファイルを編集・作成する時、TypeScript コードを書く時、alt-frontend-sv / auth-token-manager / alt-perf を実装する時。
  DO NOT TRIGGER when: テストの実行のみ、tsconfig.json の確認のみ、ファイルの読み取りのみ、他言語の作業時。
---

# TypeScript Best Practices

このスキルが発動したら、`docs/best_practices/typescript.md` を Read ツールで読み込み、
記載されたベストプラクティス（DECREE）に従ってコードを書くこと。

## 重要原則

1. **strict: true + noUncheckedIndexedAccess**: 必須設定。弱めない
2. **境界では unknown**: 外部データ・API レスポンスは `unknown` で受け、型ガードで narrowing。`any` は最小限
3. **型ガード > 型アサーション**: `as` より type predicate (`value is T`) を優先。`!` 非 null アサーション禁止
4. **satisfies でリテラル推論保持**: `Record<string, string>` 等で型チェックしつつリテラル型を維持
5. **verbatimModuleSyntax**: `import type { T }` で型のみインポートを明示
6. **判別共用体 + exhaustiveness**: tagged union + `satisfies never` で網羅性チェック
7. **Zod でランタイムバリデーション**: API 境界は Zod スキーマで型とバリデーションを一元管理
8. **起動時 env 検証で fail-fast**: 必須 env（認証トークン等）欠落は Zod で throw。「認証なしで動き続ける」フォールバック禁止（→ `.claude/rules/di-wiring.md`）
9. **redirect パラメータは URL パースで検証**: 文字列先頭チェックは `//evil.com` で破られる
10. **connect-es エラーは numeric enum + ラップ前提**: `ConnectError.code` は数値 enum（string 比較は全エラーが default 行き）。native AbortError は ConnectError にラップされ `err.name` では捕まらない。エラーマッピングのテストは real ConnectError で書く
11. **wire スキーマは canonical 一本**: producer/consumer での型再宣言は wire drift の温床。生成型 / 共有スキーマを両側で import。protojson は zero-value field を JSON から省略するので受信側は default 前提で読む
12. **`split(sep, limit)` は残りを捨てる**: Go の `SplitN` と非互換。`=` 区切りのトークン分解は `indexOf` + `substring` で書く

## 参照

完全なベストプラクティスは `docs/best_practices/typescript.md` を参照。
セクション: Strict Configuration, Type Safety, Discriminated Unions, Error Handling, Async Patterns, Zod Validation, Module Design
