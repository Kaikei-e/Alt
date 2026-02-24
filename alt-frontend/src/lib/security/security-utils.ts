/**
 * Security utilities for frontend authentication
 * Implements security best practices for the authentication system
 */

// Input validation patterns
export const ValidationPatterns = {
  email: /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/,
  password:
    /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)(?=.*[@$!%*?&])[A-Za-z\d@$!%*?&]{8,}$/,
  name: /^[a-zA-Z\s\u3040-\u309F\u30A0-\u30FF\u4E00-\u9FAF]{1,100}$/,
} as const;

// Security configuration
export const SecurityConfig = {
  maxLoginAttempts: 5,
  lockoutDuration: 15 * 60 * 1000, // 15 minutes
  sessionTimeout: 30 * 60 * 1000, // 30 minutes
  passwordMinLength: 20, // 🔧 Phase 7B: 8文字 → 20文字に変更
  csrfTokenHeader: "X-CSRF-Token",
} as const;

// Input sanitization
export function sanitizeInput(input: string): string {
  return input.trim().replace(/[<>"'&]/g, (match) => {
    const entityMap: Record<string, string> = {
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#x27;",
      "&": "&amp;",
    };
    return entityMap[match] || match;
  });
}

// Email validation
export function validateEmail(email: string): {
  isValid: boolean;
  error?: string;
} {
  const sanitized = sanitizeInput(email);

  if (!sanitized) {
    return { isValid: false, error: "メールアドレスを入力してください" };
  }

  if (sanitized.length > 254) {
    return { isValid: false, error: "メールアドレスが長すぎます" };
  }

  if (!ValidationPatterns.email.test(sanitized)) {
    return { isValid: false, error: "有効なメールアドレスを入力してください" };
  }

  return { isValid: true };
}

// Password validation - 🔧 Phase 7B: 20文字以上のシンプル判定のみ
export function validatePassword(password: string): {
  isValid: boolean;
  error?: string;
  strength: "weak" | "medium" | "strong";
} {
  if (!password) {
    return {
      isValid: false,
      error: "パスワードを入力してください",
      strength: "weak",
    };
  }

  // 🔧 Phase 7B: 20文字以上の長さチェックのみ
  if (password.length < SecurityConfig.passwordMinLength) {
    return {
      isValid: false,
      error: `パスワードは${SecurityConfig.passwordMinLength}文字以上である必要があります`,
      strength: "weak",
    };
  }

  // 上限チェックのみ残す（DoS攻撃対策）
  if (password.length > 128) {
    return {
      isValid: false,
      error: "パスワードが長すぎます（128文字以下にしてください）",
      strength: "weak",
    };
  }

  // 🔧 Phase 7B: 20文字以上なら強度問わず受け入れ
  // 長いパスワード = 強いとみなす（エントロピー理論に基づく）
  const strength: "strong" = "strong";

  return { isValid: true, strength };
}

// Name validation
export function validateName(name: string): {
  isValid: boolean;
  error?: string;
} {
  const sanitized = sanitizeInput(name);

  if (!sanitized) {
    return { isValid: false, error: "名前を入力してください" };
  }

  if (sanitized.length > 100) {
    return { isValid: false, error: "名前が長すぎます" };
  }

  if (!ValidationPatterns.name.test(sanitized)) {
    return { isValid: false, error: "有効な名前を入力してください" };
  }

  return { isValid: true };
}

// Rate limiting utilities
export class RateLimiter {
  private attempts: Map<
    string,
    { count: number; firstAttempt: number; lockedUntil?: number }
  > = new Map();

  constructor(
    private maxAttempts: number = SecurityConfig.maxLoginAttempts,
    private windowMs: number = 15 * 60 * 1000, // 15 minutes
    private lockoutMs: number = SecurityConfig.lockoutDuration,
  ) {}

  isBlocked(identifier: string): boolean {
    const record = this.attempts.get(identifier);
    if (!record) return false;

    // Check if lockout has expired
    if (record.lockedUntil && Date.now() > record.lockedUntil) {
      this.attempts.delete(identifier);
      return false;
    }

    return !!record.lockedUntil;
  }

