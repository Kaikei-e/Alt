# OWASP Top 10:2025 — Review Reference

公式出典: https://owasp.org/Top10/2025/ （2026 年 1 月公開、Web Application 向け最新版）

**注意:** これは 2021 版からの更新であり、Web App 全体向けの「2026 版」は存在しない。
一方、agentic / LLM コードには別建ての `agentic-top10-2026.md` を併用する。

## Contents
- A01:2025 Broken Access Control
- A02:2025 Security Misconfiguration
- A03:2025 Software Supply Chain Failures (new)
- A04:2025 Cryptographic Failures
- A05:2025 Injection
- A06:2025 Insecure Design
- A07:2025 Authentication Failures
- A08:2025 Software or Data Integrity Failures
- A09:2025 Security Logging and Alerting Failures
- A10:2025 Mishandling of Exceptional Conditions (new)

---

## A01:2025 Broken Access Control

**要点:** 認可の欠落・バイパス。依然としてランキング #1。

### アンチパターン
- クライアント側だけで権限を判定（サーバは素通し）
- IDOR（object reference を URL や body から取り、所有権確認なしで DB ヒット）
- デフォルト allow（許可リストでなく拒否リスト方式）
- 縦の昇格（一般ユーザーが admin 機能に到達）／横の昇格（別テナント・別ユーザーのデータ参照）
- JWT の `role` クレームを信用して再検証しない

### Review signals
```bash
# handler に認可チェックが入っているか
grep -rn "GetByID\|FindByID\|FindOne" --include='*.go' | grep -v '_test'
# 「現在のユーザー」と突き合わせているか
grep -rn "userID\|user_id\|current_user\|session\." --include='*.go'
# admin 判定の広がり
grep -rn "IsAdmin\|role ==\|hasRole" --include='*.{go,py,ts,rs}'
```

### CWE
CWE-284, CWE-285, CWE-639, CWE-862, CWE-863

### ASVS 参照
V8 Authorization

---

## A02:2025 Security Misconfiguration

**要点:** 2021 の #5 から上昇。テスト対象の 3% に観測。

### アンチパターン
- production で debug モード / stack trace 露出
- 過度に緩い CORS (`Access-Control-Allow-Origin: *` + credentials)
- デフォルト認証情報残存
- 不要なサービス / エンドポイントが稼働
- HTTP セキュリティヘッダー欠落（CSP, HSTS, X-Content-Type-Options, Referrer-Policy）
- cookie 属性欠落（HttpOnly, Secure, SameSite）

### Review signals
```bash
grep -rn "DEBUG\s*=\s*True\|debug:\s*true" --include='*.{py,yaml,toml,json}'
grep -rn "Access-Control-Allow-Origin" --include='*.{go,ts,py,rs}'
grep -rn "SameSite\|HttpOnly\|Secure:" --include='*.{go,ts}'
grep -rn "Content-Security-Policy\|Strict-Transport-Security" --include='*.{go,ts,conf,yaml}'
```

### CWE
CWE-16, CWE-260, CWE-315, CWE-614, CWE-732

### ASVS 参照
V14 Configuration, V50 Web Frontend Security

---

## A03:2025 Software Supply Chain Failures（新規）

**要点:** 2025 新設。サードパーティライブラリ、ビルドツール、パッケージマネージャ、CI/CD 経由の侵入リスク。

### アンチパターン
- lockfile 無し / drift したまま
- 依存を `latest` / `*` / 上限無し `^` で取得
- 無名義 / 無署名 tarball を直接取得
- GitHub URL 直指定で fork / 削除に弱い
- CI で使うアクションが未固定 commit SHA
- base image が `:latest`

### Review signals
```bash
# バージョン固定状況
cat go.sum package.json Cargo.lock uv.lock 2>/dev/null | head
grep -rn "latest\|master\|main" --include='Dockerfile' --include='*.yaml' | head
# CI actions が SHA pin か
grep -rn "uses:" .github/ | grep -v "@[a-f0-9]\{40\}"
```

### CWE
CWE-1357, CWE-494, CWE-829, CWE-1104

### ASVS 参照
V14.2 Dependency

---

## A04:2025 Cryptographic Failures

**要点:** 2021 の #2 から低下したが依然重要。

### アンチパターン
- 弱いアルゴリズム（MD5, SHA1 for password / signing, DES, RC4）
- 自前 crypto 実装
- 鍵の hardcode / リポ commit
- TLS 検証の無効化（`InsecureSkipVerify: true`, `verify=False`）
- `math/rand` / `Math.random()` をセキュリティ用途に使用
- パスワード保存が平文 / 単純ハッシュ（salt なし）

### Review signals
```bash
grep -rn "md5\|sha1\.New\|DES\|RC4" --include='*.{go,py,ts,rs}'
grep -rn "InsecureSkipVerify\s*:\s*true\|verify\s*=\s*False" --include='*.{go,py}'
grep -rn "math/rand" --include='*.go'
grep -rn "Math\.random()" --include='*.{ts,js}'
```

### CWE
CWE-261, CWE-296, CWE-310, CWE-326, CWE-327, CWE-331, CWE-326, CWE-798

### ASVS 参照
V11 Cryptography

---

## A05:2025 Injection

**要点:** SQL / NoSQL / OS command / LDAP / XPath / template / header injection。

### アンチパターン
- クエリを文字列連結 / f-string / `fmt.Sprintf`
- `os/exec.Command` に shell を噛ませる
- ORM でも raw SQL 部分に untrusted 入力
- HTML template を `text/template` で生成
- log / header / email に生入力を挿入

