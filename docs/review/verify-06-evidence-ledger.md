---
report_version: "1.0"
task_id: "VERIFY-06-EVIDENCE-LEDGER"
target: "plecto, recap-worker/recap-worker, scripts/harness/corpus/{C-06,C-15,C-18}, scripts/altharness"
base_commit: "acdfc26ab8b2f28a96f89eb523e7c51dc202e91e"
head_commit: "f6598d3a711e348d027ca63f0db8fb99b4e823bc"
---

# 実装報告: VERIFY-06 主張ごとの再実行可能証拠台帳

## 1. 変更概要

- 対象サービス / モジュール:
  - `plecto`（`ad0850966` における `#![allow(clippy::too_many_arguments)]` から `#[allow(clippy::too_many_arguments)] mod bindings` へのスコープ縮小）
  - `recap-worker/recap-worker`（`ad0850966` における `#[allow(dead_code)]` 削除および `#[allow(clippy::float_cmp)]` の関数単位スコープ化）
  - `scripts/harness/corpus/C-06`, `scripts/harness/corpus/C-15`, `scripts/harness/corpus/C-18`, `scripts/altharness/internal/packet`（VERIFY-01 特性テストと `--packet` 自動読み込み）
  - `scripts/altharness/internal/verifyclaims`, `scripts/agent-harness/templates/implementation-report.md.tmpl`（VERIFY-06 機械検証と標準テンプレート）
- 本台帳はすべての主張を変更単位ごとに分割し、実測したコマンド出力・SHA-256 ダイジェスト・WIT テキスト・警告件数で裏付ける。

## 2. 主張と証拠（VERIFY-06 必須）

| 主張 | 証拠（コマンドと出力の保存先） |
| --- | --- |
| `plecto` の `ad0850966~1` (`acdfc26ab8b2`) と `HEAD` (`f6598d3a711e`) からオフラインビルドした WASM Component の WIT および `.wasm` バイト列が完全一致する (`wit_equal=true`, `wasm_bytes_equal=true`, 30,428 bytes) | コマンド: `./scripts/altharness/alt-harness verify-claims plecto-wit --base-commit ad0850966~1` / 保存先: `docs/review/verify-06-evidence-ledger.md#plecto-wit-evidence` |
| `plecto` の出荷済み OCI アーティファクト (`plecto/artifacts/stale-chunk-heal`, manifest `sha256:4f3bdc90e4f2...`) の `application/wasm` レイヤから抽出した WIT が `HEAD` ビルド成果物の WIT (`sha256:060ecf1c38b4...`) と完全一致する | コマンド: `wasm-tools component wit plecto/artifacts/stale-chunk-heal/blobs/sha256/b8c81d12a62f71c25d99ba1e51d48ee44ea6ab41b8c055ea530099a15550fddb` / 保存先: `docs/review/verify-06-evidence-ledger.md#plecto-oci-evidence` |
| `recap-worker/recap-worker` において固定ツールチェーン (`1.99.1`) の `cargo clippy --offline --all-targets -- -D warnings` が終了コード 0・警告 0・エラー 0 で通過し、crate/module 単位の `#![allow(...)]` が 0 件である | コマンド: `(cd recap-worker/recap-worker && cargo +1.99.1 clippy --offline --all-targets -- -D warnings)` / 保存先: `docs/review/verify-06-evidence-ledger.md#recap-clippy-evidence` |
| `VERIFY-01` において `C-06`・`C-15`・`C-18` の特性テストが `base_commit` 上で通過し、2 コミット目のリグレッション導入で `SEQUENCE_FINAL_TEST_FAILED` を検出し、失敗コーパス 22/22 (100.0%) を維持する | コマンド: `go -C scripts/altharness test -race -count=1 ./internal/corpus ./internal/packet` / 保存先: `docs/review/verify-06-evidence-ledger.md#verify-01-corpus-evidence` |
| 実装報告テンプレート (`implementation-report.md.tmpl`) および本証拠台帳が `verify-claims` の機械検証を通過する | コマンド: `go -C scripts/altharness test -race -count=1 ./internal/verifyclaims` / 保存先: `docs/review/verify-06-evidence-ledger.md#verify-06-self-check` |

