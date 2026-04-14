# Language-Specific Security Pitfalls

Alt のスタックに合わせた言語別の危険パターン集。各項目は **grep → なぜ危険 → 安全な書き方** の 3 点セット。

**出典ポリシー:** 各項目は OWASP Top 10:2025 / ASVS 5.0 / 公式言語ドキュメントに準拠。
公式 URL は項目末に添える。

## Contents
- Go 1.26+
- Rust 1.94+
- Python 3.14+
- TypeScript / Svelte / Bun
- Deno 2.x

---

## Go 1.26+

### SQL injection via string concat
```
grep -rn 'Sprintf.*SELECT\|Sprintf.*INSERT\|Sprintf.*UPDATE\|Sprintf.*DELETE\|"SELECT.*"+\|"INSERT.*"+' --include='*.go'
```
- **危険:** `db.Query(fmt.Sprintf("SELECT * FROM u WHERE id=%s", in))` は SQLi。
- **安全:** `db.Query("SELECT * FROM u WHERE id=$1", in)` / `sqlc` / `pgx` の prepared
- 参考: https://pkg.go.dev/database/sql

### Command injection
```
grep -rn 'exec\.Command.*"sh"\|exec\.Command.*"bash"\|exec\.Command.*"-c"' --include='*.go'
```
- **危険:** `exec.Command("sh", "-c", userInput)` は shell 経由で任意コマンド実行。
- **安全:** `exec.Command("git", "log", "--", userPath)` のように **引数配列渡し**、シェルを噛ませない。

### Weak randomness
```
grep -rn '"math/rand"' --include='*.go'
```
- **危険:** `math/rand` は予測可能。token / session ID に使えない。
- **安全:** `crypto/rand.Read(...)` を使う（token / nonce / salt / UUID v4）。

### HTML injection via text/template
```
grep -rn '"text/template"' --include='*.go'
```
- **危険:** HTML 出力に `text/template` を使うと自動エスケープされない → XSS。
- **安全:** `"html/template"` を使う。ユーザー入力を `template.HTML` でキャストしない。

### TLS verification disabled
```
grep -rn 'InsecureSkipVerify' --include='*.go'
```
- **危険:** 本番で `true` なら MITM 自由。
- **安全:** `false`（デフォルト）。テスト専用なら build tag で隔離。

### Error handling fail-open
```
grep -rn 'if err != nil { return nil }\|if err != nil { return true }' --include='*.go'
```
- **危険:** 認可チェック関数で err 時に allow を返すと fail-open（A10:2025）。
- **安全:** 認可系は **deny by default**、err 時は拒否。

---

## Rust 1.94+

### Unsafe blocks
```
grep -rn 'unsafe\s*{' --include='*.rs'
```
- **危険:** `unsafe` は UB / memory corruption の入り口。
- **安全:** `// SAFETY:` コメントで不変条件を記述必須。可能なら safe な抽象で包む。

### unwrap on untrusted input
```
grep -rn '\.unwrap()\|\.expect(' --include='*.rs'
```
- **危険:** untrusted 入力の `unwrap()` は panic で DoS。サービス全体が落ちる可能性。
- **安全:** `?` で伝播、`ok_or` で `Result` に変換、あるいは適切な default。

### SQL injection via format!
```
grep -rn 'format!.*SELECT\|format!.*INSERT\|format!.*UPDATE' --include='*.rs'
```
- **危険:** `format!("SELECT * FROM u WHERE id={}", in)` は SQLi。
- **安全:** `sqlx::query!("SELECT * FROM u WHERE id = $1", in)` のマクロでコンパイル時検証。

### Command injection
```
grep -rn 'Command::new.*"sh"\|Command::new.*"bash"' --include='*.rs'
```
- **危険:** shell 経由で `-c` を使うと入力が shell メタ文字経由で実行。
- **安全:** `Command::new("git").args(["log", "--", path])` で引数分離。

### Deserialization with serde `untagged`
- **危険:** `#[serde(untagged)]` enum は untrusted input で type confusion / resource exhaustion。
- **安全:** `#[serde(tag = "type")]` の tagged 形式、サイズ制限、deserialize_in_place の評価。

---

## Python 3.14+

### shell=True
```
grep -rn 'subprocess\.\(run\|Popen\|call\|check_output\).*shell\s*=\s*True' --include='*.py'
```
- **危険:** shell 経由でコマンド実行。入力に `;` `|` `$()` が通る。
- **安全:** `shell=False`（既定）、引数をリストで渡す。

