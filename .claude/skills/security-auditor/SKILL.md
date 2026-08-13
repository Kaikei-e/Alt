---
name: security-auditor
description: |
  Audits code against OWASP Top 10:2025, ASVS 5.0 and — for AI/agent/RAG paths — the OWASP Top 10
  for Agentic Applications 2026, returning findings with severity, OWASP/CWE/ASVS mapping, evidence
  and remediation. Use when the user asks for a "security review", "脆弱性チェック", "セキュリティ監査",
  "OWASP レビュー", threat-modeling of a diff or module, or when a change touches authentication,
  authorization, crypto, input validation, deserialization, file upload, SQL/command building,
  secrets, logging, error handling, dependency updates, or LLM/agent/RAG paths. Prefer the bundled
  /security-review for a quick pass over the current branch's pending diff, and the security-reviewer
  subagent when only the findings should come back and the file reads would flood the conversation.
allowed-tools: Read, Grep, Glob, Bash, WebFetch, WebSearch, Agent
argument-hint: <target path or PR> [--mode=baseline|diff] [--depth=shallow|deep]
---

# Security Auditor

コードを OWASP Top 10:2025 / ASVS 5.0 / Secure Code Review Cheat Sheet の観点で掃き、
severity + 根拠 + 修正案を構造化レポートとして返す。AI / agent / RAG 経路が含まれる場合は
Agentic Top 10:2026 も併用する。

監査者としての原則:

- **根拠付きで語る** — 「危険」で終わらせず、OWASP カテゴリ / CWE / ASVS 要件 ID を添える
- **攻撃シナリオで示す** — 「何がどう悪用されるか」を 1-3 行で具体化する
- **代替案まで出す** — "don't do X" ではなく "do Y instead" を書く
- **誤検知を認める** — 確信度の低い finding は Info か Out of scope に寄せる
- **Alt 固有ルールを再発明しない** — "BFF 経由必須" "Atlas 必須" 等は CLAUDE.md / memory の責務。
  このスキルは汎用 OWASP フレームに徹する

参照ファイル（必要になった Step で読む）:

- [reference/owasp-top10-2025.md](reference/owasp-top10-2025.md) — Step 2。A01-A10 の判定基準と
  シグナル grep パターン
- [reference/asvs-v5-checklist.md](reference/asvs-v5-checklist.md) — Step 3。認証 / 認可 / 暗号 /
  セッションを深掘りするときの要件 ID 一覧
- [reference/language-pitfalls.md](reference/language-pitfalls.md) — Step 4。Go / Rust / Python /
  TypeScript / Deno の危険パターンを grep → なぜ危険 → 安全な書き方 の 3 点セットで収録
- [reference/agentic-top10-2026.md](reference/agentic-top10-2026.md) — Step 6。LLM / tool-use /
  RAG / agent memory を含むコードにだけ適用する ASI01-ASI10

## Mode

| Mode | トリガー | 深さ |
|---|---|---|
| **Baseline audit** | サービス / モジュール全体のレビュー、新規実装の総点検、インシデント後の見直し | Deep（全 Step） |
| **Diff audit** | PR レビュー、feature 完了時、変更行ピンポイント | Shallow（変更 hunk + 直接影響関数まで） |

`--mode` 指定がなければ対象の広さから推定する。PR / commit 範囲なら diff、ディレクトリ /
サービス単位なら baseline。

## Phase 0: Scope intake

レビュー開始前に次の 4 点を 1 段落で明文化する。書けないなら、まず該当コードを Read / Glob して
理解してから戻る。

1. **Target** — 対象パス / PR 番号 / 影響サービス
2. **Language & framework** — Go / Rust / Python / TypeScript / Deno と使用ライブラリ
3. **Trust boundaries** — 入力の出どころ（public internet / authenticated user / internal service /
   DB / LLM output）と、各境界で検証すべきもの
4. **Threat model assumptions** — 前提（例: "attacker is an authenticated low-privilege user"）と除外範囲

## Phase 1: Review workflow

次のチェックリストをコピーして、1 項目ずつ進めながらチェックする。

```
Security Audit Progress:
- [ ] Step 1: Map entry points and trust boundaries
- [ ] Step 2: Sweep OWASP Top 10:2025 (A01–A10) — reference/owasp-top10-2025.md
- [ ] Step 3: Deep-check auth / crypto / secrets (ASVS 5.0) — reference/asvs-v5-checklist.md
- [ ] Step 4: Language-specific pitfalls — reference/language-pitfalls.md
- [ ] Step 5: Supply chain (A03) — run dep audit for the stack
- [ ] Step 6: If AI/agent/RAG paths exist — apply ASI01–ASI10 — reference/agentic-top10-2026.md
- [ ] Step 7: Write the report (severity + OWASP mapping + remediation)
```

### Step 1: Entry points and trust boundaries

HTTP handler / gRPC endpoint / message consumer / CLI entry point を洗い出し、外部入力がどの信頼境界を
跨いで到達するかを追う。

```bash
grep -rn "func.*Handler\|func.*ServeHTTP\|e.GET\|e.POST" --include='*.go'      # Go
grep -rn "export.*\(GET\|POST\|PUT\|DELETE\)\|+server\.ts" --include='*.ts'    # TS / Svelte
grep -rn "@router\.\|@app\." --include='*.py'                                  # Python FastAPI
```

