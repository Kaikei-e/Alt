# adrdag

`docs/ADR` の supersedes DAG から**現在有効な意思決定（binding decisions）**を導出するミニマルな Go CLI。
`scripts/adr_graph.py` の意味論互換の後継ツール。

## 意味論（adr_graph.py と同一）

- `supersedes:` は**新しい ADR（置き換える側）だけ**が frontmatter に書く。逆辺（どの ADR に置き換えられたか）は常に**計算**され、決して手書きされない。
- **binding(A) ⇔ `status: accepted` ∧ inbound supersedes 辺なし**
- 旧 ADR の `status: superseded` は逆グラフの投影であり、`check` がドリフトを検出する。
- 読み取り専用: adrdag が ADR ファイルを書き換えることはない（唯一の書き込みは `graph --out` の使い捨て投影）。

## コマンド

```bash
adrdag --adr-dir docs/ADR check              # DAG 検証（サイクル/宙吊り参照/空スタブ/状態ドリフト）
adrdag --adr-dir docs/ADR binding            # 現在 binding な ADR 一覧（= 最新の意思決定セット）
adrdag --adr-dir docs/ADR resolve 000219     # 置換チェーンを辿って現在有効な後継 ADR へ
adrdag --adr-dir docs/ADR graph              # mermaid 描画（adr_graph.py とバイト互換）
adrdag --adr-dir docs/ADR graph --format dot # Graphviz DOT
adrdag --adr-dir docs/ADR graph --format json # node-link JSON（辺キーは "links"）
```

`--adr-dir` は環境変数 `ADRDAG_ADR_DIR` でも指定可能（フラグが優先）。
`check` / `resolve` / `binding` は `--format json` で機械可読出力に切り替わる。

## 終了コード

| code | 意味 |
|------|------|
| 0 | 成功（`check`: ERROR 所見ゼロ。WARN のみなら 0） |
| 1 | ドメイン失敗（`check` が ERROR 検出 / `resolve` の ID 不明） |
| 2 | 使用法エラー（引数・フラグ・`--format` 不正） |
| 3 | I/O エラー（`--adr-dir` 読み取り不可、`--out` 書き込み不可） |

## 設計上の決定事項

- **行ベース frontmatter パーサ**（厳密 YAML ではない）: 実コーパス 950 件中 18 件はバッククォートや `: ` を含む未クオートのスカラーを持ち、厳密な YAML としてはパース不能。正準の Python 実装と同じ行正規表現パーサを移植した。
- **DAG は手書き ~100 行**（グラフライブラリ不採用): ~1000 ノード・疎な辺では O(V+E) が数マイクロ秒であり、依存追加が正当化できない。
- **依存は cobra のみ**（+ 標準ライブラリ）。viper・yaml ライブラリ・testify は不採用。
- **mermaid 出力は adr_graph.py とバイト互換**。golden ファイルは Python 側で生成し、`make parity` が実コーパスで check/resolve/graph の完全一致を継続検証する。
- JSON グラフの辺キーは node-link 慣習に従い `"links"`（`"edges"` ではない。NetworkX のデフォルトキーに関する networkx/networkx#8611 を参照）。
- **CRLF 正規化**: Python の `Path.read_text()` は universal-newline 変換を行うため、Go 側もパース前に CRLF→LF 正規化する（敵対的レビューが検出した潜在パリティ欠陥の修正）。
- **循環コーパス上の `resolve`**: Python 版は RecursionError でクラッシュ（exit 1）するが、adrdag は「terminal ADR に到達しない」ことを明示エラーで報告して exit 1 する。exit 0 + 空出力で偽の成功を返さないための意図的な差分。

## テスト

```bash
make test          # 単体 + フィクスチャ + golden
make corpus-check  # 実コーパスの構造不変条件（マジックナンバー無し）
make parity        # python3 scripts/adr_graph.py との実コーパス差分ゼロ検証
make check-all
```