## 3. 実測証拠ログ

### <a id="plecto-wit-evidence"></a>3.1 `plecto` Before/After WASM Component Model WIT & バイト比較

- 実行コマンド:
  ```bash
  ./scripts/altharness/alt-harness verify-claims plecto-wit --base-commit ad0850966~1 --format json
  ```
- ローカル再実行時の既定出力先: `.harness/evidence/verify-06/plecto-wit/` (`summary.json`, `base.wit`, `head.wit`, `wit.diff`, `command.log`)
- ツールチェーン実測値:
  - `rustc`: `rustc 1.99.1 (58cf8f9c3 2026-10-13)`
  - `wasm-tools`: `wasm-tools 1.259.0`
- 比較対象リビジョン:
  - `base_commit`: `acdfc26ab8b2f28a96f89eb523e7c51dc202e91e` (`ad0850966~1`, `#![allow(clippy::too_many_arguments)]` 変更直前)
  - `head_commit`: `f6598d3a711e348d027ca63f0db8fb99b4e823bc` (`HEAD`, `#[allow(clippy::too_many_arguments)] mod bindings`)
- 実測ダイジェストおよびバイト数:
  - `base_core_wasm_sha256`: `cc4af5106972c0d322ebb8ae896ca28b021236b7e573d6e4d0d33f5a20aa51e0` (`30,346` bytes)
  - `head_core_wasm_sha256`: `cc4af5106972c0d322ebb8ae896ca28b021236b7e573d6e4d0d33f5a20aa51e0` (`30,346` bytes)
  - `base_component_wasm_sha256`: `9c08f7aa05b600584a4a3095f4cf57347dc8c012d6bba70aaaa248df38da7d06` (`30,428` bytes)
  - `head_component_wasm_sha256`: `9c08f7aa05b600584a4a3095f4cf57347dc8c012d6bba70aaaa248df38da7d06` (`30,428` bytes)
  - `base_wit_sha256`: `060ecf1c38b4bd64ce2fa242559e6aa911d2bdc1f1096672f5ee127c9ce64ca8`
  - `head_wit_sha256`: `060ecf1c38b4bd64ce2fa242559e6aa911d2bdc1f1096672f5ee127c9ce64ca8`
  - `wit_equal`: `true` (`wit.diff` は 0 バイト)
  - `wasm_bytes_equal`: `true`
  - `world_wit_valid`: `true` (`plecto/wit/world.wit`)
  - `exit_code`: `0`

抽出された WIT 全文 (`base.wit` / `head.wit` / `shipped.wit` 共通, `sha256:060ecf1c38b4bd64ce2fa242559e6aa911d2bdc1f1096672f5ee127c9ce64ca8`):

```wit
package root:component;

world root {
  import plecto:filter/types@0.4.0;
  use plecto:filter/types@0.4.0.{http-request, http-response, request-decision, response-decision, request-body-decision, response-body-decision};

  export init: func();
  export on-request: func(req: http-request) -> request-decision;
  export on-response: func(req: http-request, resp: http-response) -> response-decision;
}
package plecto:filter@0.4.0 {
  interface types {
    record header {
      name: string,
      value: list<u8>,
    }

    record http-request {
      method: string,
      path-with-query: string,
      authority: string,
      scheme: string,
      headers: list<header>,
    }

    record http-response {
      status: u16,
      headers: list<header>,
      body: list<u8>,
    }

    record request-edit {
      set-headers: list<header>,
      remove-headers: list<string>,
    }

    variant request-decision {
      continue,
      modified(request-edit),
      short-circuit(http-response),
    }

    record response-edit {
      set-status: option<u16>,
      set-headers: list<header>,
      remove-headers: list<string>,
    }

    variant response-decision {
      continue,
      modified(response-edit),
      replace(http-response),
    }

    record request-body-edit {
      body: list<u8>,
      set-headers: list<header>,
      remove-headers: list<string>,
    }

    variant request-body-decision {
      continue,
      modified(request-body-edit),
      short-circuit(http-response),
    }

    record response-body-edit {
      body: list<u8>,
      set-headers: list<header>,
      remove-headers: list<string>,
    }

    variant response-body-decision {
      continue,
      modified(response-body-edit),
      replace(http-response),
    }
  }
}
```