### pickle / yaml.load / eval / exec
```
grep -rn 'pickle\.loads\|yaml\.load(\|eval(\|exec(' --include='*.py'
```
- **危険:**
  - `pickle.loads(untrusted)` → 任意コード実行（RCE）
  - `yaml.load(x)` は `Loader=yaml.SafeLoader` 無しだと RCE
  - `eval` / `exec` は原則禁止
- **安全:**
  - `json` / `msgpack`
  - `yaml.safe_load(x)`
  - `ast.literal_eval` に置換

### SQL via f-string
```
grep -rn 'f".*SELECT\|f".*INSERT\|f".*UPDATE\|f".*DELETE\|%\s*s.*execute' --include='*.py'
```
- **危険:** `cur.execute(f"SELECT * FROM u WHERE id={id}")` は SQLi。
- **安全:** `cur.execute("SELECT * FROM u WHERE id = %s", (id,))` パラメータ渡し。

### TLS verification disabled
```
grep -rn 'verify\s*=\s*False\|ssl\._create_unverified_context' --include='*.py'
```
- **危険:** `requests.get(url, verify=False)` は MITM 許容。
- **安全:** 既定 `True`。社内 CA なら `verify='/path/to/ca.pem'`。

### FastAPI router isolation
- **注意:** module-level の `APIRouter()` はテスト隔離で問題を起こすが、セキュリティ上も
  起動時に環境依存の状態を固定化する恐れ。`Depends()` で注入する設計に寄せる。

---

## TypeScript / Svelte / Bun

### XSS via {@html} and innerHTML
```
grep -rn '{@html\|innerHTML\s*=' --include='*.{svelte,ts,tsx,js,jsx}'
```
- **危険:** 未サニタイズな untrusted HTML を挿入すると XSS。
- **安全:** Svelte はデフォルトエスケープを使う。どうしても必要なら DOMPurify でサニタイズ。

### eval / Function()
```
grep -rn '\beval(\|new Function(' --include='*.{ts,tsx,js,jsx,svelte}'
```
- **危険:** 入力次第で RCE。
- **安全:** JSON.parse, 明示的な DSL パーサ。

### JWT verification skipped
```
grep -rn 'jwt\.decode\|verify\s*:\s*false' --include='*.{ts,js}'
```
- **危険:** `jwt.decode` は **署名を検証しない**。`jwt.verify(token, secret, {algorithms: ['RS256']})` を使う。
- **安全:** algorithms を明示 allowlist。`none` / HS vs RS の混同を防ぐ。

### Secrets in client bundle
```
grep -rn 'PUBLIC_\|NEXT_PUBLIC_\|VITE_' --include='*.{ts,svelte}' | grep -i 'secret\|key\|token\|password'
```
- **危険:** `PUBLIC_*` / `VITE_*` は client bundle に同梱される。秘密鍵を入れない。
- **安全:** server-only env（`$env/static/private` 等）を使う。

### Log leakage
```
grep -rn 'console\.log' --include='*.{ts,tsx,svelte}' | head
```
- **注意:** 本番で stacktrace / cookie / token を console 出力していないか確認。

---

## Deno 2.x

### Permission flags
- **危険:** `deno run --allow-all` を本番で使う。
- **安全:** `--allow-net=api.example.com --allow-read=./data` のように最小権限。

### Fetch without redirect control
```
grep -rn 'fetch(' --include='*.{ts,tsx}'
```
- **注意:** デフォルトで redirect follow する。SSRF を考慮する経路では `redirect: 'manual'` +
  destination allowlist を検討。

### File path traversal
```
grep -rn 'Deno\.\(readFile\|writeFile\)' --include='*.ts'
```
- **危険:** ユーザー入力パスを正規化せずに読み書きすると traversal。
- **安全:** `path.resolve` → allowed prefix プレフィックス比較。

---

## 使い方

1. 対象言語を `Glob` で特定
2. 上記 grep を回して hit を列挙
3. hit ごとに context を Read で確認
4. 誤検知を除いた残りを report の Findings に起票（OWASP カテゴリ + ASVS ID を必ず添える）

grep で hit しない = 安全ではない（書き方を変えれば同等に危険なので、パターンはあくまで入口）。
重要なのは **データフロー** と **信頼境界** の確認。
