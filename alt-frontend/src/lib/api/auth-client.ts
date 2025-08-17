import type { User, LoginFlow, RegistrationFlow, UserPreferences } from '@/types/auth';

export class AuthAPIClient {
  private baseURL: string;
  private debugMode: boolean;
  private requestId: number;
  private idpOrigin: string;

  constructor() {
    // Use relative API proxy endpoints for secure HTTPS communication
    // This avoids mixed content issues and keeps internal URLs secure
    this.baseURL = '/api/auth';
    this.debugMode = process.env.NODE_ENV === 'development';
    this.requestId = 0;
    // TODO.md要件: Kratos 公開URL直接アクセス用
    this.idpOrigin = process.env.NEXT_PUBLIC_IDP_ORIGIN ?? 'https://id.curionoah.com';
    
    // TODO.md 手順0: 配信中のバンドルの値を確認
    console.log('[AUTH-CLIENT] IDP_ORIGIN =', this.idpOrigin);
    console.log('[AUTH-CLIENT] NEXT_PUBLIC_IDP_ORIGIN =', process.env.NEXT_PUBLIC_IDP_ORIGIN);
  }

  // 接続テスト機能追加 (X1.md 1.3.2 実装)
  async testConnection(): Promise<boolean> {
    try {
      const response = await fetch(`${this.baseURL}`, {
        method: 'GET',
        signal: AbortSignal.timeout(5000)
      });
      return response.ok;
    } catch {
      return false;
    }
  }

  async initiateLogin(): Promise<LoginFlow> {
    // TODO.md要件: 初期化はKratos直接アクセス (CSRFクッキー受け取り用)
    const res = await fetch(`${this.idpOrigin}/self-service/login/browser`, {
      method: 'GET',                      // ブラウザフロー初期化は GET
      credentials: 'include',             // ← CSRF Cookie を受け取る
      headers: { Accept: 'application/json' },
    });
    if (!res.ok) throw new Error(`Failed to init login: ${res.status}`);
    return await res.json() as LoginFlow; // ui.action / ui.nodes を含む
  }

  async completeLogin(flowId: string, email: string, password: string): Promise<User> {
    return this.loginWithBrowserFlow(flowId, email, password);
  }

  // 🚨 LEGACY: X22 Auto-retry login (replaced by X27 Browser Flow)
  // Kept for backward compatibility - will be removed in future versions
  private async loginWithRetry(flowId: string, email: string, password: string, maxRetries: number = 2): Promise<User> {
    console.warn('⚠️ DEPRECATED: loginWithRetry is deprecated, using Browser Flow instead');
    return this.loginWithBrowserFlow(flowId, email, password);
  }

  // 🚨 CRITICAL: X22 Phase 1 - CSRF error detection
  private isCSRFError(error: unknown): boolean {
    if (!(error instanceof Error)) return false;

    const message = error.message.toLowerCase();
    return message.includes('csrf') ||
           message.includes('token') ||
           message.includes('400') ||
           message.includes('500') ||
           message.includes('forbidden');
  }

  // 🚀 X27 Browser Flow Methods - Ory Kratos Compliance

