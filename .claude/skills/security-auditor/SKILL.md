---
name: security-auditor
description: |
  Audits code for security vulnerabilities using OWASP Top 10:2025, OWASP ASVS 5.0,
  the OWASP Secure Code Review Cheat Sheet, and (for AI/agentic code paths) the
  OWASP Top 10 for Agentic Applications 2026 as authoritative sources. Produces a
  structured findings report with severity, OWASP/CWE mapping, evidence, and
  concrete remediation. Use when the user asks for "security review", "脆弱性チェック",
  "セキュリティ監査", "OWASP レビュー", threat-modeling a diff/module, or when a
  change touches authentication, authorization, crypto, input validation,
  deserialization, file upload, SQL/NoSQL/command building, secrets, logging,
  error handling, dependency updates, or LLM/agent/RAG code paths.
user-invocable: true
allowed-tools: Read, Grep, Glob, Bash, WebFetch, WebSearch, Agent
argument-hint: <target path or PR> [--mode=baseline|diff] [--depth=shallow|deep]
---

# Security Auditor

OWASP 準拠のセキュリティ監査スキル。コードを Top 10:2025 / ASVS 5.0 / Secure Code
Review Cheat Sheet の観点で掃き、severity + 根拠 + 修正案を構造化レポートとして
返す。AI / agent / RAG 経路が含まれる場合は Agentic Top 10:2026 も補助適用する。

監査者としての原則:

- **根拠付きで語る** — 「危険」とだけ書かない。OWASP カテゴリ / CWE / ASVS 要件 ID を必ず添える
- **攻撃シナリオで示す** — 「何がどう悪用されるか」を 1-3 行で具体化
- **代替案まで出す** — "don't do X" で終わらず、"do Y instead" を必ず書く
- **誤検知を認める** — 不確実な finding は Info で出すか、Out of scope に明記
- **既存コードの慣行を尊重** — プロジェクト固有の設計（Clean Arch, BFF 必須等）は
  CLAUDE.md / memory 側の責務。このスキルは **汎用 OWASP フレーム** に徹する

## When to engage

| Mode | トリガー | 深さ |
|---|---|---|
| **Baseline audit** | サービス / モジュール全体のレビュー、新規実装の総点検、インシデント後の見直し | Deep（全 Phase） |
| **Diff audit** | PR レビュー、feature 完了時、変更行ピンポイント | Shallow（変更 hunk + 直接影響関数まで） |

`--mode` 指定がなければ、対象の広さから推定する。PR / 特定 commit 範囲なら diff、
ディレクトリ / サービス単位なら baseline。

## Phase 0: Scope intake

レビュー開始前に、次の 4 点を 1 段落で明文化する:

1. **Target** — 対象パス / PR 番号 / 影響サービス
2. **Language & framework** — Go / Rust / Python / TypeScript / Deno などと使用ライブラリ
3. **Trust boundaries** — 入力は誰から来るか（public internet / authenticated user /
   internal service / DB / LLM output）。それぞれの境界で何を検証すべきか
4. **Threat model assumptions** — 前提（例: "attacker is an authenticated low-privilege
   user"）と除外範囲

これを書けない場合、まず該当コードを Read / Glob で理解してから戻る。

## Phase 1: Review workflow

以下のチェックリストをコピーして、1 項目ずつ進めながらチェックしていく。

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

- HTTP handlers / gRPC endpoints / message consumers / CLI entry points を洗い出す
- 外部入力の到達経路を図示（mental model でよい）
- 何がどの信頼境界を跨ぐか明確化

確認例:

```bash
# Go handlers
grep -rn "func.*Handler\|func.*ServeHTTP\|e.GET\|e.POST" --include='*.go'
# TS/Svelte endpoints
grep -rn "export.*\(GET\|POST\|PUT\|DELETE\)\|+server\.ts" --include='*.ts'
# Python FastAPI
grep -rn "@router\.\|@app\." --include='*.py'
```

### Step 2: OWASP Top 10:2025 sweep

各カテゴリについて、詳細基準とシグナル grep パターンは `reference/owasp-top10-2025.md` を参照。

1. **A01 Broken Access Control** — IDOR、縦横の権限昇格、デフォルト allow、欠落したテナント境界
2. **A02 Security Misconfiguration** — debug モード残存、過度な CORS、デフォルト認証情報、verbose error
3. **A03 Software Supply Chain Failures** — 未固定の依存、署名検証欠落、typosquat、CI で使うツール
4. **A04 Cryptographic Failures** — 弱いアルゴリズム、自前 crypto、TLS 無効化、秘密のログ出し
5. **A05 Injection** — SQL/NoSQL/OS command/template/LDAP、未パラメータ化クエリ
6. **A06 Insecure Design** — 設計段階で対策が無い（rate limiting / 負荷制御 / 不可逆操作の確認）
7. **A07 Authentication Failures** — 弱いパスワード、欠落した MFA、session fixation、弱いトークン
8. **A08 Software or Data Integrity Failures** — 未検証の deserialization、無署名アップデート、CI 汚染
9. **A09 Security Logging and Alerting Failures** — 監査ログ欠落、機微情報のログ出し、ログ偽装可能
10. **A10 Mishandling of Exceptional Conditions** — fail-open、握り潰し、例外時に制御フローが崩れる

