---
name: language-conventions
description: Alt の言語別コーディング規約を Go / Python / Rust / Svelte 5 / TypeScript の 5 言語ぶん収録する。エラーハンドリング、型と所有権、並行・非同期、ログ、テスト、fail-fast 設定、無言フォールバック禁止といった言語ごとの決まりを `references/<lang>.md` に分けて持つ。`.go` `.py` `.rs` `.svelte` `.ts` `.tsx` を書く・直す・レビューするときに使う。ユーザが言語名や「規約」「ベストプラクティス」に触れなくても、Alt のサービス実装・修正に入るなら使う。実装をどの層（Handler / Usecase / Port / Gateway / Driver）に置くかだけが論点なら clean-architecture を使う。
paths:
  - "**/*.go"
  - "**/*.py"
  - "**/*.rs"
  - "**/*.svelte"
  - "**/*.ts"
  - "**/*.tsx"
---

# Alt Language Conventions

各言語の規約は下表の reference にある。**編集する言語の 1 ファイルだけを Read する**（他言語は読まない）。
書かれているのはタスク全体を通じて有効な standing rules であり、一度読んで終わる手順ではない。
該当言語のコードを書くたびに適用する。

| 言語 | 読むファイル | 適用対象 |
|---|---|---|
| Go | [references/go.md](references/go.md) | `*.go` — alt-backend (backend / harvester / datahub), auth-hub, pre-processor, search-indexer, mq-hub, rag-orchestrator, altctl |
| Python | [references/python.md](references/python.md) | `*.py` — news-creator, tag-generator, metrics, recap-subworker, recap-evaluator, acolyte-orchestrator |
| Rust | [references/rust.md](references/rust.md) | `*.rs` — rask-log-aggregator, rask-log-forwarder, recap-worker |
| Svelte | [references/svelte.md](references/svelte.md) | `*.svelte` と `alt-frontend-sv/src/routes/**/*.ts` |
| TypeScript | [references/typescript.md](references/typescript.md) | `*.ts` / `*.tsx` — alt-frontend-sv, auth-token-manager, alt-perf |

Svelte のルートや load 関数を触るときは svelte.md と typescript.md の両方が効く。

## さらに深い根拠が要るとき

各 reference の先頭に対応する `docs/best_practices/<lang>.md` を書いてある。数百〜千行あるので、
根拠やコード例が必要になった時点で**該当セクションだけ**を Read する。全文は読まない。