### Review signals
```bash
grep -rn 'Sprintf.*SELECT\|Sprintf.*INSERT\|Sprintf.*DELETE' --include='*.go'
grep -rn 'f".*SELECT\|".*SELECT.*{' --include='*.py'
grep -rn "shell=True\|exec\.Command.*sh" --include='*.{py,go}'
grep -rn "text/template" --include='*.go'
grep -rn "innerHTML\|{@html" --include='*.{ts,svelte}'
```

### CWE
CWE-20, CWE-74, CWE-75, CWE-77, CWE-78, CWE-79, CWE-89, CWE-94

### ASVS 参照
V5 Validation, Sanitization, Encoding

---

## A06:2025 Insecure Design

**要点:** 実装のバグではなく設計の抜け。

### アンチパターン
- rate limiting / captcha 無しの認証エンドポイント
- irreversible 操作（delete / transfer）の 2-step confirm 無し
- business logic の race condition を想定していない
- 想定されない workflow 順序（status を飛ばす / 戻せる）
- 全ユーザーに同一の秘密鍵 / 共有 session

### Review
設計レベルの質問を投げる（threat modeling / STRIDE）:

- この機能は spoofing / tampering / repudiation / information disclosure / DoS /
  elevation of privilege のどれに弱いか？
- 1 ユーザーが 1 秒に 1000 回叩いたら何が起きる？
- DB トランザクションは必要な粒度で張られているか？

### CWE
CWE-73, CWE-183, CWE-209, CWE-213, CWE-235, CWE-501, CWE-522, CWE-653, CWE-656, CWE-657

### ASVS 参照
V1 Architecture

---

## A07:2025 Authentication Failures

**要点:** credential stuffing / session 固定 / 弱い token。

### アンチパターン
- パスワード最小長 / 辞書チェック無し
- MFA 無し（特に admin アカウント）
- session token が短すぎる / 予測可能
- login 成功後に session ID を回さない（session fixation）
- password reset token に TTL 無し / 無効化されない
- rate limit / account lockout 無し

### Review signals
```bash
grep -rn "bcrypt\|argon2\|scrypt" --include='*.{go,py,ts,rs}'
grep -rn "SessionID\|SessionToken\|sessionid" --include='*.{go,ts}'
grep -rn "resetToken\|password_reset" --include='*.{go,ts,py}'
```

### CWE
CWE-287, CWE-297, CWE-384, CWE-521, CWE-613, CWE-620

### ASVS 参照
V6 Authentication, V7 Session Management

---

## A08:2025 Software or Data Integrity Failures

**要点:** 未検証な deserialization / update / artifact。

### アンチパターン
- `pickle.loads` / `yaml.load` / `ObjectInputStream` を untrusted データに使う
- auto-update が署名検証無し
- CI artifact が改竄検出なしで deploy
- CDN や外部スクリプトを SRI 無しで `<script src>`

### Review signals
```bash
grep -rn "pickle\.loads\|yaml\.load(" --include='*.py' | grep -v Loader=yaml.SafeLoader
grep -rn "ObjectInputStream\|readObject" --include='*.java'
grep -rn "integrity=" --include='*.html' --include='*.svelte'
```

### CWE
CWE-345, CWE-353, CWE-426, CWE-494, CWE-502, CWE-565, CWE-784, CWE-829

### ASVS 参照
V10 Malicious Code, V14.2 Dependency

---

## A09:2025 Security Logging and Alerting Failures

**要点:** 検知できない侵入は止められない。

### アンチパターン
- 認証失敗 / 認可拒否がログに残らない
- log に平文パスワード / トークン / PII
- log が user input を検証せず格納 → log injection / CRLF
- log が mutable（attacker が消せる / 上書きできる）
- 異常トラフィックに alert が無い

### Review signals
```bash
grep -rn "log\.\(Info\|Debug\|Error\)\|logger\.\(info\|error\)" --include='*.{go,py,ts}' | grep -i 'password\|token\|secret'
grep -rn "\\\\r\\\\n\|%0d%0a" --include='*.{go,py,ts}'
```

### CWE
CWE-117, CWE-223, CWE-532, CWE-778

### ASVS 参照
V16 Security Logging

---

## A10:2025 Mishandling of Exceptional Conditions（新規）

**要点:** 2025 新設。24 CWE を統合。例外時に fail-open する、握り潰す、制御フローが崩れる。

### アンチパターン
- `except Exception: pass` / 空の catch block
- error 時にデフォルト「成功」を返す
- panic / unwrap で全プロセスが落ちる（他リクエストを巻き込む）
- 認可チェック関数が error 時に allow を返す
- 例外時に transaction rollback し忘れ / lock 解放漏れ

### Review signals
```bash
grep -rn "except.*pass\|catch.*{}\|catch (_)" --include='*.{py,ts,js,rs}'
grep -rn "\.unwrap()\|\.expect(" --include='*.rs'
grep -rn "if err != nil { return nil }" --include='*.go'  # fail-open の疑い
```

### CWE
CWE-209, CWE-248, CWE-252, CWE-390, CWE-391, CWE-392, CWE-393, CWE-394, CWE-396, CWE-397, CWE-460, CWE-544, CWE-584, CWE-600, CWE-617, CWE-636, CWE-703, CWE-754, CWE-755

### ASVS 参照
V17 Error Handling