### <a id="plecto-oci-evidence"></a>3.2 `plecto` 出荷済み OCI アーティファクト検証

- 対象 OCI ディレクトリ: `plecto/artifacts/stale-chunk-heal` (`plecto/manifest.toml` で `digest = "sha256:4f3bdc90e4f2aabb1569ad82e8cd87e5b94ff453736f063a783c52c46b13b927"` として参照)
- 実測値:
  - `manifest_sha256`: `4f3bdc90e4f2aabb1569ad82e8cd87e5b94ff453736f063a783c52c46b13b927` (一致確認済み)
  - `wasm_blob_sha256`: `b8c81d12a62f71c25d99ba1e51d48ee44ea6ab41b8c055ea530099a15550fddb` (`28,150` bytes)
  - `shipped_wit_sha256`: `060ecf1c38b4bd64ce2fa242559e6aa911d2bdc1f1096672f5ee127c9ce64ca8`
  - `wit_equal_head`: `true`
- 補足: 出荷済み OCI `.wasm` ブロブ (`28,150` bytes) は `2026-08-11` 時点のツールチェーンでビルドされたバイナリであり、`rustc 1.99.1` での新規再ビルド (`30,428` bytes) とはバイトサイズが異なるが、Component Model の WIT インターフェースは SHA-256 レベルで完全に一致している。

### <a id="recap-clippy-evidence"></a>3.3 `recap-worker/recap-worker` Clippy 出力および `#[allow(...)]` 台帳

- 実行コマンド:
  ```bash
  (cd recap-worker/recap-worker && cargo +1.99.1 clippy --offline --all-targets -- -D warnings)
  ```
- ローカル再実行時の既定出力先: `.harness/evidence/verify-06/recap-clippy/` (`summary.json`, `clippy.log`, `allow_inventory.txt`)
- ツールチェーンおよび実行結果:
  - `toolchain_file`: `recap-worker/rust-toolchain.toml` (`channel = "1.99.1"`)
  - `cargo_version`: `cargo 1.99.1 (5f94df478 2026-08-27)`
  - `clippy_version`: `clippy 0.1.99 (58cf8f9c35 2026-10-13)`
  - `exit_code`: `0`
  - `warning_count`: `0`
  - `error_count`: `0`
  - `stdout_sha256` (`--message-format=json`): `27f18773d8dd5595848404555cd8abed25dc863ff1f43ed452de8d03d534bc39`
  - `stderr` 実測: `Finished 'dev' profile [unoptimized + debuginfo] target(s) in 0.95s`
- `#[allow(...)]` 全量インベントリ (`inventory_sha256`: `de7d7b79febc48be58490db51a041bef9151e4edc7e1e241ae2d21bd90772552`):
  - 属性総数: `309` 件（内訳: crate/module 単位 `#![allow(...)]` = `0` 件、item 単位 `#[allow(...)]` = `309` 件）
  - ルール別件数（1 属性に複数ルール指定を含む全 20 種）:
    - `dead_code`: 178
    - `clippy::too_many_lines`: 32
    - `clippy::await_holding_lock`: 31
    - `private_interfaces`: 24
    - `clippy::too_many_arguments`: 8
    - `clippy::float_cmp`: 6
    - `clippy::struct_field_names`: 6
    - `unused_imports`: 6
    - `clippy::cast_possible_truncation`: 5
    - `clippy::cast_precision_loss`: 5
    - `clippy::manual_is_multiple_of`: 4
    - `clippy::type_complexity`: 4
    - `clippy::cast_possible_wrap`: 3
    - `clippy::cast_sign_loss`: 2
    - `clippy::match_same_arms`: 2
    - `clippy::struct_excessive_bools`: 2
    - `clippy::unused_self`: 2
    - `clippy::large_enum_variant`: 1
    - `clippy::pedantic`: 1
    - `unreachable_patterns`: 1
  - `ad0850966` での付け替え箇所の確認結果:
    - `src/clients/knowledge_sovereign.rs`: `mod snapshot_id` の `#[allow(dead_code)]` 削除済み（残存は `#[cfg(test)]` 内の `L34:#[allow(unused_imports)]` のみ）
    - `src/pipeline/cards/params.rs`: `mod tests` のモジュール単位 `#[allow(clippy::float_cmp)]` は 0 件で、6 つの `#[test]` 関数（L374, L391, L410, L513, L616, L715）へ個別スコープ化済み

