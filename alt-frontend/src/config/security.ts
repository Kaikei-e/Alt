export interface SecurityConfig {
  contentSecurityPolicy: string;
  frameOptions: string;
  contentTypeOptions: string;
  referrerPolicy: string;
}

export function getSecurityConfig(nonce: string): SecurityConfig {
  const isDevelopment = process.env.NODE_ENV === "development";

  return {
    contentSecurityPolicy: buildCSPHeader(isDevelopment, nonce),
    frameOptions: "DENY",
    contentTypeOptions: "nosniff",
    referrerPolicy: "strict-origin-when-cross-origin",
  };
}

function buildCSPHeader(isDevelopment: boolean, nonce: string): string {
  const baseDirectives = [
    "default-src 'self'",
    "img-src 'self' blob: data: https:",
    "font-src 'self' data:",
    "frame-ancestors 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "upgrade-insecure-requests",
  ];

  // Next.js 15 + React 19 に最適化されたCSP設定
  const scriptDirectives = isDevelopment
    ? [
        "script-src 'self' 'unsafe-inline' 'unsafe-eval'", // 開発時のみ
        "script-src-elem 'self' 'unsafe-inline'",
      ]
    : [
        `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'`, // React 19のstrictモードに対応
        `script-src-elem 'self' 'nonce-${nonce}'`,
      ];

  // Chakra UI + Emotion に最適化されたスタイル設定
  const styleDirectives = [
    "style-src 'self' 'unsafe-inline'", // Emotion requires unsafe-inline
    "style-src-elem 'self' 'unsafe-inline'",
  ];

  // バックエンドAPIとの通信設定
  const connectDirectives = [
    `connect-src 'self' ${getBackendApiUrl()} ${getWebSocketUrl()}`,
    "report-uri /security/csp-report", // CSP違反レポート
  ];

  return [
    ...baseDirectives,
    ...scriptDirectives,
    ...styleDirectives,
    ...connectDirectives,
  ]
    .join("; ")
    .replace(/\s{2,}/g, " ")
    .trim();
}

function getBackendApiUrl(): string {
  return process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
}

function getWebSocketUrl(): string {
  return process.env.NEXT_PUBLIC_WS_URL || "ws://localhost:8080";
}

export function securityHeaders(nonce: string): Record<string, string> {
  const config = getSecurityConfig(nonce);

  return {
    "Content-Security-Policy": config.contentSecurityPolicy,
    "X-Frame-Options": config.frameOptions,
    "X-Content-Type-Options": config.contentTypeOptions,
    "Referrer-Policy": config.referrerPolicy,
    "X-XSS-Protection": "0", // 現代のブラウザーでは無効化が推奨
    "Strict-Transport-Security": "max-age=31536000; includeSubDomains; preload",
    "Permissions-Policy":
      "camera=(), microphone=(), geolocation=(), payment=()",
    "Cross-Origin-Opener-Policy": "same-origin",
    "Cross-Origin-Embedder-Policy": "unsafe-none", // COEP緩和: 外部画像フォールバック表示を可能にする
  };
}
