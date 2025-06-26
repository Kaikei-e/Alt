# /fix-rust-types
Run `cargo check --message-format=short`.
Analyze the compiler errors and modify only what is necessary
to eliminate type, lifetime, and borrow checker issues.
Do not add new features; keep diffs minimal (<120 tokens).