  recordAttempt(identifier: string): {
    allowed: boolean;
    attemptsRemaining: number;
    resetTime?: number;
  } {
    const now = Date.now();
    const record = this.attempts.get(identifier);

    if (!record) {
      this.attempts.set(identifier, { count: 1, firstAttempt: now });
      return { allowed: true, attemptsRemaining: this.maxAttempts - 1 };
    }

    // Reset if window has expired
    if (now - record.firstAttempt > this.windowMs) {
      this.attempts.set(identifier, { count: 1, firstAttempt: now });
      return { allowed: true, attemptsRemaining: this.maxAttempts - 1 };
    }

    // Check if already locked
    if (record.lockedUntil && now < record.lockedUntil) {
      return {
        allowed: false,
        attemptsRemaining: 0,
        resetTime: record.lockedUntil,
      };
    }

    record.count++;

    // Lock if max attempts reached
    if (record.count >= this.maxAttempts) {
      record.lockedUntil = now + this.lockoutMs;
      return {
        allowed: false,
        attemptsRemaining: 0,
        resetTime: record.lockedUntil,
      };
    }

    return {
      allowed: true,
      attemptsRemaining: this.maxAttempts - record.count,
    };
  }

  reset(identifier: string): void {
    this.attempts.delete(identifier);
  }
}

// Secure storage utilities
export class SecureStorage {
  private static readonly prefix = "alt_auth_";

  static setItem(key: string, value: string, encrypt: boolean = true): void {
    try {
      const storageKey = SecureStorage.prefix + key;
      const storageValue = encrypt ? SecureStorage.encrypt(value) : value;
      localStorage.setItem(storageKey, storageValue);
    } catch (_error) {}
  }

  static getItem(key: string, decrypt: boolean = true): string | null {
    try {
      const storageKey = SecureStorage.prefix + key;
      const value = localStorage.getItem(storageKey);
      if (!value) return null;
      return decrypt ? SecureStorage.decrypt(value) : value;
    } catch (_error) {
      return null;
    }
  }

  static removeItem(key: string): void {
    try {
      const storageKey = SecureStorage.prefix + key;
      localStorage.removeItem(storageKey);
    } catch (_error) {}
  }

  static clearAll(): void {
    try {
      const keysToRemove = [];
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i);
        if (key?.startsWith(SecureStorage.prefix)) {
          keysToRemove.push(key);
        }
      }
      keysToRemove.forEach((key) => localStorage.removeItem(key));
    } catch (_error) {}
  }

  private static encrypt(value: string): string {
    // Simple base64 encoding (not cryptographically secure, but better than plain text)
    // In production, use proper encryption with a key derived from user session
    try {
      return btoa(encodeURIComponent(value));
    } catch (_error) {
      return value;
    }
  }

  private static decrypt(value: string): string {
    try {
      return decodeURIComponent(atob(value));
    } catch (_error) {
      return value;
    }
  }
}

// Content Security Policy helpers
export function generateNonce(): string {
  const array = new Uint8Array(16);
  crypto.getRandomValues(array);
  return Array.from(array, (byte) => byte.toString(16).padStart(2, "0")).join(
    "",
  );
}

// Security headers validation
export function validateSecurityHeaders(response: Response): boolean {
  const requiredHeaders = [
    "x-frame-options",
    "x-content-type-options",
    "x-xss-protection",
  ];

  return requiredHeaders.every((header) => response.headers.has(header));
}

// CSRF token management
export class CSRFManager {
  private static token: string | null = null;

  static setToken(token: string): void {
    CSRFManager.token = token;
    SecureStorage.setItem("csrf_token", token);
  }

  static getToken(): string | null {
    if (CSRFManager.token) return CSRFManager.token;

    CSRFManager.token = SecureStorage.getItem("csrf_token");
    return CSRFManager.token;
  }

  static clearToken(): void {
    CSRFManager.token = null;
    SecureStorage.removeItem("csrf_token");
  }

  static getHeaders(): Record<string, string> {
    const token = CSRFManager.getToken();
    return token ? { [SecurityConfig.csrfTokenHeader]: token } : {};
  }
}