### Step 3: ASVS deep checks

認証 / 認可 / 暗号 / セッションは OWASP Top 10 のスキャンより精度を上げる必要がある。
`reference/asvs-v5-checklist.md` を使う。各 finding に ASVS 要件 ID（例: `V6.2.3`）を付与する。

### Step 4: Language-specific pitfalls

`reference/language-pitfalls.md` の grep 一覧を走らせる。各言語で最低以下は必ず見る:

- **Go** — `fmt.Sprintf.*SELECT`, `os/exec` with shell, `math/rand` 使用箇所, `html/template` vs `text/template`
- **Rust** — `unsafe {`, `.unwrap()` on untrusted input, raw SQL via `format!`, `Command::arg` injection
- **Python** — `shell=True`, `pickle.loads`, `yaml.load(`, `eval(`, `exec(`, f-string SQL
- **TypeScript / Svelte** — `{@html`, `innerHTML`, `eval(`, `dangerouslySetInnerHTML`, 秘密を含む `console.log`

### Step 5: Supply chain (A03)

対象サービスに応じて選択:

```bash
# Go
cd <service>/app && go list -m -u all

# Rust
cd <service>/app && cargo tree && cargo audit  # cargo audit があれば

# Python (uv)
cd <service>/app && uv pip list --outdated

# TypeScript (bun)
cd <service> && bun pm ls && bun audit  # 対応している場合
```

観点:

- version pinning（`^` / `~` の振れ幅）
- post-install script 実行の有無
- 最近追加された未知の package / GitHub URL 直指定
- lockfile の drift / 不整合

### Step 6: Agentic / LLM paths（該当時のみ）

対象が次を含むなら `reference/agentic-top10-2026.md` を適用する:

- LLM への prompt 組み立て（user input を含む）
- tool-use / function calling / MCP
- RAG retrieval → context 注入
- agent memory の永続化 / 再利用
- multi-agent / agent-to-agent 通信

Alt で該当しやすい箇所: `news-creator` の生成経路、`Acolyte` の tool-use、`rag-orchestrator`、
`tag-generator` の prompt、`Augur`/`MorningLetter` の chat。

### Step 7: Write report

次セクションのテンプレートに従う。

## Severity rubric

| Severity | 判定基準 |
|---|---|
| **Critical** | 認証不要 / 低スキルで data exfiltration, RCE, full account takeover。攻撃条件が揃っている |
| **High** | 条件付きで上記と同等。または機微データ露出・権限昇格が実装上明確 |
| **Medium** | 攻撃成立に追加条件が要る。または影響が限定的（単一ユーザー、少量データ） |
| **Low** | 防御の深さ（defense in depth）。単体では exploit 不能だが他脆弱性と連鎖すると問題 |
| **Info** | 観察・ハードニング提案。現時点で脆弱ではない |

判定式（ざっくり）: `severity ≈ exploitability × impact × exposure`

- exploitability: 必要な特権 / 前提条件の少なさ
- impact: data / integrity / availability への被害規模
- exposure: attack surface（internet 公開 / internal / 認証後のみ）

## Report template

レポートは **必ずこの構造で** 出力する:

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

#### F-002 [Severity] ...

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

Finding の並びは **Severity 降順**、同 severity 内は影響範囲降順。

## Guardrails

- **読み取り専用** — Read / Grep / Glob / Bash の read-only 用途と WebFetch のみ使う。
  コード・設定・依存関係を書き換えない
- **秘密情報は原文転記しない** — `.env` や config に秘密があっても、パスと「秘密がハードコードされている」事実を示すにとどめる
- **PoC exploit は作らない** — 脆弱性を指摘するのみ。動作する攻撃コードを生成しない
- **既知の false positive を認める** — 確信度が低い finding は `Info` または `Out of scope` に寄せる
- **出典の無い主張は書かない** — OWASP / CWE / ASVS のいずれかに紐付ける。紐付かない場合は「general best practice」と明示
- **Alt 固有ルールを再発明しない** — CLAUDE.md / memory にある "BFF 経由必須" "Atlas 必須" などはスキル側で
  重複定義しない。既存ガイドに委ねる

## Optional SAST tooling

使える場合は使う（必須ではない）:

- **Go**: `gosec ./...`, `staticcheck ./...`
- **Rust**: `cargo clippy -- -W clippy::pedantic`, `cargo audit`
- **Python**: `bandit -r .`, `pip-audit`
- **TypeScript**: `bun audit`（対応時）, `eslint-plugin-security`
- **汎用**: `semgrep --config=auto`

いずれも「補助」であり、手動レビューを置き換えるものではない。出力は参考情報としてレポートに添付。