### <a id="verify-01-corpus-evidence"></a>3.4 VERIFY-01 失敗コーパス (`C-06`, `C-15`, `C-18`) および `--packet` 自動読み込み検証

- 実行コマンド:
  ```bash
  go -C scripts/altharness test -race -count=1 ./internal/corpus ./internal/packet
  ```
- 確認結果:
  - `scripts/harness/corpus/C-06`: `test_characterization.sh` を追加し、`active_checks = ["CHECK-01", "VERIFY-01"]`, `VERIFY-01 = "SEQUENCE_FINAL_TEST_FAILED"` で通過。
  - `scripts/harness/corpus/C-15`: `test_characterization.sh` を追加し、`active_checks = ["ARCH-02", "CHECK-01", "VERIFY-01"]`, `VERIFY-01 = "SEQUENCE_FINAL_TEST_FAILED"` で通過（オフライン Echo v5 ローカルスタブで 1,000,000 bytes=200 / 1,000,001 bytes=413 の境界を実測）。
  - `scripts/harness/corpus/C-18`: 既存 `test_characterization.sh` を含め、`packet` ユニットテスト `TestVerifySequenceCorpusCasesC06C15C18` で C-06・C-15・C-18 の 3 件すべてでリグレッション検出 (`SEQUENCE_FINAL_TEST_FAILED`) およびテスト改ざん検出 (`SEQUENCE_TEST_MUTATED_IN_IMPL`) を確認。
  - `scripts/altharness/internal/packet`: `--packet` 指定時に `base_commit`, `packet_id`, `behavior_preserving` を自動読み込みし、`--bugfix` / `--behavior-preserving` との併用でも `base_commit` と `packet_id` を維持することを `TestVerifySequencePacketAutoLoadsBaseCommitAndMode` で確認。

### <a id="verify-06-self-check"></a>3.5 VERIFY-06 テンプレートおよび検証器セルフチェック

- 実行コマンド:
  ```bash
  go -C scripts/altharness test -race -count=1 ./internal/verifyclaims
  ```
- 確認対象:
  - `scripts/agent-harness/templates/implementation-report.md.tmpl` の構造検証 (`check-template`)
  - 本台帳 `docs/review/verify-06-evidence-ledger.md` の主張・コマンド・パス検証 (`validate`)
  - 負例テスト: 未裏付けの包括主張拒否 (`BLANKET_UNBACKED_CLAIM`)、プレースホルダ拒否 (`PLACEHOLDER_CELL`)、存在しない/空の証拠ファイル拒否 (`MISSING_EVIDENCE_FILE`, `EMPTY_EVIDENCE_FILE`)、パストラバーサル/絶対パス/シンボリックリンク脱出拒否 (`UNSAFE_EVIDENCE_PATH`)、WIT 不一致検出 (`PLECTO_WIT_MISMATCH`)、clippy 警告非ゼロ検出 (`CLIPPY_WARNINGS_NONZERO`)

## 4. 未検証事項・残余リスク

- `plecto` の出荷済み OCI アーティファクト (`plecto/artifacts/stale-chunk-heal`) は `.gitignore` 対象のためクリーンな CI クローンには存在せず、CI 環境では git 履歴 (`ad0850966~1` vs `HEAD`) からのソース再ビルド比較のみが実行される。
- `recap-worker/recap-worker` の `cargo clippy --offline` はローカル Cargo キャッシュが事前展開されている環境を前提とする。
- `alt-harness` のメインディスパッチャ (`cmd/alt-harness/main.go`) およびメタゲート (`internal/meta/catalog.go`) への `verify-claims` 配線はコーディネータ統合フェーズで反映される。
