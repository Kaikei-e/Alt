# OWASP ASVS 5.0 — Deep Check Reference

公式出典: https://github.com/OWASP/ASVS （5.0.0, 2025-05 リリース、現行最新）

**使い方:** OWASP Top 10 sweep で hit したカテゴリの精度を上げるために使う。
Finding には対応する ASVS 要件 ID（例: `V6.2.3`）を必ず付与する。

**取り扱い:** 本書は **要約抜粋**。正確な要件文・Level 1/2/3 の区分は上記 GitHub の
最新版 ASVS 本体（英語 PDF / CSV）を `WebFetch` して参照すること。

## Contents
- V1 Architecture, Design and Threat Modeling
- V2 Authentication
- V3 Session Management
- V4 Access Control
- V5 Validation, Sanitization and Encoding
- V6 Stored Cryptography
- V7 Error Handling and Logging
- V8 Data Protection
- V9 Communication
- V10 Malicious Code
- V11 Business Logic
- V12 Files and Resources
- V13 API and Web Service
- V14 Configuration
- V15 Secure Coding and Architecture
- V16 Security Logging and Error Handling
- V17 WebRTC（5.0 で新設）

---

## V1 Architecture, Design, Threat Modeling

- V1.1 Secure SDLC: 設計フェーズで threat modeling を行い、記録が残っているか
- V1.2 認証 / 認可 / 秘密管理が **集中制御** されているか（各ハンドラで re-implement していないか）
- V1.4 信頼境界が定義され、通過する全データに検証があるか

→ **A06:2025 Insecure Design** とセットで使う。

---

## V2 Authentication

- V2.1 パスワードは最低 12 文字、一般的辞書との照合
- V2.2 MFA が利用可能（admin / 高権限は必須）
- V2.3 credential stuffing 対策（rate limit, breached password check）
- V2.4 パスワードハッシュは `bcrypt` / `scrypt` / `argon2` / `PBKDF2`（salt 自動生成）
- V2.5 reset flow はランダム token + 有効期限 + 一度きり + 送信先確認
- V2.7 "forgot password" で user 存在を露呈しない

→ **A07:2025 Authentication Failures**

---

## V3 Session Management

- V3.1 session token は **サーバ側生成**、128 bit 以上のエントロピー
- V3.2 login 成功時に session ID を再生成（session fixation 対策）
- V3.3 logout / idle timeout / absolute timeout すべて実装
- V3.4 cookie は `HttpOnly; Secure; SameSite=Lax or Strict`
- V3.5 機微操作では再認証

→ **A07:2025**

---

## V4 Access Control

- V4.1 デフォルト **deny**、明示的に許可したものだけ通す
- V4.2 認可は **サーバ側で毎回** 評価（client hint を信用しない）
- V4.3 垂直 / 水平権限昇格のテストが仕込まれているか
- V4.2.1 IDOR 対策: 直接参照を **所有権 + 認可** で常に検証

→ **A01:2025 Broken Access Control**

---

## V5 Validation, Sanitization, Encoding

- V5.1 入力検証は **allowlist**（blocklist 禁止）
- V5.2 サニタイズは出力コンテキストに応じて切り替え（HTML / attribute / JS / URL / CSS）
- V5.3 SQL は必ず parameterized / prepared statement
- V5.4 OS command は引数配列渡し、シェルメタキャラを通さない
- V5.5 deserialization は型固定 + サイズ制限

→ **A05:2025 Injection**、**A08:2025 Integrity**

---

## V6 Stored Cryptography

- V6.1 鍵は KMS / vault に保管、コードに埋めない
- V6.2 対称暗号は AEAD（AES-GCM / ChaCha20-Poly1305）
- V6.3 非対称は RSA 2048+ / ECDSA P-256+ / Ed25519
- V6.4 乱数は暗号学的（`crypto/rand`, `secrets`, `getrandom`）
- V6.5 鍵ローテーション手順が存在