  // TODO.md compliant Browser Flow login method
  private async loginWithBrowserFlow(_: string, email: string, password: string): Promise<User> {
    console.log('🚀 Starting TODO.md compliant Browser Flow login...');

    try {
      // 1) 初期化（直）- TODO.md要件: /api経由の初期化をやめる
      const flow = await this.initiateLogin();
      const action = flow.ui?.action;
      if (!action) throw new Error('Login flow missing ui.action');

      const csrf = flow.ui.nodes.find(n => n.attributes?.name === 'csrf_token')?.attributes?.value;
      if (!csrf) throw new Error('CSRF token not found in flow');

      // 2) 完了（ui.action へフォームPOST）
      const form = new URLSearchParams();
      form.set('method', 'password');
      form.set('identifier', email.trim().toLowerCase());
      form.set('password', password.trim());
      form.set('csrf_token', csrf);

      // TODO.md 手順0: POSTが本当に走っているかを可視化
      console.log('[AUTH-CLIENT] POST to action:', action, { origin: new URL(action).origin });
      const resp = await fetch(action, {
        method: 'POST',
        credentials: 'include',             // ← Cookie 同送
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/x-www-form-urlencoded' // プリフライト不要のまま
        },
        body: form,                         // ← JSON禁止（公式推奨）
      });
      console.log('[AUTH-CLIENT] action POST status =', resp.status);
      if (!resp.ok) throw new Error(`Login failed: ${resp.status} ${await resp.text()}`);
      return await resp.json() as User;

    } catch (error) {
      console.error('❌ [AUTH-CLIENT] TODO.md compliant Login FAILED:', error);
      throw error;
    }
  }



  async initiateRegistration(): Promise<RegistrationFlow> {
    // TODO.md要件: 初期化はKratos直接アクセス
    const res = await fetch(`${this.idpOrigin}/self-service/registration/browser`, {
      method: 'GET',
      credentials: 'include',
      headers: { Accept: 'application/json' },
    });
    if (!res.ok) throw new Error(`Failed to init registration: ${res.status}`);
    return await res.json() as RegistrationFlow;
  }

  async completeRegistration(flowId: string, email: string, password: string, name?: string): Promise<User> {
    // Basic validation
    if (!email || !email.includes('@')) {
      throw new Error('Valid email address is required');
    }
    if (!password || password.length < 8) {
      throw new Error('Password must be at least 8 characters');
    }

    try {
      const flow = await this.initiateRegistration();
      const action = flow.ui?.action;
      if (!action) throw new Error('Registration flow missing ui.action');

      const csrf = flow.ui.nodes.find(n => n.attributes?.name === 'csrf_token')?.attributes?.value;
      if (!csrf) throw new Error('CSRF token not found in flow');

      const form = new URLSearchParams();
      form.set('method', 'password');
      form.set('traits.email', email.trim().toLowerCase());
      form.set('password', password.trim());
      form.set('csrf_token', csrf);
      if (name?.trim()) {
        const [first, ...rest] = name.trim().split(/\s+/);
        form.set('traits.name.first', first ?? '');
        if (rest.length) form.set('traits.name.last', rest.join(' '));
      }

      // TODO.md 手順0: POSTが本当に走っているかを可視化
      console.log('[AUTH-CLIENT] POST to action:', action, { origin: new URL(action).origin });
      const resp = await fetch(action, {
        method: 'POST',
        credentials: 'include',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/x-www-form-urlencoded'
        },
        body: form,
      });
      console.log('[AUTH-CLIENT] action POST status =', resp.status);
      if (!resp.ok) throw new Error(`Registration failed: ${resp.status} ${await resp.text()}`);
      return await resp.json() as User;

    } catch (error) {
      console.error('❌ [AUTH-CLIENT] Registration FAILED:', error);
      throw error;
    }
  }

  async logout(): Promise<void> {
    await this.makeRequest('POST', '/logout');
  }

  async getCurrentUser(): Promise<User | null> {
    try {
      const url = `${this.baseURL}/validate`;
      const response = await fetch(url, {
        method: 'GET',
        credentials: 'include',
      });

      if (response.status === 401) {
        return null; // Not authenticated
      }

      if (!response.ok) {
        throw new Error(this.getMethodDescription('GET', '/validate'));
      }

      const data = await response.json();
      return data.data as User;
    } catch (error: unknown) {
      if (error instanceof Error && error.message && (error.message.includes('401') || error.message.includes('Unauthorized'))) {
        return null; // Not authenticated
      }
      throw error;
    }
  }

  async getCSRFToken(): Promise<string | null> {
    try {
      // 🚀 X29 FIX: Use direct nginx route for CSRF token instead of frontend proxy
      return await this.getCSRFTokenInternal();
    } catch (error: unknown) {
      console.warn('Failed to get CSRF token:', error);
      return null;
    }
  }


  async updateProfile(profile: Partial<User>): Promise<User> {
    const response = await this.makeRequest('PUT', '/profile', profile);
    return response.data as User;
  }

  async getUserSettings(): Promise<UserPreferences> {
    const response = await this.makeRequest('GET', '/settings');
    return response.data as UserPreferences;
  }

  async updateUserSettings(settings: UserPreferences): Promise<void> {
    await this.makeRequest('PUT', '/settings', settings);
  }






  private async makeRequest(method: string, endpoint: string, body?: unknown): Promise<{ data: unknown }> {
    const url = `${this.baseURL}${endpoint}`;
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };

    // 🚀 X26 Phase 2: Enhanced S2S authentication headers for auth-service compatibility
    // Following Ory Kratos official recommendations for service-to-service communication

    // Add CSRF token for unsafe methods (except CSRF endpoint to avoid circular dependency)
    const isUnsafeMethod = ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase());
    const isCsrfEndpoint = endpoint.includes('/csrf');

    if (isUnsafeMethod && !isCsrfEndpoint) {
      const csrfToken = await this.getCSRFTokenInternal();
      if (csrfToken) {
        headers['X-CSRF-Token'] = csrfToken;
        // 🔑 Ory Kratos recommended: X-Session-Token for S2S auth reliability
        headers['X-Session-Token'] = csrfToken;
      }
    }

    // 🚀 X26 Phase 2: Additional headers for enhanced auth-service compatibility
    headers['X-Requested-With'] = 'XMLHttpRequest';
    headers['X-Client-Type'] = 'frontend-spa';

    // 🚀 X29 FIX: CSRF requests should use nginx direct route, not frontend proxy
    if (isCsrfEndpoint) {
      console.warn('⚠️ DEPRECATED: makeRequest() called for CSRF endpoint. Use getCSRFTokenInternal() instead for nginx direct route.');
      headers['X-Auth-Flow'] = 'csrf-request';
      headers['X-Internal-Request'] = 'true';
    }

    const config: RequestInit = {
      method,
      credentials: 'include', // 🔑 CRITICAL: Always include credentials for Kratos session cookies
      headers,
    };

    if (body) {
      config.body = JSON.stringify(body);
    }

    try {
      const response = await fetch(url, config);

      if (!response.ok) {
        const error = new Error(`HTTP ${response.status}: ${method} ${endpoint}`);
        throw this.handleError(error, `${method} ${endpoint}`);
      }

      return await response.json();
    } catch (error) {
      throw this.handleError(error, `${method} ${endpoint}`);
    }
  }

  private async getCSRFTokenInternal(): Promise<string | null> {
    try {
      // 🚀 X29 FIX: Use nginx direct route for CSRF token requests
      // This bypasses the frontend proxy and goes directly through nginx to auth-service
      const url = '/api/auth/csrf';

      // 🚀 X26 Phase 2: Enhanced CSRF request with proper headers for direct auth-service routing
      const response = await fetch(url, {
        method: 'POST',
        credentials: 'include', // 🔑 Essential for session cookie transmission
        headers: {
          'Content-Type': 'application/json',
          'X-Auth-Flow': 'csrf-request',
          'X-Internal-Request': 'true',
          'X-Requested-With': 'XMLHttpRequest',
          'X-Client-Type': 'frontend-spa',
          // 🚀 X29 FIX: Add header to ensure nginx direct route usage
          'X-Route-Type': 'nginx-direct',
        },
      });

      if (!response.ok) {
        console.error('🚨 CSRF token request failed:', {
          status: response.status,
          statusText: response.statusText,
          url,
          headers: Object.fromEntries(response.headers.entries())
        });
        return null;
      }

      const data = await response.json();
      const token = data.data?.csrf_token || data.csrf_token || null;

      if (token) {
        console.log('✅ CSRF token retrieved successfully via direct auth-service route');
      } else {
        console.warn('⚠️ CSRF response received but no token found:', data);
      }

      return token;
    } catch (error) {
      console.error('🚨 CSRF token request error:', error);
      return null;
    }
  }

  // 詳細エラー分析とKratos固有エラー変換
  private handleError(error: unknown, context: string): Error {
    // 詳細診断ログ
    console.error(`Auth API Error [${context}]:`, error);
    console.error(`Auth API Error [${context}] - Type:`, typeof error);

    if (error instanceof Error) {
      console.error(`Auth API Error [${context}] - Message:`, error.message);
      console.error(`Auth API Error [${context}] - Name:`, error.name);

      // Kratos固有エラーの判定と適切な変換
      if (error.message.includes('Property email is missing')) {
        return new Error(`VALIDATION_FAILED: Property email is missing - ${context}`);
      }

      if (error.message.includes('already registered') || error.message.includes('User already exists')) {
        return new Error(`USER_ALREADY_EXISTS: User already exists - ${context}`);
      }

      if (error.message.includes('flow expired') || error.message.includes('410')) {
        return new Error(`FLOW_EXPIRED: Registration flow expired - ${context}`);
      }

      if (error.message.includes('502') || error.message.includes('503')) {
        return new Error(`KRATOS_SERVICE_ERROR: Authentication service unavailable - ${context}`);
      }

      // HTTPステータスコード別の処理
      if (error.message.includes('HTTP 400')) {
        return new Error(`VALIDATION_FAILED: Bad request - ${context}: ${error.message}`);
      }

      // 🚨 FIX: HTTP 401 専用ハンドリング追加
      if (error.message.includes('HTTP 401')) {
        return new Error(`SESSION_NOT_FOUND: Authentication required - ${context}: ${error.message}`);
      }

      // 🚨 FIX: HTTP 404 専用ハンドリング追加
      if (error.message.includes('HTTP 404')) {
        return new Error(`KRATOS_SERVICE_ERROR: Authentication endpoint not found - ${context}: ${error.message}`);
      }

      if (error.message.includes('HTTP 409')) {
        return new Error(`USER_ALREADY_EXISTS: Conflict - ${context}: ${error.message}`);
      }

      if (error.message.includes('HTTP 410')) {
        return new Error(`FLOW_EXPIRED: Gone - ${context}: ${error.message}`);
      }

      return new Error(`${context}: ${error.message}`);
    }

    return new Error(`${context}: Unknown error occurred`);
  }

  private getMethodDescription(method: string, endpoint: string): string {
    if (endpoint.includes('/login') && method === 'POST' && !endpoint.includes('/login/')) {
      return 'Failed to initiate login';
    }
    if (endpoint.includes('/login/') && method === 'POST') {
      return 'Failed to complete login';
    }
    if (endpoint.includes('/register') && method === 'POST' && !endpoint.includes('/register/')) {
      return 'Failed to initiate registration';
    }
    if (endpoint.includes('/register/') && method === 'POST') {
      return 'Failed to complete registration';
    }
    if (endpoint.includes('/logout')) {
      return 'Failed to logout';
    }
    if (endpoint.includes('/validate')) {
      return 'Failed to get current user';
    }
    if (endpoint.includes('/csrf')) {
      return 'Failed to get CSRF token';
    }
    if (endpoint.includes('/profile')) {
      return 'Failed to update profile';
    }
    if (endpoint.includes('/settings')) {
      return method === 'GET' ? 'Failed to get user settings' : 'Failed to update user settings';
    }
    return `Request failed`;
  }
}

// Export singleton instance
export const authAPI = new AuthAPIClient();