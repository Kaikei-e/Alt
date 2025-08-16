import type { User, LoginFlow, RegistrationFlow, UserPreferences } from '@/types/auth';

export class AuthAPIClient {
  private baseURL: string;
  private debugMode: boolean;
  private requestId: number;

  constructor() {
    // Use relative API proxy endpoints for secure HTTPS communication
    // This avoids mixed content issues and keeps internal URLs secure
    this.baseURL = '/api/auth';
    this.debugMode = process.env.NODE_ENV === 'development';
    this.requestId = 0;
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
    const response = await this.makeRequest('POST', '/login');

    // 防御的プログラミング: レスポンスデータの検証
    if (!response || !response.data || typeof response.data !== 'object') {
      throw new Error('Invalid login flow response format');
    }

    const loginFlow = response.data as LoginFlow;
    if (!loginFlow.id) {
      throw new Error('Login flow response missing required ID');
    }

    return loginFlow;
  }

  async completeLogin(flowId: string, email: string, password: string): Promise<User> {
    console.log('🚀 Starting Kratos-compliant login process...');
    
    // 🎯 Kratosスキーマ完全準拠ログインペイロード生成
    const payload = this.createKratosCompliantLoginPayload(email, password);

    console.log('[AUTH-CLIENT] Sending Kratos-compliant login payload:', {
      flowId: flowId,
      hasIdentifier: !!payload.identifier,
      hasPassword: !!payload.password,
      method: payload.method
    });

    const response = await this.makeRequest('POST', `/login/${flowId}`, payload);
    console.log('✅ Login request completed successfully');
    return response.data as User;
  }

  async initiateRegistration(): Promise<RegistrationFlow> {
    const response = await this.makeRequest('POST', '/register');

    // 防御的プログラミング: レスポンスデータの検証
    if (!response || !response.data || typeof response.data !== 'object') {
      throw new Error('Invalid registration flow response format');
    }

    const registrationFlow = response.data as RegistrationFlow;
    if (!registrationFlow.id) {
      throw new Error('Registration flow response missing required ID');
    }

    return registrationFlow;
  }

  async completeRegistration(flowId: string, email: string, password: string, name?: string): Promise<User> {
    console.log('🚀 Starting registration completion...', { flowId });

    // Basic validation
    if (!flowId) {
      throw new Error('Flow ID is required');
    }
    if (!email || !email.includes('@')) {
      throw new Error('Valid email address is required');
    }
    if (!password || password.length < 8) {
      throw new Error('Password must be at least 8 characters');
    }

    // Create registration payload
    const payload = this.createKratosCompliantRegistrationPayload(email, password, name);

    console.log('[AUTH-CLIENT] Sending registration payload:', {
      flowId: flowId,
      payloadStructure: {
        hasTraits: !!payload.traits,
        hasEmail: !!(payload.traits as any)?.email,
        hasName: !!(payload.traits as any)?.name,
        hasPassword: !!payload.password,
        method: payload.method
      },
      payloadSize: JSON.stringify(payload).length
    });

    try {
      const response = await this.makeRequest('POST', `/register/${flowId}`, payload);
      const endTime = performance.now();
      const duration = endTime - startTime;
      
      // 🎉 SUCCESS: 完全な成功レスポンス診断
      const successDiagnostic = {
        requestId: diagnosticInfo.requestId,
        status: 'SUCCESS',
        duration: `${duration.toFixed(2)}ms`,
        responseSize: JSON.stringify(response).length,
        hasData: !!response.data,
        userData: response.data ? {
          hasId: !!(response.data as User).id,
          hasEmail: !!(response.data as User).email,
          hasName: !!(response.data as User).name
        } : null,
        timestamp: new Date().toISOString()
      };
      
      console.log('✅ [AUTH-CLIENT] Registration SUCCESS:', successDiagnostic);
      
      if (this.debugMode && response.data) {
        console.log('📄 Full Response Data (DEBUG):', JSON.stringify(response.data, null, 2));
      }
      
      console.groupEnd();
      return response.data as User;
      
    } catch (error) {
      const endTime = performance.now();
      const duration = endTime - startTime;
      
      // 🚨 ERROR: 完全なエラー診断情報
      const errorDiagnostic = {
        requestId: diagnosticInfo.requestId,
        status: 'ERROR',
        duration: `${duration.toFixed(2)}ms`,
        error: {
          name: error instanceof Error ? error.name : 'Unknown',
          message: error instanceof Error ? error.message : String(error),
          stack: error instanceof Error ? error.stack?.split('\n').slice(0, 5) : null
        },
        requestInfo: {
          flowId: flowId,
          email: email ? `${email.substring(0, 3)}***@${email.split('@')[1] || 'unknown'}` : 'missing',
          payloadSize: JSON.stringify(payload).length
        },
        timestamp: new Date().toISOString()
      };
      
      console.error('❌ [AUTH-CLIENT] Registration FAILED:', errorDiagnostic);
      
      if (this.debugMode) {
        console.error('📄 Full Error Details (DEBUG):', error);
        console.error('📄 Sent Payload (DEBUG):', JSON.stringify(payload, null, 2));
      }
      
      console.groupEnd();
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
      const response = await this.makeRequest('POST', '/csrf');

      // 防御的プログラミング: CSRF レスポンスの検証強化
      if (!response || !response.data || typeof response.data !== 'object') {
        console.warn('CSRF response invalid format:', response);
        return null;
      }

      const csrfData = response.data as { csrf_token?: string };
      if (!csrfData.csrf_token || typeof csrfData.csrf_token !== 'string') {
        console.warn('CSRF response missing token:', csrfData);
        return null;
      }

      return csrfData.csrf_token;
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




  private createKratosCompliantRegistrationPayload(email: string, password: string, name?: string): any {
    const payload: any = {
      method: "password",
      password: password.trim(),
      traits: {
        email: email.trim().toLowerCase()
      }
    };

    // Add name if provided
    if (name && name.trim()) {
      const nameParts = name.trim().split(/\s+/);
      payload.traits.name = {
        first: nameParts[0] || "",
        last: nameParts.slice(1).join(" ") || ""
      };
      
      // Remove empty last name
      if (!payload.traits.name.last) {
        delete payload.traits.name.last;
      }
    }

    return payload;
  }

  private createKratosCompliantLoginPayload(email: string, password: string): any {
    return {
      method: "password",
      identifier: email.trim().toLowerCase(),
      password: password.trim()
    };
  }


  private async makeRequest(method: string, endpoint: string, body?: unknown): Promise<{ data: unknown }> {
    const url = `${this.baseURL}${endpoint}`;
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };

    // Add CSRF token for unsafe methods (except CSRF endpoint to avoid circular dependency)
    const isUnsafeMethod = ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase());
    const isCsrfEndpoint = endpoint.includes('/csrf');
    
    if (isUnsafeMethod && !isCsrfEndpoint) {
      const csrfToken = await this.getCSRFTokenInternal();
      if (csrfToken) {
        headers['X-CSRF-Token'] = csrfToken;
      }
    }

    const config: RequestInit = {
      method,
      credentials: 'include',
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
      const url = `${this.baseURL}/csrf`;
      const response = await fetch(url, {
        method: 'POST',
        credentials: 'include',
      });

      if (!response.ok) {
        return null;
      }

      const data = await response.json();
      return data.data?.csrf_token || null;
    } catch {
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