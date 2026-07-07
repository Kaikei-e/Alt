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
8. **エラー分岐は errors.Is/As**: `err.Error()` の文字列比較禁止
9. **http.Server は 4 タイムアウト明示**: `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout`。裸の `ListenAndServe` 禁止
10. **Redis Streams**: XACK は durable 書き込み後のみ。XREADGROUP には XAUTOCLAIM 回収ループが必須ペア（→ `.claude/rules/event-stream-consumer.md`）
11. **retry 中の裸 time.Sleep 禁止**: `select` + `ctx.Done()` + jitter 付き backoff
12. **fail-fast 設定**: 必須 config 欠落は起動失敗。無言 no-op / nil-guard フォールバック禁止（→ `.claude/rules/di-wiring.md`）
13. **DB 書き込みの silent success 禁止**: トランザクションは無条件 `defer tx.Rollback(ctx)`。行の存在を前提とする UPDATE は `rows_affected == 0` をエラーに
14. **time.Duration に untyped int 禁止**: `15 * 1000` は 15µs（ナノ秒解釈）。必ず `15 * time.Second` の単位定数を掛ける
15. **streaming はタイムアウト例外 + Accept-Encoding 手動設定禁止**: streaming server は `WriteTimeout: 0`、streaming client は `http.Client.Timeout: 0` + context deadline。`Accept-Encoding` を手動設定すると透過 gzip 解凍が無効化される

## 参照

完全なベストプラクティスは `docs/best_practices/go.md` を参照。
セクション: Project Structure, Error Handling, Concurrency, Context, Logging, Testing, Database, HTTP/API, Configuration
