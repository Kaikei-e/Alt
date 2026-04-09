---
name: bp-rust
description: Rust ベストプラクティス。Rust コードの品質を保つための規約とパターン集（Edition 2024）。
  TRIGGER when: .rs ファイルを編集・作成する時、Rust コードを書く時、Rust サービス（rask-log-aggregator, rask-log-forwarder, recap-worker）を実装する時。
  DO NOT TRIGGER when: テストの実行のみ、Cargo.toml の確認のみ、ファイルの読み取りのみ、他言語の作業時。
---

# Rust Best Practices

このスキルが発動したら、`docs/best_practices/rust.md` を Read ツールで読み込み、
記載されたベストプラクティス（DECREE）に従ってコードを書くこと。

## 重要原則

1. **Edition 2024**: `edition = "2024"` 必須。`unsafe extern` ブロック、RPIT lifetime capture の変更に注意
2. **thiserror でエラー型**: `#[derive(Debug, Error)]` でドメインエラー定義。`anyhow` はバイナリエントリポイントのみ
3. **pub(crate) デフォルト**: 公開 API でないものは `pub(crate)` に。`pub` は意図的な公開のみ
4. **借用優先**: `.clone()` を安易に使わない。`&str` > `String`、`&[T]` > `Vec<T>` を引数に
5. **tokio + tracing**: 非同期ランタイムは `tokio`、ログは `tracing` クレート。`println!` / `eprintln!` 禁止
6. **main.rs は薄く**: `lib.rs` でモジュール宣言、`main.rs` はサーバー起動 + graceful shutdown のみ
7. **match 網羅性**: `_` ワイルドカードより明示的なバリアント列挙。将来の追加を検出

## 参照

完全なベストプラクティスは `docs/best_practices/rust.md` を参照。
セクション: Edition 2024 Essentials, Project Structure, Error Handling, Ownership & Borrowing, Async, Testing, Database, Logging
