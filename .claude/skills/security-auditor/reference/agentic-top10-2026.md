# OWASP Top 10 for Agentic Applications 2026 — Supplementary Reference

公式出典: https://genai.owasp.org/resource/owasp-top-10-for-agentic-applications-for-2026/

**重要:** これは Web App 全体向けの OWASP Top 10 の「2026 版」ではない。**OWASP Top 10 の Web App
向け最新版は依然として 2025**（`owasp-top10-2025.md`）。本書は **別建ての agentic / LLM 専用リスト**
で、2025 年末〜2026 年初に公開された。

**適用対象:** 以下のいずれかを含むコードだけに併用する。

- LLM への prompt 組み立て（user input / 外部文書 / tool 出力を含む）
- tool-use / function calling / MCP
- RAG retrieval → context 注入
- agent memory の永続化と再利用
- multi-agent / agent-to-agent 通信
- 自律的な副作用（mail / shell / API 呼び出し）を伴うエージェント

Alt で該当しやすい箇所:

- `news-creator` の生成経路
- `Acolyte` の tool-use（AIX 経由の Gemma4 26B 推論）
- `rag-orchestrator` の retrieval → context 注入
- `tag-generator` の prompt
- `Augur` / `MorningLetter` の chat
- MCP 連携（Obsidian / Drive 等）

## Contents
- ASI01 Agent Goal Hijack
- ASI02 Tool Misuse and Exploitation
- ASI03 Identity and Privilege Abuse
- ASI04 Agentic Supply Chain Vulnerabilities
- ASI05 Code Execution Abuse
- ASI06 Memory / Context Poisoning
- ASI07 Inter-Agent Communication Exploits
- ASI08 Cascading Failures / Loss of Control
- ASI09 Inadequate Audit / Traceability
- ASI10 Insecure Autonomy / Missing Human Oversight

各項目は公式ドキュメントをベースとした概要。詳細は上記 URL を `WebFetch` して参照すること。

---

## ASI01 Agent Goal Hijack

**要点:** Prompt injection の agentic 版。攻撃者が自然言語入力・文書・retrieved content に指示を仕込み、
エージェントの目標を静かに書き換える。

### Alt 上の該当例
- RSS feed 本文を retrieval して LLM に流す経路（`pre-processor` → `news-creator`）
- user からの chat 入力（Augur, MorningLetter, Acolyte）
- Obsidian / 外部文書を context に混ぜる経路

### レビュー観点
- trusted instruction（システム prompt）と untrusted context（RSS 本文 / user input / retrieved doc）が
  **構造的に分離**されているか（delimiter 区切り、role 分離、明示 tag）
- "ignore previous instructions" 的なパターンを検知する入口フィルタがあるか
- tool 呼び出し前に **"これはユーザーの真の意図か"** を再確認するステップ

---

## ASI02 Tool Misuse and Exploitation

**要点:** 与えた tool（mail / CRM / browser / DNS / internal API）を、**許可範囲内のまま** 破壊的に使う。
削除、大量送信、機微情報の外部送信など。

### レビュー観点
- tool allowlist は最小特権か（read-only で済むものに write を与えていない）
- 破壊的 tool（delete, send, transfer）は human-in-the-loop または dry-run を要求
- argument 値の検証（path traversal, SQL, URL schema の allowlist）

---

## ASI03 Identity and Privilege Abuse

**要点:** Agent が user session を継承、secret を使い回す、cross-agent で implicit trust が成立する。

### レビュー観点
- agent は user の trust を **委譲** して動くのか、**共有** して動くのか明示されているか
- 非人間 identity（NHI）の監査ログが存在するか
- token はエージェントごとにスコープされ、短寿命化されているか

---

## ASI04 Agentic Supply Chain Vulnerabilities

**要点:** 悪意 / 改竄されたモデル、ツール、プラグイン、MCP サーバ、prompt テンプレートから入る脆弱性。

### レビュー観点
- model / embedding が **信頼できるレジストリから** 取得されているか
- prompt テンプレートはバージョン管理され、改竄検知可能か
- MCP サーバは **許可されたホストのみ** と接続、未知の MCP が勝手に追加されないか
- plugin / tool のコードが review されているか

### 一般 OWASP Top 10 の A03:2025 と併せて見る

---

## ASI05 Code Execution Abuse

**要点:** Agent が code interpreter を持つ場合、生成コードが sandbox を抜ける / 内部リソースに到達。

### レビュー観点
- sandbox（gVisor / Firecracker / seccomp）は有効か
- 実行環境から internal DB / vault / metadata service（169.254.169.254）への route が封鎖されているか
- 生成コードの timeout / memory / CPU 上限

---

## ASI06 Memory / Context Poisoning

**要点:** Agent の長期メモリ / vector store / conversation history に毒を仕込み、後続セッションを操る。

### レビュー観点
- memory 書き込みは **サニタイズ + 署名 + retention policy**
- vector store の insert はロール制御、外部文書由来は trust タグで区別
- 「メモリをクリアして」という user 指示が agent の副作用でスキップされない

---

## ASI07 Inter-Agent Communication Exploits

**要点:** Multi-agent 構成で、1 つのエージェントが他を騙す / 感染させる。

### レビュー観点
- agent 間メッセージは署名 / schema 検証
- orchestrator は child agent の output を untrusted 扱い
- ループ検出（A が B に投げ、B が A に投げ返す無限連鎖）

---

## ASI08 Cascading Failures / Loss of Control

**要点:** 一部エージェントの異常が他に伝播し、全体が暴走 / 停止する。

### レビュー観点
- circuit breaker / rate limit / max depth
- kill switch（human override で全エージェントを即時停止）
- retry policy が exponential + jitter で DoS を自己発生させない

---

## ASI09 Inadequate Audit / Traceability

**要点:** agent が行った決定・tool 呼び出し・context 取り込みが追跡不能。事故後の原因究明ができない。

### レビュー観点
- tool 呼び出しごとに input / output / timestamp / agent identity を log
- log は tamper-evident（append-only / signed / externally shipped）
- prompt と生成結果のペアが保存されリプレイ可能

→ 一般 OWASP A09:2025 と併読

---

## ASI10 Insecure Autonomy / Missing Human Oversight

**要点:** 影響の大きい操作を human confirmation 無しで実行する設計。

### レビュー観点
- 不可逆操作（削除、送金、送信、外部公開）に confirmation step
- confidence 低い時に abstain する枠組み
- 操作の blast radius が事前に可視化されるか

---

## レポートでの扱い

Finding では OWASP 2025 と併用する形で書く。例:

```
- OWASP: ASI01 Agent Goal Hijack（+ A05:2025 Injection aspect）
- CWE: CWE-1039 (Inadequate Detection of Adversarial Inputs)
```

ASI 系 finding は **LLM 経路を含む場合のみ** 起票する。通常の HTTP ハンドラや DB アクセスを
ASI でラベル付けしない（Top 10:2025 に寄せる）。