→ **A04:2025 Cryptographic Failures**

---

## V7 Error Handling and Logging（旧 ASVS）→ ASVS 5.0 では V16 に統合

（後述 V16 を参照）

---

## V8 Data Protection

- V8.1 PII の最小化（必要なものだけ保持）
- V8.2 retention policy が明記され、自動削除が動く
- V8.3 at-rest 暗号化（機微フィールドは column level も検討）
- V8.4 client 側ストレージ（localStorage / sessionStorage）に機微情報を置かない

---

## V9 Communication

- V9.1 すべての通信は TLS 1.2+（推奨 1.3）、弱い cipher 無効化
- V9.2 証明書検証を **必ず** 有効にする（`InsecureSkipVerify` / `verify=False` 禁止）
- V9.3 mTLS を内部サービス間で検討

→ **A04:2025**

---

## V10 Malicious Code

- V10.1 依存ライブラリは既知の典型 supply chain 攻撃に照らし評価済み
- V10.2 subresource integrity（SRI）を外部 script に適用
- V10.3 CI/CD で artifact が署名される

→ **A03:2025**、**A08:2025**

---

## V11 Business Logic

- V11.1 ワークフロー状態遷移を enforce（状態飛ばし / 戻しを拒否）
- V11.2 race condition（TOCTOU, 同時 click で 2 回処理）対策
- V11.3 rate limit / 経済的制限（支払回数, メッセージ送信数等）

→ **A06:2025**

---

## V12 Files and Resources

- V12.1 upload: 拡張子 **と** MIME **と** magic bytes の三点確認
- V12.2 upload パスは正規化後に allowed prefix 内に収まるか確認（path traversal 対策）
- V12.3 保存先と serving 先が分離（upload したファイルをそのまま script 実行可能な URL で返さない）
- V12.4 ZIP / tar bomb 対策（解凍時のサイズ上限）

---

## V13 API and Web Service

- V13.1 REST/gRPC でも `Content-Type` / schema 検証
- V13.2 GraphQL はクエリ深さ / 複雑さ上限、introspection の本番公開可否を判断
- V13.3 JWT は署名アルゴリズムの allowlist（特に `none` と `HS256/RS256` mix 混乱に注意）
- V13.4 API rate limit / quota

---

## V14 Configuration

- V14.1 本番に debug / stack trace 露出しない
- V14.2 依存バージョンの固定 + 脆弱性スキャン（→ A03）
- V14.3 HTTP security headers（CSP, HSTS, XFO, XCTO, Referrer-Policy, Permissions-Policy）
- V14.4 container / OS は最小権限、non-root で動く

→ **A02:2025**

---

## V15 Secure Coding and Architecture

- V15.1 入力 / 出力を汚染フロー（taint）で追跡する仕組みがあるか
- V15.2 機微操作は feature flag / kill switch を持つ
- V15.3 third-party コードは sandbox で実行するか、権限分離

---

## V16 Security Logging and Error Handling（ASVS 5.0）

- V16.1 認証失敗 / 認可拒否 / 機微操作を必ずログ
- V16.2 ログに機微情報（password, token, PII）を含めない
- V16.3 ログは append-only、改竄検出
- V16.4 例外時に fail-closed（認可関数は deny, transaction は rollback）
- V16.5 エラーメッセージにスタックトレース / 内部パスを返さない

→ **A09:2025**、**A10:2025**

---

## V17 WebRTC（ASVS 5.0 新設）

プロジェクトが WebRTC を使っていない場合はスキップ。Alt は該当しない想定。

---

## 使い方の流れ

1. Finding を書くとき、該当する **OWASP Top 10 カテゴリ** と **ASVS 要件 ID** を両方添える
2. 要件 ID を添えることで「OWASP 的に何を満たしていないか」が具体化される
3. 要件の正確な文面が必要な finding は本家 ASVS 5.0 CSV / PDF を `WebFetch` して引用する