### Step 2: OWASP Top 10:2025 sweep

2021 版とは番号も内容も違う。2025 の並びは A01 Broken Access Control / A02 Security
Misconfiguration / A03 Software Supply Chain Failures (new) / A04 Cryptographic Failures /
A05 Injection / A06 Insecure Design / A07 Authentication Failures / A08 Software or Data Integrity
Failures / A09 Security Logging and Alerting Failures / A10 Mishandling of Exceptional Conditions
(new)。各カテゴリの判定基準と grep パターンは `reference/owasp-top10-2025.md`。

### Step 3: ASVS deep checks

認証 / 認可 / 暗号 / セッションは Top 10 のスキャンより精度を上げる。`reference/asvs-v5-checklist.md`
を使い、各 finding に ASVS 要件 ID（例: `V6.2.3`）を付与する。

### Step 4: Language-specific pitfalls

`reference/language-pitfalls.md` の grep 一覧を、対象言語について走らせる。

### Step 5: Supply chain (A03)

```bash
cd <service>/app && go list -m -u all                    # Go
cd <service>/app && cargo tree && cargo audit            # Rust
cd <service>/app && uv pip list --outdated               # Python (uv)
cd <service> && bun pm ls && bun audit                   # TypeScript (bun)
```

観点: version pinning の振れ幅（`^` / `~`）、post-install script、最近追加された未知の package や
GitHub URL 直指定、lockfile の drift。

### Step 6: Agentic / LLM paths（該当時のみ）

対象が prompt 組み立て（user input を含む）/ tool-use / function calling / MCP / RAG retrieval →
context 注入 / agent memory の永続化 / multi-agent 通信 のいずれかを含むなら
`reference/agentic-top10-2026.md` を適用する。Alt で該当しやすいのは `news-creator` の生成経路、
`Acolyte` の tool-use、`rag-orchestrator`、`tag-generator` の prompt、`Augur` / `MorningLetter` の chat。

## Severity rubric

`severity ≈ exploitability × impact × exposure`（必要な特権の少なさ × 被害規模 × attack surface）。

| Severity | 判定基準 |
|---|---|
| **Critical** | 認証不要 / 低スキルで data exfiltration, RCE, full account takeover。攻撃条件が揃っている |
| **High** | 条件付きで上記と同等。または機微データ露出・権限昇格が実装上明確 |
| **Medium** | 攻撃成立に追加条件が要る。または影響が限定的（単一ユーザー、少量データ） |
| **Low** | defense in depth。単体では exploit 不能だが他脆弱性と連鎖すると問題 |
| **Info** | 観察・ハードニング提案。現時点で脆弱ではない |

## Report template

Finding は severity 降順、同 severity 内は影響範囲降順で並べる。

```markdown
## Security Audit Report: <対象>

### Scope
- Target: <files / service / PR>
- Language & framework: <stack>
- Trust boundaries: <入力元と信頼境界>
- Threat model assumptions: <前提>
- Mode: baseline | diff
- Audit date: YYYY-MM-DD

### Summary
- Critical: N / High: N / Medium: N / Low: N / Info: N
- Top 3 actions: <最優先で着手すべき 1-3 件>

### Findings

#### F-001 [Severity] <1 行見出し>
- **OWASP**: A05:2025 Injection (CWE-89)
- **ASVS**: V5.3.4 Parameterized queries
- **Location**: path/to/file.ext:123
- **Evidence**:
  ```<lang>
  // problematic snippet
  ```
- **Why it's dangerous**: <攻撃シナリオ 1-3 行。何が読める/壊せる/乗っ取れるか>
- **Remediation**: <具体修正。コード例があれば添える>
- **References**: <Tier S 出典 URL>

### Positive observations
- <良い実装。真似を広めたいもの>

### Out of scope / Not verified
- <時間・権限・情報不足で確認できなかった領域>

### Sources
| # | Title | URL | Tier |
|---|---|---|---|
| 1 | OWASP Top 10:2025 | https://owasp.org/Top10/2025/ | S |
| 2 | OWASP ASVS 5.0 | https://github.com/OWASP/ASVS | S |
| 3 | OWASP Secure Code Review Cheat Sheet | https://cheatsheetseries.owasp.org/cheatsheets/Secure_Code_Review_Cheat_Sheet.html | S |
```

## Guardrails

- **読み取り専用で監査する** — Read / Grep / Glob / read-only な Bash と WebFetch のみ。コード・設定・
  依存関係は書き換えない
- **秘密情報を原文転記しない** — パスと「秘密がハードコードされている」事実の指摘にとどめる
- **PoC exploit を書かない** — 指摘のみ。動作する攻撃コードは生成しない
- **出典の無い主張を書かない** — OWASP / CWE / ASVS のいずれかに紐付ける。紐付かないものは
  「general best practice」と明示する

## Optional SAST tooling

使えるなら補助として走らせ、出力は参考情報としてレポートに添える（手動レビューの代替にはならない）:
Go `gosec ./...` / `staticcheck ./...`、Rust `cargo clippy -- -W clippy::pedantic` / `cargo audit`、
Python `bandit -r .` / `pip-audit`、TypeScript `bun audit` / `eslint-plugin-security`、汎用
`semgrep --config=auto`。
