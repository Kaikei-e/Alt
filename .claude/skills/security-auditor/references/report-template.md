# Security Audit Severity Rubric & Report Template

Load this reference at **Step 7** when grading findings and writing the structured audit report.

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
