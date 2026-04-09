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

## 参照

完全なベストプラクティスは `docs/best_practices/typescript.md` を参照。
セクション: Strict Configuration, Type Safety, Discriminated Unions, Error Handling, Async Patterns, Zod Validation, Module Design
