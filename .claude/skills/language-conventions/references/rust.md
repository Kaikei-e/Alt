# Rust Edition 2024 — Alt の規約

詳細な根拠とコード例は `docs/best_practices/rust.md`（17 セクション・1124 行）の該当セクションだけを Read する。
セクション: Edition 2024 Essentials, Project Structure, Error Handling, Ownership & Borrowing, Async, Testing, Database, Logging

## 重要原則

1. **Edition 2024**: `edition = "2024"` 必須。`unsafe extern` ブロック、RPIT lifetime capture の変更に注意
2. **thiserror でエラー型**: `#[derive(Debug, Error)]` でドメインエラー定義。`anyhow` はバイナリエントリポイントのみ
3. **pub(crate) デフォルト**: 公開 API でないものは `pub(crate)` に。`pub` は意図的な公開のみ
4. **借用優先**: `.clone()` を安易に使わない。`&str` > `String`、`&[T]` > `Vec<T>` を引数に
5. **tokio + tracing**: 非同期ランタイムは `tokio`、ログは `tracing` クレート。`println!` / `eprintln!` 禁止
6. **main.rs は薄く**: `lib.rs` でモジュール宣言、`main.rs` はサーバー起動 + graceful shutdown のみ
7. **match 網羅性**: `_` ワイルドカードより明示的なバリアント列挙。将来の追加を検出
8. **配線失敗を `.ok()` で握り潰さない**: コンストラクタ / DI の Result は `?` で起動失敗に。ダミー値・乱数フォールバックで成功を偽装しない（→ `.claude/rules/di-wiring.md`）
9. **flush タスクは fire-and-forget 禁止**: JoinHandle / TaskTracker を保持し shutdown で await。buffer の drain は書き込み成功後のみ
10. **文字列切り詰めは char boundary**: `&s[..n]` はマルチバイトで panic。`char_indices` を使う
11. **リトライ境界でエラー型を保持**: `anyhow::bail!` での文字列化は `downcast_ref` ベースのリトライ判定を殺す（429 がリトライ不能に）。ステータスコードを保持する thiserror 型で伝播（ADR-000390）
12. **「全部失敗」を「空の成功」にしない**: `Ok(_)` を成功と見なさず、結果オブジェクトの中身（genres_stored 等）を検査して JobOutcome を判定（ADR-000149）
13. **libtorch 常駐は jemalloc**: glibc malloc のフラグメンテーションで RSS 1.5-2 倍 → OOM kill。`unprefixed_malloc_on_supported_platforms` 付きでグローバルアロケータに（ADR-000547）
