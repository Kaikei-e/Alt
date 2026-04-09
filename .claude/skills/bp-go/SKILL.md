---
name: bp-go
description: Go ベストプラクティス。Go コードの品質を保つための規約とパターン集。
  TRIGGER when: .go ファイルを編集・作成する時、Go コードを書く時、Go サービス（alt-backend, auth-hub, pre-processor, search-indexer, mq-hub, altctl）を実装する時。
  DO NOT TRIGGER when: テストの実行のみ、go.mod の確認のみ、ファイルの読み取りのみ、他言語の作業時。
---

# Go Best Practices

このスキルが発動したら、`docs/best_practices/go.md` を Read ツールで読み込み、
記載されたベストプラクティス（DECREE）に従ってコードを書くこと。

## 重要原則

1. **エラーラップ必須**: `fmt.Errorf("action: %w", err)` でコンテキスト付きラップ。裸の `return nil, err` 禁止
2. **main.go は薄く**: config 読込 → deps 接続 → handler 配線 → server 起動 → signal 待機。ビジネスロジック禁止
3. **context.Context は第一引数**: I/O を行う全関数で `ctx context.Context` を第一引数に。構造体フィールドに保持しない
4. **slog 構造化ログ**: `log` パッケージ不可。`slog.With("key", value)` でキー付きログ
5. **テーブル駆動テスト**: `[]struct{ name string; ... }` + `t.Run(tt.name, ...)` パターン。`testify/assert` 使用
6. **defer で解放**: `Close()`, `Unlock()`, `cancel()` は取得直後に `defer`
7. **internal/ パッケージ**: 公開 API でないものは `internal/` に配置

## 参照

完全なベストプラクティスは `docs/best_practices/go.md` を参照。
セクション: Project Structure, Error Handling, Concurrency, Context, Logging, Testing, Database, HTTP/API, Configuration
